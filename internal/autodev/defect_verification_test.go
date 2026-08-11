package autodev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type sabotagingClock struct {
	mu         sync.Mutex
	now        time.Time
	calls      int
	failAtCall int
	ledgerPath string
}

type defectReporter struct{ *eventRecorder }

func (r *defectReporter) OnInfo(_ context.Context, message string) {
	r.add("info:" + message)
}

func (c *sabotagingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls == c.failAtCall {
		_ = os.MkdirAll(filepath.Dir(c.ledgerPath), 0o755)
		_ = os.Remove(c.ledgerPath)
		_ = os.Mkdir(c.ledgerPath, 0o755)
	}
	return c.now
}

func TestDVAUT001LedgerSaveFailureDoesNotStopIrreversibleWorkflow(t *testing.T) {
	transitions := []string{
		"in-progress", "materialize", "generate-spec", "spec-to-plan", "plan-to-tasks",
		"implement-tasks", "publish", "issue", "pr", "done",
	}
	for i, transition := range transitions {
		t.Run(transition, func(t *testing.T) {
			repoRoot := t.TempDir()
			deps, recorder, _, git, gh := testDeps(t, repoRoot, `## [feature] Durable item

**Priority**: high
**Description**: preserve every transition
`)
			deps.Reporter = &defectReporter{eventRecorder: recorder}
			ledgerPath := filepath.Join(repoRoot, ".foxharness", "autodev-state.json")
			deps.Clock = &sabotagingClock{
				now:        time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
				failAtCall: i + 2, // Seed is call 1; every later call precedes a record Save.
				ledgerPath: ledgerPath,
			}

			if err := New(deps).Run(context.Background()); err != nil {
				t.Fatalf("Run returned %v after %s ledger failure; current behavior continues", err, transition)
			}
			if info, err := os.Stat(ledgerPath); err != nil || !info.IsDir() {
				t.Fatalf("ledger sabotage not active: info=%v err=%v", info, err)
			}
			if len(gh.issues) != 1 {
				t.Errorf("issues = %v, want publication to continue after ledger failure", gh.issues)
			}
			removed := false
			for _, call := range git.calls {
				removed = removed || strings.HasPrefix(call, "worktree remove")
			}
			if !removed {
				t.Error("worktree was not removed after the unrecorded publication")
			}
			warned := false
			for _, event := range recorder.list() {
				warned = warned || strings.Contains(event, "WARNING: failed to save ledger")
			}
			if !warned {
				t.Error("ledger failure was not reported as a warning")
			}
		})
	}
}

func TestDVAUT001InitialLedgerSaveFailureStopsBeforeWorktreeCreation(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, _, git, _ := testDeps(t, repoRoot, `## Initial save

**Description**: fail before work
`)
	deps.Clock = &sabotagingClock{
		now:        time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
		failAtCall: 1,
		ledgerPath: filepath.Join(repoRoot, ".foxharness", "autodev-state.json"),
	}
	if err := New(deps).Run(context.Background()); err == nil {
		t.Fatal("initial ledger failure returned nil error")
	}
	for _, call := range git.calls {
		if strings.HasPrefix(call, "worktree add") {
			t.Errorf("initial ledger failure still created a worktree: %q", call)
		}
	}
}

