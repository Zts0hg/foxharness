package autodev

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// LedgerItem is one item's authoritative progress record (REQ-021). The
// Description is supplied by the backlog at seed time and is intentionally
// not persisted: the backlog owns the requirement text, the ledger owns the
// processing state (REQ-028).
type LedgerItem struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Priority  Priority  `json:"priority"`
	Status    Status    `json:"status"`
	Branch    string    `json:"branch,omitempty"`
	Stage     string    `json:"stage,omitempty"`
	Issue     int       `json:"issue,omitempty"`
	PR        int       `json:"pr,omitempty"`
	SpecDir   string    `json:"spec_dir,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`

	Description string `json:"-"`
}

// Ledger is the durable, authoritative progress store backed by a JSON file
// (default .foxharness/autodev-state.json). It seeds entries from the
// backlog, never lets backlog status override recorded progress, and
// selects pending work by priority (REQ-021/022/028).
type Ledger struct {
	path  string
	clock Clock
	items []*LedgerItem
}

// ledgerFile is the persisted JSON form.
type ledgerFile struct {
	Items []*LedgerItem `json:"items"`
}

// LoadLedger reads the ledger at path, returning an empty ledger when the
// file does not exist yet. clock stamps subsequent mutations.
func LoadLedger(path string, clock Clock) (*Ledger, error) {
	if clock == nil {
		clock = SystemClock{}
	}
	led := &Ledger{path: path, clock: clock}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return led, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ledger: %w", err)
	}
	var file ledgerFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse ledger: %w", err)
	}
	led.items = file.Items
	return led, nil
}

// Seed reconciles the ledger with the backlog item set (REQ-028). Items
// absent from the ledger are appended with status pending regardless of the
// advisory backlog Status. Items already present have their Title, Priority,
// and Description refreshed from the backlog — the backlog owns those — but
// their Status and progress fields are never touched.
func (l *Ledger) Seed(items []Item) {
	// Duplicate titles are legal in the backlog (the slug disambiguates
	// them), so matching consumes ledger entries per title in order rather
	// than mapping every duplicate onto the first entry.
	byTitle := make(map[string][]*LedgerItem, len(l.items))
	taken := make(map[string]bool, len(l.items))
	for _, it := range l.items {
		taken[it.Slug] = true
		byTitle[it.Title] = append(byTitle[it.Title], it)
	}

	for _, src := range items {
		if queue := byTitle[src.Title]; len(queue) > 0 {
			existing := queue[0]
			byTitle[src.Title] = queue[1:]
			existing.Priority = src.Priority
			existing.Description = src.Description
			continue
		}
		slug := Slug(src.Title, taken)
		taken[slug] = true
		l.items = append(l.items, &LedgerItem{
			Slug:        slug,
			Title:       src.Title,
			Priority:    src.Priority,
			Status:      StatusPending,
			Description: src.Description,
			UpdatedAt:   l.clock.Now(),
		})
	}
}

// Pending returns items whose authoritative status is pending, ordered by
// priority high → low with ties broken by seed (document) order (REQ-002).
func (l *Ledger) Pending() []LedgerItem {
	return l.selectByStatus(StatusPending)
}

// InProgress returns in-progress items in priority order; the orchestrator
// resumes these before starting new pending work (REQ-022).
func (l *Ledger) InProgress() []LedgerItem {
	return l.selectByStatus(StatusInProgress)
}

func (l *Ledger) selectByStatus(status Status) []LedgerItem {
	type indexed struct {
		item  LedgerItem
		order int
	}
	var picked []indexed
	for i, it := range l.items {
		if it.Status == status {
			picked = append(picked, indexed{item: *it, order: i})
		}
	}
	sort.SliceStable(picked, func(a, b int) bool {
		ra, rb := picked[a].item.Priority.Rank(), picked[b].item.Priority.Rank()
		if ra != rb {
			return ra < rb
		}
		return picked[a].order < picked[b].order
	})
	out := make([]LedgerItem, 0, len(picked))
	for _, p := range picked {
		out = append(out, p.item)
	}
	return out
}

// Mark applies mut to the item identified by slug and stamps UpdatedAt from
// the ledger clock. Unknown slugs are a no-op.
func (l *Ledger) Mark(slug string, mut func(*LedgerItem)) {
	for _, it := range l.items {
		if it.Slug == slug {
			mut(it)
			it.UpdatedAt = l.clock.Now()
			return
		}
	}
}

// Get returns a copy of the item identified by slug.
func (l *Ledger) Get(slug string) (LedgerItem, bool) {
	for _, it := range l.items {
		if it.Slug == slug {
			return *it, true
		}
	}
	return LedgerItem{}, false
}

// IsDone reports whether the item identified by slug is recorded done.
func (l *Ledger) IsDone(slug string) bool {
	it, ok := l.Get(slug)
	return ok && it.Status == StatusDone
}

// Save persists the ledger to its JSON file, creating parent directories
// as needed. The write is atomic (temp file + rename): the ledger is the
// authoritative resume source (REQ-021), so a crash mid-write must never
// leave a torn file behind.
func (l *Ledger) Save() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("create ledger dir: %w", err)
	}
	data, err := json.MarshalIndent(ledgerFile{Items: l.items}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ledger: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), filepath.Base(l.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create ledger temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write ledger: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close ledger temp file: %w", err)
	}
	if err := os.Rename(tmpPath, l.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("commit ledger: %w", err)
	}
	return nil
}
