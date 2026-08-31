package autodev

import "context"

// Question is one Engineer-mediated choice requested by the core runtime.
type Question struct {
	Header      string
	Prompt      string
	Options     []Option
	MultiSelect bool
}

// Option is one selectable answer to a Question.
type Option struct {
	Label       string
	Description string
	Preview     string
}

// Answer is one Engineer response correlated by exact question text.
type Answer struct {
	QuestionText string
	Value        string
	Preview      string
	Notes        string
}

// QuestionAsker is the Autodev-owned question port installed on a core runner.
type QuestionAsker interface {
	Ask(context.Context, []Question) ([]Answer, error)
}
