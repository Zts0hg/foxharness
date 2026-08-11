package autodev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
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
		{name: "issue-delivery-ack", failAtCall: 22, maxCoreRuns: 6, maxIssueQueries: 1, maxIssueReports: 1},
		{name: "pr-intent", failAtCall: 23, maxCoreRuns: 6, maxIssueQueries: 1, maxIssueReports: 1},
		{name: "pr-binding", failAtCall: 24, maxCoreRuns: 7, maxIssueQueries: 1, maxPRQueries: 1, maxIssueReports: 1},
		{name: "done", failAtCall: 25, maxCoreRuns: 7, maxIssueQueries: 1, maxPRQueries: 1, maxIssueReports: 1, maxPRReports: 1},
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

func (g *symlinkProvisioningGit) Run(ctx context.Context, dir string, args ...string) (CommandResult, error) {
	if len(args) >= 5 && args[0] == "worktree" && args[1] == "add" && args[2] == "-b" {
		g.mu.Lock()
		g.calls = append(g.calls, strings.Join(args, " "))
		g.mu.Unlock()
		worktreePath := args[4]
		if err := os.MkdirAll(worktreePath, 0o755); err != nil {
			return CommandResult{}, err
		}
		if err := os.Symlink(g.outside, filepath.Join(worktreePath, ".codexspec")); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{}, nil
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

func TestDVAUT006ExecRunnerBoundsIndependentOutputStreams(t *testing.T) {
	t.Setenv("FOX_AUTODEV_HELPER", "1")
	result, err := NewExecCommandRunner().Run(context.Background(), t.TempDir(), os.Args[0],
		"-test.run=TestAutodevExecHelperProcess", "--", "output")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stdout) != maxCommandStreamBytes || len(result.Stderr) != maxCommandStreamBytes {
		t.Fatalf("captured stdout/stderr bytes = %d/%d, want %d each", len(result.Stdout), len(result.Stderr), maxCommandStreamBytes)
	}
	var overflow *CommandOverflowError
	if !errors.As(result.OverflowError(), &overflow) || !overflow.Stdout || !overflow.Stderr {
		t.Fatalf("overflow evidence = %#v, want typed stdout+stderr overflow", result.OverflowError())
	}
}

func TestDVAUT006CancellationReapsDescendantAndSuppressesLaterGates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process liveness proof")
	}
	t.Setenv("FOX_AUTODEV_HELPER", "1")
	runner := NewExecCommandRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err := runner.Run(ctx, t.TempDir(), os.Args[0], "-test.run=TestAutodevExecHelperProcess", "--", "spawn")
	if err == nil {
		t.Fatal("cancelled helper returned nil error")
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(result.Stdout))
	if parseErr != nil {
		t.Fatalf("parse descendant pid from %q: %v", result.Stdout, parseErr)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && testProcessAlive(pid) {
		time.Sleep(10 * time.Millisecond)
	}
	if testProcessAlive(pid) {
		testForceKill(pid)
		t.Fatal("descendant remained alive after process-tree cancellation and reaping")
	}

	fx := &cancelCountingExec{}
	gate := NewGateRunner(fx, nil)
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := gate.Check(cancelled, t.TempDir(), GateConfig{Build: true, Test: true, Gofmt: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Gate Check error = %v, want context.Canceled", err)
	}
	if fx.calls != 0 {
		t.Fatalf("commands started after cancellation = %d, want zero", fx.calls)
	}
}

func TestDVAUT006CancellationEscalatesTERMToKILL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix signal escalation proof")
	}
	t.Setenv("FOX_AUTODEV_HELPER", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := NewExecCommandRunner().Run(ctx, t.TempDir(), os.Args[0], "-test.run=TestAutodevExecHelperProcess", "--", "spawn-ignore-term")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed < commandTerminateGrace {
		t.Fatalf("termination elapsed = %v, want TERM grace before KILL", elapsed)
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(result.Stdout))
	if parseErr != nil {
		t.Fatalf("parse descendant pid from %q: %v", result.Stdout, parseErr)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && testProcessAlive(pid) {
		time.Sleep(10 * time.Millisecond)
	}
	if testProcessAlive(pid) {
		testForceKill(pid)
		t.Fatal("TERM-resistant descendant remained alive after KILL escalation")
	}
}

type cancelCountingExec struct{ calls int }

func (e *cancelCountingExec) Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error) {
	e.calls++
	return CommandResult{Stderr: "cancelled"}, ctx.Err()
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
		_, _ = io.WriteString(os.Stderr, strings.Repeat("y", 2*1024*1024))
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
	case "spawn-ignore-term":
		signal.Ignore(syscall.SIGTERM)
		child := exec.Command(os.Args[0], "-test.run=TestAutodevExecHelperProcess", "--", "ignore-term-sleep")
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
	case "ignore-term-sleep":
		signal.Ignore(syscall.SIGTERM)
		time.Sleep(10 * time.Second)
	}
	os.Exit(0)
}

