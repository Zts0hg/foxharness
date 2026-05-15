package agentops

// Task represents a single incident-analysis request received from the IM
// gateway.  It carries routing identifiers (TaskID, ChatID, SenderID,
// MessageID), the raw user text, and optional structured fields (Service,
// Since, Query) used by log-search-driven workflows.
type Task struct {
	TaskID    string
	ChatID    string
	SenderID  string
	MessageID string
	Text      string

	Service string
	Since   string
	Query   string
}

// Parse constructs a minimal Task from raw message text, copying the text
// into both Text and Query fields.
func Parse(text string) Task {
	return Task{
		Text:  text,
		Query: text,
	}
}
