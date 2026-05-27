package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/slash"
)

type restrictedFakeRunner struct {
	*fakeRunner
	restrictedRuns   []string
	restrictedAllow  []string
	restrictedResult *engine.RunResult
}

func (r *restrictedFakeRunner) RunRestricted(ctx context.Context, prompt string, allowed []string, reporter engine.Reporter) (*engine.RunResult, error) {
	r.restrictedRuns = append(r.restrictedRuns, prompt)
	r.restrictedAllow = append([]string(nil), allowed...)
	// Emit a minimal run lifecycle through the reporter so the TUI's
	// channelReporter pipeline completes — without delegating to the
	// underlying fakeRunner.Run (which would mutate fakeRunner.runs and
	// hide whether the unrestricted path was used).
	runID := "restricted-1"
	reporter.OnRunStart(ctx, r.fakeRunner.sessionID, runID)
	reporter.OnMessage(ctx, "restricted answer: "+prompt)
	if r.restrictedResult != nil {
		reporter.OnRunComplete(ctx, *r.restrictedResult)
		return r.restrictedResult, nil
	}
	res := &engine.RunResult{
		FinalMessage: "restricted answer: " + prompt,
		SessionID:    r.fakeRunner.sessionID,
		RunID:        runID,
	}
	reporter.OnRunComplete(ctx, *res)
	return res, nil
}

func newRegistryWithPromptCommand(t *testing.T, name, body string) *slash.Registry {
	t.Helper()
	r := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	r.Register(&slash.Command{
		Type:        slash.CommandPrompt,
		Name:        name,
		Description: "test " + name,
		Source:      slash.SourceProject,
		Content:     body,
		Frontmatter: slash.Frontmatter{UserInvocable: true},
	})
	return r
}

func TestModel_FileBasedCommandAppearsInAutocomplete(t *testing.T) {
	runner := newFakeRunner()
	registry := newRegistryWithPromptCommand(t, "review", "Review: $ARGUMENTS")
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(registry, slash.NewExecutor())

	m, _ = update(t, m, keyRunes("/"))
	matches := m.matchingSlashCommands()
	found := false
	for _, c := range matches {
		if c.Name == "/review" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("/review not in autocomplete: %+v", matches)
	}
}

// drivePromptCommand runs the two-stage async pipeline: stage 1
// (exec.Execute via tea.Cmd) emits promptCommandReadyMsg, stage 2
// (runner.Run via tea.Cmd) emits runFinishedMsg. The helper drives both
// stages and returns the final Model. The cancellation contract is
// preserved end-to-end because each stage re-derives the runCtx from
// m.ctx and stores its cancel func on the model.
func drivePromptCommand(t *testing.T, m Model) Model {
	t.Helper()
	m, execCmd := update(t, m, keyEnter())
	if execCmd == nil {
		t.Fatal("submit returned nil cmd")
	}
	readyMsg := execCmd()
	if readyMsg == nil {
		t.Fatal("exec cmd produced nil msg")
	}
	m, runCmd := update(t, m, readyMsg)
	if runCmd == nil {
		// Inline+empty / fork / error paths legitimately return no cmd.
		return m
	}
	finishMsg := runCmd()
	m, _ = update(t, m, finishMsg)
	return m
}

func TestModel_FileBasedCommand_DispatchesThroughExecutor(t *testing.T) {
	runner := newFakeRunner()
	registry := newRegistryWithPromptCommand(t, "review", "Review: $ARGUMENTS")
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(registry, slash.NewExecutor())

	m, _ = update(t, m, keyRunes("/review pr-9"))
	m = drivePromptCommand(t, m)

	if len(runner.runs) == 0 {
		t.Fatal("runner.Run was never called for /review pr-9")
	}
	if !strings.Contains(runner.runs[0], "Review: pr-9") {
		t.Errorf("runner received %q, want substring 'Review: pr-9'", runner.runs[0])
	}
}

