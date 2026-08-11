package autodev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClock is a deterministic Clock for ledger tests.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

func newTestClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)}
}

func ledgerPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".foxharness", "autodev-state.json")
}

func TestLoadLedgerMissingFileYieldsEmptyLedger(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	if got := len(led.Pending()); got != 0 {
		t.Errorf("Pending() len = %d, want 0", got)
	}
}

func TestLedgerValidatesRemoteEventOutboxIdentity(t *testing.T) {
	title, description := "Durable event", "persist issue observation"
	bytes, hash := requirementRevisionIdentity(title, description)
	valid := LedgerItem{
		ItemID:           "item-durable-event",
		Slug:             "durable-event",
		Title:            title,
		Description:      description,
		RequirementBytes: bytes,
		RequirementHash:  hash,
		RevisionFrozen:   true,
		SourceState:      SourceStateCurrent,
		Priority:         PriorityHigh,
		Status:           StatusInProgress,
		Stage:            StageIssue,
		StageState:       StageStateVerified,
		Issue:            31,
		Outbox: []RemoteEventRecord{{
			EventID: "issue:item-durable-event:31",
			ItemID:  "item-durable-event",
			Kind:    RemoteEventIssue,
			Number:  31,
		}},
	}

	tests := []struct {
		name string
		mut  func(*LedgerItem)
	}{
		{name: "empty-event-id", mut: func(item *LedgerItem) { item.Outbox[0].EventID = "" }},
		{name: "wrong-item-id", mut: func(item *LedgerItem) { item.Outbox[0].ItemID = "another-item" }},
		{name: "wrong-kind", mut: func(item *LedgerItem) { item.Outbox[0].Kind = "pr" }},
		{name: "wrong-number", mut: func(item *LedgerItem) { item.Outbox[0].Number = 0 }},
		{name: "unstable-event-id", mut: func(item *LedgerItem) { item.Outbox[0].EventID = "random-id" }},
		{name: "duplicate-event-id", mut: func(item *LedgerItem) { item.Outbox = append(item.Outbox, item.Outbox[0]) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := valid
			item.Outbox = append([]RemoteEventRecord(nil), valid.Outbox...)
			tt.mut(&item)
			if err := validateLedgerItems([]*LedgerItem{&item}, ledgerSchemaVersion); err == nil {
				t.Fatal("validateLedgerItems returned nil for invalid outbox")
			}
		})
	}
	if err := validateLedgerItems([]*LedgerItem{&valid}, ledgerSchemaVersion); err != nil {
		t.Fatalf("valid outbox rejected: %v", err)
	}
}

func TestWorkflowIssueBindingRequiresPendingOutboxInSameMutation(t *testing.T) {
	before := happyItem()
	after := before
	after.Status = StatusInProgress
	after.Stage = StageIssue
	after.StageState = StageStateVerified
	after.Issue = 31
	if err := validateWorkflowMutation(before, after); err == nil {
		t.Fatal("issue binding without its pending outbox event was accepted")
	}

	after.Outbox = []RemoteEventRecord{{
		EventID: "issue:item-test:31",
		ItemID:  before.ItemID,
		Kind:    RemoteEventIssue,
		Number:  31,
	}}
	if err := validateWorkflowMutation(before, after); err != nil {
		t.Fatalf("atomic issue binding and outbox event rejected: %v", err)
	}
}

func TestWorkflowCannotRewriteDurableIssueOrOutboxDelivery(t *testing.T) {
	before := happyItem()
	before.Issue = 31
	before.Outbox = []RemoteEventRecord{{
		EventID:   "issue:item-test:31",
		ItemID:    before.ItemID,
		Kind:      RemoteEventIssue,
		Number:    31,
		Delivered: true,
	}}

	changedIssue := cloneLedgerItem(before)
	changedIssue.Issue = 32
	if err := validateWorkflowMutation(before, changedIssue); err == nil {
		t.Fatal("recorded issue binding rewrite was accepted")
	}

	reopened := cloneLedgerItem(before)
	reopened.Outbox[0].Delivered = false
	if err := validateWorkflowMutation(before, reopened); err == nil {
		t.Fatal("delivered outbox event was allowed to become pending")
	}
}

func TestLedgerGetReturnsIndependentOutboxCopy(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	led.Seed([]Item{{Title: "Outbox copy", Description: "protect authority"}})
	item, _ := led.Get("outbox-copy")
	if err := led.Mark(item.Slug, func(it *LedgerItem) {
		it.Status = StatusInProgress
	}); err != nil {
		t.Fatal(err)
	}
	if err := led.Mark(item.Slug, func(it *LedgerItem) {
		it.Stage = StageIssue
		it.StageState = StageStateVerified
		it.Issue = 31
		it.Outbox = append(it.Outbox, RemoteEventRecord{
			EventID: issueEventID(it.ItemID, 31),
			ItemID:  it.ItemID,
			Kind:    RemoteEventIssue,
			Number:  31,
		})
	}); err != nil {
		t.Fatal(err)
	}

	copy, _ := led.Get(item.Slug)
	copy.Outbox[0].Delivered = true
	again, _ := led.Get(item.Slug)
	if again.Outbox[0].Delivered {
		t.Fatal("mutating the value returned by Get changed authoritative outbox state")
	}
}