func TestDVAUT002UnknownRecordedStagesBypassTheSDDPipeline(t *testing.T) {
	repoRoot := t.TempDir()
	deps, recorder, _, _, _ := testDeps(t, repoRoot, `## [feature] Resume item

**Priority**: high
**Description**: resume safely
`)
	o := New(deps)
	known := []string{"materialize-requirements", "generate-spec", "spec-to-plan", "plan-to-tasks", "implement-tasks"}
	for i, stage := range known {
		if got := o.resumeIndex(LedgerItem{Stage: stage}); got != i {
			t.Errorf("resumeIndex(%q) = %d, want %d", stage, got, i)
		}
	}
	if got := o.resumeIndex(LedgerItem{}); got != 0 {
		t.Errorf("empty stage index = %d, want 0", got)
	}
	for _, stage := range []string{"publish", "done", "renamed-stage", " GENERATE-SPEC ", "future-v2", "unknown"} {
		if got := o.resumeIndex(LedgerItem{Stage: stage}); got != len(known) {
			t.Errorf("resumeIndex(%q) = %d, want pipeline bypass %d", stage, got, len(known))
		}
	}

	led, err := LoadLedger(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	items, err := Parse(filepath.Join(repoRoot, "BACKLOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	led.Seed(items)
	led.Mark("resume-item", func(it *LedgerItem) {
		it.Status = StatusInProgress
		it.Branch = "auto/resume-item"
		it.Stage = "malformed-future-stage"
	})
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}

	if err := o.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, event := range recorder.list() {
		if strings.HasPrefix(event, "stage:resume-item:") && !strings.Contains(event, ":stage-changes") &&
			!strings.Contains(event, ":commit-staged") && !strings.Contains(event, ":push") &&
			!strings.Contains(event, ":issue") && !strings.Contains(event, ":pr") {
			t.Errorf("unknown stage unexpectedly ran SDD event %q", event)
		}
	}
	reloaded, err := LoadLedger(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	item, ok := reloaded.Get("resume-item")
	if !ok || item.Status != StatusDone || item.Issue == 0 || item.PR == 0 {
		t.Fatalf("unknown-stage item = %+v, want SDD bypass followed by publication and done", item)
	}
}

func TestDVAUT003BacklogReconciliationUsesMutableTitlesAndRetainsStaleItems(t *testing.T) {
	path := ledgerPath(t)
	clock := newTestClock()
	led, err := LoadLedger(path, clock)
	if err != nil {
		t.Fatal(err)
	}
	led.Seed([]Item{
		{Title: "Alpha", Priority: PriorityLow, Description: "old alpha"},
		{Title: "Beta", Priority: PriorityLow, Description: "old beta"},
		{Title: "Duplicate", Priority: PriorityLow, Description: "first"},
		{Title: "Duplicate", Priority: PriorityLow, Description: "second"},
	})
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadLedger(path, clock)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.Seed([]Item{
		{Title: "Beta", Priority: PriorityLow, Description: "edited beta"},
		{Title: "Alpha renamed", Priority: PriorityHigh, Description: "renamed"},
		{Title: "Duplicate", Priority: PriorityLow, Description: "only survivor"},
	})
	pending := reloaded.Pending()
	titles := make([]string, 0, len(pending))
	for _, item := range pending {
		titles = append(titles, item.Title)
	}
	for _, stale := range []string{"Alpha", "Duplicate"} {
		count := 0
		for _, title := range titles {
			if title == stale {
				count++
			}
		}
		if count == 0 {
			t.Errorf("stale title %q was dropped; current ledger retains absent entries", stale)
		}
	}
	if !containsString(titles, "Alpha renamed") {
		t.Errorf("renamed backlog item was not duplicated as new identity: %v", titles)
	}
	if pending[0].Title != "Alpha renamed" {
		t.Errorf("priority refresh/order = %v, want renamed high-priority duplicate first", titles)
	}
	alphaIndex, betaIndex := -1, -1
	for i, title := range titles {
		switch title {
		case "Alpha":
			alphaIndex = i
		case "Beta":
			betaIndex = i
		}
	}
	if alphaIndex < 0 || betaIndex < 0 || alphaIndex > betaIndex {
		t.Errorf("ledger order followed current backlog reorder: %v; want stale Alpha before moved Beta", titles)
	}
	beta, ok := findLedgerItemByTitle(pending, "Beta")
	if !ok || beta.Description != "edited beta" {
		t.Errorf("matching-title description = %q, want current backlog edit", beta.Description)
	}
	alpha, _ := findLedgerItemByTitle(pending, "Alpha")
	if alpha.Description != "" {
		t.Errorf("removed persisted item description = %q, want current empty-loss behavior", alpha.Description)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func findLedgerItemByTitle(items []LedgerItem, title string) (LedgerItem, bool) {
	for _, item := range items {
		if item.Title == title {
			return item, true
		}
	}
	return LedgerItem{}, false
}

func TestDVAUT004RequirementMaterializationLosesAuthoritativeFormattingAndLength(t *testing.T) {
	description := "first paragraph\n\n```go\nfmt.Println(\"你好\")\n```\nlast paragraph"
	sc := &StageContext{
		Item:       Item{Title: "Formatting", Description: description},
		Slug:       "formatting",
		FeatureDir: ".codexspec/specs/2026-0811-1200ab-formatting",
	}
	doc := requirementsDocument(sc, time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC))
	if strings.Contains(doc, description) {
		t.Fatal("materialized document unexpectedly preserved the exact multiline description")
	}
	if !strings.Contains(doc, "first paragraph ```go fmt.Println(\"你好\") ``` last paragraph") {
		t.Fatalf("materialized statement did not exhibit one-line collapse:\n%s", doc)
	}

	long := strings.Repeat("界", 4500)
	sc.Item.Description = long
	doc = requirementsDocument(sc, time.Now())
	if strings.Contains(doc, long) {
		t.Fatal("materialized document unexpectedly retained the complete >4000-rune description")
	}
	if !strings.Contains(doc, strings.Repeat("界", 3997)+"...") {
		t.Error("materialized document does not exhibit the fixed 4000-rune truncation")
	}

	sc.Item.Description = ""
	if doc := requirementsDocument(sc, time.Now()); !strings.Contains(doc, "- **Statement**: Formatting") {
		t.Error("empty description does not fall back deterministically to the title")
	}
}

func TestDVAUT004BacklogScannerRejectsAnOtherwiseValidLargeDescriptionLine(t *testing.T) {
	path := writeBacklog(t, "## Large\n\n**Description**: "+strings.Repeat("x", 1024*1024+1)+"\n")
	if _, err := Parse(path); err == nil {
		t.Fatal("Parse accepted a description line beyond the fixed Scanner maximum")
	}
}

func TestDVAUT005PersistedFeatureDirEscapesWorktreeForMaterializationAndVerification(t *testing.T) {
	clock := newTestClock()
	workDir := t.TempDir()
	outside := t.TempDir()

	t.Run("absolute-is-lexically-reanchored", func(t *testing.T) {
		absolute := filepath.Join(outside, "absolute-feature")
		sc := &StageContext{Item: Item{Title: "Absolute", Description: "reanchored"}, Slug: "absolute", WorkDir: workDir, FeatureDir: absolute}
		if err := materializeRequirements(clock)(context.Background(), sc); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(absolute, "requirements.md")); !os.IsNotExist(err) {
			t.Fatalf("absolute FeatureDir reached its absolute target: err=%v", err)
		}
		reanchored := filepath.Join(workDir, absolute, "requirements.md")
		if _, err := os.Stat(reanchored); err != nil {
			t.Fatalf("absolute FeatureDir was not reanchored by filepath.Join: %v", err)
		}
	})

	t.Run("traversal", func(t *testing.T) {
		escapeDir := filepath.Join(workDir, "..", filepath.Base(outside), "traversal-feature")
		rel, err := filepath.Rel(workDir, escapeDir)
		if err != nil {
			t.Fatal(err)
		}
		sc := &StageContext{Item: Item{Title: "Escape", Description: "outside"}, Slug: "escape", WorkDir: workDir, FeatureDir: rel}
		if err := materializeRequirements(clock)(context.Background(), sc); err != nil {
			t.Fatal(err)
		}
		outsideFile := filepath.Join(escapeDir, "requirements.md")
		if _, err := os.Stat(outsideFile); err != nil {
			t.Fatalf("outside materialization not observed: %v", err)
		}
		if ok, gap := verifySpecArtifact("requirements.md")(context.Background(), sc); !ok {
			t.Fatalf("verification rejected outside artifact: %s", gap)
		}
	})

	t.Run("directory-symlink", func(t *testing.T) {
		linkParent := filepath.Join(workDir, ".codexspec", "specs")
		if err := os.MkdirAll(linkParent, 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(linkParent, "linked-feature")
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		sc := &StageContext{Item: Item{Title: "Link", Description: "outside"}, Slug: "link", WorkDir: workDir, FeatureDir: filepath.Join(".codexspec", "specs", "linked-feature")}
		if err := materializeRequirements(clock)(context.Background(), sc); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(outside, "requirements.md")); err != nil {
			t.Fatalf("directory symlink was not followed: %v", err)
		}
	})

	t.Run("file-symlink", func(t *testing.T) {
		feature := filepath.Join(workDir, ".codexspec", "specs", "file-link")
		if err := os.MkdirAll(feature, 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(outside, "external-requirements.md")
		if err := os.WriteFile(target, []byte("external authority"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(feature, "requirements.md")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		sc := &StageContext{WorkDir: workDir, FeatureDir: filepath.Join(".codexspec", "specs", "file-link")}
		if ok, gap := verifySpecArtifact("requirements.md")(context.Background(), sc); !ok {
			t.Fatalf("verification rejected outside file symlink: %s", gap)
		}
	})
}

func TestDVAUT006ExecRunnerRetainsUnboundedOutput(t *testing.T) {
	t.Setenv("FOX_AUTODEV_HELPER", "1")
	out, err := NewExecCommandRunner().Run(context.Background(), t.TempDir(), os.Args[0],
		"-test.run=TestAutodevExecHelperProcess", "--", "output")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2*1024*1024 {
		t.Fatalf("captured output bytes = %d, want entire unbounded 2 MiB", len(out))
	}
}

func TestDVAUT006CancellationLeavesDescendantAliveAndStartsLaterGates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process liveness proof")
	}
	t.Setenv("FOX_AUTODEV_HELPER", "1")
	runner := NewExecCommandRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	out, err := runner.Run(ctx, t.TempDir(), os.Args[0], "-test.run=TestAutodevExecHelperProcess", "--", "spawn")
	if err == nil {
		t.Fatal("cancelled helper returned nil error")
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(out))
	if parseErr != nil {
		t.Fatalf("parse descendant pid from %q: %v", out, parseErr)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("descendant was not alive after immediate-parent cancellation: %v", err)
	}

	fx := &cancelCountingExec{}
	gate := NewGateRunner(fx, nil)
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := gate.Check(cancelled, t.TempDir(), GateConfig{Build: true, Test: true, Gofmt: true}); err != nil {
		t.Fatal(err)
	}
	if fx.calls != 3 {
		t.Fatalf("commands started after cancellation = %d, want all 3 current gate steps", fx.calls)
	}
}

type cancelCountingExec struct{ calls int }

func (e *cancelCountingExec) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	e.calls++
	return "cancelled", ctx.Err()
}

func TestAutodevExecHelperProcess(t *testing.T) {
	if os.Getenv("FOX_AUTODEV_HELPER") != "1" {
		return
	}
	mode := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
			break
		}
	}
	switch mode {
	case "output":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", 2*1024*1024))
	case "spawn":
		child := exec.Command(os.Args[0], "-test.run=TestAutodevExecHelperProcess", "--", "sleep")
		child.Env = append(os.Environ(), "FOX_AUTODEV_HELPER=1")
		child.Stdout = io.Discard
		child.Stderr = io.Discard
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stdout, child.Process.Pid)
		_ = os.Stdout.Sync()
		_ = child.Wait()
	case "sleep":
		time.Sleep(10 * time.Second)
	}
	os.Exit(0)
}