func TestModel_FileBasedCommand_RendersOriginalInput(t *testing.T) {
	runner := newFakeRunner()
	registry := newRegistryWithPromptCommand(t, "review", "Review: $ARGUMENTS")
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(registry, slash.NewExecutor())

	m, _ = update(t, m, keyRunes("/review pr-9"))
	m = drivePromptCommand(t, m)

	if len(runner.runs) != 1 || runner.runs[0] != "Review: pr-9" {
		t.Fatalf("runner.runs = %#v, want expanded prompt", runner.runs)
	}
	if !entriesContain(m.entries, "user", "/review pr-9") {
		t.Fatalf("entries missing original command: %#v", m.entries)
	}
	if entriesContain(m.entries, "user", "Review: pr-9") {
		t.Fatalf("entries rendered expanded prompt instead of original command: %#v", m.entries)
	}
}

func TestModel_PromptCommand_PrepareStageIsAsync(t *testing.T) {
	// While the exec.Execute closure is pending (we have not yet
	// invoked its tea.Cmd), runner.Run must NOT have fired and the
	// model must report "running" with a cancel func wired up. This
	// is the central guarantee of the async refactor: the key handler
	// returns control to Bubble Tea immediately.
	runner := newFakeRunner()
	registry := newRegistryWithPromptCommand(t, "review", "Review: $ARGUMENTS")
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(registry, slash.NewExecutor())

	m, _ = update(t, m, keyRunes("/review"))
	m, execCmd := update(t, m, keyEnter())
	if execCmd == nil {
		t.Fatal("submit returned nil cmd")
	}
	if !m.running {
		t.Error("model must mark running=true before exec stage starts")
	}
	if m.cancelRun == nil {
		t.Error("cancelRun must be wired before exec stage starts")
	}
	if len(runner.runs) != 0 {
		t.Errorf("runner.Run must not be called before the exec stage completes, got %v", runner.runs)
	}
}

func TestModel_BuiltinCommandsUnaffectedByRegistry(t *testing.T) {
	runner := newFakeRunner()
	registry := newRegistryWithPromptCommand(t, "review", "x")
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(registry, slash.NewExecutor())

	m, _ = update(t, m, keyRunes("/clear"))
	m, _ = update(t, m, keyEnter())

	if len(m.entries) != 0 {
		t.Errorf("/clear should still wipe entries, got %d", len(m.entries))
	}
}

func TestModel_BuiltinCommandWinsOverFileBasedNameCollision(t *testing.T) {
	runner := newFakeRunner()
	registry := newRegistryWithPromptCommand(t, "help", "project help body")
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(registry, slash.NewExecutor())

	m, _ = update(t, m, keyRunes("/help"))
	m, _ = update(t, m, keyEnter())

	if len(runner.runs) != 0 {
		t.Fatalf("/help collision should dispatch built-in command, got runner runs %v", runner.runs)
	}
	if !entriesContain(m.entries, "command", "/session") {
		t.Fatalf("/help collision did not render built-in help, entries=%+v", m.entries)
	}
}

func TestModel_FuzzyAutocomplete(t *testing.T) {
	runner := newFakeRunner()
	registry := newRegistryWithPromptCommand(t, "review", "Review: $ARGUMENTS")
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(registry, slash.NewExecutor())

	// "/rev" should match /review even though it's not a builtin prefix.
	m, _ = update(t, m, keyRunes("/rev"))
	matches := m.matchingSlashCommands()
	if len(matches) == 0 {
		t.Fatal("expected fuzzy match for /rev")
	}
	if matches[0].Name != "/review" {
		t.Errorf("expected /review first, got %q", matches[0].Name)
	}
}

func TestModel_NoRegistry_BehavesAsBefore(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/"))
	matches := m.matchingSlashCommands()
	// Original 10 builtins, no file-based.
	if len(matches) != len(slashCommands) {
		t.Errorf("expected %d builtins, got %d", len(slashCommands), len(matches))
	}
}

func TestModel_AllowedTools_RoutesToRunRestricted(t *testing.T) {
	runner := &restrictedFakeRunner{fakeRunner: newFakeRunner()}
	r := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	r.Register(&slash.Command{
		Type:        slash.CommandPrompt,
		Name:        "scan",
		Description: "scan",
		Source:      slash.SourceProject,
		Content:     "Scan the code",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			AllowedTools:  []string{"read_file"},
		},
	})
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(r, slash.NewExecutor())

	m, _ = update(t, m, keyRunes("/scan"))
	m = drivePromptCommand(t, m)

	if len(runner.restrictedRuns) != 1 {
		t.Fatalf("expected RunRestricted to be called once, got %d", len(runner.restrictedRuns))
	}
	if !strings.Contains(runner.restrictedRuns[0], "Scan the code") {
		t.Errorf("prompt = %q", runner.restrictedRuns[0])
	}
	if len(runner.restrictedAllow) != 1 || runner.restrictedAllow[0] != "read_file" {
		t.Errorf("allowedTools = %v", runner.restrictedAllow)
	}
	// Regular Run path must NOT be called when restriction applies.
	if len(runner.fakeRunner.runs) != 0 {
		t.Errorf("unrestricted Run should not be called, got %v", runner.fakeRunner.runs)
	}
}