func TestSeedAddsUnknownItemsAsPending(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}

	led.Seed([]Item{
		{Title: "First item", Priority: PriorityHigh, Status: StatusInProgress, Description: "desc one"},
		{Title: "Second item", Priority: PriorityLow, Description: "desc two"},
	})

	pending := led.Pending()
	if len(pending) != 2 {
		t.Fatalf("Pending() len = %d, want 2 (backlog Status is advisory; seeding is always pending)", len(pending))
	}
	if pending[0].Slug != "first-item" {
		t.Errorf("slug = %q, want first-item", pending[0].Slug)
	}
	if pending[0].Status != StatusPending {
		t.Errorf("Status = %q, want pending (TC-023)", pending[0].Status)
	}
	if pending[0].Description != "desc one" {
		t.Errorf("Description = %q, want carried from backlog", pending[0].Description)
	}
}

func TestSeedNeverOverridesExistingLedgerStatus(t *testing.T) {
	path := ledgerPath(t)
	clk := newTestClock()

	led, err := LoadLedger(path, clk)
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	led.Seed([]Item{{Title: "Ship it", Priority: PriorityHigh}})
	led.Mark("ship-it", func(it *LedgerItem) {
		it.Status = StatusDone
		it.Stage = StageDone
		it.StageState = StageStateVerified
		it.Issue = 31
		it.PR = 32
	})
	if err := led.Save(); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	reloaded, err := LoadLedger(path, clk)
	if err != nil {
		t.Fatalf("LoadLedger reload returned error: %v", err)
	}
	reloaded.Seed([]Item{{Title: "Ship it", Priority: PriorityHigh, Status: StatusPending}})

	if !reloaded.IsDone("ship-it") {
		t.Error("IsDone = false after reseed, want true (ledger precedence, TC-023)")
	}
	if len(reloaded.Pending()) != 0 {
		t.Errorf("Pending() = %v, want empty (done item never reselected)", reloaded.Pending())
	}
}

func TestSeedRefreshesPriorityAndDescription(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	led.Seed([]Item{{Title: "Tune it", Priority: PriorityLow, Description: "old"}})
	led.Seed([]Item{{Title: "Tune it", Priority: PriorityHigh, Description: "new"}})

	pending := led.Pending()
	if len(pending) != 1 {
		t.Fatalf("Pending() len = %d, want 1 (no duplicate seeding)", len(pending))
	}
	if pending[0].Priority != PriorityHigh {
		t.Errorf("Priority = %q, want refreshed high", pending[0].Priority)
	}
	if pending[0].Description != "new" {
		t.Errorf("Description = %q, want refreshed", pending[0].Description)
	}
}

func TestPendingOrdersByPriorityThenDocumentOrder(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	led.Seed([]Item{
		{Title: "Low one", Priority: PriorityLow},
		{Title: "High one", Priority: PriorityHigh},
		{Title: "Medium one", Priority: PriorityMedium},
		{Title: "High two", Priority: PriorityHigh},
	})

	pending := led.Pending()
	got := make([]string, 0, len(pending))
	for _, it := range pending {
		got = append(got, it.Slug)
	}
	want := []string{"high-one", "high-two", "medium-one", "low-one"}
	if len(got) != len(want) {
		t.Fatalf("Pending() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Pending() order = %v, want %v (TC-002)", got, want)
		}
	}
}

func TestPendingSkipsDoneAndInProgress(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	led.Seed([]Item{
		{Title: "Done item", Priority: PriorityHigh},
		{Title: "Running item", Priority: PriorityHigh},
		{Title: "Fresh item", Priority: PriorityLow},
	})
	led.Mark("done-item", func(it *LedgerItem) { it.Status = StatusDone })
	led.Mark("running-item", func(it *LedgerItem) { it.Status = StatusInProgress })

	pending := led.Pending()
	if len(pending) != 1 || pending[0].Slug != "fresh-item" {
		t.Errorf("Pending() = %v, want only fresh-item (TC-003)", pending)
	}

	inProgress := led.InProgress()
	if len(inProgress) != 1 || inProgress[0].Slug != "running-item" {
		t.Errorf("InProgress() = %v, want only running-item", inProgress)
	}
}

func TestMarkStampsUpdatedAtFromClock(t *testing.T) {
	clk := newTestClock()
	led, err := LoadLedger(ledgerPath(t), clk)
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	led.Seed([]Item{{Title: "Stamp me", Priority: PriorityHigh}})

	clk.now = clk.now.Add(42 * time.Minute)
	led.Mark("stamp-me", func(it *LedgerItem) { it.Status = StatusInProgress })

	it, ok := led.Get("stamp-me")
	if !ok {
		t.Fatal("Get returned ok=false")
	}
	if !it.UpdatedAt.Equal(clk.now) {
		t.Errorf("UpdatedAt = %v, want %v", it.UpdatedAt, clk.now)
	}
}

