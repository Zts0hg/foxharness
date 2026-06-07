package keeprun

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCmd is a canned response for a single command key in fakeRunner.
type fakeCmd struct {
	out []byte
	err error
}

// fakeRunner builds a CommandRunner that replies from a "name args..." -> result
// map and fails the test on any unexpected command.
func fakeRunner(t *testing.T, responses map[string]fakeCmd) CommandRunner {
	t.Helper()
	return func(_ context.Context, _, name string, args ...string) ([]byte, error) {
		key := strings.TrimSpace(name + " " + strings.Join(args, " "))
		r, ok := responses[key]
		if !ok {
			t.Fatalf("unexpected command: %q", key)
		}
		return r.out, r.err
	}
}

func phaseCmd(command string) Phase { return Phase{Command: command} }

func TestReviewClean(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"pass", `report...
<!-- keep-run-verdict: {"status":"pass","critical":0,"high":0} -->`, true},
		{"needs_work", `<!-- keep-run-verdict: {"status":"needs_work","critical":0,"high":2} -->`, false},
		{"fail", `<!-- keep-run-verdict: {"status":"fail","critical":1,"high":0} -->`, false},
		{"missing_block", "a review with no verdict block", false},
		{"malformed_json", `<!-- keep-run-verdict: {status: pass} -->`, false},
		{"extra_whitespace", "<!--   keep-run-verdict:   {\"status\":\"pass\"}   -->", true},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReviewClean(PhaseOutcome{Output: tt.output}); got != tt.want {
				t.Errorf("ReviewClean(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestVerifyPhaseFileGates(t *testing.T) {
	ctx := context.Background()
	v := NewVerifier()

	cases := []struct {
		command string
		file    string
	}{
		{"codexspec:generate-spec", "spec.md"},
		{"codexspec:spec-to-plan", "plan.md"},
		{"codexspec:plan-to-tasks", "tasks.md"},
	}
	for _, c := range cases {
		t.Run(c.command+"_present", func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, c.file), []byte("# content\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := v.VerifyPhase(ctx, phaseCmd(c.command), TaskContext{SpecDir: dir}, PhaseOutcome{}); err != nil {
				t.Errorf("VerifyPhase(%s) = %v, want nil", c.command, err)
			}
		})
		t.Run(c.command+"_missing", func(t *testing.T) {
			if err := v.VerifyPhase(ctx, phaseCmd(c.command), TaskContext{SpecDir: t.TempDir()}, PhaseOutcome{}); err == nil {
				t.Errorf("VerifyPhase(%s) missing file: want error, got nil", c.command)
			}
		})
		t.Run(c.command+"_empty", func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, c.file), []byte("   \n\t"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := v.VerifyPhase(ctx, phaseCmd(c.command), TaskContext{SpecDir: dir}, PhaseOutcome{}); err == nil {
				t.Errorf("VerifyPhase(%s) whitespace-only file: want error, got nil", c.command)
			}
		})
	}
}

func TestVerifyPhaseOutputGate(t *testing.T) {
	ctx := context.Background()
	v := NewVerifier()
	for _, command := range []string{"codexspec:specify", "codexspec:clarify"} {
		if err := v.VerifyPhase(ctx, phaseCmd(command), TaskContext{}, PhaseOutcome{Output: "decided X"}); err != nil {
			t.Errorf("VerifyPhase(%s) non-empty: %v", command, err)
		}
		if err := v.VerifyPhase(ctx, phaseCmd(command), TaskContext{}, PhaseOutcome{Output: "   "}); err == nil {
			t.Errorf("VerifyPhase(%s) empty: want error", command)
		}
	}
}

func TestVerifyPhaseReviewGate(t *testing.T) {
	ctx := context.Background()
	v := NewVerifier()
	clean := PhaseOutcome{Output: `<!-- keep-run-verdict: {"status":"pass"} -->`}
	dirty := PhaseOutcome{Output: `<!-- keep-run-verdict: {"status":"needs_work"} -->`}
	for _, command := range []string{"codexspec:review-spec", "codexspec:review-plan", "codexspec:review-tasks"} {
		if err := v.VerifyPhase(ctx, phaseCmd(command), TaskContext{}, clean); err != nil {
			t.Errorf("VerifyPhase(%s) clean: %v", command, err)
		}
		if err := v.VerifyPhase(ctx, phaseCmd(command), TaskContext{}, dirty); err == nil {
			t.Errorf("VerifyPhase(%s) not clean: want error", command)
		}
	}
}

