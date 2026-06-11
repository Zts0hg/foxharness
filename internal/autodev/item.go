package autodev

// Priority orders backlog items for selection. Items are processed
// high → medium → low, with document order breaking ties.
type Priority string

// Priority buckets recognized in the backlog. Unknown or missing values
// default to PriorityLow so malformed items sink to the end of the queue.
const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

// Rank maps a Priority to a sortable integer where a smaller value is
// processed first. Unknown priorities rank with PriorityLow.
func (p Priority) Rank() int {
	switch p {
	case PriorityHigh:
		return 0
	case PriorityMedium:
		return 1
	default:
		return 2
	}
}

// Status is the processing state of an item. The ledger's Status is
// authoritative; the backlog Status field is advisory/initial only.
type Status string

// Item lifecycle states recorded in the ledger.
const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in-progress"
	StatusDone       Status = "done"
)

// Item is one backlog requirement parsed from the backlog markdown file.
// The backlog supplies the item set, the ordering input (Priority), and the
// Description; the ledger supplies the authoritative processing status.
type Item struct {
	// Type is the bracketed category from the heading, e.g. "feature".
	Type string
	// Title is the heading text after the type bracket.
	Title string
	// Priority orders selection; missing values default to PriorityLow.
	Priority Priority
	// Status is the advisory initial state; missing defaults to StatusPending.
	Status Status
	// Description is the free-text requirement supplied to generate-spec
	// as the already-clarified input.
	Description string
}

// Worktree describes one isolated git worktree created for an item.
type Worktree struct {
	// Path is the absolute worktree directory.
	Path string
	// Branch is the dedicated branch, normally "auto/<slug>".
	Branch string
	// Slug is the item slug the worktree belongs to.
	Slug string
	// Resumed reports whether an existing worktree was reused rather than
	// freshly created.
	Resumed bool
}

// GateStep is the outcome of one completion-gate command.
type GateStep struct {
	// Name identifies the gate: "build", "test", or "gofmt".
	Name string
	// Passed reports whether the gate command succeeded.
	Passed bool
	// Skipped reports whether the gate was disabled by configuration.
	Skipped bool
	// Output is the raw command output, kept for diagnostics.
	Output string
}

// GateResult aggregates the completion-gate outcome for one worktree.
type GateResult struct {
	// Passed is true when every enabled gate succeeded.
	Passed bool
	// Steps lists each gate in execution order.
	Steps []GateStep
}

// PublishResult records the verified ground truth produced by the remote
// publishing sequence.
type PublishResult struct {
	// Branch is the pushed branch name.
	Branch string
	// Issue is the GitHub issue number created for the item.
	Issue int
	// PR is the GitHub pull-request number opened for the item.
	PR int
}

// StageContext carries the per-item state threaded through stage prompts,
// engineer decisions, and Verify predicates. Stage Verify functions may
// mutate it (e.g. binding SpecDir after generate-spec).
type StageContext struct {
	// Item is the backlog requirement being processed.
	Item Item
	// Slug is the item's unique kebab-case identifier.
	Slug string
	// WorkDir is the item's worktree directory (the core Agent's scope).
	WorkDir string
	// RepoRoot is the main repository root (the control plane's scope).
	RepoRoot string
	// Branch is the item's dedicated branch.
	Branch string
	// BaseBranch is the branch worktrees fork from, normally "main".
	BaseBranch string
	// Remote is the git remote name used for push verification.
	Remote string
	// Stage is the name of the step currently being driven.
	Stage string
	// SpecDir is the spec directory bound after generate-spec (REQ-011),
	// relative to WorkDir.
	SpecDir string
	// PreexistingSpecDirs snapshots .codexspec/specs/ entries before
	// generate-spec so the new directory can be detected by diff.
	PreexistingSpecDirs map[string]bool
	// BaseHead is the worktree HEAD recorded before the commit step so the
	// commit Verify can observe that HEAD advanced.
	BaseHead string
	// Issue is the GitHub issue number once verified, used by the PR step
	// to check the "Closes #N" link.
	Issue int
	// PR is the pull-request number once verified.
	PR int
}