func TestLedgerCommitFailureDoesNotMutateAuthoritativeMemory(t *testing.T) {
	path := ledgerPath(t)
	led, err := LoadLedger(path, newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	led.Seed([]Item{{Title: "Atomic transition", Priority: PriorityHigh}})
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	err = led.Commit("atomic-transition", func(it *LedgerItem) {
		it.Status = StatusDone
		it.Stage = StageDone
		it.StageState = StageStateVerified
	})
	if err == nil {
		t.Fatal("Commit returned nil, want persistence failure")
	}
	item, ok := led.Get("atomic-transition")
	if !ok {
		t.Fatal("item disappeared after failed Commit")
	}
	if item.Status != StatusPending || item.Stage != "" {
		t.Fatalf("item after failed Commit = %+v, want original pending state", item)
	}
}

func TestSaveAndReloadRoundTrip(t *testing.T) {
	path := ledgerPath(t)
	clk := newTestClock()

	led, err := LoadLedger(path, clk)
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	led.Seed([]Item{{Title: "Round trip", Priority: PriorityMedium, Description: "the description"}})
	led.Mark("round-trip", func(it *LedgerItem) {
		it.Status = StatusInProgress
		it.Branch = "auto/round-trip"
		it.Stage = StageSpecToPlan
		it.StageState = StageStateRunning
		it.Issue = 7
		it.PR = 8
		it.FeatureDir = ".codexspec/specs/2026-0610-1200ab-x"
	})
	if err := led.Save(); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	reloaded, err := LoadLedger(path, clk)
	if err != nil {
		t.Fatalf("LoadLedger reload returned error: %v", err)
	}
	it, ok := reloaded.Get("round-trip")
	if !ok {
		t.Fatal("Get returned ok=false after reload")
	}
	if it.Status != StatusInProgress || it.Branch != "auto/round-trip" || it.Stage != "spec-to-plan" ||
		it.Issue != 7 || it.PR != 8 || it.FeatureDir != ".codexspec/specs/2026-0610-1200ab-x" {
		t.Errorf("reloaded item = %+v, want persisted fields intact", it)
	}

	// Description is supplied by the backlog, not the ledger (REQ-028);
	// reseeding restores it after a reload.
	reloaded.Seed([]Item{{Title: "Round trip", Priority: PriorityMedium, Description: "the description"}})
	it, _ = reloaded.Get("round-trip")
	if it.Description != "the description" {
		t.Errorf("Description after reseed = %q, want restored from backlog", it.Description)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger file: %v", err)
	}
	if contains := string(data); contains == "" {
		t.Error("ledger file is empty")
	}
}

func TestLoadLedgerMigratesKnownLegacyStageAndWritesCurrentVersion(t *testing.T) {
	path := ledgerPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"items":[{"slug":"legacy","title":"Legacy","priority":"high","status":"in-progress","stage":"spec-to-plan"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	led, err := LoadLedger(path, newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	item, ok := led.Get("legacy")
	if !ok {
		t.Fatal("legacy item missing")
	}
	if item.Stage != StageSpecToPlan || item.StageState != StageStateRunning {
		t.Fatalf("legacy stage = %q/%q, want %q/%q", item.Stage, item.StageState, StageSpecToPlan, StageStateRunning)
	}
	if err := led.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"version": 2`) || !strings.Contains(string(data), `"stage_state": "running"`) {
		t.Fatalf("migrated ledger = %s, want versioned running state", data)
	}
}

func TestSaveIsAtomicAndLeavesNoTempFiles(t *testing.T) {
	path := ledgerPath(t)
	led, err := LoadLedger(path, newTestClock())
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	led.Seed([]Item{{Title: "Atomic", Priority: PriorityHigh}})

	// Saving twice must atomically replace the file and leave no
	// temporary artifacts behind (the ledger is the authoritative resume
	// source; a torn write would break recovery).
	for i := 0; i < 2; i++ {
		if err := led.Save(); err != nil {
			t.Fatalf("Save #%d returned error: %v", i+1, err)
		}
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(path) {
			t.Errorf("unexpected file %q next to the ledger, want temp files cleaned up", e.Name())
		}
	}

	if _, err := LoadLedger(path, newTestClock()); err != nil {
		t.Fatalf("reload after Save returned error: %v", err)
	}
}

func TestSeedDisambiguatesSlugCollisions(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	led.Seed([]Item{
		{SourceID: "same-title-high", Title: "Same title", Priority: PriorityHigh},
		{SourceID: "same-title-low", Title: "Same title", Priority: PriorityLow},
	})

	pending := led.Pending()
	if len(pending) != 2 {
		t.Fatalf("Pending() len = %d, want 2", len(pending))
	}
	if pending[0].Slug == pending[1].Slug {
		t.Errorf("slugs collide: %q", pending[0].Slug)
	}
}