type fixedExec struct {
	out   string
	calls [][]string
}

func (e *fixedExec) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	e.calls = append(e.calls, append([]string{name}, args...))
	return e.out, nil
}

func TestDVAUT007IssueVerificationUsesFirstMatchingTitleWithinTwentyResults(t *testing.T) {
	reporter := NewTerminalReporter(io.Discard)
	cases := []struct {
		name      string
		output    string
		wantOK    bool
		wantIssue int
	}{
		{name: "duplicate-title", output: `[{"number":7,"title":"Same"},{"number":9,"title":"Same"}]`, wantOK: true, wantIssue: 7},
		{name: "closed-title", output: `[{"number":11,"title":"Same"}]`, wantOK: true, wantIssue: 11},
		{name: "outside-first-page", output: `[{"number":1,"title":"Different"}]`, wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := &fixedExec{out: tc.output}
			publisher := NewRemotePublisher(newTestMachine(&reviewingEngineer{}), &orchestraGit{insideWT: true}, exec, reporter, defaultConfig(t.TempDir()))
			sc := &StageContext{Item: Item{Title: "Same"}, WorkDir: t.TempDir()}
			records := 0
			ok, _ := publisher.verifyIssue(func(mut func(*LedgerItem)) {
				records++
				item := &LedgerItem{}
				mut(item)
			})(context.Background(), sc)
			if ok != tc.wantOK || sc.Issue != tc.wantIssue {
				t.Fatalf("ok/issue = %v/%d, want %v/%d", ok, sc.Issue, tc.wantOK, tc.wantIssue)
			}
			joined := strings.Join(exec.calls[0], " ")
			if !strings.Contains(joined, "--state all") || !strings.Contains(joined, "--limit 20") {
				t.Errorf("issue query = %q, want closed-inclusive fixed first page", joined)
			}
			if records != boolInt(tc.wantOK) {
				t.Errorf("record calls = %d, want %d", records, boolInt(tc.wantOK))
			}
		})
	}
}

