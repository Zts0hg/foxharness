package feishu

// Task carries the full context of a single Feishu message event that has
// been parsed and is ready for execution by the Runner.
type Task struct {
	TaskID    string
	ChatID    string
	SenderID  string
	MessageID string
	Text      string
}
