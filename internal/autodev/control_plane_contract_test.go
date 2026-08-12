package autodev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type cancelledPreconditionGit struct{ calls int }

func (g *cancelledPreconditionGit) Run(ctx context.Context, _ string, _ ...string) (CommandResult, error) {
	g.calls++
	return CommandResult{}, ctx.Err()
}

func TestCPAUT008StartupCancellationIsNotARepositoryPreconditionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	git := &cancelledPreconditionGit{}
	orchestrator := New(Deps{
		Config: AutodevConfig{
			Concurrency: "serial",
			RemoteFlow: RemoteFlowConfig{
				CreateIssue: false,
				OpenPR:      false,
			},
		},
		RepoRoot: t.TempDir(),
		Git:      git,
		Reporter: NewTerminalReporter(io.Discard),
	})

	err := orchestrator.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	var precondition *PreconditionError
	if errors.As(err, &precondition) {
		t.Fatalf("Run classified cancellation as PreconditionError: %v", err)
	}
	if git.calls > 1 {
		t.Fatalf("git calls = %d, want at most the interrupted repository probe", git.calls)
	}
}

func TestCPAUT001ConfigurationBoundaryMatrix(t *testing.T) {
	t.Run("explicit empty strings retain defaults and relative values retain their basis", func(t *testing.T) {
		repoRoot := filepath.Join(t.TempDir(), "project")
		if err := os.MkdirAll(repoRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		writeConfig(t, repoRoot, `
backlog_file: ""
worktree_dir: relative/worktrees
base_branch: ""
remote: upstream
model: fixture-model
engineer_prompt: inline persona
engineer_prompt_file: relative/persona.md
gates: { build: false, test: false, gofmt: false }
remote_flow: { create_issue: false, open_pr: true, link_issue: false, auto_merge: true }
`)
		cfg, err := Load(repoRoot)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.BacklogFile != "BACKLOG.md" || cfg.BaseBranch != "main" {
			t.Fatalf("empty-string merge = backlog %q base %q, want defaults", cfg.BacklogFile, cfg.BaseBranch)
		}
		if cfg.WorktreeDir != "relative/worktrees" || cfg.EngineerPromptFile != "relative/persona.md" {
			t.Fatalf("relative values = worktree %q persona %q, want unchanged config-relative strings", cfg.WorktreeDir, cfg.EngineerPromptFile)
		}
		if cfg.Remote != "upstream" || cfg.Model != "fixture-model" || cfg.EngineerPrompt != "inline persona" {
			t.Fatalf("string merge = %+v, want explicit remote/model/persona", cfg)
		}
		if cfg.Gates != (GateConfig{Build: false, Test: true, Gofmt: false}) {
			t.Fatalf("gates = %+v, want disabled optionals and mandatory test", cfg.Gates)
		}
		if cfg.RemoteFlow != (RemoteFlowConfig{CreateIssue: false, OpenPR: true, LinkIssue: false, AutoMerge: false}) {
			t.Fatalf("remote flow = %+v, want exact booleans with auto-merge forced off", cfg.RemoteFlow)
		}
		joined := strings.ToLower(strings.Join(cfg.Warnings, "\n"))
		for _, marker := range []string{"build", "test", "gofmt", "auto_merge"} {
			if !strings.Contains(joined, marker) {
				t.Errorf("warnings %q missing %q", joined, marker)
			}
		}
	})

	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "duplicate root key", content: "model: one\nmodel: two\n", want: "already defined"},
		{name: "duplicate nested key", content: "gates:\n  build: true\n  build: false\n", want: "already defined"},
		{name: "unknown nested field", content: "remote_flow:\n  merge_method: squash\n", want: "field merge_method not found"},
		{name: "malformed", content: "gates: [unterminated\n", want: "parse autodev.yml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			writeConfig(t, repoRoot, tc.content)
			_, err := Load(repoRoot)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("Load error = %v, want diagnostic containing %q", err, tc.want)
			}
		})
	}

	t.Run("read error remains a config diagnostic", func(t *testing.T) {
		repoRoot := t.TempDir()
		path := filepath.Join(repoRoot, ".foxharness", "autodev.yml")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := Load(repoRoot)
		if err == nil || !strings.Contains(err.Error(), "read autodev.yml") {
			t.Fatalf("Load error = %v, want read autodev.yml diagnostic", err)
		}
	})
}

