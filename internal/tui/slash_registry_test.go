package tui

import (
	"context"
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

func TestModel_FileBasedCommand_DispatchesThroughExecutor(t *testing.T) {
	runner := newFakeRunner()
	registry := newRegistryWithPromptCommand(t, "review", "Review: $ARGUMENTS")
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(registry, slash.NewExecutor())

	m, _ = update(t, m, keyRunes("/review pr-9"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("submit cmd nil")
	}
	// Drive the deferred runner.Run via the returned command.
	_, _ = update(t, m, cmd())

	if len(runner.runs) == 0 {
		t.Fatal("runner.Run was never called for /review pr-9")
	}
	if !strings.Contains(runner.runs[0], "Review: pr-9") {
		t.Errorf("runner received %q, want substring 'Review: pr-9'", runner.runs[0])
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
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("submit cmd nil")
	}
	_, _ = update(t, m, cmd())

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
	m, cmd := update(t, m, keyEnter())
	_, _ = update(t, m, cmd())

	if len(runner.restrictedRuns) != 0 {
		t.Errorf("RunRestricted should not be used when no allowed-tools: %v", runner.restrictedRuns)
	}
	if len(runner.fakeRunner.runs) != 1 {
		t.Errorf("expected unrestricted Run, got %v", runner.fakeRunner.runs)
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
	m, _ = update(t, m, keyEnter())

	if len(runner.runs) != 0 {
		t.Errorf("Runner.Run should not be called when restriction can't be enforced: %v", runner.runs)
	}
	if !entriesContain(m.entries, "error", "allowed-tools") {
		t.Errorf("expected error entry mentioning allowed-tools, got %+v", m.entries)
	}
}
