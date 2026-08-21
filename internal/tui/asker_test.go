package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/app"
)

func TestAskerDeliversAnswers(t *testing.T) {
	a := NewAsker()
	request := app.QuestionRequest{Correlation: app.InteractionCorrelation{ID: "question-1"}, Questions: []app.Question{{Prompt: "Q1?", Options: []app.QuestionOption{{Label: "a"}, {Label: "b"}}}}}

	go func() {
		req := <-a.Requests()
		req.reply <- answerResult{answers: []app.QuestionAnswer{{QuestionText: "Q1?", Value: "a"}}}
	}()

	got, err := a.AskQuestions(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Answers) != 1 || got.Answers[0].Value != "a" || got.CorrelationID != "question-1" {
		t.Fatalf("unexpected answers: %+v", got)
	}
}

func TestAskerCancelledReply(t *testing.T) {
	a := NewAsker()
	go func() {
		req := <-a.Requests()
		req.reply <- answerResult{cancelled: true}
	}()

	_, err := a.AskQuestions(context.Background(), app.QuestionRequest{Correlation: app.InteractionCorrelation{ID: "question-1"}})
	if !errors.Is(err, app.ErrQuestionCancelled) {
		t.Fatalf("expected ErrUserCancelled, got %v", err)
	}
}

func TestAskerContextCancelledWhileSending(t *testing.T) {
	a := NewAsker() // no reader on Requests()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := a.AskQuestions(ctx, app.QuestionRequest{})
		done <- err
	}()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Ask did not return promptly after context cancellation")
	}
}

func TestAskerContextCancelledWhileWaiting(t *testing.T) {
	a := NewAsker()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := a.AskQuestions(ctx, app.QuestionRequest{})
		done <- err
	}()

	// Consume the request but never reply, then cancel.
	<-a.Requests()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Ask did not return promptly after context cancellation while waiting")
	}
}