func TestCPAUT002And023RemoteToggleMatrix(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		createIssue := mask&1 != 0
		openPR := mask&2 != 0
		linkIssue := mask&4 != 0
		name := fmt.Sprintf("issue=%t/pr=%t/link=%t", createIssue, openPR, linkIssue)
		t.Run(name, func(t *testing.T) {
			cfg := remoteConfig()
			cfg.RemoteFlow = RemoteFlowConfig{
				CreateIssue: createIssue,
				OpenPR:      openPR,
				LinkIssue:   linkIssue,
				AutoMerge:   false,
			}
			publisher := NewRemotePublisher(NewStageMachine(&reviewingEngineer{}, NewTerminalReporter(io.Discard)), nil, nil, NewTerminalReporter(io.Discard), cfg)
			var got []string
			for _, stage := range publisher.steps() {
				got = append(got, stage.Name)
			}
			want := []string{"stage-changes", "commit-staged", "push"}
			if createIssue {
				want = append(want, "issue")
			}
			if openPR {
				want = append(want, "pr")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("steps = %v, want %v", got, want)
			}

			git := &preconditionProbeGit{}
			exec := &preconditionProbeExec{}
			orchestrator := New(Deps{Config: cfg, RepoRoot: t.TempDir(), Git: git, Exec: exec, Reporter: NewTerminalReporter(io.Discard)})
			if err := orchestrator.checkPreconditions(context.Background()); err != nil {
				t.Fatalf("checkPreconditions: %v", err)
			}
			wantGH := 0
			if createIssue || openPR {
				wantGH = 1
			}
			if exec.calls != wantGH {
				t.Fatalf("gh auth calls = %d, want %d", exec.calls, wantGH)
			}
		})
	}
}

type preconditionProbeGit struct{ calls int }

func (g *preconditionProbeGit) Run(context.Context, string, ...string) (CommandResult, error) {
	g.calls++
	return stdoutResult("true\n"), nil
}

type preconditionProbeExec struct {
	calls int
	err   error
}

func (e *preconditionProbeExec) Run(context.Context, string, string, ...string) (CommandResult, error) {
	e.calls++
	if e.err != nil {
		return CommandResult{Stderr: e.err.Error(), ExitCode: 1}, e.err
	}
	return stdoutResult("authenticated\n"), nil
}

func TestCPAUT003BacklogGrammarAndInputMatrix(t *testing.T) {
	longLine := strings.Repeat("x", 128<<10)
	path := writeBacklog(t, strings.ReplaceAll(`# ignored preamble

**Priority**: high

## [Feature] 重构 Runtime

**ID**: AUT-runtime
**Unknown**: ignored metadata before description
**Priority**: HIGH
**Status**: DONE
**Description**: 第一行

第二行 `+longLine+`
~~~md
## [not-an-item] fenced heading
**Priority**: low
~~~

## [fix] Duplicate title

**Priority**: unexpected
**Status**: unexpected
**Description**: first duplicate

## [fix] Duplicate title

**Priority**: medium
**Status**: in-progress
**Description**: second duplicate
`, "\n", "\r\n"))

	items, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %+v, want three ordered headings", items)
	}
	if first := items[0]; first.Type != "Feature" || first.Title != "重构 Runtime" || first.SourceID != "AUT-runtime" ||
		first.Priority != PriorityHigh || first.Status != StatusDone || !strings.Contains(first.Description, longLine) ||
		!strings.Contains(first.Description, "## [not-an-item] fenced heading") {
		t.Fatalf("first item = %+v, want exact normalized Unicode and multiline/fenced authority", first)
	}
	if items[1].Title != items[2].Title || items[1].Description != "first duplicate" || items[2].Description != "second duplicate" {
		t.Fatalf("duplicate items = %+v / %+v, want distinct ordered records", items[1], items[2])
	}
	if items[1].Priority != PriorityLow || items[1].Status != StatusPending || items[2].Priority != PriorityMedium || items[2].Status != StatusInProgress {
		t.Fatalf("priority/status normalization = %+v / %+v", items[1], items[2])
	}
	if strings.Contains(items[0].Description, "ignored metadata") {
		t.Fatalf("unknown field leaked into description: %q", items[0].Description)
	}

	t.Run("directory produces read error", func(t *testing.T) {
		_, err := Parse(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "read backlog") {
			t.Fatalf("Parse error = %v, want read backlog diagnostic", err)
		}
	})
	t.Run("missing file produces open error", func(t *testing.T) {
		_, err := Parse(filepath.Join(t.TempDir(), "missing.md"))
		if err == nil || !strings.Contains(err.Error(), "open backlog") {
			t.Fatalf("Parse error = %v, want open backlog diagnostic", err)
		}
	})
}

