package selector

// RestoreAction describes what the rewind flow should restore.
type RestoreAction int

const (
	// ActionNone indicates no restoration should happen.
	ActionNone RestoreAction = iota
	// ActionRestoreBoth restores code and conversation.
	ActionRestoreBoth
	// ActionRestoreConversation restores conversation only.
	ActionRestoreConversation
	// ActionRestoreCode restores code only.
	ActionRestoreCode
	// ActionCancelled indicates the selector was cancelled.
	ActionCancelled
)

// ViewState is the selector's active screen.
type ViewState int

const (
	listView ViewState = iota
	previewView
)

// ResultMsg is sent to the parent TUI model when the selector completes.
type ResultMsg struct {
	Action    RestoreAction
	MessageID string
}
