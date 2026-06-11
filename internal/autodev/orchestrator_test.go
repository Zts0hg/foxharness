package autodev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// eventRecorder captures the orchestration event sequence for ordering
// assertions while discarding the engine-level stream.
type eventRecorder struct {
	*TerminalReporter
	mu     sync.Mutex
	events []string
}

func newEventRecorder() *eventRecorder {
	return &eventRecorder{TerminalReporter: NewTerminalReporter(io.Discard)}
}

func (r *eventRecorder) add(e string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *eventRecorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func (r *eventRecorder) OnItemStart(ctx context.Context, index, total int, item LedgerItem) {
	r.add("start:" + item.Slug)
}

func (r *eventRecorder) OnStageStart(ctx context.Context, slug, stage string) {
	r.add("stage:" + slug + ":" + stage)
}

func (r *eventRecorder) OnItemDone(ctx context.Context, item LedgerItem) {
	r.add("done:" + item.Slug)
}

// stubCore is a no-op CoreRunner for orchestrator flow tests.
type stubCore struct {
	workDir string
	asker   tools.UserAsker
	runs    int
}

func (c *stubCore) Run(ctx context.Context, prompt string, r engine.Reporter) (*engine.RunResult, error) {
	c.runs++
	return &engine.RunResult{FinalMessage: "ok"}, nil
}
func (c *stubCore) SetUserAsker(a tools.UserAsker) { c.asker = a }
func (c *stubCore) SetModel(model string) error    { return nil }
func (c *stubCore) WorkDir() string                { return c.workDir }
func (c *stubCore) StagePrompt(ctx context.Context, command, args string) (string, error) {
	return fmt.Sprintf("PROMPT[%s|%s]", command, args), nil
}

// stubCoreFactory records every CoreRunner it creates.
type stubCoreFactory struct {
	mu      sync.Mutex
	created []*stubCore
	models  []string
}

func (f *stubCoreFactory) New(ctx context.Context, workDir, model string) (CoreRunner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	core := &stubCore{workDir: workDir}
	f.created = append(f.created, core)
	f.models = append(f.models, model)
	return core, nil
}

// orchestraGit answers every read-only query as "already published" so the
// remote steps verify immediately, and accepts worktree add/remove.
type orchestraGit struct {
	mu       sync.Mutex
	calls    []string
	insideWT bool
}

func (g *orchestraGit) Run(ctx context.Context, dir string, args ...string) (string, error) {
	g.mu.Lock()
	g.calls = append(g.calls, strings.Join(args, " "))
	g.mu.Unlock()
	switch args[0] {
	case "rev-parse":
		if len(args) > 1 && args[1] == "--is-inside-work-tree" {
			if g.insideWT {
				return "true\n", nil
			}
			return "fatal: not a git repository", errors.New("exit status 128")
		}
		return "abc123\n", nil
	case "status":
		return "", nil
	case "rev-list":
		return "1\n", nil
	case "ls-remote":
		return "abc123\trefs/heads/x\n", nil
	case "diff":
		return "", nil
	case "worktree":
		return "", nil
	}
	return "", nil
}

// orchestraGH serves gh auth/issue/pr queries from in-memory state.
type orchestraGH struct {
	mu       sync.Mutex
	calls    []string
	authOK   bool
	issueSeq int
	issues   map[string]int
}

func (g *orchestraGH) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, name+" "+strings.Join(args, " "))
	if name != "gh" {
		return "", errors.New("unexpected command")
	}
	switch args[0] {
	case "auth":
		if g.authOK {
			return "Logged in to github.com", nil
		}
		return "you are not logged in", errors.New("exit status 1")
	case "issue":
		title := ""
		for i, a := range args {
			if a == "--search" && i+1 < len(args) {
				title = args[i+1]
			}
		}
		if g.issues == nil {
			g.issues = map[string]int{}
		}
		if _, ok := g.issues[title]; !ok {
			g.issueSeq++
			g.issues[title] = g.issueSeq
		}
		return fmt.Sprintf(`[{"number":%d,"title":%q}]`, g.issues[title], title), nil
	case "pr":
		links := make([]string, 0, len(g.issues))
		for _, n := range g.issues {
			links = append(links, fmt.Sprintf("Closes #%d", n))
		}
		return fmt.Sprintf(`{"number":%d,"body":%q}`, 1000+len(g.issues), strings.Join(links, "\n")), nil
	}
	return "", errors.New("unexpected gh args")
}

