package agentops

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

func Parse(text string) Task {
	return Task{
		Text:  text,
		Query: text,
	}
}
