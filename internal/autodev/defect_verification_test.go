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

func (r *defectReporter) OnIssue(_ context.Context, number int) {
	r.add(fmt.Sprintf("issue:%d", number))
}

func (r *defectReporter) OnPR(_ context.Context, number int) {
	r.add(fmt.Sprintf("pr:%d", number))
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

func TestDVAUT001LedgerSaveFailureStopsBeforeDependentWorkflow(t *testing.T) {
	tests := []struct {
		name            string
		failAtCall      int
		maxCoreRuns     int
		maxIssueQueries int
		maxPRQueries    int
		maxIssueReports int
		maxPRReports    int
	}{
		{name: "in-progress", failAtCall: 2},
		{name: "materialize-intent", failAtCall: 3},
		{name: "materialize-verified", failAtCall: 4, maxCoreRuns: 1},
		{name: "generate-spec-intent", failAtCall: 5, maxCoreRuns: 1},
		{name: "generate-spec-verified", failAtCall: 6, maxCoreRuns: 2},
		{name: "spec-to-plan-intent", failAtCall: 7, maxCoreRuns: 2},
		{name: "spec-to-plan-verified", failAtCall: 8, maxCoreRuns: 3},
		{name: "plan-to-tasks-intent", failAtCall: 9, maxCoreRuns: 3},
		{name: "plan-to-tasks-verified", failAtCall: 10, maxCoreRuns: 4},
		{name: "implement-tasks-intent", failAtCall: 11, maxCoreRuns: 4},
		{name: "implement-tasks-verified", failAtCall: 12, maxCoreRuns: 5},
		{name: "publish-intent", failAtCall: 13, maxCoreRuns: 5},
		{name: "stage-changes-intent", failAtCall: 14, maxCoreRuns: 5},
		{name: "stage-changes-verified", failAtCall: 15, maxCoreRuns: 5},
		{name: "commit-staged-intent", failAtCall: 16, maxCoreRuns: 5},
		{name: "commit-staged-verified", failAtCall: 17, maxCoreRuns: 5},
		{name: "push-intent", failAtCall: 18, maxCoreRuns: 5},
		{name: "push-verified", failAtCall: 19, maxCoreRuns: 5},
		{name: "issue-intent", failAtCall: 20, maxCoreRuns: 5},
		{name: "issue-binding", failAtCall: 21, maxCoreRuns: 6, maxIssueQueries: 1},
		{name: "pr-intent", failAtCall: 22, maxCoreRuns: 6, maxIssueQueries: 1, maxIssueReports: 1},
		{name: "pr-binding", failAtCall: 23, maxCoreRuns: 7, maxIssueQueries: 1, maxPRQueries: 1, maxIssueReports: 1},
		{name: "done", failAtCall: 24, maxCoreRuns: 7, maxIssueQueries: 1, maxPRQueries: 1, maxIssueReports: 1, maxPRReports: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			deps, recorder, factory, git, gh := testDeps(t, repoRoot, `## [feature] Durable item

**Priority**: high
**Description**: preserve every transition
`)
			deps.Reporter = &defectReporter{eventRecorder: recorder}
			ledgerPath := filepath.Join(repoRoot, ".foxharness", "autodev-state.json")
			deps.Clock = &sabotagingClock{
				now:        time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
				failAtCall: tt.failAtCall,
				ledgerPath: ledgerPath,
			}

			err := New(deps).Run(context.Background())
			var commitErr *LedgerCommitError
			if !errors.As(err, &commitErr) {
				t.Fatalf("Run error = %v, want *LedgerCommitError at %s", err, tt.name)
			}
			if info, err := os.Stat(ledgerPath); err != nil || !info.IsDir() {
				t.Fatalf("ledger sabotage not active: info=%v err=%v", info, err)
			}

			coreRuns := 0
			for _, core := range factory.created {
				coreRuns += core.runs
			}
			if coreRuns > tt.maxCoreRuns {
				t.Errorf("core runs = %d, want at most %d after %s failure", coreRuns, tt.maxCoreRuns, tt.name)
			}
			issueQueries, prQueries := 0, 0
			for _, call := range gh.calls {
				if strings.HasPrefix(call, "gh issue") {
					issueQueries++
				}
				if strings.HasPrefix(call, "gh pr") {
					prQueries++
				}
			}
			if issueQueries > tt.maxIssueQueries || prQueries > tt.maxPRQueries {
				t.Errorf("remote queries issue/pr = %d/%d, want at most %d/%d after %s failure",
					issueQueries, prQueries, tt.maxIssueQueries, tt.maxPRQueries, tt.name)
			}
			removed := false
			for _, call := range git.calls {
				removed = removed || strings.HasPrefix(call, "worktree remove")
			}
			if removed {
				t.Error("worktree was removed after an authoritative ledger failure")
			}
			issueReports, prReports := 0, 0
			for _, event := range recorder.list() {
				if strings.HasPrefix(event, "done:") {
					t.Errorf("done event emitted after %s ledger failure", tt.name)
				}
				if strings.HasPrefix(event, "issue:") {
					issueReports++
				}
				if strings.HasPrefix(event, "pr:") {
					prReports++
				}
			}
			if issueReports > tt.maxIssueReports || prReports > tt.maxPRReports {
				t.Errorf("reports issue/pr = %d/%d, want at most %d/%d after %s failure",
					issueReports, prReports, tt.maxIssueReports, tt.maxPRReports, tt.name)
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

func TestDVAUT002InvalidRecordedStateFailsBeforeExternalWork(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{name: "unknown-stage", json: `{"version":1,"items":[{"slug":"resume-item","title":"Resume item","priority":"high","status":"in-progress","stage":"unknown","stage_state":"running"}]}`},
		{name: "future-version", json: `{"version":3,"items":[]}`},
		{name: "done-running", json: `{"version":1,"items":[{"slug":"resume-item","title":"Resume item","priority":"high","status":"done","stage":"done","stage_state":"running"}]}`},
		{name: "verified-issue-without-binding", json: `{"version":1,"items":[{"slug":"resume-item","title":"Resume item","priority":"high","status":"in-progress","stage":"issue","stage_state":"verified"}]}`},
		{name: "verified-pr-without-binding", json: `{"version":1,"items":[{"slug":"resume-item","title":"Resume item","priority":"high","status":"in-progress","stage":"pr","stage_state":"verified"}]}`},
		{name: "malformed-legacy-stage-state", json: `{"items":[{"slug":"resume-item","title":"Resume item","priority":"high","status":"in-progress","stage":"spec-to-plan","stage_state":"bogus"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			deps, _, factory, git, gh := testDeps(t, repoRoot, `## [feature] Resume item

**Priority**: high
**Description**: resume safely
`)
			ledgerPath := filepath.Join(repoRoot, ".foxharness", "autodev-state.json")
			if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(ledgerPath, []byte(tt.json), 0o644); err != nil {
				t.Fatal(err)
			}

			err := New(deps).Run(context.Background())
			var invalid *InvalidLedgerStateError
			if !errors.As(err, &invalid) {
				t.Fatalf("Run error = %v, want *InvalidLedgerStateError", err)
			}
			if len(git.calls) != 0 || len(gh.calls) != 0 || len(factory.created) != 0 {
				t.Fatalf("invalid state reached external work: git=%v gh=%v core=%d", git.calls, gh.calls, len(factory.created))
			}
		})
	}
}

func TestDVAUT002ResumeIndexDistinguishesRunningAndVerifiedStages(t *testing.T) {
	o := &Orchestrator{pipeline: trivialStages("materialize-requirements", "generate-spec", "spec-to-plan", "plan-to-tasks", "implement-tasks")(PipelineDeps{})}
	for i, stage := range []PipelineStage{StageMaterializeRequirements, StageGenerateSpec, StageSpecToPlan, StagePlanToTasks, StageImplementTasks} {
		if got := o.resumeIndex(LedgerItem{Status: StatusInProgress, Stage: stage, StageState: StageStateRunning}); got != i {
			t.Errorf("running %q index = %d, want %d", stage, got, i)
		}
		if got := o.resumeIndex(LedgerItem{Status: StatusInProgress, Stage: stage, StageState: StageStateVerified}); got != i+1 {
			t.Errorf("verified %q index = %d, want %d", stage, got, i+1)
		}
	}
}

func TestDVAUT002RunningStageVerifiesBeforeCoreExecution(t *testing.T) {
	artifact := "already present"
	core := &fakeCore{}
	machine := newTestMachine(&reviewingEngineer{})

	if err := machine.ResumeStep(context.Background(), core, &StageContext{Slug: "resume"}, artifactStage("generate-spec", &artifact)); err != nil {
		t.Fatal(err)
	}
	if len(core.prompts) != 0 {
		t.Fatalf("core runs = %d, want 0 when the running stage already verifies", len(core.prompts))
	}
}

func TestDVAUT002RemoteRunningIssueVerifiesBeforeCoreExecution(t *testing.T) {
	state := &repoState{issues: map[int]string{31: "Engine memory writes"}}
	pub, _, _, _ := newPublisher(t, state)
	pub.cfg.RemoteFlow.OpenPR = false
	item := happyItem()
	item.Stage = StageIssue
	item.StageState = StageStateRunning
	core := &remoteCore{}

	result, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, recordRemoteItem(&item, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(core.prompts) != 0 {
		t.Fatalf("core runs = %d, want 0 when the recorded issue exists", len(core.prompts))
	}
	if result.Issue != 31 || item.Issue != 31 || item.StageState != StageStateVerified {
		t.Fatalf("result/item = %+v / %+v, want verified issue 31", result, item)
	}
}

func TestDVAUT003ExplicitIdentitySurvivesRenameAndUsesCurrentOrder(t *testing.T) {
	path := ledgerPath(t)
	clock := newTestClock()
	led, err := LoadLedger(path, clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := led.Seed([]Item{
		{SourceID: "REQ-alpha", Title: "Alpha", Priority: PriorityLow, Description: "old alpha"},
		{SourceID: "REQ-beta", Title: "Beta", Priority: PriorityLow, Description: "old beta"},
	}); err != nil {
		t.Fatal(err)
	}
	alphaBefore, _ := led.Get("alpha")
	betaBefore, _ := led.Get("beta")
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadLedger(path, clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Seed([]Item{
		{SourceID: "REQ-beta", Title: "Beta renamed", Priority: PriorityLow, Status: StatusDone, Description: "edited beta"},
		{SourceID: "REQ-alpha", Title: "Alpha renamed", Priority: PriorityLow, Status: StatusInProgress, Description: "edited alpha"},
	}); err != nil {
		t.Fatal(err)
	}
	pending := reloaded.Pending()
	if len(pending) != 2 || pending[0].SourceID != "REQ-beta" || pending[1].SourceID != "REQ-alpha" {
		t.Fatalf("pending order = %+v, want current backlog order beta then alpha", pending)
	}
	betaAfter, _ := reloaded.Get("beta")
	alphaAfter, _ := reloaded.Get("alpha")
	if alphaAfter.ItemID != alphaBefore.ItemID || betaAfter.ItemID != betaBefore.ItemID {
		t.Fatalf("item IDs changed across rename: alpha %q/%q beta %q/%q", alphaBefore.ItemID, alphaAfter.ItemID, betaBefore.ItemID, betaAfter.ItemID)
	}
	if alphaAfter.Title != "Alpha renamed" || alphaAfter.Description != "edited alpha" || betaAfter.Title != "Beta renamed" || betaAfter.Description != "edited beta" {
		t.Fatalf("pending source fields were not refreshed: alpha=%+v beta=%+v", alphaAfter, betaAfter)
	}
	if alphaAfter.Status != StatusPending || betaAfter.Status != StatusPending {
		t.Fatalf("advisory source status overwrote ledger status: alpha=%q beta=%q", alphaAfter.Status, betaAfter.Status)
	}
}

func TestDVAUT003AmbiguousAndDuplicateIdentityFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		first  []Item
		second []Item
	}{
		{
			name: "duplicate-source-id",
			first: []Item{
				{SourceID: "REQ-1", Title: "One"},
				{SourceID: "REQ-1", Title: "Two"},
			},
		},
		{
			name: "duplicate-title-without-source-id",
			first: []Item{
				{Title: "Duplicate"},
				{Title: "Duplicate"},
			},
		},
		{
			name:   "rename-without-source-id",
			first:  []Item{{Title: "Original"}},
			second: []Item{{Title: "Renamed"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			led, err := LoadLedger(ledgerPath(t), newTestClock())
			if err != nil {
				t.Fatal(err)
			}
			seedErr := led.Seed(tt.first)
			if len(tt.second) != 0 && seedErr == nil {
				seedErr = led.Seed(tt.second)
			}
			var conflict *ReconciliationError
			if !errors.As(seedErr, &conflict) {
				t.Fatalf("Seed error = %v, want *ReconciliationError", seedErr)
			}
		})
	}
}

func TestDVAUT003UniqueLegacyTitleBindsOnceAndAmbiguousLegacyConflicts(t *testing.T) {
	path := ledgerPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"version":1,"items":[{"slug":"legacy","title":"Legacy","priority":"high","status":"pending","stage_state":"pending"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	led, err := LoadLedger(path, newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	if err := led.Seed([]Item{{SourceID: "REQ-legacy", Title: "Legacy", Description: "complete requirement"}}); err != nil {
		t.Fatal(err)
	}
	item, _ := led.Get("legacy")
	if item.ItemID == "" || item.SourceID != "REQ-legacy" || item.Description != "complete requirement" {
		t.Fatalf("bound legacy item = %+v, want durable item/source identity and requirement", item)
	}
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}

	activePath := ledgerPath(t)
	if err := os.MkdirAll(filepath.Dir(activePath), 0o755); err != nil {
		t.Fatal(err)
	}
	activeLegacy := `{"version":1,"items":[{"slug":"active","title":"Active","priority":"high","status":"in-progress","stage":"spec-to-plan","stage_state":"running"}]}`
	if err := os.WriteFile(activePath, []byte(activeLegacy), 0o644); err != nil {
		t.Fatal(err)
	}
	activeLedger, err := LoadLedger(activePath, newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	if err := activeLedger.Seed([]Item{{SourceID: "REQ-active", Title: "Active", Description: "recovered active requirement"}}); err != nil {
		t.Fatalf("unique active legacy binding failed: %v", err)
	}
	active, _ := activeLedger.Get("active")
	if active.Description != "recovered active requirement" || !active.RevisionFrozen {
		t.Fatalf("active legacy binding = %+v, want recovered frozen revision", active)
	}

	deferredPath := ledgerPath(t)
	if err := os.MkdirAll(filepath.Dir(deferredPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deferredPath, []byte(activeLegacy), 0o644); err != nil {
		t.Fatal(err)
	}
	deferred, err := LoadLedger(deferredPath, newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	var missing *ReconciliationError
	if err := deferred.Seed(nil); !errors.As(err, &missing) {
		t.Fatalf("missing legacy active error = %v, want *ReconciliationError", err)
	}
	if err := deferred.Save(); err != nil {
		t.Fatal(err)
	}
	deferred, err = LoadLedger(deferredPath, newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	if err := deferred.Seed([]Item{{SourceID: "REQ-active", Title: "Active", Description: "recovered after restart"}}); err != nil {
		t.Fatalf("deferred legacy binding failed after v2 reload: %v", err)
	}
	deferredActive, _ := deferred.Get("active")
	if deferredActive.LegacyBindingPending || deferredActive.Description != "recovered after restart" {
		t.Fatalf("deferred active binding = %+v, want completed one-time binding", deferredActive)
	}

	ambiguousPath := ledgerPath(t)
	if err := os.MkdirAll(filepath.Dir(ambiguousPath), 0o755); err != nil {
		t.Fatal(err)
	}
	ambiguous := `{"version":1,"items":[{"slug":"dup-1","title":"Duplicate","priority":"high","status":"pending","stage_state":"pending"},{"slug":"dup-2","title":"Duplicate","priority":"low","status":"pending","stage_state":"pending"}]}`
	if err := os.WriteFile(ambiguousPath, []byte(ambiguous), 0o644); err != nil {
		t.Fatal(err)
	}
	ambiguousLedger, err := LoadLedger(ambiguousPath, newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	var conflict *ReconciliationError
	if err := ambiguousLedger.Seed([]Item{{Title: "Duplicate"}}); !errors.As(err, &conflict) {
		t.Fatalf("ambiguous legacy Seed error = %v, want *ReconciliationError", err)
	}
}

func TestDVAUT003MissingItemsBecomeOrphanedBlockedOrHistorical(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	if err := led.Seed([]Item{
		{SourceID: "REQ-pending", Title: "Pending"},
		{SourceID: "REQ-running", Title: "Running"},
		{SourceID: "REQ-done", Title: "Done"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := led.Commit("running", func(item *LedgerItem) { item.Status = StatusInProgress }); err != nil {
		t.Fatal(err)
	}
	if err := led.Commit("done", func(item *LedgerItem) { item.Status = StatusInProgress }); err != nil {
		t.Fatal(err)
	}
	if err := led.Commit("done", func(item *LedgerItem) {
		item.Status = StatusDone
		item.Stage = StageDone
		item.StageState = StageStateVerified
	}); err != nil {
		t.Fatal(err)
	}

	var blocked *ReconciliationError
	if err := led.Seed(nil); !errors.As(err, &blocked) {
		t.Fatalf("missing active Seed error = %v, want *ReconciliationError", err)
	}
	pending, _ := led.Get("pending")
	running, _ := led.Get("running")
	done, _ := led.Get("done")
	if pending.SourceState != SourceStateOrphaned || running.SourceState != SourceStateBlocked || done.SourceState != SourceStateHistorical {
		t.Fatalf("missing states pending/running/done = %q/%q/%q", pending.SourceState, running.SourceState, done.SourceState)
	}
	if len(led.Pending()) != 0 || len(led.InProgress()) != 0 {
		t.Fatalf("orphaned or blocked items remained runnable: pending=%v in-progress=%v", led.Pending(), led.InProgress())
	}
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadLedger(led.path, newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	reloadedPending, _ := reloaded.Get("pending")
	reloadedRunning, _ := reloaded.Get("running")
	if reloadedPending.SourceState != SourceStateOrphaned || reloadedRunning.SourceState != SourceStateBlocked {
		t.Fatalf("reloaded missing states = %q/%q", reloadedPending.SourceState, reloadedRunning.SourceState)
	}
}

func TestDVAUT003InProgressRequirementRevisionIsFrozen(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	if err := led.Seed([]Item{{SourceID: "REQ-frozen", Title: "Frozen", Description: "original requirement"}}); err != nil {
		t.Fatal(err)
	}
	if err := led.Commit("frozen", func(item *LedgerItem) { item.Status = StatusInProgress }); err != nil {
		t.Fatal(err)
	}
	before, _ := led.Get("frozen")
	if !before.RevisionFrozen || before.RequirementBytes != len("original requirement") || before.RequirementHash == "" {
		t.Fatalf("in-progress revision not frozen: %+v", before)
	}

	var conflict *ReconciliationError
	err = led.Seed([]Item{{SourceID: "REQ-frozen", Title: "Frozen renamed", Description: "edited requirement"}})
	if !errors.As(err, &conflict) {
		t.Fatalf("edited active Seed error = %v, want *ReconciliationError", err)
	}
	after, _ := led.Get("frozen")
	if after.Title != before.Title || after.Description != before.Description || after.RequirementHash != before.RequirementHash {
		t.Fatalf("active requirement changed despite conflict: before=%+v after=%+v", before, after)
	}
	if after.SourceState != SourceStateBlocked {
		t.Fatalf("edited active source state = %q, want blocked", after.SourceState)
	}
}

func TestDVAUT003ReconciliationConflictStopsBeforeWorktreeAndCore(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, factory, git, gh := testDeps(t, repoRoot, `## Renamed

**Description**: ambiguous rename
`)
	led, err := LoadLedger(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	if err := led.Seed([]Item{{Title: "Original", Description: "original requirement"}}); err != nil {
		t.Fatal(err)
	}
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}

	err = New(deps).Run(context.Background())
	var conflict *ReconciliationError
	if !errors.As(err, &conflict) {
		t.Fatalf("Run error = %v, want *ReconciliationError", err)
	}
	for _, call := range git.calls {
		if strings.HasPrefix(call, "worktree add") {
			t.Fatalf("reconciliation conflict created worktree: %q", call)
		}
	}
	for _, call := range gh.calls {
		if strings.HasPrefix(call, "gh issue") || strings.HasPrefix(call, "gh pr") {
			t.Fatalf("reconciliation conflict reached remote publication: %q", call)
		}
	}
	if len(factory.created) != 0 {
		t.Fatalf("reconciliation conflict created %d core runners", len(factory.created))
	}
}

func TestDVAUT003WorkflowCommitCannotRewriteIdentityOrFrozenRevision(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	if err := led.Seed([]Item{{SourceID: "REQ-immutable", Title: "Immutable", Description: "frozen text"}}); err != nil {
		t.Fatal(err)
	}
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}
	if err := led.Commit("immutable", func(item *LedgerItem) { item.ItemID = "item-rewritten" }); err == nil {
		t.Fatal("identity rewrite Commit returned nil")
	}
	if err := led.Commit("immutable", func(item *LedgerItem) { item.Status = StatusInProgress }); err != nil {
		t.Fatal(err)
	}
	before, _ := led.Get("immutable")
	err = led.Commit("immutable", func(item *LedgerItem) {
		item.Title = "Rewritten"
		item.Description = "rewritten text"
		item.RequirementBytes, item.RequirementHash = requirementIdentity(item.Description)
	})
	if err == nil {
		t.Fatal("frozen revision rewrite Commit returned nil")
	}
	after, _ := led.Get("immutable")
	if after.ItemID != before.ItemID || after.Title != before.Title || after.Description != before.Description || after.RequirementHash != before.RequirementHash {
		t.Fatalf("failed workflow mutation changed authoritative state: before=%+v after=%+v", before, after)
	}
}

func TestDVAUT003UnfrozenActiveLedgerFailsClosed(t *testing.T) {
	path := ledgerPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	invalid := `{"version":2,"items":[{"item_id":"item-invalid","slug":"invalid","title":"Invalid","description":"","requirement_bytes":7,"requirement_hash":"sha256:96c34a0719eb565c268a1626c44313a20de4f83927890933bd9b480849adb76c","revision_frozen":false,"source_state":"current","source_order":0,"priority":"high","status":"in-progress","stage_state":"pending"}]}`
	if err := os.WriteFile(path, []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadLedger(path, newTestClock())
	var stateErr *InvalidLedgerStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("LoadLedger error = %v, want *InvalidLedgerStateError", err)
	}
}

func TestDVAUT004RequirementMaterializationPreservesAuthorityAndIdentity(t *testing.T) {
	description := "first paragraph\n\n```go\nfmt.Println(\"你好\")\n```\nlast paragraph"
	bytes, hash := requirementIdentity(description)
	sc := &StageContext{
		Item:             Item{Title: "Formatting", Description: description},
		ItemID:           "item-formatting",
		RequirementBytes: bytes,
		RequirementHash:  hash,
		Slug:             "formatting",
		FeatureDir:       ".codexspec/specs/2026-0811-1200ab-formatting",
	}
	doc := requirementsDocument(sc, time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(doc, description) {
		t.Fatalf("materialized document lost exact multiline description:\n%s", doc)
	}
	for _, marker := range []string{
		"**Item ID**: `item-formatting`",
		fmt.Sprintf("**Requirement Bytes**: %d", bytes),
		fmt.Sprintf("**Requirement SHA-256**: `%s`", hash),
	} {
		if !strings.Contains(doc, marker) {
			t.Errorf("materialized document missing %q", marker)
		}
	}

	long := strings.Repeat("界", 4500)
	sc.Item.Description = long
	sc.RequirementBytes, sc.RequirementHash = requirementIdentity(long)
	doc = requirementsDocument(sc, time.Now())
	if !strings.Contains(doc, long) {
		t.Fatal("materialized document truncated the authoritative >4000-rune description")
	}

	sc.Item.Description = ""
	sc.RequirementBytes, sc.RequirementHash = requirementIdentity("Formatting")
	if doc := requirementsDocument(sc, time.Now()); !strings.Contains(doc, "## Authoritative Requirement\n\nFormatting") {
		t.Error("empty description does not fall back deterministically to the title")
	}
}

func TestDVAUT004BacklogPreservesExactNormalizedUTF8WithinWholeFileLimit(t *testing.T) {
	description := "first paragraph\r\n\r\n```md\r\n**Status**: literal metadata\r\n## [feature] literal heading\r\n你好\r\n```\r\nlast paragraph"
	path := writeBacklog(t, "## Exact\r\n\r\n**Description**: "+description+"\r\n")
	items, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll(description, "\r\n", "\n")
	if len(items) != 1 || items[0].Description != want {
		t.Fatalf("description = %q, want exact normalized %q", items[0].Description, want)
	}

	large := strings.Repeat("x", 2*1024*1024)
	path = writeBacklog(t, "## Large\n\n**Description**: "+large+"\n")
	items, err = Parse(path)
	if err != nil || len(items) != 1 || items[0].Description != large {
		t.Fatalf("large description bytes = %d, items=%d err=%v", lenItemDescription(items), len(items), err)
	}
}

func TestDVAUT004BacklogRejectsOversizeAndInvalidUTF8Atomically(t *testing.T) {
	oversize := filepath.Join(t.TempDir(), "oversize.md")
	if err := os.WriteFile(oversize, []byte("## Oversize\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(oversize, maxBacklogBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(oversize); err == nil {
		t.Fatal("Parse accepted a backlog beyond the 64 MiB whole-file ceiling")
	}

	invalid := filepath.Join(t.TempDir(), "invalid.md")
	if err := os.WriteFile(invalid, []byte{'#', '#', ' ', 0xff, '\n'}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(invalid); err == nil {
		t.Fatal("Parse accepted invalid UTF-8")
	}
}

func TestDVAUT004MaterializationReplacesStaleTruncatedAuthority(t *testing.T) {
	workDir := t.TempDir()
	featureDir := ".codexspec/specs/2026-0811-1200ab-stale"
	requirementsPath := filepath.Join(workDir, featureDir, "requirements.md")
	if err := os.MkdirAll(filepath.Dir(requirementsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(requirementsPath, []byte("old truncated authority"), 0o644); err != nil {
		t.Fatal(err)
	}
	description := strings.Repeat("complete\n", 1000)
	bytes, hash := requirementIdentity(description)
	sc := &StageContext{
		Item:             Item{Title: "Stale", Description: description},
		ItemID:           "item-stale",
		RequirementBytes: bytes,
		RequirementHash:  hash,
		Slug:             "stale",
		WorkDir:          workDir,
		FeatureDir:       featureDir,
	}
	if err := materializeRequirements(newTestClock())(context.Background(), sc); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(requirementsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), description) || strings.Contains(string(data), "old truncated authority") {
		t.Fatal("materialization retained stale truncated authority")
	}
}

func lenItemDescription(items []Item) int {
	if len(items) == 0 {
		return 0
	}
	return len(items[0].Description)
}

func TestDVAUT005FeatureWorkspaceRejectsInvalidFeatureDirectories(t *testing.T) {
	clock := newTestClock()
	outside := t.TempDir()
	cases := map[string]string{
		"absolute":           filepath.Join(outside, "absolute-feature"),
		"traversal":          ".codexspec/specs/../escape",
		"missing-prefix":     "specs/feature",
		"extra-component":    ".codexspec/specs/feature/nested",
		"empty-component":    ".codexspec/specs//feature",
		"malformed-name":     ".codexspec/specs/feature name",
		"platform-separator": `.codexspec\specs\feature`,
	}
	for name, featureDir := range cases {
		t.Run(name, func(t *testing.T) {
			workDir := t.TempDir()
			sc := &StageContext{
				Item:       Item{Title: "Invalid", Description: "must remain rooted"},
				Slug:       "invalid",
				WorkDir:    workDir,
				FeatureDir: featureDir,
			}
			if err := materializeRequirements(clock)(context.Background(), sc); err == nil {
				t.Fatalf("materializeRequirements accepted FeatureDir %q", featureDir)
			}
			if ok, _ := verifySpecArtifact("requirements.md")(context.Background(), sc); ok {
				t.Fatalf("verifySpecArtifact accepted FeatureDir %q", featureDir)
			}
		})
	}
}

func TestDVAUT005FeatureWorkspaceRejectsSymlinksAndNonRegularTargets(t *testing.T) {
	clock := newTestClock()

	for _, component := range []string{".codexspec", "specs", "linked-feature"} {
		t.Run("directory-symlink-"+component, func(t *testing.T) {
			workDir := t.TempDir()
			outside := t.TempDir()
			link := filepath.Join(workDir, ".codexspec")
			switch component {
			case "specs":
				if err := os.MkdirAll(link, 0o755); err != nil {
					t.Fatal(err)
				}
				link = filepath.Join(link, "specs")
			case "linked-feature":
				link = filepath.Join(link, "specs")
				if err := os.MkdirAll(link, 0o755); err != nil {
					t.Fatal(err)
				}
				link = filepath.Join(link, "linked-feature")
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			sc := &StageContext{Item: Item{Title: "Link", Description: "outside"}, Slug: "link", WorkDir: workDir, FeatureDir: ".codexspec/specs/linked-feature"}
			if err := materializeRequirements(clock)(context.Background(), sc); err == nil {
				t.Fatalf("materialization accepted symlink component %s", component)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("materialization changed symlink target: entries=%v err=%v", entries, err)
			}
		})
	}

	t.Run("materialization-file-symlink", func(t *testing.T) {
		workDir := t.TempDir()
		outside := t.TempDir()
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
		sc := &StageContext{Item: Item{Title: "Link", Description: "replacement"}, WorkDir: workDir, FeatureDir: ".codexspec/specs/file-link"}
		if err := materializeRequirements(clock)(context.Background(), sc); err == nil {
			t.Fatal("materialization accepted a symlink requirements target")
		}
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "external authority" {
			t.Fatalf("external target changed to %q", data)
		}
	})

	t.Run("all-read-consumers", func(t *testing.T) {
		workDir := t.TempDir()
		outside := t.TempDir()
		featureDir := ".codexspec/specs/read-links"
		feature := filepath.Join(workDir, filepath.FromSlash(featureDir))
		if err := os.MkdirAll(feature, 0o755); err != nil {
			t.Fatal(err)
		}
		artifacts := map[string]string{
			"requirements.md": "authority",
			"review-spec.md":  "**Overall Status**: PASS\n",
			"tasks.md":        "- [x] complete\n",
		}
		for name, content := range artifacts {
			target := filepath.Join(outside, name)
			if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(feature, name)); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
		}
		if err := os.WriteFile(filepath.Join(feature, "spec.md"), []byte("spec"), 0o644); err != nil {
			t.Fatal(err)
		}
		sc := &StageContext{WorkDir: workDir, FeatureDir: featureDir}
		if ok, _ := verifySpecArtifact("requirements.md")(context.Background(), sc); ok {
			t.Fatal("stage verification accepted a symlink artifact")
		}
		if ok, _ := verifyReviewedArtifact("spec.md", "review-spec.md")(context.Background(), sc); ok {
			t.Fatal("review parsing accepted a symlink artifact")
		}
		if ok, _ := verifyTasksComplete(sc); ok {
			t.Fatal("task inspection accepted a symlink artifact")
		}

		if err := os.Remove(filepath.Join(feature, "requirements.md")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(feature, "requirements.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		if ok, _ := verifySpecArtifact("requirements.md")(context.Background(), sc); ok {
			t.Fatal("stage verification accepted a non-regular artifact")
		}
	})
}

func TestDVAUT005LegacyLedgerRejectsIllegalFeatureDir(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, factory, git, gh := testDeps(t, repoRoot, `## [feature] Legacy

**Priority**: high
**Description**: authority
`)
	path := filepath.Join(repoRoot, ".foxharness", "autodev-state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"items":[{"slug":"legacy","title":"Legacy","description":"authority","priority":"high","status":"in-progress","stage":"generate-spec","feature_dir":"../escape"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	err := New(deps).Run(context.Background())
	var invalid *InvalidLedgerStateError
	if !errors.As(err, &invalid) {
		t.Fatalf("Run error = %T %v, want InvalidLedgerStateError", err, err)
	}
	if len(git.calls) != 0 || len(gh.calls) != 0 || len(factory.created) != 0 {
		t.Fatalf("illegal legacy feature directory reached external work: git=%v gh=%v core=%d", git.calls, gh.calls, len(factory.created))
	}
}

type symlinkProvisioningGit struct {
	orchestraGit
	outside string
}

func (g *symlinkProvisioningGit) Run(ctx context.Context, dir string, args ...string) (string, error) {
	if len(args) >= 5 && args[0] == "worktree" && args[1] == "add" && args[2] == "-b" {
		g.mu.Lock()
		g.calls = append(g.calls, strings.Join(args, " "))
		g.mu.Unlock()
		worktreePath := args[4]
		if err := os.MkdirAll(worktreePath, 0o755); err != nil {
			return "", err
		}
		if err := os.Symlink(g.outside, filepath.Join(worktreePath, ".codexspec")); err != nil {
			return "", err
		}
		return "", nil
	}
	return g.orchestraGit.Run(ctx, dir, args...)
}

func TestDVAUT005WorkspaceViolationStopsBeforeCoreExecution(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, factory, _, _ := testDeps(t, repoRoot, `## [feature] Rooted

**Priority**: high
**Description**: remain inside the worktree
`)
	git := &symlinkProvisioningGit{outside: t.TempDir()}
	git.insideWT = true
	deps.Git = git
	deps.BuildPipeline = RequirementsFirstPipeline

	err := New(deps).Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Run error = %v, want workspace symlink rejection", err)
	}
	if len(factory.created) != 0 {
		t.Fatalf("workspace violation created %d core runner(s), want zero", len(factory.created))
	}
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
			ok, _ := publisher.verifyIssue()(context.Background(), sc)
			if ok != tc.wantOK || sc.Issue != tc.wantIssue {
				t.Fatalf("ok/issue = %v/%d, want %v/%d", ok, sc.Issue, tc.wantOK, tc.wantIssue)
			}
			joined := strings.Join(exec.calls[0], " ")
			if !strings.Contains(joined, "--state all") || !strings.Contains(joined, "--limit 20") {
				t.Errorf("issue query = %q, want closed-inclusive fixed first page", joined)
			}
		})
	}
}

func TestDVAUT007RecordedIssueSkipsVerificationWithoutStableCorrelation(t *testing.T) {
	exec := &fixedExec{out: `[]`}
	cfg := defaultConfig(t.TempDir())
	publisher := NewRemotePublisher(newTestMachine(&reviewingEngineer{}), &orchestraGit{insideWT: true}, exec, NewTerminalReporter(io.Discard), cfg)
	steps := publisher.steps()
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
