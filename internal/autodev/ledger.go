package autodev

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// LedgerItem is one item's authoritative identity, frozen requirement
// revision, source-reconciliation state, and workflow progress record.
type LedgerItem struct {
	ItemID               ItemID        `json:"item_id"`
	SourceID             string        `json:"source_id,omitempty"`
	Slug                 string        `json:"slug"`
	Title                string        `json:"title"`
	Description          string        `json:"description"`
	RequirementBytes     int           `json:"requirement_bytes"`
	RequirementHash      string        `json:"requirement_hash"`
	RevisionFrozen       bool          `json:"revision_frozen"`
	LegacyBindingPending bool          `json:"legacy_binding_pending,omitempty"`
	SourceState          SourceState   `json:"source_state"`
	SourceOrder          int           `json:"source_order"`
	Priority             Priority      `json:"priority"`
	Status               Status        `json:"status"`
	Branch               string        `json:"branch,omitempty"`
	Stage                PipelineStage `json:"stage,omitempty"`
	StageState           StageState    `json:"stage_state"`
	Issue                int           `json:"issue,omitempty"`
	PR                   int           `json:"pr,omitempty"`
	FeatureDir           string        `json:"feature_dir,omitempty"`
	UpdatedAt            time.Time     `json:"updated_at,omitempty"`
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

const ledgerSchemaVersion = 2

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

// ReconciliationError reports an identity or active-revision conflict that
// cannot be resolved without guessing. Blocked item state may accompany the
// error and must be persisted before orchestration stops.
type ReconciliationError struct {
	Reason string
}

// Error implements error.
func (e *ReconciliationError) Error() string {
	return "autodev backlog reconciliation failed: " + e.Reason
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
		initializeLegacyIdentities(file.Items)
		if err := validateLedgerItems(file.Items, ledgerSchemaVersion); err != nil {
			return nil, err
		}
	case 1:
		if err := validateLedgerItems(file.Items, file.Version); err != nil {
			return nil, err
		}
		initializeLegacyIdentities(file.Items)
		if err := validateLedgerItems(file.Items, ledgerSchemaVersion); err != nil {
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

func initializeLegacyIdentities(items []*LedgerItem) {
	for _, item := range items {
		item.ItemID = itemIDFromSeed("legacy", item.Slug+"\x00"+item.Title)
		item.RequirementBytes, item.RequirementHash = requirementIdentity(item.Description)
		item.RevisionFrozen = item.Status != StatusPending
		item.LegacyBindingPending = true
		switch item.Status {
		case StatusPending:
			item.SourceState = SourceStateOrphaned
		case StatusInProgress:
			item.SourceState = SourceStateBlocked
		case StatusDone:
			item.SourceState = SourceStateHistorical
		}
	}
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
	itemIDs := make(map[ItemID]bool, len(items))
	sourceIDs := make(map[string]bool, len(items))
	for _, item := range items {
		if item == nil {
			return invalidLedgerItem(version, nil, "item is null")
		}
		if version >= 2 {
			if item.ItemID == "" {
				return invalidLedgerItem(version, item, "item_id is required")
			}
			if itemIDs[item.ItemID] {
				return invalidLedgerItem(version, item, fmt.Sprintf("duplicate item_id %q", item.ItemID))
			}
			itemIDs[item.ItemID] = true
			if item.SourceID != "" {
				if strings.TrimSpace(item.SourceID) != item.SourceID {
					return invalidLedgerItem(version, item, "source_id is not normalized")
				}
				if sourceIDs[item.SourceID] {
					return invalidLedgerItem(version, item, fmt.Sprintf("duplicate source_id %q", item.SourceID))
				}
				sourceIDs[item.SourceID] = true
			}
			if item.LegacyBindingPending && item.SourceID != "" {
				return invalidLedgerItem(version, item, "legacy binding cannot already have a source_id")
			}
			if item.LegacyBindingPending {
				expected := SourceStateHistorical
				if item.Status == StatusPending {
					expected = SourceStateOrphaned
				} else if item.Status == StatusInProgress {
					expected = SourceStateBlocked
				}
				if item.SourceState != expected {
					return invalidLedgerItem(version, item, fmt.Sprintf("pending legacy binding requires source state %q", expected))
				}
			}
			bytes, hash := requirementIdentity(item.Description)
			if item.RequirementBytes != bytes || item.RequirementHash != hash {
				return invalidLedgerItem(version, item, "requirement byte length or hash does not match the persisted description")
			}
		}
		switch item.Status {
		case StatusPending:
			if item.Stage != StageNone || item.StageState != StageStatePending {
				return invalidLedgerItem(version, item, "pending requires an empty stage and pending stage state")
			}
			if version >= 2 && item.RevisionFrozen {
				return invalidLedgerItem(version, item, "pending requirement revision cannot be frozen")
			}
			if version >= 2 && item.SourceState != SourceStateCurrent && item.SourceState != SourceStateOrphaned {
				return invalidLedgerItem(version, item, "pending source state requires current or orphaned")
			}
		case StatusInProgress:
			if item.Stage == StageNone {
				if item.StageState != StageStatePending {
					return invalidLedgerItem(version, item, "in-progress without a stage requires pending stage state")
				}
			} else {
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
			}
			if version >= 2 && !item.RevisionFrozen {
				return invalidLedgerItem(version, item, "in-progress requirement revision must be frozen")
			}
			if version >= 2 && item.SourceState != SourceStateCurrent && item.SourceState != SourceStateBlocked {
				return invalidLedgerItem(version, item, "in-progress source state requires current or blocked")
			}
		case StatusDone:
			if item.Stage != StageDone || item.StageState != StageStateVerified {
				return invalidLedgerItem(version, item, "done requires the done stage in verified state")
			}
			if version >= 2 && !item.RevisionFrozen {
				return invalidLedgerItem(version, item, "done requirement revision must be frozen")
			}
			if version >= 2 && item.SourceState != SourceStateCurrent && item.SourceState != SourceStateHistorical {
				return invalidLedgerItem(version, item, "done source state requires current or historical")
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

func itemIDFromSeed(namespace, value string) ItemID {
	sum := sha256.Sum256([]byte("fox-autodev-item\x00" + namespace + "\x00" + value))
	return ItemID(fmt.Sprintf("item-%x", sum[:16]))
}

func requirementIdentity(description string) (int, string) {
	sum := sha256.Sum256([]byte(description))
	return len([]byte(description)), fmt.Sprintf("sha256:%x", sum)
}

// Seed reconciles normalized backlog items against immutable ledger
// identities. It never guesses through duplicate or rename ambiguity.
// Blocked/orphaned state is retained in memory so callers can durably save it
// before returning a ReconciliationError.
func (l *Ledger) Seed(items []Item) error {
	normalized := append([]Item(nil), items...)
	if err := validateSourceItems(normalized); err != nil {
		return err
	}

	candidate := cloneLedgerItems(l.items)
	bySource := make(map[string][]*LedgerItem, len(candidate))
	byTitleWithoutSource := make(map[string][]*LedgerItem, len(candidate))
	takenSlugs := make(map[string]bool, len(candidate))
	for _, item := range candidate {
		takenSlugs[item.Slug] = true
		if item.SourceID != "" {
			bySource[item.SourceID] = append(bySource[item.SourceID], item)
		} else {
			byTitleWithoutSource[item.Title] = append(byTitleWithoutSource[item.Title], item)
		}
	}

	type sourceMatch struct {
		source Item
		order  int
		item   *LedgerItem
	}
	matches := make([]sourceMatch, len(normalized))
	matchedItems := make(map[*LedgerItem]bool, len(candidate))
	for i, source := range normalized {
		matches[i] = sourceMatch{source: source, order: i}
		var choices []*LedgerItem
		if source.SourceID != "" {
			choices = unmatchedLedgerItems(bySource[source.SourceID], matchedItems)
		}
		if len(choices) == 0 {
			choices = unmatchedLedgerItems(byTitleWithoutSource[source.Title], matchedItems)
		}
		if len(choices) > 1 {
			return &ReconciliationError{Reason: fmt.Sprintf("source item %q matches multiple ledger records; add unique **ID** values", source.Title)}
		}
		if len(choices) == 1 {
			matches[i].item = choices[0]
			matchedItems[choices[0]] = true
		}
	}

	for _, match := range matches {
		if match.item != nil {
			continue
		}
		for _, existing := range candidate {
			if matchedItems[existing] {
				continue
			}
			if existing.SourceID == "" || existing.Title == match.source.Title {
				return &ReconciliationError{Reason: fmt.Sprintf(
					"cannot distinguish new item %q from rename or replacement of ledger item %q; add or restore a stable **ID**",
					match.source.Title, existing.Title)}
			}
		}
	}

	var blockedErr error
	for _, match := range matches {
		if match.item == nil {
			continue
		}
		if err := reconcileMatchedItem(match.item, match.source, match.order, l.clock.Now()); err != nil && blockedErr == nil {
			blockedErr = err
		}
	}
	for _, existing := range candidate {
		if matchedItems[existing] {
			continue
		}
		existing.UpdatedAt = l.clock.Now()
		switch existing.Status {
		case StatusPending:
			existing.SourceState = SourceStateOrphaned
		case StatusInProgress:
			existing.SourceState = SourceStateBlocked
			if blockedErr == nil {
				blockedErr = &ReconciliationError{Reason: fmt.Sprintf("in-progress item %q is missing from the current backlog", existing.Title)}
			}
		case StatusDone:
			existing.SourceState = SourceStateHistorical
		}
	}
	for _, match := range matches {
		if match.item != nil {
			continue
		}
		itemID := itemIDFromSeed("title", match.source.Title)
		if match.source.SourceID != "" {
			itemID = itemIDFromSeed("source", match.source.SourceID)
		}
		slug := Slug(match.source.Title, takenSlugs)
		takenSlugs[slug] = true
		bytes, hash := requirementIdentity(match.source.Description)
		candidate = append(candidate, &LedgerItem{
			ItemID:           itemID,
			SourceID:         match.source.SourceID,
			Slug:             slug,
			Title:            match.source.Title,
			Description:      match.source.Description,
			RequirementBytes: bytes,
			RequirementHash:  hash,
			SourceState:      SourceStateCurrent,
			SourceOrder:      match.order,
			Priority:         match.source.Priority,
			Status:           StatusPending,
			StageState:       StageStatePending,
			UpdatedAt:        l.clock.Now(),
		})
	}
	l.items = candidate
	return blockedErr
}

func validateSourceItems(items []Item) error {
	sourceIDs := make(map[string]bool, len(items))
	titlesWithoutSource := make(map[string]bool, len(items))
	for i := range items {
		items[i].SourceID = strings.TrimSpace(items[i].SourceID)
		if items[i].SourceID != "" {
			if sourceIDs[items[i].SourceID] {
				return &ReconciliationError{Reason: fmt.Sprintf("duplicate backlog ID %q", items[i].SourceID)}
			}
			sourceIDs[items[i].SourceID] = true
			continue
		}
		if titlesWithoutSource[items[i].Title] {
			return &ReconciliationError{Reason: fmt.Sprintf("duplicate backlog title %q requires explicit unique **ID** values", items[i].Title)}
		}
		titlesWithoutSource[items[i].Title] = true
	}
	return nil
}

func unmatchedLedgerItems(items []*LedgerItem, matched map[*LedgerItem]bool) []*LedgerItem {
	var out []*LedgerItem
	for _, item := range items {
		if !matched[item] {
			out = append(out, item)
		}
	}
	return out
}

func reconcileMatchedItem(item *LedgerItem, source Item, order int, now time.Time) error {
	item.SourceOrder = order
	item.UpdatedAt = now
	if item.LegacyBindingPending {
		item.SourceID = source.SourceID
		item.Title = source.Title
		item.Description = source.Description
		item.RequirementBytes, item.RequirementHash = requirementIdentity(source.Description)
		item.Priority = source.Priority
		item.RevisionFrozen = item.Status != StatusPending
		item.LegacyBindingPending = false
		item.SourceState = SourceStateCurrent
		return nil
	}
	if item.Status == StatusInProgress && (item.Title != source.Title || item.Description != source.Description) {
		item.SourceState = SourceStateBlocked
		return &ReconciliationError{Reason: fmt.Sprintf("in-progress item %q changed in the backlog; resolve the frozen revision explicitly", item.Title)}
	}
	if item.SourceID == "" {
		item.SourceID = source.SourceID
	}
	item.SourceState = SourceStateCurrent
	if item.Status != StatusPending {
		return nil
	}
	item.Title = source.Title
	item.Description = source.Description
	item.RequirementBytes, item.RequirementHash = requirementIdentity(source.Description)
	item.Priority = source.Priority
	return nil
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
		if it.Status == status && it.SourceState == SourceStateCurrent {
			picked = append(picked, indexed{item: *it, order: i})
		}
	}
	sort.SliceStable(picked, func(a, b int) bool {
		ra, rb := picked[a].item.Priority.Rank(), picked[b].item.Priority.Rank()
		if ra != rb {
			return ra < rb
		}
		if picked[a].item.SourceOrder != picked[b].item.SourceOrder {
			return picked[a].item.SourceOrder < picked[b].item.SourceOrder
		}
		return picked[a].order < picked[b].order
	})
	out := make([]LedgerItem, 0, len(picked))
	for _, p := range picked {
		out = append(out, p.item)
	}
	return out
}

// Mark applies an in-memory workflow mutation and stamps UpdatedAt. Source
// identity and requirement fields remain owned by reconciliation. Unknown
// slugs are a no-op.
func (l *Ledger) Mark(slug string, mut func(*LedgerItem)) error {
	for _, it := range l.items {
		if it.Slug == slug {
			before := *it
			beforeStatus := it.Status
			mut(it)
			freezeRequirementTransition(it, beforeStatus)
			if err := validateWorkflowMutation(before, *it); err != nil {
				*it = before
				return err
			}
			it.UpdatedAt = l.clock.Now()
			return nil
		}
	}
	return nil
}

// Commit applies one item mutation to a candidate snapshot and makes it
// authoritative only after that snapshot has been durably persisted.
func (l *Ledger) Commit(slug string, mut func(*LedgerItem)) error {
	candidate := cloneLedgerItems(l.items)
	for _, it := range candidate {
		if it.Slug != slug {
			continue
		}
		before := *it
		beforeStatus := before.Status
		mut(it)
		freezeRequirementTransition(it, beforeStatus)
		if err := validateWorkflowMutation(before, *it); err != nil {
			return err
		}
		it.UpdatedAt = l.clock.Now()
		committed, err := l.persistItems(candidate)
		if committed {
			l.items = candidate
		}
		return err
	}
	return fmt.Errorf("ledger item %q not found", slug)
}

func validateWorkflowMutation(before, after LedgerItem) error {
	if before.ItemID != after.ItemID || before.SourceID != after.SourceID || before.Slug != after.Slug {
		return invalidLedgerItem(ledgerSchemaVersion, &after, "workflow mutation cannot change item or source identity")
	}
	if before.Title != after.Title || before.Description != after.Description ||
		before.RequirementBytes != after.RequirementBytes || before.RequirementHash != after.RequirementHash ||
		before.Priority != after.Priority || before.SourceState != after.SourceState ||
		before.SourceOrder != after.SourceOrder || before.LegacyBindingPending != after.LegacyBindingPending {
		return invalidLedgerItem(ledgerSchemaVersion, &after, "workflow mutation cannot change source-owned requirement fields")
	}
	expectedFrozen := before.RevisionFrozen || (before.Status == StatusPending && after.Status != StatusPending)
	if after.RevisionFrozen != expectedFrozen {
		return invalidLedgerItem(ledgerSchemaVersion, &after, "workflow mutation cannot alter requirement freeze state")
	}
	return nil
}

func freezeRequirementTransition(item *LedgerItem, before Status) {
	if before == StatusPending && item.Status != StatusPending {
		item.RevisionFrozen = true
	}
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