func TestDVAUT007RecordedIssueSkipsVerificationWithoutStableCorrelation(t *testing.T) {
	exec := &fixedExec{out: `[]`}
	cfg := defaultConfig(t.TempDir())
	publisher := NewRemotePublisher(newTestMachine(&reviewingEngineer{}), &orchestraGit{insideWT: true}, exec, NewTerminalReporter(io.Discard), cfg)
	steps := publisher.steps(nil)
	var issue Stage
	for _, step := range steps {
		if step.Name == "issue" {
			issue = step
		}
	}
	sc := &StageContext{Issue: 404, Item: Item{Title: "Renamed title"}}
	if issue.Skip == nil || !issue.Skip(context.Background(), sc) {
		t.Fatal("recorded issue was not trusted unconditionally")
	}
	if len(exec.calls) != 0 {
		t.Errorf("recorded issue triggered re-verification calls: %v", exec.calls)
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type lifecycleCore struct {
	stubCore
	release <-chan struct{}
	done    chan<- struct{}
	once    sync.Once
}

func (c *lifecycleCore) Run(ctx context.Context, prompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	c.once.Do(func() {
		go func() {
			<-c.release
			close(c.done)
		}()
	})
	return c.stubCore.Run(ctx, prompt, reporter)
}

type lifecycleCoreFactory struct {
	release <-chan struct{}
	done    chan<- struct{}
}

func (f *lifecycleCoreFactory) New(ctx context.Context, workDir, model string) (CoreRunner, error) {
	return &lifecycleCore{stubCore: stubCore{workDir: workDir}, release: f.release, done: f.done}, nil
}

func TestDVAUT008OrchestratorCleansWorktreeWithoutAsyncRuntimeDrain(t *testing.T) {
	coreType := reflect.TypeOf((*CoreRunner)(nil)).Elem()
	if _, ok := coreType.MethodByName("WaitForExtraction"); ok {
		t.Fatal("CoreRunner unexpectedly exposes an extraction drain")
	}

	repoRoot := t.TempDir()
	deps, _, _, git, _ := testDeps(t, repoRoot, `## Item

**Description**: async extraction
`)
	release := make(chan struct{})
	done := make(chan struct{})
	deps.CoreFactory = &lifecycleCoreFactory{release: release, done: done}
	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		t.Fatal("async post-run work completed before the orchestrator returned")
	default:
	}
	removed := false
	for _, call := range git.calls {
		removed = removed || strings.HasPrefix(call, "worktree remove")
	}
	if !removed {
		t.Fatal("orchestrator did not remove the worktree before returning")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("async proof goroutine did not finish")
	}
}

func TestDVAUT009ConcurrencyConfigurationAcceptsUnsupportedValuesButAlwaysRunsSerially(t *testing.T) {
	for _, value := range []string{"parallel", "8", "future-mode", " serial "} {
		t.Run(strings.ReplaceAll(value, " ", "_"), func(t *testing.T) {
			repoRoot := t.TempDir()
			writeConfig(t, repoRoot, "concurrency: \""+value+"\"\n")
			cfg, err := Load(repoRoot)
			if err != nil {
				t.Fatalf("Load rejected unsupported value %q: %v", value, err)
			}
			if cfg.Concurrency != value {
				t.Errorf("Concurrency = %q, want accepted raw value %q", cfg.Concurrency, value)
			}
			if len(cfg.Warnings) != 0 {
				t.Errorf("Warnings = %v, want no unsupported-concurrency warning", cfg.Warnings)
			}
		})
	}

	repoRoot := t.TempDir()
	deps, recorder, _, _, _ := testDeps(t, repoRoot, twoItemBacklog)
	deps.Config.Concurrency = "parallel"
	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var itemEvents []string
	for _, event := range recorder.list() {
		if strings.HasPrefix(event, "start:") || strings.HasPrefix(event, "done:") {
			itemEvents = append(itemEvents, event)
		}
	}
	want := []string{"start:first-item", "done:first-item", "start:second-item", "done:second-item"}
	if !reflect.DeepEqual(itemEvents, want) {
		t.Fatalf("parallel-config events = %v, want unchanged serial order %v", itemEvents, want)
	}
}

type partialErrorCore struct {
	result *engine.RunResult
	err    error
}

func (c *partialErrorCore) Run(context.Context, string, engine.Reporter) (*engine.RunResult, error) {
	return c.result, c.err
}
func (*partialErrorCore) SetUserAsker(tools.UserAsker) {}
func (*partialErrorCore) SetModel(string) error        { return nil }
func (*partialErrorCore) WorkDir() string              { return "" }
func (*partialErrorCore) StagePrompt(context.Context, string, string) (string, error) {
	return "seed", nil
}

func TestDVAUT010StageMachineDiscardsEveryPartialCoreOutcomeBeforeVerification(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "provider", err: errors.New("provider failed")},
		{name: "tool", err: errors.New("tool failed")},
		{name: "persistence", err: errors.New("persistence failed")},
		{name: "compaction", err: errors.New("compaction failed")},
		{name: "turn-limit", err: &engine.TurnLimitError{MaxTurns: 3}},
		{name: "cancel", err: context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core := &partialErrorCore{
				result: &engine.RunResult{SessionID: "session-1", RunID: "run-1", FinalMessage: "durable partial evidence"},
				err:    tc.err,
			}
			engineer := &reviewingEngineer{}
			verified := 0
			stage := Stage{
				Name:   "implement",
				Prompt: func(*StageContext) string { return "run" },
				Verify: func(context.Context, *StageContext) (bool, string) {
					verified++
					return true, ""
				},
			}
			err := newTestMachine(engineer).RunStep(context.Background(), core, &StageContext{}, stage)
			if err == nil || !errors.Is(err, tc.err) {
				t.Fatalf("RunStep error = %v, want wrapped %v", err, tc.err)
			}
			if strings.Contains(err.Error(), "durable partial evidence") || strings.Contains(err.Error(), "session-1") || strings.Contains(err.Error(), "run-1") {
				t.Errorf("error unexpectedly retained partial outcome correlation: %v", err)
			}
			if verified != 0 || engineer.reviewCalls != 0 {
				t.Errorf("verify/review calls = %d/%d, want 0/0 after partial result+error", verified, engineer.reviewCalls)
			}
		})
	}
}
