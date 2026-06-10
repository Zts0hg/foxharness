package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestAskerDeliversAnswers(t *testing.T) {
	a := NewAsker()
	questions := []tools.Question{{Prompt: "Q1?", Options: []tools.Option{{Label: "a"}, {Label: "b"}}}}

	go func() {
		req := <-a.Requests()
		req.reply <- answerResult{answers: []tools.Answer{{QuestionText: "Q1?", Value: "a"}}}
	}()

	got, err := a.Ask(context.Background(), questions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Value != "a" {
		t.Fatalf("unexpected answers: %+v", got)
	}
}

func TestAskerCancelledReply(t *testing.T) {
	a := NewAsker()
	go func() {
		req := <-a.Requests()
		req.reply <- answerResult{cancelled: true}
	}()

	_, err := a.Ask(context.Background(), []tools.Question{{Prompt: "Q?"}})
	if !errors.Is(err, tools.ErrUserCancelled) {
		t.Fatalf("expected ErrUserCancelled, got %v", err)
	}
}

func TestAskerContextCancelledWhileSending(t *testing.T) {
	a := NewAsker() // no reader on Requests()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := a.Ask(ctx, []tools.Question{{Prompt: "Q?"}})
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
		_, err := a.Ask(ctx, []tools.Question{{Prompt: "Q?"}})
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
