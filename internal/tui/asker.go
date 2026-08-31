package tui

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

// askRequest carries one ask_user_question invocation from the engine goroutine
// to the Bubble Tea update loop, along with the channel on which the loop sends
// the collected result back.
type askRequest struct {
	request app.QuestionRequest
	reply   chan answerResult
}

// answerResult is the outcome of presenting an askRequest: the collected answers,
// or cancelled when the user dismissed the prompt.
type answerResult struct {
	answers   []app.QuestionAnswer
	cancelled bool
}

// Asker is the interactive UserAsker. It bridges the synchronous tool Execute
// (running on the engine goroutine) and the Bubble Tea update loop using a
// long-lived request channel; each request carries its own reply channel. The
// TUI model listens on Requests() and replies once the user has answered.
type Asker struct {
	requests chan askRequest
}

// NewAsker creates an Asker with an unbuffered request channel.
func NewAsker() *Asker {
	return &Asker{requests: make(chan askRequest)}
}

// Requests exposes the channel the model listens on for incoming questions.
func (a *Asker) Requests() <-chan askRequest {
	return a.requests
}

// AskQuestions sends the questions to the UI loop and blocks until the user answers or the
// context is cancelled. It returns app.ErrQuestionCancelled if the user dismissed
// the prompt, or the context error if cancelled while sending or waiting.
func (a *Asker) AskQuestions(ctx context.Context, request app.QuestionRequest) (app.QuestionResponse, error) {
	reply := make(chan answerResult, 1)
	req := askRequest{request: request, reply: reply}

	select {
	case a.requests <- req:
	case <-ctx.Done():
		return app.QuestionResponse{}, ctx.Err()
	}

	select {
	case res := <-reply:
		if res.cancelled {
			return app.QuestionResponse{CorrelationID: request.Correlation.ID}, app.ErrQuestionCancelled
		}
		return app.QuestionResponse{CorrelationID: request.Correlation.ID, Answers: res.answers}, nil
	case <-ctx.Done():
		return app.QuestionResponse{}, ctx.Err()
	}
}

var _ app.QuestionPort = (*Asker)(nil)

// askUserMsg delivers an incoming question request to the Bubble Tea update loop.
type askUserMsg struct {
	req askRequest
}

// listenForAsk returns a command that waits for the next question request and
// delivers it as an askUserMsg. It returns no message if the asker is nil or the
// context is cancelled, so it never forces the program to quit.
func listenForAsk(ctx context.Context, a *Asker) tea.Cmd {
	if a == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case req := <-a.Requests():
			return askUserMsg{req: req}
		case <-ctx.Done():
			return nil
		}
	}
}