func TestModel_NoAllowedTools_UsesRegularRun(t *testing.T) {
	runner := &restrictedFakeRunner{fakeRunner: newFakeRunner()}
	r := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	r.Register(&slash.Command{
		Type:        slash.CommandPrompt,
		Name:        "plain",
		Description: "plain",
		Source:      slash.SourceProject,
		Content:     "Plain body",
		Frontmatter: slash.Frontmatter{UserInvocable: true},
	})
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(r, slash.NewExecutor())

	m, _ = update(t, m, keyRunes("/plain"))
	m = drivePromptCommand(t, m)

	if len(runner.restrictedRuns) != 0 {
		t.Errorf("RunRestricted should not be used when no allowed-tools: %v", runner.restrictedRuns)
	}
	if len(runner.fakeRunner.runs) != 1 {
		t.Errorf("expected unrestricted Run, got %v", runner.fakeRunner.runs)
	}
}

func TestModel_AfterHook_FiresAfterRunCompletes(t *testing.T) {
	// A command with hooks.after must have the hook fire AFTER the
	// model run completes (runner.Run returns), not when the executor
	// returned its ExecutionResult. The marker file is the witness.
	wd := t.TempDir()
	marker := wd + "/tui-after.marker"

	runner := newFakeRunner()
	runner.workDir = wd
	r := slash.NewRegistry(wd).WithoutDiscovery()
	r.Register(&slash.Command{
		Type:        slash.CommandPrompt,
		Name:        "review",
		Description: "review",
		Source:      slash.SourceProject,
		Content:     "Review",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			Hooks:         &slash.FrontmatterHooks{After: "touch " + marker},
		},
	})
	exec := slash.NewExecutor(slash.WithWorkDir(wd))
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(r, exec)

	m, _ = update(t, m, keyRunes("/review"))
	m, execCmd := update(t, m, keyEnter())
	if execCmd == nil {
		t.Fatal("submit returned nil cmd")
	}
	// Before the prepare stage runs, the marker cannot exist.
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("after-hook fired before exec stage ran")
	}
	// Drive the prepare stage. The after-hook still must not have fired
	// because the run stage hasn't started yet (only the executor's
	// pipeline ran — substitution/shell/variables/before-hook).
	readyMsg := execCmd()
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("after-hook fired between prepare and run stages (deferred-inside-Execute regression)")
	}
	// Drive the run stage; runner.Run completes inside the cmd closure
	// and the after-hook fires before runFinishedMsg is returned.
	m, runCmd := update(t, m, readyMsg)
	if runCmd == nil {
		t.Fatal("inline result should produce a run cmd")
	}
	_, _ = update(t, m, runCmd())

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("after-hook did not fire after run completion: %v", err)
	}
}

func TestModel_AllowedTools_UnsupportedRunner_ErrorsOut(t *testing.T) {
	// A plain fakeRunner does NOT implement restrictedRunner. Submitting a
	// command with allowed-tools must show an error rather than silently
	// falling through to an unrestricted Run.
	runner := newFakeRunner()
	r := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	r.Register(&slash.Command{
		Type:        slash.CommandPrompt,
		Name:        "scan",
		Description: "scan",
		Source:      slash.SourceProject,
		Content:     "Scan",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			AllowedTools:  []string{"read_file"},
		},
	})
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(r, slash.NewExecutor())

	m, _ = update(t, m, keyRunes("/scan"))
	m = drivePromptCommand(t, m)

	if len(runner.runs) != 0 {
		t.Errorf("Runner.Run should not be called when restriction can't be enforced: %v", runner.runs)
	}
	if !entriesContain(m.entries, "error", "allowed-tools") {
		t.Errorf("expected error entry mentioning allowed-tools, got %+v", m.entries)
	}
}
