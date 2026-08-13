package autodev

import (
	"encoding/json"
	"errors"
	"io"
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
	if !strings.Contains(string(data), `"version": 3`) || !strings.Contains(string(data), `"stage_state": "running"`) {
		t.Fatalf("migrated ledger = %s, want versioned running state", data)
	}
}

func TestDVAUT010LoadSchemaV2PreservesBehaviorAndUpgradesOnSave(t *testing.T) {
	path := ledgerPath(t)
	seeded, err := LoadLedger(path, newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	if err := seeded.Seed([]Item{{Title: "Schema compatibility", Description: "preserve v2 state"}}); err != nil {
		t.Fatal(err)
	}
	if err := seeded.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var prior ledgerFile
	if err := json.Unmarshal(data, &prior); err != nil {
		t.Fatal(err)
	}
	prior.Version = 2
	data, err = json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	ledger, err := LoadLedger(path, newTestClock())
	if err != nil {
		t.Fatalf("LoadLedger(schema v2) = %v", err)
	}
	loaded, ok := ledger.Get("schema-compatibility")
	if !ok || loaded.ItemID == "" || len(loaded.CoreAttempts) != 0 {
		t.Fatalf("schema v2 item = %#v", loaded)
	}
	if err := ledger.Save(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(upgraded), `"version": 3`) {
		t.Fatalf("upgraded ledger = %s", upgraded)
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
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("ledger permissions = %o, want 600", got)
	}
}

type scriptedLedgerTempFile struct {
	events     *[]string
	shortWrite bool
	writeErr   error
	syncErr    error
	closeErr   error
}

func (f *scriptedLedgerTempFile) Name() string { return "/fixture/ledger.tmp" }

func (f *scriptedLedgerTempFile) Write(p []byte) (int, error) {
	*f.events = append(*f.events, "write")
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.shortWrite {
		return len(p) - 1, nil
	}
	return len(p), nil
}

func (f *scriptedLedgerTempFile) Sync() error {
	*f.events = append(*f.events, "sync-file")
	return f.syncErr
}

func (f *scriptedLedgerTempFile) Close() error {
	*f.events = append(*f.events, "close-file")
	return f.closeErr
}

type scriptedLedgerDirFile struct {
	events   *[]string
	syncErr  error
	closeErr error
}

func (f *scriptedLedgerDirFile) Sync() error {
	*f.events = append(*f.events, "sync-dir")
	return f.syncErr
}

func (f *scriptedLedgerDirFile) Close() error {
	*f.events = append(*f.events, "close-dir")
	return f.closeErr
}

func TestCPAUT006LedgerPersistenceFailureMatrix(t *testing.T) {
	type failureCase struct {
		name          string
		stage         string
		want          string
		wantCommitted bool
		wantCleanup   bool
	}
	for _, tc := range []failureCase{
		{name: "directory", stage: "mkdir", want: "create ledger dir"},
		{name: "encode", stage: "encode", want: "encode ledger"},
		{name: "create", stage: "create", want: "create ledger temp file"},
		{name: "write", stage: "write", want: "write ledger", wantCleanup: true},
		{name: "short write", stage: "short-write", want: "write ledger", wantCleanup: true},
		{name: "file sync", stage: "sync-file", want: "flush ledger", wantCleanup: true},
		{name: "file close", stage: "close-file", want: "close ledger temp file", wantCleanup: true},
		{name: "rename", stage: "rename", want: "commit ledger", wantCleanup: true},
		{name: "directory open", stage: "open-dir", want: "open ledger dir for flush", wantCommitted: true},
		{name: "directory sync", stage: "sync-dir", want: "flush ledger dir", wantCommitted: true},
		{name: "directory close", stage: "close-dir", want: "close ledger dir after flush", wantCommitted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			injected := errors.New("injected " + tc.stage + " failure")
			expected := injected
			if tc.stage == "short-write" {
				expected = io.ErrShortWrite
			}
			var events []string
			temp := &scriptedLedgerTempFile{events: &events}
			dir := &scriptedLedgerDirFile{events: &events}
			ops := ledgerPersistenceOps{
				mkdirAll: func(string, os.FileMode) error {
					events = append(events, "mkdir")
					if tc.stage == "mkdir" {
						return injected
					}
					return nil
				},
				encode: func(ledgerFile) ([]byte, error) {
					events = append(events, "encode")
					if tc.stage == "encode" {
						return nil, injected
					}
					return []byte(`{"version":3,"items":[]}`), nil
				},
				createTemp: func(string, string) (ledgerTempFile, error) {
					events = append(events, "create")
					if tc.stage == "create" {
						return nil, injected
					}
					return temp, nil
				},
				rename: func(string, string) error {
					events = append(events, "rename")
					if tc.stage == "rename" {
						return injected
					}
					return nil
				},
				remove: func(string) error {
					events = append(events, "remove")
					return nil
				},
				openDir: func(string) (ledgerDirFile, error) {
					events = append(events, "open-dir")
					if tc.stage == "open-dir" {
						return nil, injected
					}
					return dir, nil
				},
				syncDir: true,
			}
			switch tc.stage {
			case "write":
				temp.writeErr = injected
			case "short-write":
				temp.shortWrite = true
			case "sync-file":
				temp.syncErr = injected
			case "close-file":
				temp.closeErr = injected
			case "sync-dir":
				dir.syncErr = injected
			case "close-dir":
				dir.closeErr = injected
			}

			led, err := LoadLedger(filepath.Join(t.TempDir(), "autodev-state.json"), newTestClock())
			if err != nil {
				t.Fatal(err)
			}
			if err := led.Seed([]Item{{Title: "Durable transition", Priority: PriorityHigh}}); err != nil {
				t.Fatal(err)
			}
			led.persistence = ops
			err = led.Commit("durable-transition", func(item *LedgerItem) {
				item.Status = StatusInProgress
			})
			if !errors.Is(err, expected) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Commit error = %v, want injected error classified as %q", err, tc.want)
			}
			item, ok := led.Get("durable-transition")
			if !ok {
				t.Fatal("ledger item disappeared")
			}
			if gotCommitted := item.Status == StatusInProgress; gotCommitted != tc.wantCommitted {
				t.Fatalf("in-memory committed = %t, want %t after %s; events=%v", gotCommitted, tc.wantCommitted, tc.stage, events)
			}
			gotCleanup := false
			for _, event := range events {
				gotCleanup = gotCleanup || event == "remove"
			}
			if gotCleanup != tc.wantCleanup {
				t.Fatalf("temp cleanup = %t, want %t after %s; events=%v", gotCleanup, tc.wantCleanup, tc.stage, events)
			}
		})
	}
}