type fixedExec struct {
	out    string
	result *CommandResult
	calls  [][]string
}

func (e *fixedExec) Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error) {
	e.calls = append(e.calls, append([]string{name}, args...))
	if e.result != nil {
		return *e.result, nil
	}
	return stdoutResult(e.out), nil
}

func TestDVAUT006StrictJSONQueryRejectsOverflow(t *testing.T) {
	exec := &fixedExec{result: &CommandResult{
		Stdout:         `[{"number":7,"title":"Same"}]`,
		StdoutOverflow: true,
	}}
	publisher := NewRemotePublisher(newTestMachine(&reviewingEngineer{}), &orchestraGit{insideWT: true}, exec, NewTerminalReporter(io.Discard), defaultConfig(t.TempDir()))
	ok, gap := publisher.verifyIssue()(context.Background(), &StageContext{ItemID: "item-overflow", Item: Item{Title: "Same"}, WorkDir: t.TempDir()})
	if ok || !strings.Contains(gap, "capture limit") {
		t.Fatalf("verifyIssue = %v, gap %q, want strict overflow failure", ok, gap)
	}
}

type overflowGateExec struct{}

func (overflowGateExec) Run(context.Context, string, string, ...string) (CommandResult, error) {
	return CommandResult{Stdout: "successful diagnostics", StdoutOverflow: true}, nil
}

func TestDVAUT006QualityGateReportsOverflowWithoutChangingExitStatus(t *testing.T) {
	result, err := NewGateRunner(overflowGateExec{}, nil).Check(context.Background(), t.TempDir(), GateConfig{Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || len(result.Steps) != 3 || !result.Steps[1].Passed || !result.Steps[1].OutputTruncated {
		t.Fatalf("gate result = %+v, want passing test step with truncation evidence", result)
	}
	if !strings.Contains(result.Steps[1].Output, "output truncated") {
		t.Fatalf("gate output = %q, want explicit truncation marker", result.Steps[1].Output)
	}
}

type deadlineCore struct {
	stubCore
	remaining time.Duration
	has       bool
}

func (c *deadlineCore) Run(ctx context.Context, prompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	deadline, ok := ctx.Deadline()
	c.has = ok
	if ok {
		c.remaining = time.Until(deadline)
	}
	return &engine.RunResult{FinalMessage: "done"}, nil
}

func TestDVAUT006CoreAttemptReceivesDefaultDeadline(t *testing.T) {
	core := &deadlineCore{}
	stage := Stage{Name: "generate-spec", Prompt: func(*StageContext) string { return "work" }, Verify: func(context.Context, *StageContext) (bool, string) { return true, "" }}
	if err := newTestMachine(&reviewingEngineer{}).RunStep(context.Background(), core, &StageContext{}, stage); err != nil {
		t.Fatal(err)
	}
	if !core.has || core.remaining < 29*time.Minute || core.remaining > stageAttemptTimeout {
		t.Fatalf("core deadline remaining = %v (present=%v), want 30-minute default", core.remaining, core.has)
	}
}

func TestDVAUT006EarlierCallerDeadlineWins(t *testing.T) {
	t.Setenv("FOX_AUTODEV_HELPER", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := NewExecCommandRunner().Run(ctx, t.TempDir(), os.Args[0], "-test.run=TestAutodevExecHelperProcess", "--", "sleep")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want caller deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("caller deadline took %v, want earlier than command default", elapsed)
	}
}

func TestDVAUT006CommandClassesUseRequiredDefaultDeadlines(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "git-query", got: gitCommandTimeout([]string{"status", "--porcelain"}), want: 30 * time.Second},
		{name: "worktree-list-query", got: gitCommandTimeout([]string{"worktree", "list", "--porcelain"}), want: 30 * time.Second},
		{name: "worktree-mutation", got: gitCommandTimeout([]string{"worktree", "add"}), want: 30 * time.Minute},
		{name: "github-query", got: execCommandTimeout("gh"), want: 30 * time.Second},
		{name: "quality-gate", got: execCommandTimeout("go"), want: 10 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("default timeout = %v, want %v", tc.got, tc.want)
			}
		})
	}
}

