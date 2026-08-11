package autodev

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

// LedgerItem is one item's authoritative progress record (REQ-021). The
// Description is supplied by the backlog at seed time and is intentionally
// not persisted: the backlog owns the requirement text, the ledger owns the
// processing state (REQ-028).
type LedgerItem struct {
	Slug       string        `json:"slug"`
	Title      string        `json:"title"`
	Priority   Priority      `json:"priority"`
	Status     Status        `json:"status"`
	Branch     string        `json:"branch,omitempty"`
	Stage      PipelineStage `json:"stage,omitempty"`
	StageState StageState    `json:"stage_state"`
	Issue      int           `json:"issue,omitempty"`
	PR         int           `json:"pr,omitempty"`
	FeatureDir string        `json:"feature_dir,omitempty"`
	UpdatedAt  time.Time     `json:"updated_at,omitempty"`

	Description string `json:"-"`
}

// PipelineStage is the closed vocabulary of durable workflow positions.
// StageContext.Stage remains a runtime string; only values in this set may
// control crash recovery.
type PipelineStage string

const (
	StageNone                    PipelineStage = ""
	StageMaterializeRequirements PipelineStage = "materialize-requirements"
	StageGenerateSpec            PipelineStage = "generate-spec"
	StageSpecToPlan              PipelineStage = "spec-to-plan"
	StagePlanToTasks             PipelineStage = "plan-to-tasks"
	StageImplementTasks          PipelineStage = "implement-tasks"
	StagePublish                 PipelineStage = "publish"
	StageStageChanges            PipelineStage = "stage-changes"
	StageCommitStaged            PipelineStage = "commit-staged"
	StagePush                    PipelineStage = "push"
	StageIssue                   PipelineStage = "issue"
	StagePR                      PipelineStage = "pr"
	StageDone                    PipelineStage = "done"
)

// StageState records whether a stage is awaiting execution, has a durable
// execution intent, or has passed ground-truth verification.
type StageState string

const (
	StageStatePending  StageState = "pending"
	StageStateRunning  StageState = "running"
	StageStateVerified StageState = "verified"
)

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
	Version int           `json:"version"`
	Items   []*LedgerItem `json:"items"`
}

const ledgerSchemaVersion = 1

// LedgerCommitError reports an authoritative ledger transition that could
// not be durably committed. Callers must stop dependent work and retain any
// recovery evidence such as the item worktree.
type LedgerCommitError struct {
	Operation string
	Err       error
}

// Error implements error.
func (e *LedgerCommitError) Error() string {
	return fmt.Sprintf("commit ledger operation %s: %v", e.Operation, e.Err)
}

// Unwrap returns the underlying persistence error.
func (e *LedgerCommitError) Unwrap() error { return e.Err }

// InvalidLedgerStateError reports a schema version or state combination
// that cannot be interpreted safely. The orchestrator must fail closed
// before invoking external tools.
type InvalidLedgerStateError struct {
	Version int
	Slug    string
	Reason  string
}

// Error implements error.
func (e *InvalidLedgerStateError) Error() string {
	location := "ledger"
	if e.Slug != "" {
		location = fmt.Sprintf("ledger item %q", e.Slug)
	}
	return fmt.Sprintf("invalid %s state (schema version %d): %s", location, e.Version, e.Reason)
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
	switch file.Version {
	case 0:
		if err := migrateLegacyLedger(file.Items); err != nil {
			return nil, err
		}
	case ledgerSchemaVersion:
		if err := validateLedgerItems(file.Items, file.Version); err != nil {
			return nil, err
		}
	default:
		return nil, &InvalidLedgerStateError{
			Version: file.Version,
			Reason:  fmt.Sprintf("unsupported schema version; current version is %d", ledgerSchemaVersion),
		}
	}
	led.items = file.Items
	return led, nil
}

func migrateLegacyLedger(items []*LedgerItem) error {
	for _, item := range items {
		if item == nil {
			return invalidLedgerItem(0, nil, "item is null")
		}
		if item.StageState != "" {
			return invalidLedgerItem(0, item, "versionless item unexpectedly contains stage_state")
		}
		switch item.Status {
		case StatusPending:
			if item.Stage != StageNone {
				return invalidLedgerItem(0, item, "pending item has a recorded stage")
			}
			item.StageState = StageStatePending
		case StatusInProgress:
			if item.Stage == StageNone {
				item.StageState = StageStatePending
				continue
			}
			if !isExecutableStage(item.Stage) {
				return invalidLedgerItem(0, item, fmt.Sprintf("unknown in-progress stage %q", item.Stage))
			}
			item.StageState = StageStateRunning
		case StatusDone:
			if item.Stage == StageNone {
				item.Stage = StageDone
			}
			if item.Stage != StageDone {
				return invalidLedgerItem(0, item, fmt.Sprintf("done item has stage %q", item.Stage))
			}
			item.StageState = StageStateVerified
		default:
			return invalidLedgerItem(0, item, fmt.Sprintf("unknown status %q", item.Status))
		}
	}
	return validateLedgerItems(items, 0)
}