// trivialStages returns a pipeline whose stages verify immediately, for
// flow tests that do not exercise artifact production.
func trivialStages(names ...string) func(PipelineDeps) []Stage {
	return func(PipelineDeps) []Stage {
		stages := make([]Stage, 0, len(names))
		for _, name := range names {
			stages = append(stages, Stage{
				Name:    name,
				Command: "codexspec:" + name,
				Verify:  func(ctx context.Context, sc *StageContext) (bool, string) { return true, "" },
			})
		}
		return stages
	}
}

func testDeps(t *testing.T, repoRoot string, backlog string) (Deps, *eventRecorder, *stubCoreFactory, *orchestraGit, *orchestraGH) {
	t.Helper()
	if backlog != "" {
		if err := os.WriteFile(filepath.Join(repoRoot, "BACKLOG.md"), []byte(backlog), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := defaultConfig(repoRoot)
	recorder := newEventRecorder()
	factory := &stubCoreFactory{}
	git := &orchestraGit{insideWT: true}
	gh := &orchestraGH{authOK: true}
	deps := Deps{
		Config:        cfg,
		RepoRoot:      repoRoot,
		CoreFactory:   factory,
		Engineer:      &reviewingEngineer{},
		Git:           git,
		Exec:          gh,
		Reporter:      recorder,
		Clock:         newTestClock(),
		BuildPipeline: trivialStages("generate-spec", "spec-to-plan", "plan-to-tasks", "implement-tasks"),
	}
	return deps, recorder, factory, git, gh
}

const twoItemBacklog = `# Backlog

## [feature] First item

**Priority**: high
**Status**: pending
**Description**: First description.

## [feature] Second item

**Priority**: medium
**Status**: pending
**Description**: Second description.
`

func TestOrchestratorFailsFastWhenNotAGitRepo(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, _, git, _ := testDeps(t, repoRoot, twoItemBacklog)
	git.insideWT = false

	err := New(deps).Run(context.Background())
	var pre *PreconditionError
	if !errors.As(err, &pre) {
		t.Fatalf("Run error = %v, want *PreconditionError for a non-repo", err)
	}
}

func TestOrchestratorFailsFastWhenGhUnavailable(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, _, _, gh := testDeps(t, repoRoot, twoItemBacklog)
	gh.authOK = false

	err := New(deps).Run(context.Background())
	var pre *PreconditionError
	if !errors.As(err, &pre) {
		t.Fatalf("Run error = %v, want *PreconditionError for missing gh auth", err)
	}
	if !strings.Contains(strings.ToLower(pre.Error()), "gh") {
		t.Errorf("error = %q, want a clear gh message", pre.Error())
	}
}

func TestOrchestratorEmptyBacklogIsNoOp(t *testing.T) {
	repoRoot := t.TempDir()
	deps, recorder, factory, _, _ := testDeps(t, repoRoot, "# Backlog\n")

	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for _, e := range recorder.list() {
		if strings.HasPrefix(e, "start:") {
			t.Errorf("event %q recorded for an empty backlog, want clean no-op", e)
		}
	}
	if len(factory.created) != 0 {
		t.Errorf("core runners created = %d, want 0", len(factory.created))
	}
}

func TestOrchestratorProcessesItemsStrictlySerially(t *testing.T) {
	repoRoot := t.TempDir()
	deps, recorder, factory, _, _ := testDeps(t, repoRoot, twoItemBacklog)

	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var itemEvents []string
	for _, e := range recorder.list() {
		if strings.HasPrefix(e, "start:") || strings.HasPrefix(e, "done:") {
			itemEvents = append(itemEvents, e)
		}
	}
	want := []string{"start:first-item", "done:first-item", "start:second-item", "done:second-item"}
	if len(itemEvents) != len(want) {
		t.Fatalf("item events = %v, want %v (TC-018)", itemEvents, want)
	}
	for i := range want {
		if itemEvents[i] != want[i] {
			t.Fatalf("item events = %v, want %v (strict serialization, TC-018)", itemEvents, want)
		}
	}

	if len(factory.created) != 2 {
		t.Fatalf("core runners = %d, want one per item (NFR-003)", len(factory.created))
	}
	if factory.created[0].asker == nil {
		t.Error("EngineerAsker was not installed on the core runner (REQ-013)")
	}
	if !strings.Contains(factory.created[0].workDir, "first-item") {
		t.Errorf("first core workDir = %q, want the item worktree", factory.created[0].workDir)
	}

	led, err := LoadLedger(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), newTestClock())
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	for _, slug := range []string{"first-item", "second-item"} {
		it, ok := led.Get(slug)
		if !ok {
			t.Fatalf("ledger missing %s", slug)
		}
		if it.Status != StatusDone {
			t.Errorf("%s status = %q, want done (TC-013)", slug, it.Status)
		}
		if it.Issue == 0 || it.PR == 0 {
			t.Errorf("%s issue/pr = %d/%d, want recorded ground truth", slug, it.Issue, it.PR)
		}
		if it.Branch != "auto/"+slug {
			t.Errorf("%s branch = %q, want auto/%s", slug, it.Branch, slug)
		}
	}
}

func TestOrchestratorRemovesWorktreeAfterSuccess(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, _, git, _ := testDeps(t, repoRoot, twoItemBacklog)

	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	removes := 0
	for _, call := range git.calls {
		if strings.HasPrefix(call, "worktree remove") {
			removes++
		}
	}
	if removes != 2 {
		t.Errorf("worktree remove calls = %d, want 2 (TC-014)", removes)
	}
}

func TestOrchestratorResumesInProgressFromRecordedStage(t *testing.T) {
	repoRoot := t.TempDir()
	deps, recorder, _, _, _ := testDeps(t, repoRoot, twoItemBacklog)

	// Pre-record: first-item is mid-flight at plan-to-tasks with its spec
	// dir bound; the worktree directory exists from the prior run.
	led, err := LoadLedger(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	items, err := Parse(filepath.Join(repoRoot, "BACKLOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	led.Seed(items)
	led.Mark("first-item", func(it *LedgerItem) {
		it.Status = StatusInProgress
		it.Branch = "auto/first-item"
		it.Stage = "plan-to-tasks"
		it.SpecDir = ".codexspec/specs/first"
	})
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(filepath.Dir(repoRoot), filepath.Base(repoRoot)+"-worktrees", "first-item")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	events := recorder.list()
	for _, e := range events {
		if e == "stage:first-item:generate-spec" || e == "stage:first-item:spec-to-plan" {
			t.Errorf("event %q recorded, want resume from plan-to-tasks (REQ-022)", e)
		}
	}
	sawResumeStage := false
	for _, e := range events {
		if e == "stage:first-item:plan-to-tasks" {
			sawResumeStage = true
		}
	}
	if !sawResumeStage {
		t.Errorf("events = %v, want plan-to-tasks driven on resume", events)
	}
	if events[0] != "start:first-item" {
		t.Errorf("first event = %q, want the in-progress item resumed before pending work", events[0])
	}
}

func TestOrchestratorSkipsDoneItems(t *testing.T) {
	repoRoot := t.TempDir()
	deps, recorder, _, _, _ := testDeps(t, repoRoot, twoItemBacklog)

	led, err := LoadLedger(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	items, err := Parse(filepath.Join(repoRoot, "BACKLOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	led.Seed(items)
	led.Mark("first-item", func(it *LedgerItem) { it.Status = StatusDone })
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}

	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for _, e := range recorder.list() {
		if e == "start:first-item" {
			t.Error("done item was reprocessed, want skipped (TC-003)")
		}
	}
}

func TestOrchestratorPassesConfiguredModelToFactory(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, factory, _, _ := testDeps(t, repoRoot, twoItemBacklog)
	deps.Config.Model = "glm-4.7"

	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for _, m := range factory.models {
		if m != "glm-4.7" {
			t.Errorf("factory model = %q, want glm-4.7 (REQ-016)", m)
		}
	}
}