func TestDVAUT006CancelledContextSuppressesStageAndRemoteOperations(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	core := &stubCore{}
	stage := Stage{Name: "generate-spec", Prompt: func(*StageContext) string { return "work" }, Verify: func(context.Context, *StageContext) (bool, string) { return true, "" }}
	if err := newTestMachine(&reviewingEngineer{}).RunStep(cancelled, core, &StageContext{}, stage); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunStep error = %v, want context.Canceled", err)
	}
	if core.runs != 0 {
		t.Fatalf("cancelled stage core runs = %d, want zero", core.runs)
	}

	records := 0
	publisher := NewRemotePublisher(newTestMachine(&reviewingEngineer{}), &orchestraGit{insideWT: true}, &fixedExec{}, NewTerminalReporter(io.Discard), defaultConfig(t.TempDir()))
	_, err := publisher.Publish(cancelled, core, Worktree{Path: t.TempDir(), Branch: "auto/x"}, LedgerItem{
		Slug: "x", Status: StatusInProgress, Stage: StagePublish, StageState: StageStateRunning,
	}, func(string, func(*LedgerItem)) error {
		records++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Publish error = %v, want context.Canceled", err)
	}
	if records != 0 || core.runs != 0 {
		t.Fatalf("cancelled publish records/core runs = %d/%d, want zero", records, core.runs)
	}
}

type issueQueryExec struct {
	view   CommandResult
	search CommandResult
	calls  []string
}

func (e *issueQueryExec) Run(ctx context.Context, dir, name string, args ...string) (CommandResult, error) {
	call := name + " " + strings.Join(args, " ")
	e.calls = append(e.calls, call)
	if len(args) >= 2 && args[0] == "issue" && args[1] == "view" {
		return e.view, nil
	}
	if len(args) >= 2 && args[0] == "api" && args[1] == "--paginate" {
		return e.search, nil
	}
	return CommandResult{}, fmt.Errorf("unexpected issue query: %s", call)
}

