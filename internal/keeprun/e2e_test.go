package keeprun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitInDir runs a git command in dir, supplying a deterministic identity so
// commits work without depending on the host's global git configuration.
func gitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// scriptedRunner is a PhaseRunner that reproduces the filesystem and git effects
// of the real /codexspec:* commands without an LLM, so the orchestration —
// worktree lifecycle, spec-dir detection, gates, and BACKLOG bookkeeping — can be
// exercised end-to-end and deterministically. Critically, generate-spec owns the
// feature-directory name (a timestamp+random prefix), exactly the behavior that
// the detection fix must accommodate.
type scriptedRunner struct {
	t             *testing.T
	featureDir    string
	specToPlanDir string
}

const e2eFeatureDirName = "2026-0604-1200zz-e2e-demo"

func (s *scriptedRunner) RunPhase(_ context.Context, req PhaseRequest) (PhaseOutcome, error) {
	wt := req.WorktreeDir
	switch req.Phase.Command {
	case "codexspec:specify", "codexspec:clarify":
		return PhaseOutcome{Output: "clarified by " + req.Phase.Command}, nil
	case "codexspec:generate-spec":
		s.featureDir = filepath.Join(wt, ".codexspec", "specs", e2eFeatureDirName)
		writeFileT(s.t, filepath.Join(s.featureDir, "spec.md"), "# Spec\n\nEnd-to-end spec.\n")
		return PhaseOutcome{Output: "spec generated"}, nil
	case "codexspec:spec-to-plan":
		s.specToPlanDir = req.SpecDir
		writeFileT(s.t, filepath.Join(req.SpecDir, "plan.md"), "# Plan\n\nEnd-to-end plan.\n")
		return PhaseOutcome{Output: "plan generated"}, nil
	case "codexspec:plan-to-tasks":
		writeFileT(s.t, filepath.Join(req.SpecDir, "tasks.md"), "# Tasks\n\n- [ ] T001 do it\n")
		return PhaseOutcome{Output: "tasks generated"}, nil
	case "codexspec:implement-tasks":
		writeFileT(s.t, filepath.Join(wt, "feature.go"),
			"package foo\n\n// Feature returns the implemented value.\nfunc Feature() int {\n\treturn 42\n}\n")
		return PhaseOutcome{Output: "implemented"}, nil
	case "codexspec:review-spec", "codexspec:review-plan", "codexspec:review-tasks", "codexspec:review-code":
		return PhaseOutcome{Output: "review complete\n" +
			`<!-- keep-run-verdict: {"status":"pass","critical":0,"high":0} -->` + "\n"}, nil
	case "codexspec:commit-staged":
		gitInDir(s.t, wt, "add", "-A")
		gitInDir(s.t, wt, "commit", "-m", "feat: e2e demo")
		return PhaseOutcome{Output: "committed"}, nil
	default:
		return PhaseOutcome{Output: "ok"}, nil
	}
}

func (s *scriptedRunner) CompactSession(_ context.Context) error {
	// No-op for E2E tests
	return nil
}

// TestEndToEndPipelineRealGit drives the full local pipeline against a real git
// repository. The worktree manager, orchestrator, spec-directory detection, the
// filesystem artifact gates, and the git commit gate are all real; only the Go
// toolchain gates are stubbed, since a scripted run produces no real code to
// compile. It is the regression guard for the spec-dir construction bug: with a
// timestamped feature directory owned by generate-spec, the pipeline must detect
// it (not a bare-slug path) and run to completion.
func TestEndToEndPipelineRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	gitInDir(t, repo, "init")
	gitInDir(t, repo, "config", "user.email", "t@t")
	gitInDir(t, repo, "config", "user.name", "t")
	writeFileT(t, filepath.Join(repo, "BACKLOG.md"),
		"## [feature] e2e demo\n**Status**: pending\n**Description**: d\n")
	writeFileT(t, filepath.Join(repo, "keep-run.config.json"), `{"remote_enabled": false}`)
	// An inherited, committed feature directory; detection must not pick it.
	writeFileT(t, filepath.Join(repo, ".codexspec/specs/2026-0101-0000aa-inherited/spec.md"), "inherited\n")
	gitInDir(t, repo, "add", "-A")
	gitInDir(t, repo, "commit", "-m", "init")

	// Real git everywhere it matters; stub only the orthogonal Go toolchain gates.
	gitRealStubGo := func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		if name == "git" {
			return exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		}
		return nil, nil
	}

	runner := &scriptedRunner{t: t}
	sink := &capSink{}
	o := NewOrchestrator(repo, runner,
		WithVerifier(NewVerifier(WithCommandRunner(gitRealStubGo))),
		WithProgressSink(sink), WithSleeper(noSleep))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := o.Run(ctx); err != nil {
		t.Fatalf("end-to-end Run failed: %v", err)
	}

	wantDir := filepath.Join(repo, ".claude", "worktrees", "e2e-demo",
		".codexspec", "specs", e2eFeatureDirName)
	if runner.specToPlanDir != wantDir {
		t.Errorf("spec-to-plan SpecDir = %q, want the detected timestamped dir %q", runner.specToPlanDir, wantDir)
	}
	if got := sink.count(EventPhaseComplete); got != 11 {
		t.Errorf("completed phases = %d, want 11 (pr skipped when remote disabled)", got)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "BACKLOG.md")); !strings.Contains(string(b), "**Status**: done") {
		t.Errorf("BACKLOG not marked done:\n%s", b)
	}
	if _, err := os.Stat(filepath.Join(repo, ".claude", "worktrees", "e2e-demo")); !os.IsNotExist(err) {
		t.Errorf("worktree not cleaned up (stat err = %v)", err)
	}
}