func TestCPAUT006LedgerReportsTempCloseAndCleanupFailures(t *testing.T) {
	writeErr := errors.New("injected write failure")
	closeErr := errors.New("injected close failure")
	removeErr := errors.New("injected cleanup failure")
	var events []string
	temp := &scriptedLedgerTempFile{events: &events, writeErr: writeErr, closeErr: closeErr}
	led, err := LoadLedger(filepath.Join(t.TempDir(), "autodev-state.json"), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	if err := led.Seed([]Item{{Title: "Cleanup evidence", Priority: PriorityHigh}}); err != nil {
		t.Fatal(err)
	}
	led.persistence = ledgerPersistenceOps{
		mkdirAll: func(string, os.FileMode) error { return nil },
		encode:   func(ledgerFile) ([]byte, error) { return []byte(`{}`), nil },
		createTemp: func(string, string) (ledgerTempFile, error) {
			return temp, nil
		},
		rename: func(string, string) error { return nil },
		remove: func(string) error {
			events = append(events, "remove")
			return removeErr
		},
		openDir: func(string) (ledgerDirFile, error) {
			return &scriptedLedgerDirFile{events: &events}, nil
		},
		syncDir: true,
	}
	err = led.Commit("cleanup-evidence", func(item *LedgerItem) {
		item.Status = StatusInProgress
	})
	for _, want := range []error{writeErr, closeErr, removeErr} {
		if !errors.Is(err, want) {
			t.Errorf("Commit error = %v, want joined %v", err, want)
		}
	}
	if item, _ := led.Get("cleanup-evidence"); item.Status != StatusPending {
		t.Fatalf("item status = %s, want pending after pre-commit cleanup failure", item.Status)
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
