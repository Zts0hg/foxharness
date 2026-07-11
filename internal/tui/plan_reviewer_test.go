package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestPlanReviewerDeliversExactPlanAndDecision(t *testing.T) {
	reviewer := NewPlanReviewer()
	plan := "\n# Exact proposal\n\nNo trailing newline"

	go func() {
		req := <-reviewer.Requests()
		if req.planMarkdown != plan {
			t.Errorf("request plan = %q, want exact %q", req.planMarkdown, plan)
		}
		req.reply <- planReviewResult{review: tools.PlanReview{
			Decision: tools.PlanContinuePlanning,
			Feedback: "add rollback steps",
		}}
	}()

	got, err := reviewer.ReviewPlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("ReviewPlan() error = %v", err)
	}
	if got.Decision != tools.PlanContinuePlanning || got.Feedback != "add rollback steps" {
		t.Fatalf("ReviewPlan() = %#v", got)
	}
}

func TestPlanReviewerCancelledReply(t *testing.T) {
	reviewer := NewPlanReviewer()
	go func() {
		req := <-reviewer.Requests()
		req.reply <- planReviewResult{cancelled: true}
	}()

	_, err := reviewer.ReviewPlan(context.Background(), "proposal")
	if !errors.Is(err, tools.ErrPlanReviewCancelled) {
		t.Fatalf("ReviewPlan() error = %v, want ErrPlanReviewCancelled", err)
	}
}

func TestPlanReviewerContextCancellation(t *testing.T) {
	reviewer := NewPlanReviewer()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := reviewer.ReviewPlan(ctx, "proposal")
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
