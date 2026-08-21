package app

import (
	"context"
	"time"
)

/* RunCapabilities describes optional command dimensions supported by an interactive application. */
type RunCapabilities struct {
	ToolRestrictions bool
	EffortOverrides  bool
}

/* InteractiveSessionState is one presentation-safe snapshot of the selected live session. */
type InteractiveSessionState struct {
	Session           SessionInfo
	WorkDir           string
	Model             string
	Effort            string
	ContextUsage      string
	CollaborationMode string
	AutoMemoryIndex   string
	RewindAvailable   bool
	RunCapabilities   RunCapabilities
}

/* ConversationToolCall is the presentation-safe tool invocation stored with an assistant message. */
type ConversationToolCall struct {
	ID        string
	Name      string
	Arguments string
}

/* ConversationRecord is the application-owned projection of one persisted conversation record. */
type ConversationRecord struct {
	Sequence                  int64
	Time                      time.Time
	Role                      string
	Content                   string
	DisplayContent            string
	ToolCallID                string
	ToolCalls                 []ConversationToolCall
	IsMeta                    bool
	IsCompactSummary          bool
	IsVisibleInTranscriptOnly bool
}

/* HumanContent returns the presentation content selected for a stored user message. */
func (r ConversationRecord) HumanContent() string {
	if r.DisplayContent != "" {
		return r.DisplayContent
	}
	return r.Content
}

/* NewSessionCommand requests a fresh session under the application's fixed launch profile. */
type NewSessionCommand struct{}

/* ModelCommand changes the model snapshot used by future interactive runs. */
type ModelCommand struct {
	Model string
}

/* EffortCommand changes the session-level effort used by future interactive runs. */
type EffortCommand struct {
	Effort string
}

/* CollaborationCommand changes the collaboration mode selected for future interactive runs. */
type CollaborationCommand struct {
	Mode string
}

/* CompactCommand requests manual compaction with optional user instructions. */
type CompactCommand struct {
	Instructions string
}

/* CompactOutcome contains the stable statistics shown after manual compaction. */
type CompactOutcome struct {
	PreTokens          int
	PostTokens         int
	MessagesSummarized int
}

/* RewindDiff summarizes code changes relative to one rewind target. */
type RewindDiff struct {
	FilesChanged int
	Insertions   int
	Deletions    int
	ChangedFiles []string
}

/* RewindTarget is one user-authored conversation point selectable for restore. */
type RewindTarget struct {
	Sequence  int64
	Content   string
	Timestamp time.Time
	IsCurrent bool
	Diff      RewindDiff
	DiffError string
}

/* RewindAction selects the state dimensions restored by a rewind command. */
type RewindAction string

const (
	/* RewindBoth restores code and recoverable conversation state. */
	RewindBoth RewindAction = "both"
	/* RewindConversation restores recoverable conversation state only. */
	RewindConversation RewindAction = "conversation"
	/* RewindCode restores code only. */
	RewindCode RewindAction = "code"
)

/* RewindCommand requests restoration to the state immediately before one message sequence. */
type RewindCommand struct {
	Sequence int64
	Action   RewindAction
}

/* RewindOutcome preserves each ordered partial result of a rewind operation. */
type RewindOutcome struct {
	Error                 string
	CodeAttempted         bool
	CodeFiles             []string
	CodeError             string
	ConversationAttempted bool
	Conversation          []ConversationRecord
	RestoredInput         string
	ConversationError     string
	SessionStateAttempted bool
	SessionStateRestored  bool
	SessionStateError     string
}

/* RestoreInputOutcome describes an automatic cancellation restore without code or session-state changes. */
type RestoreInputOutcome struct {
	Attempted    bool
	Restored     bool
	Conversation []ConversationRecord
	Input        string
}

/* InteractiveStateReader exposes presentation-safe queries for one selected interactive session. */
type InteractiveStateReader interface {
	State() InteractiveSessionState
	Conversation(context.Context) ([]ConversationRecord, error)
	ProjectInputHistory(context.Context, int) ([]string, error)
	RewindTargets(context.Context) ([]RewindTarget, error)
}

/* InteractiveStateController exposes typed mutations of one selected interactive session. */
type InteractiveStateController interface {
	NewSession(context.Context, NewSessionCommand) (InteractiveSessionState, error)
	UpdateModel(context.Context, ModelCommand) (InteractiveSessionState, error)
	UpdateEffort(context.Context, EffortCommand) InteractiveSessionState
	UpdateCollaborationMode(context.Context, CollaborationCommand) InteractiveSessionState
	Compact(context.Context, CompactCommand) (CompactOutcome, error)
	Rewind(context.Context, RewindCommand) RewindOutcome
	RestoreLatestInput(context.Context) (RestoreInputOutcome, error)
}

/* InteractiveApplication is the complete UI-neutral capability set consumed by the TUI adapter. */
type InteractiveApplication interface {
	RunUseCase
	InteractiveStateReader
	InteractiveStateController
	InteractivePermissionController
}