func TestDVAUT007IssueMarkerResolutionAndPagination(t *testing.T) {
	itemID := ItemID("item-stable")
	marker := "<!-- fox-autodev-item-id:item-stable -->"
	cases := []struct {
		name      string
		recorded  int
		view      string
		search    string
		wantOK    bool
		wantIssue int
		conflict  bool
	}{
		{name: "recorded-renamed-closed", recorded: 404, view: `{"number":404,"title":"Renamed","body":"` + marker + `","state":"CLOSED"}`, wantOK: true, wantIssue: 404},
		{name: "recorded-wrong-marker", recorded: 404, view: `{"number":404,"body":"other","state":"OPEN"}`, wantIssue: 404, conflict: true},
		{name: "zero-match", search: `[{"items":[{"number":1,"body":"other"}]},{"items":[]}]`},
		{name: "one-match-later-page", search: `[{"items":[{"number":1,"body":"other"}]},{"items":[{"number":7,"title":"Renamed","body":"` + marker + `","state":"CLOSED"}]}]`, wantOK: true, wantIssue: 7},
		{name: "multiple-match-conflict", search: `[{"items":[{"number":7,"body":"` + marker + `"}]},{"items":[{"number":9,"body":"` + marker + `"}]}]`, conflict: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := &issueQueryExec{view: stdoutResult(tc.view), search: stdoutResult(tc.search)}
			publisher := NewRemotePublisher(newTestMachine(&reviewingEngineer{}), &orchestraGit{insideWT: true}, exec, NewTerminalReporter(io.Discard), defaultConfig(t.TempDir()))
			sc := &StageContext{ItemID: itemID, Item: Item{Title: "Mutable title"}, WorkDir: t.TempDir(), Issue: tc.recorded}
			ok, _, err := publisher.resolveIssue(context.Background(), sc)
			var conflict *IssueIdentityConflictError
			if tc.conflict != errors.As(err, &conflict) {
				t.Fatalf("conflict error = %T %v, want conflict=%v", err, err, tc.conflict)
			}
			if !tc.conflict && err != nil {
				t.Fatal(err)
			}
			if ok != tc.wantOK || sc.Issue != tc.wantIssue {
				t.Fatalf("ok/issue = %v/%d, want %v/%d", ok, sc.Issue, tc.wantOK, tc.wantIssue)
			}
			joined := strings.Join(exec.calls, "\n")
			if tc.recorded != 0 && !strings.Contains(joined, "issue view 404") {
				t.Fatalf("recorded issue query = %q, want issue view", joined)
			}
			if tc.recorded == 0 && (!strings.Contains(joined, "api --paginate --slurp") || !strings.Contains(joined, "fox-autodev-item-id%3Aitem-stable")) {
				t.Fatalf("unbound issue query = %q, want complete marker pagination", joined)
			}
		})
	}
}

func TestDVAUT007IssueCreationPromptCarriesExactItemMarker(t *testing.T) {
	publisher := NewRemotePublisher(newTestMachine(&reviewingEngineer{}), &orchestraGit{insideWT: true}, &fixedExec{}, NewTerminalReporter(io.Discard), defaultConfig(t.TempDir()))
	for _, stage := range publisher.steps() {
		if stage.Name != "issue" {
			continue
		}
		prompt := stage.Prompt(&StageContext{ItemID: "item-stable", Item: Item{Title: "Mutable title"}})
		if !strings.Contains(prompt, "<!-- fox-autodev-item-id:item-stable -->") {
			t.Fatalf("issue prompt missing exact durable marker: %q", prompt)
		}
		return
	}
	t.Fatal("issue stage not configured")
}

func TestDVAUT007TerminalReporterConsumesEventIDIdempotently(t *testing.T) {
	var out strings.Builder
	reporter := NewTerminalReporter(&out)
	event := RemoteEvent{EventID: "issue:item-stable:7", ItemID: "item-stable", Kind: RemoteEventIssue, Number: 7}
	if err := reporter.OnRemoteEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := reporter.OnRemoteEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(out.String(), "issue #7"); got != 1 {
		t.Fatalf("issue report count = %d, want one logical event for duplicate EventID\n%s", got, out.String())
	}
}

type lifecycleCore struct {
	stubCore
	release      <-chan struct{}
	closeRelease <-chan struct{}
	done         chan struct{}
	drainStarted chan struct{}
	closeStarted chan struct{}
	launchOnce   sync.Once
	drainOnce    sync.Once
	closeOnce    sync.Once
	mu           sync.Mutex
	runCount     int
	drainedRuns  int
	closeCalls   int
}