func TestCPAUT008StartupFailurePrecedence(t *testing.T) {
	t.Run("malformed ledger precedes repository probe", func(t *testing.T) {
		repoRoot := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repoRoot, ".foxharness"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		git := &preconditionProbeGit{}
		err := New(Deps{Config: controlPlaneConfig(), RepoRoot: repoRoot, Git: git, Reporter: NewTerminalReporter(io.Discard)}).Run(context.Background())
		if err == nil || !strings.Contains(err.Error(), "parse ledger") || git.calls != 0 {
			t.Fatalf("Run error/calls = %v / %d, want ledger parse before repository probe", err, git.calls)
		}
	})

	t.Run("repository and gh precede backlog", func(t *testing.T) {
		repoRoot := t.TempDir()
		git := &preconditionProbeGit{}
		exec := &preconditionProbeExec{err: errors.New("gh unavailable")}
		err := New(Deps{Config: controlPlaneConfig(), RepoRoot: repoRoot, Git: git, Exec: exec, Reporter: NewTerminalReporter(io.Discard)}).Run(context.Background())
		var precondition *PreconditionError
		if !errors.As(err, &precondition) || exec.calls != 1 {
			t.Fatalf("Run error/gh calls = %v / %d, want gh precondition before missing backlog", err, exec.calls)
		}
	})

	t.Run("github-disabled reaches ordinary backlog failure", func(t *testing.T) {
		cfg := controlPlaneConfig()
		cfg.RemoteFlow = RemoteFlowConfig{}
		exec := &preconditionProbeExec{err: errors.New("must not run")}
		err := New(Deps{Config: cfg, RepoRoot: t.TempDir(), Git: &preconditionProbeGit{}, Exec: exec, Reporter: NewTerminalReporter(io.Discard)}).Run(context.Background())
		var precondition *PreconditionError
		if err == nil || errors.As(err, &precondition) || !strings.Contains(err.Error(), "open backlog") || exec.calls != 0 {
			t.Fatalf("Run error/gh calls = %v / %d, want ordinary backlog failure with GitHub disabled", err, exec.calls)
		}
	})
}

func controlPlaneConfig() AutodevConfig {
	return AutodevConfig{
		BacklogFile: "BACKLOG.md",
		Concurrency: "serial",
		RemoteFlow: RemoteFlowConfig{
			CreateIssue: true,
			OpenPR:      true,
		},
	}
}

func TestCPAUT018GateFailuresDoNotShortCircuitLaterConfiguredSteps(t *testing.T) {
	exec := &fakeExec{
		outputs: map[string]string{
			"go build ./...": "build failed",
			"go test ./...":  "tests failed",
			"gofmt -l .":     "dirty.go\n",
		},
		errs: map[string]error{
			"go build ./...": errors.New("build exit"),
			"go test ./...":  errors.New("test exit"),
		},
	}
	result, err := NewGateRunner(exec, NewTerminalReporter(io.Discard)).Check(context.Background(), t.TempDir(), allGates())
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{"go build ./...", "go test ./...", "gofmt -l ."}
	if !reflect.DeepEqual(exec.calls, wantCalls) {
		t.Fatalf("gate calls = %v, want fixed non-short-circuit order %v", exec.calls, wantCalls)
	}
	if result.Passed || len(result.Steps) != 3 {
		t.Fatalf("gate result = %+v, want three retained failures and aggregate failure", result)
	}
	for i, step := range result.Steps {
		if step.Passed || strings.TrimSpace(step.Output) == "" {
			t.Errorf("step[%d] = %+v, want failed step with diagnostic output", i, step)
		}
	}
}

type panicCore struct {
	drains int
	closes int
}

type panicCoreFactory struct{ core *panicCore }

