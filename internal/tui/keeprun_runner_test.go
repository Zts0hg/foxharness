package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/keeprun"
)

type fakeEngine struct {
	lastPrompt  string
	lastAllowed []string
	result      *engine.RunResult
	err         error
}

func (f *fakeEngine) RunRestricted(_ context.Context, prompt string, allowed []string, _ engine.Reporter) (*engine.RunResult, error) {
	f.lastPrompt = prompt
	f.lastAllowed = allowed
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func (f *fakeEngine) RunRestrictedInDir(_ context.Context, prompt string, workDir string, allowed []string, _ engine.Reporter) (*engine.RunResult, error) {
	f.lastPrompt = prompt
	f.lastAllowed = allowed
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func (f *fakeEngine) SessionID() string { return "sess-1" }

func (f *fakeEngine) CompactSession(_ context.Context) error {
	// No-op for tests
	return nil
}

func TestKeepRunPhaseRunnerRunPhase(t *testing.T) {
	eng := &fakeEngine{result: &engine.RunResult{FinalMessage: "phase output"}}
	r := &keepRunPhaseRunner{
		engine:   eng,
		reporter: keepRunNopReporter{},
		resolve: func(_ context.Context, command, sessionID string) (string, error) {
			if sessionID != "sess-1" {
				t.Errorf("resolve sessionID = %q, want sess-1", sessionID)
			}
			return "BODY[" + command + "]", nil
		},
	}
	out, err := r.RunPhase(context.Background(), keeprun.PhaseRequest{
		Phase:       keeprun.Phase{Command: "codexspec:specify"},
		WorktreeDir: "/wt",
		SpecDir:     "/wt/specs",
		Instruction: "INSTRUCTION-X",
	})
	if err != nil {
		t.Fatalf("RunPhase error: %v", err)
	}
	if out.Output != "phase output" {
		t.Errorf("Output = %q, want %q", out.Output, "phase output")
	}
	if !strings.Contains(eng.lastPrompt, "BODY[codexspec:specify]") {
		t.Errorf("prompt missing resolved body: %q", eng.lastPrompt)
	}
	if !strings.Contains(eng.lastPrompt, "/wt") || !strings.Contains(eng.lastPrompt, "INSTRUCTION-X") {
		t.Errorf("prompt missing worktree/instruction: %q", eng.lastPrompt)
	}
}

func TestKeepRunDisplayPromptOmitsTemplateBody(t *testing.T) {
	req := keeprun.PhaseRequest{
		Phase:       keeprun.Phase{Command: "codexspec:specify"},
		WorktreeDir: "/wt",
		SpecDir:     "/wt/specs",
		Instruction: "INJ-INSTRUCTION",
	}
	body := "TEMPLATE-BODY-FROM-MD"
	full := buildKeepRunPrompt(body, req)
	disp := buildKeepRunDisplayPrompt(req)

	if !strings.Contains(full, body) {
		t.Fatalf("engine prompt must contain the template body: %q", full)
	}
	if strings.Contains(disp, body) {
		t.Errorf("display prompt must omit the template body (it lives in the .md): %q", disp)
	}
	if !strings.Contains(disp, "/codexspec:specify") {
		t.Errorf("display prompt must show the command name: %q", disp)
	}
	for _, want := range []string{"/wt", "/wt/specs", "Do not merge", "INJ-INSTRUCTION"} {
		if !strings.Contains(disp, want) {
			t.Errorf("display prompt must show injected instruction %q: %q", want, disp)
		}
		if !strings.Contains(full, want) {
			t.Errorf("engine prompt must also contain injected instruction %q: %q", want, full)
		}
	}
}

func TestKeepRunPhaseRunnerEmitsHeaderBeforeRun(t *testing.T) {
	eng := &fakeEngine{result: &engine.RunResult{FinalMessage: "out"}}
	var header string
	beforeRun := false
	r := &keepRunPhaseRunner{
		engine:   eng,
		reporter: keepRunNopReporter{},
		resolve: func(_ context.Context, command, _ string) (string, error) {
			return "BODY-" + command, nil
		},
		onPrompt: func(_ context.Context, display string) {
			header = display
			beforeRun = eng.lastPrompt == ""
		},
	}
	if _, err := r.RunPhase(context.Background(), keeprun.PhaseRequest{
		Phase:       keeprun.Phase{Command: "codexspec:specify"},
		WorktreeDir: "/wt",
		Instruction: "INJ",
	}); err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	if !beforeRun {
		t.Error("onPrompt must fire before the engine run, like a user turn preceding the response")
	}
	if !strings.Contains(header, "/codexspec:specify") || !strings.Contains(header, "INJ") {
		t.Errorf("header = %q, want command name and injected instruction", header)
	}
	if strings.Contains(header, "BODY-codexspec:specify") {
		t.Errorf("header must not include the resolved template body: %q", header)
	}
}

func TestKeepRunAllowedToolsAndMergeGuard(t *testing.T) {
	hasBash := false
	for _, x := range keepRunAllowedTools() {
		if x == "bash" {
			hasBash = true
		}
		if strings.Contains(x, "merge") {
			t.Errorf("unexpected merge-capable tool in allow-list: %q", x)
		}
	}
	if !hasBash {
		t.Error("expected bash in keep-run allowed tools")
	}
	// TC-013 intent: merge operations are blocked by the guard, not the tool list.
	if !keeprun.MergeProhibited("git merge main") || !keeprun.MergeProhibited("gh pr merge 1") {
		t.Error("merge guard must block git merge and gh pr merge")
	}
	if keeprun.MergeProhibited("git push -u origin keep-run-x") {
		t.Error("merge guard must not block a normal branch push")
	}
}

func TestKeepRunPhaseRunnerResolveError(t *testing.T) {
	r := &keepRunPhaseRunner{
		engine:   &fakeEngine{},
		reporter: keepRunNopReporter{},
		resolve: func(context.Context, string, string) (string, error) {
			return "", context.DeadlineExceeded
		},
	}
	if _, err := r.RunPhase(context.Background(), keeprun.PhaseRequest{Phase: keeprun.Phase{Command: "codexspec:x"}}); err == nil {
		t.Error("expected error when command resolution fails")
	}
}
