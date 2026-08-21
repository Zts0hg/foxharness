package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/app"
)

func TestPlanReviewerDeliversExactPlanAndDecision(t *testing.T) {
	reviewer := NewPlanReviewer()
	plan := "\n# Exact proposal\n\nNo trailing newline"

	go func() {
		req := <-reviewer.Requests()
		if req.request.PlanMarkdown != plan {
			t.Errorf("request plan = %q, want exact %q", req.request.PlanMarkdown, plan)
		}
		req.reply <- planReviewResult{response: app.PlanReviewResponse{
			CorrelationID: req.request.Correlation.ID, Decision: app.PlanContinuePlanning,
			Feedback: "add rollback steps",
		}}
	}()

	got, err := reviewer.ReviewPlan(context.Background(), app.PlanReviewRequest{
		Correlation: app.InteractionCorrelation{ID: "plan-1"}, PlanMarkdown: plan,
	})
	if err != nil {
		t.Fatalf("ReviewPlan() error = %v", err)
	}
	if got.Decision != app.PlanContinuePlanning || got.Feedback != "add rollback steps" || got.CorrelationID != "plan-1" {
		t.Fatalf("ReviewPlan() = %#v", got)
	}
}

func TestPlanReviewerCancelledReply(t *testing.T) {
	reviewer := NewPlanReviewer()
	go func() {
		req := <-reviewer.Requests()
		req.reply <- planReviewResult{cancelled: true}
	}()

	_, err := reviewer.ReviewPlan(context.Background(), app.PlanReviewRequest{Correlation: app.InteractionCorrelation{ID: "plan-1"}})
	if !errors.Is(err, app.ErrPlanReviewCancelled) {
		t.Fatalf("ReviewPlan() error = %v, want ErrPlanReviewCancelled", err)
	}
}

func TestPlanReviewerContextCancellation(t *testing.T) {
	reviewer := NewPlanReviewer()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := reviewer.ReviewPlan(ctx, app.PlanReviewRequest{})
		done <- err
	}()

	<-reviewer.Requests()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ReviewPlan() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ReviewPlan did not return after context cancellation")
	}
}
