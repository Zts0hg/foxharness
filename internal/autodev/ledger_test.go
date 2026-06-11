package autodev

import (
	"os"
	"path/filepath"
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
		it.Stage = "spec-to-plan"
		it.Issue = 7
		it.PR = 8
		it.SpecDir = ".codexspec/specs/x"
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
		it.Issue != 7 || it.PR != 8 || it.SpecDir != ".codexspec/specs/x" {
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

func TestSeedDisambiguatesSlugCollisions(t *testing.T) {
	led, err := LoadLedger(ledgerPath(t), newTestClock())
	if err != nil {
		t.Fatalf("LoadLedger returned error: %v", err)
	}
	led.Seed([]Item{
		{Title: "Same title", Priority: PriorityHigh},
		{Title: "Same title", Priority: PriorityLow},
	})

	pending := led.Pending()
	if len(pending) != 2 {
		t.Fatalf("Pending() len = %d, want 2", len(pending))
	}
	if pending[0].Slug == pending[1].Slug {
		t.Errorf("slugs collide: %q", pending[0].Slug)
	}
}