func (c *lifecycleCore) Run(ctx context.Context, prompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	c.mu.Lock()
	if c.drainedRuns != c.runCount {
		c.mu.Unlock()
		return nil, errors.New("next core run started before prior post-run work drained")
	}
	c.runCount++
	c.mu.Unlock()
	c.launchOnce.Do(func() {
		go func() {
			<-c.release
			close(c.done)
		}()
	})
	return c.stubCore.Run(ctx, prompt, reporter)
}

func (c *lifecycleCore) Drain(ctx context.Context) error {
	c.drainOnce.Do(func() { close(c.drainStarted) })
	select {
	case <-c.done:
		c.mu.Lock()
		c.drainedRuns = c.runCount
		c.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *lifecycleCore) Close(ctx context.Context) error {
	c.mu.Lock()
	c.closeCalls++
	c.mu.Unlock()
	c.closeOnce.Do(func() { close(c.closeStarted) })
	select {
	case <-c.closeRelease:
		return c.Drain(ctx)
	case <-ctx.Done():
		return ctx.Err()
	}
}

type lifecycleCoreFactory struct {
	core *lifecycleCore
}

func (f *lifecycleCoreFactory) New(ctx context.Context, workDir, model string) (CoreRunner, error) {
	f.core.workDir = workDir
	return f.core, nil
}

func TestDVAUT008OrchestratorDrainsEveryRunAndClosesBeforeWorktreeRemoval(t *testing.T) {
	coreType := reflect.TypeOf((*CoreRunner)(nil)).Elem()
	for _, method := range []string{"Drain", "Close"} {
		if _, ok := coreType.MethodByName(method); !ok {
			t.Fatalf("CoreRunner does not expose %s lifecycle ownership", method)
		}
	}

	repoRoot := t.TempDir()
	deps, _, _, git, _ := testDeps(t, repoRoot, `## Item

**Description**: async extraction
`)
	release := make(chan struct{})
	closeRelease := make(chan struct{})
	core := &lifecycleCore{
		release:      release,
		closeRelease: closeRelease,
		done:         make(chan struct{}),
		drainStarted: make(chan struct{}),
		closeStarted: make(chan struct{}),
	}
	deps.CoreFactory = &lifecycleCoreFactory{core: core}
	runDone := make(chan error, 1)
	go func() { runDone <- New(deps).Run(context.Background()) }()
	select {
	case <-core.drainStarted:
	case <-time.After(time.Second):
		t.Fatal("orchestrator did not drain after the first core run")
	}
	select {
	case err := <-runDone:
		t.Fatalf("orchestrator returned before extraction completed: %v", err)
	default:
	}
	removed := false
	for _, call := range git.calls {
		removed = removed || strings.HasPrefix(call, "worktree remove")
	}
	if removed {
		t.Fatal("orchestrator removed the worktree while post-run work was pending")
	}
	close(release)
	select {
	case <-core.closeStarted:
	case <-time.After(time.Second):
		t.Fatal("orchestrator did not perform final CoreRunner.Close")
	}
	for _, call := range git.calls {
		if strings.HasPrefix(call, "worktree remove") {
			t.Fatal("orchestrator removed the worktree before CoreRunner.Close completed")
		}
	}
	close(closeRelease)
	if err := <-runDone; err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	runCount, drainedRuns, closeCalls := core.runCount, core.drainedRuns, core.closeCalls
	core.mu.Unlock()
	if runCount == 0 || drainedRuns != runCount || closeCalls != 1 {
		t.Fatalf("lifecycle runs/drained/close = %d/%d/%d, want every run drained and one close", runCount, drainedRuns, closeCalls)
	}
	removed = false
	for _, call := range git.calls {
		removed = removed || strings.HasPrefix(call, "worktree remove")
	}
	if !removed {
		t.Fatal("orchestrator did not remove the worktree after lifecycle close")
	}
}

type failingCloseCore struct{ stubCore }

func (c *failingCloseCore) Drain(context.Context) error { return nil }
func (c *failingCloseCore) Close(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

type failingCloseFactory struct{ core *failingCloseCore }

func (f failingCloseFactory) New(ctx context.Context, workDir, model string) (CoreRunner, error) {
	f.core.workDir = workDir
	return f.core, nil
}

func TestDVAUT008UndrainedLifecycleFailsAndRetainsWorktree(t *testing.T) {
	originalTimeout := coreLifecycleTimeout
	coreLifecycleTimeout = 50 * time.Millisecond
	defer func() { coreLifecycleTimeout = originalTimeout }()
	started := time.Now()
	repoRoot := t.TempDir()
	deps, recorder, _, git, _ := testDeps(t, repoRoot, `## Item

**Description**: retain undrained work
`)
	deps.CoreFactory = failingCloseFactory{core: &failingCloseCore{}}
	err := New(deps).Run(context.Background())
	var lifecycleErr *CoreLifecycleError
	if !errors.As(err, &lifecycleErr) {
		t.Fatalf("Run error = %T %v, want *CoreLifecycleError", err, err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded lifecycle close took %v, want under one second", elapsed)
	}
	for _, call := range git.calls {
		if strings.HasPrefix(call, "worktree remove") {
			t.Fatal("undrained lifecycle removed the item worktree")
		}
	}
	for _, event := range recorder.list() {
		if strings.HasPrefix(event, "done:") {
			t.Fatal("undrained lifecycle reported the item done")
		}
	}
}

type cancelingLifecycleCore struct {
	stubCore
	cancel     context.CancelFunc
	drainFresh bool
	closeFresh bool
}

func (c *cancelingLifecycleCore) Run(context.Context, string, engine.Reporter) (*engine.RunResult, error) {
	c.cancel()
	return &engine.RunResult{FinalMessage: "partial"}, context.Canceled
}

func (c *cancelingLifecycleCore) Drain(ctx context.Context) error {
	_, hasDeadline := ctx.Deadline()
	c.drainFresh = ctx.Err() == nil && hasDeadline
	return nil
}

func (c *cancelingLifecycleCore) Close(ctx context.Context) error {
	_, hasDeadline := ctx.Deadline()
	c.closeFresh = ctx.Err() == nil && hasDeadline
	return nil
}

type cancelingLifecycleFactory struct{ core *cancelingLifecycleCore }

func (f cancelingLifecycleFactory) New(ctx context.Context, workDir, model string) (CoreRunner, error) {
	f.core.workDir = workDir
	return f.core, nil
}

func TestDVAUT008ParentCancellationUsesFreshBoundedDrainAndClose(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, _, git, _ := testDeps(t, repoRoot, `## Item

**Description**: cancel extraction
`)
	ctx, cancel := context.WithCancel(context.Background())
	core := &cancelingLifecycleCore{cancel: cancel}
	deps.CoreFactory = cancelingLifecycleFactory{core: core}
	err := New(deps).Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context cancellation", err)
	}
	if !core.drainFresh || !core.closeFresh {
		t.Fatalf("fresh bounded lifecycle contexts drain/close = %v/%v, want true/true", core.drainFresh, core.closeFresh)
	}
	for _, call := range git.calls {
		if strings.HasPrefix(call, "worktree remove") {
			t.Fatal("canceled lifecycle removed the item worktree")
		}
	}
}

type visibilityLifecycleFactory struct {
	mu             sync.Mutex
	createdItems   int
	completedItems int
}

type visibilityLifecycleCore struct {
	stubCore
	factory *visibilityLifecycleFactory
	index   int
	pending bool
	closed  bool
}

func (f *visibilityLifecycleFactory) New(ctx context.Context, workDir, model string) (CoreRunner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := f.createdItems
	if f.completedItems != index {
		return nil, fmt.Errorf("item %d started before %d prior item lifecycles completed", index, index)
	}
	f.createdItems++
	return &visibilityLifecycleCore{stubCore: stubCore{workDir: workDir}, factory: f, index: index}, nil
}

func (c *visibilityLifecycleCore) Run(ctx context.Context, prompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	if c.pending {
		return nil, errors.New("next run started before prior extraction became visible")
	}
	c.pending = true
	return c.stubCore.Run(ctx, prompt, reporter)
}

func (c *visibilityLifecycleCore) Drain(context.Context) error {
	if !c.pending {
		return errors.New("drain did not correspond to one completed run")
	}
	c.pending = false
	return nil
}

func (c *visibilityLifecycleCore) Close(context.Context) error {
	if c.pending {
		return errors.New("item closed with pending extraction")
	}
	if c.closed {
		return nil
	}
	c.closed = true
	c.factory.mu.Lock()
	c.factory.completedItems++
	c.factory.mu.Unlock()
	return nil
}

func TestDVAUT008NextRunAndItemObserveCompletedPostRunState(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, _, _, _ := testDeps(t, repoRoot, twoItemBacklog)
	factory := &visibilityLifecycleFactory{}
	deps.CoreFactory = factory
	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	factory.mu.Lock()
	created, completed := factory.createdItems, factory.completedItems
	factory.mu.Unlock()
	if created != 2 || completed != 2 {
		t.Fatalf("item lifecycles created/completed = %d/%d, want 2/2", created, completed)
	}
}

func TestDVAUT009ConcurrencyAcceptsOnlyOmissionOrExactSerial(t *testing.T) {
	invalid := map[string]string{
		"parallel":        "concurrency: parallel\n",
		"numeric":         "concurrency: 8\n",
		"future":          "concurrency: future-mode\n",
		"whitespace":      "concurrency: \" serial \"\n",
		"case":            "concurrency: Serial\n",
		"empty":           "concurrency: \"\"\n",
		"null":            "concurrency: null\n",
		"non-scalar-list": "concurrency: [serial]\n",
	}
	for name, content := range invalid {
		t.Run(name, func(t *testing.T) {
			repoRoot := t.TempDir()
			writeConfig(t, repoRoot, content)
			_, err := Load(repoRoot)
			var concurrencyErr *InvalidConcurrencyError
			if !errors.As(err, &concurrencyErr) {
				t.Fatalf("Load error = %T %v, want *InvalidConcurrencyError", err, err)
			}
		})
	}
	for name, content := range map[string]string{
		"omitted": "model: test\n",
		"plain":   "concurrency: serial\n",
		"quoted":  "concurrency: \"serial\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			repoRoot := t.TempDir()
			writeConfig(t, repoRoot, content)
			cfg, err := Load(repoRoot)
			if err != nil || cfg.Concurrency != "serial" {
				t.Fatalf("Load = concurrency %q, error %v; want exact serial", cfg.Concurrency, err)
			}
		})
	}

	repoRoot := t.TempDir()
	deps, _, _, git, gh := testDeps(t, repoRoot, twoItemBacklog)
	deps.Config.Concurrency = "parallel"
	err := New(deps).Run(context.Background())
	var concurrencyErr *InvalidConcurrencyError
	if !errors.As(err, &concurrencyErr) {
		t.Fatalf("Run error = %T %v, want *InvalidConcurrencyError", err, err)
	}
	if len(git.calls) != 0 || len(gh.calls) != 0 {
		t.Fatalf("invalid concurrency reached preconditions: git=%v gh=%v", git.calls, gh.calls)
	}

	repoRoot = t.TempDir()
	deps, recorder, _, _, _ := testDeps(t, repoRoot, twoItemBacklog)
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
		t.Fatalf("serial events = %v, want strict serial order %v", itemEvents, want)
	}
}

type partialErrorCore struct {
	result *engine.RunResult
	err    error
}

func (c *partialErrorCore) Run(context.Context, string, engine.Reporter) (*engine.RunResult, error) {
	return c.result, c.err
}
func (*partialErrorCore) Drain(context.Context) error  { return nil }
func (*partialErrorCore) Close(context.Context) error  { return nil }
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