func validateLedgerItems(items []*LedgerItem, version int) error {
	for _, item := range items {
		if item == nil {
			return invalidLedgerItem(version, nil, "item is null")
		}
		switch item.Status {
		case StatusPending:
			if item.Stage != StageNone || item.StageState != StageStatePending {
				return invalidLedgerItem(version, item, "pending requires an empty stage and pending stage state")
			}
		case StatusInProgress:
			if item.Stage == StageNone {
				if item.StageState != StageStatePending {
					return invalidLedgerItem(version, item, "in-progress without a stage requires pending stage state")
				}
				continue
			}
			if !isExecutableStage(item.Stage) {
				return invalidLedgerItem(version, item, fmt.Sprintf("unknown in-progress stage %q", item.Stage))
			}
			if item.StageState != StageStateRunning && item.StageState != StageStateVerified {
				return invalidLedgerItem(version, item, "in-progress stage requires running or verified stage state")
			}
			if item.Stage == StageIssue && item.StageState == StageStateVerified && item.Issue <= 0 {
				return invalidLedgerItem(version, item, "verified issue stage requires a positive issue binding")
			}
			if item.Stage == StagePR && item.StageState == StageStateVerified && item.PR <= 0 {
				return invalidLedgerItem(version, item, "verified PR stage requires a positive PR binding")
			}
		case StatusDone:
			if item.Stage != StageDone || item.StageState != StageStateVerified {
				return invalidLedgerItem(version, item, "done requires the done stage in verified state")
			}
		default:
			return invalidLedgerItem(version, item, fmt.Sprintf("unknown status %q", item.Status))
		}
	}
	return nil
}

func invalidLedgerItem(version int, item *LedgerItem, reason string) error {
	slug := ""
	if item != nil {
		slug = item.Slug
	}
	return &InvalidLedgerStateError{Version: version, Slug: slug, Reason: reason}
}

func isExecutableStage(stage PipelineStage) bool {
	switch stage {
	case StageMaterializeRequirements, StageGenerateSpec, StageSpecToPlan,
		StagePlanToTasks, StageImplementTasks, StagePublish, StageStageChanges,
		StageCommitStaged, StagePush, StageIssue, StagePR:
		return true
	default:
		return false
	}
}

func pipelineStage(name string) (PipelineStage, bool) {
	stage := PipelineStage(name)
	return stage, isExecutableStage(stage)
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
			StageState:  StageStatePending,
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

// Commit applies one item mutation to a candidate snapshot and makes it
// authoritative only after that snapshot has been durably persisted.
func (l *Ledger) Commit(slug string, mut func(*LedgerItem)) error {
	candidate := cloneLedgerItems(l.items)
	for _, it := range candidate {
		if it.Slug != slug {
			continue
		}
		mut(it)
		it.UpdatedAt = l.clock.Now()
		committed, err := l.persistItems(candidate)
		if committed {
			l.items = candidate
		}
		return err
	}
	return fmt.Errorf("ledger item %q not found", slug)
}

func cloneLedgerItems(items []*LedgerItem) []*LedgerItem {
	cloned := make([]*LedgerItem, len(items))
	for i, item := range items {
		copy := *item
		cloned[i] = &copy
	}
	return cloned
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
	_, err := l.persistItems(l.items)
	return err
}

func (l *Ledger) persistItems(items []*LedgerItem) (bool, error) {
	if err := validateLedgerItems(items, ledgerSchemaVersion); err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return false, fmt.Errorf("create ledger dir: %w", err)
	}
	data, err := json.MarshalIndent(ledgerFile{Version: ledgerSchemaVersion, Items: items}, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode ledger: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), filepath.Base(l.path)+".tmp-*")
	if err != nil {
		return false, fmt.Errorf("create ledger temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return false, fmt.Errorf("write ledger: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return false, fmt.Errorf("flush ledger: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return false, fmt.Errorf("close ledger temp file: %w", err)
	}
	if err := os.Rename(tmpPath, l.path); err != nil {
		os.Remove(tmpPath)
		return false, fmt.Errorf("commit ledger: %w", err)
	}
	if runtime.GOOS == "windows" {
		return true, nil
	}
	dir, err := os.Open(filepath.Dir(l.path))
	if err != nil {
		return true, fmt.Errorf("open ledger dir for flush: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return true, fmt.Errorf("flush ledger dir: %w", err)
	}
	return true, nil
}