func (f *panicCoreFactory) New(context.Context, string, string) (CoreRunner, error) {
	return f.core, nil
}

func (*panicCore) Run(context.Context, CoreAttempt, engine.Reporter) CoreOutcome {
	panic("core fixture panic")
}

func (c *panicCore) Drain(context.Context) error {
	c.drains++
	return nil
}

func (c *panicCore) Close(context.Context) error {
	c.closes++
	return nil
}

func (*panicCore) SetUserAsker(tools.UserAsker) {}
func (*panicCore) SetModel(string) error        { return nil }
func (*panicCore) WorkDir() string              { return "/fixture" }
func (*panicCore) StagePrompt(context.Context, string, string) (string, error) {
	return "fixture prompt", nil
}

func TestCPAUT025CorePanicBecomesOneCorrelatedTerminalAttempt(t *testing.T) {
	core := &panicCore{}
	var records []CoreAttemptRecord
	sc := &StageContext{
		ItemID: "panic-item",
		Slug:   "panic-item",
		RecordCoreAttempt: func(record CoreAttemptRecord) error {
			records = append(records, record)
			return nil
		},
	}
	verifyCalls := 0
	stage := Stage{
		Name:   string(StageGenerateSpec),
		Prompt: func(*StageContext) string { return "run fixture" },
		Verify: func(context.Context, *StageContext) (bool, string) {
			verifyCalls++
			return false, "not expected"
		},
	}

	err := NewStageMachine(&reviewingEngineer{}, NewTerminalReporter(io.Discard)).RunStep(context.Background(), core, sc, stage)
	var panicErr *CorePanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("RunStep error = %v, want CorePanicError", err)
	}
	if len(records) != 2 || records[0].State != CoreAttemptRunning || records[1].State != CoreAttemptTerminal {
		t.Fatalf("attempt records = %+v, want one running and one terminal record", records)
	}
	terminal := records[1]
	if terminal.AttemptID != records[0].AttemptID || terminal.CorrelationID != records[0].CorrelationID ||
		terminal.OutcomeStatus != CoreOutcomeStartFailed || terminal.RetryClass != CoreRetryNever ||
		!strings.Contains(terminal.Cause, "core fixture panic") {
		t.Fatalf("terminal record = %+v, want correlated non-retryable start_failed panic", terminal)
	}
	if verifyCalls != 0 || core.drains != 0 {
		t.Fatalf("verify/drain calls = %d/%d, want no invented started-run lifecycle", verifyCalls, core.drains)
	}
}

func TestCPAUT025CorePanicRetainsResumableWorktreeAndClosesRunner(t *testing.T) {
	repoRoot := t.TempDir()
	deps, reporter, _, git, _ := testDeps(t, repoRoot, `## [fix] Panic item

**ID**: panic-item
**Description**: preserve recovery state
`)
	core := &panicCore{}
	deps.CoreFactory = &panicCoreFactory{core: core}
	deps.BuildPipeline = trivialStages(string(StageMaterializeRequirements))

	err := New(deps).Run(context.Background())
	var panicErr *CorePanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("Run error = %v, want CorePanicError", err)
	}
	if core.closes != 1 {
		t.Fatalf("core closes = %d, want one bounded close after panic", core.closes)
	}
	for _, call := range git.calls {
		if strings.HasPrefix(call, "worktree remove") {
			t.Fatalf("git calls = %v, want panic worktree retained for resume", git.calls)
		}
	}
	for _, event := range reporter.list() {
		if strings.HasPrefix(event, "done:") {
			t.Fatalf("events = %v, want no item-done after panic", reporter.list())
		}
	}

	ledger, loadErr := LoadLedger(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), newTestClock())
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	item, ok := ledger.Get("panic-item")
	if !ok || item.Status != StatusInProgress || item.Stage != StageMaterializeRequirements || item.StageState != StageStateRunning {
		t.Fatalf("ledger item = %+v (present %t), want resumable running stage", item, ok)
	}
	if len(item.CoreAttempts) != 1 || item.CoreAttempts[0].State != CoreAttemptTerminal || item.CoreAttempts[0].OutcomeStatus != CoreOutcomeStartFailed {
		t.Fatalf("core attempts = %+v, want one durable terminal panic outcome", item.CoreAttempts)
	}
}