func TestVerifyPhaseReviewCodeGate(t *testing.T) {
	ctx := context.Background()
	clean := PhaseOutcome{Output: `<!-- keep-run-verdict: {"status":"pass"} -->`}

	passing := map[string]fakeCmd{
		"go build ./...": {},
		"go vet ./...":   {},
		"go test ./...":  {},
		"gofmt -l .":     {out: []byte("")},
	}

	t.Run("clean_and_gates_pass", func(t *testing.T) {
		v := NewVerifier(WithCommandRunner(fakeRunner(t, passing)))
		if err := v.VerifyPhase(ctx, phaseCmd("codexspec:review-code"), TaskContext{WorktreeDir: "/w"}, clean); err != nil {
			t.Errorf("review-code clean+pass: %v", err)
		}
	})

	t.Run("unformatted_files_fail", func(t *testing.T) {
		responses := map[string]fakeCmd{
			"go build ./...": {},
			"go vet ./...":   {},
			"go test ./...":  {},
			"gofmt -l .":     {out: []byte("internal/x/foo.go\n")},
		}
		v := NewVerifier(WithCommandRunner(fakeRunner(t, responses)))
		if err := v.VerifyPhase(ctx, phaseCmd("codexspec:review-code"), TaskContext{WorktreeDir: "/w"}, clean); err == nil {
			t.Error("review-code with unformatted files: want error")
		}
	})

	t.Run("review_not_clean_skips_gates", func(t *testing.T) {
		// No runner responses registered: if a gate were run it would fail the
		// test, proving ReviewClean is checked before the objective gates.
		v := NewVerifier(WithCommandRunner(fakeRunner(t, map[string]fakeCmd{})))
		dirty := PhaseOutcome{Output: `<!-- keep-run-verdict: {"status":"fail"} -->`}
		if err := v.VerifyPhase(ctx, phaseCmd("codexspec:review-code"), TaskContext{WorktreeDir: "/w"}, dirty); err == nil {
			t.Error("review-code not clean: want error")
		}
	})
}

func TestVerifyPhaseCommitGate(t *testing.T) {
	ctx := context.Background()
	tc := TaskContext{WorktreeDir: "/w", HeadCommitBefore: "oldsha"}

	t.Run("committed_clean", func(t *testing.T) {
		v := NewVerifier(WithCommandRunner(fakeRunner(t, map[string]fakeCmd{
			"git rev-parse HEAD":     {out: []byte("newsha\n")},
			"git status --porcelain": {out: []byte("")},
		})))
		if err := v.VerifyPhase(ctx, phaseCmd("codexspec:commit-staged"), tc, PhaseOutcome{}); err != nil {
			t.Errorf("commit gate: %v", err)
		}
	})

	t.Run("head_not_advanced", func(t *testing.T) {
		v := NewVerifier(WithCommandRunner(fakeRunner(t, map[string]fakeCmd{
			"git rev-parse HEAD": {out: []byte("oldsha\n")},
		})))
		if err := v.VerifyPhase(ctx, phaseCmd("codexspec:commit-staged"), tc, PhaseOutcome{}); err == nil {
			t.Error("commit gate, HEAD unchanged: want error")
		}
	})

	t.Run("dirty_tree", func(t *testing.T) {
		v := NewVerifier(WithCommandRunner(fakeRunner(t, map[string]fakeCmd{
			"git rev-parse HEAD":     {out: []byte("newsha\n")},
			"git status --porcelain": {out: []byte(" M file.go\n")},
		})))
		if err := v.VerifyPhase(ctx, phaseCmd("codexspec:commit-staged"), tc, PhaseOutcome{}); err == nil {
			t.Error("commit gate, dirty tree: want error")
		}
	})
}

func TestVerifyPhaseImplementGate(t *testing.T) {
	ctx := context.Background()
	tc := TaskContext{WorktreeDir: "/w"}

	t.Run("tests_pass_with_changes", func(t *testing.T) {
		v := NewVerifier(WithCommandRunner(fakeRunner(t, map[string]fakeCmd{
			"go test ./...":          {},
			"git status --porcelain": {out: []byte(" M internal/x/foo.go\n")},
		})))
		if err := v.VerifyPhase(ctx, phaseCmd("codexspec:implement-tasks"), tc, PhaseOutcome{}); err != nil {
			t.Errorf("implement gate: %v", err)
		}
	})

	t.Run("tests_fail", func(t *testing.T) {
		v := NewVerifier(WithCommandRunner(fakeRunner(t, map[string]fakeCmd{
			"go test ./...": {out: []byte("FAIL"), err: fmt.Errorf("exit 1")},
		})))
		if err := v.VerifyPhase(ctx, phaseCmd("codexspec:implement-tasks"), tc, PhaseOutcome{}); err == nil {
			t.Error("implement gate, tests fail: want error")
		}
	})

	t.Run("no_changes", func(t *testing.T) {
		v := NewVerifier(WithCommandRunner(fakeRunner(t, map[string]fakeCmd{
			"go test ./...":          {},
			"git status --porcelain": {out: []byte("")},
		})))
		if err := v.VerifyPhase(ctx, phaseCmd("codexspec:implement-tasks"), tc, PhaseOutcome{}); err == nil {
			t.Error("implement gate, no changes: want error")
		}
	})
}

func TestVerifyPhasePRGate(t *testing.T) {
	ctx := context.Background()
	v := NewVerifier()

	ok := PhaseOutcome{Output: "Issue #7 created\nhttps://github.com/o/r/pull/12 (Closes #7)"}
	if err := v.VerifyPhase(ctx, phaseCmd("codexspec:pr"), TaskContext{}, ok); err != nil {
		t.Errorf("pr gate valid: %v", err)
	}
	noURL := PhaseOutcome{Output: "created issue #7 but PR failed"}
	if err := v.VerifyPhase(ctx, phaseCmd("codexspec:pr"), TaskContext{}, noURL); err == nil {
		t.Error("pr gate no URL: want error")
	}
	noIssue := PhaseOutcome{Output: "https://github.com/o/r/pull/12"}
	if err := v.VerifyPhase(ctx, phaseCmd("codexspec:pr"), TaskContext{}, noIssue); err == nil {
		t.Error("pr gate no issue ref: want error")
	}
}
