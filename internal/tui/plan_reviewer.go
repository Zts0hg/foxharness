package tui

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
)

type planReviewRequest struct {
	planMarkdown string
	reply        chan planReviewResult
}

type planReviewResult struct {
	review    tools.PlanReview
	cancelled bool
}

// PlanReviewer bridges synchronous submit_plan execution to Bubble Tea.
type PlanReviewer struct {
	requests chan planReviewRequest
}

// NewPlanReviewer creates an interactive plan reviewer request channel.
func NewPlanReviewer() *PlanReviewer {
	return &PlanReviewer{requests: make(chan planReviewRequest)}
}

// Requests exposes incoming plan review requests to the TUI event loop.
func (r *PlanReviewer) Requests() <-chan planReviewRequest {
	return r.requests
}

// ReviewPlan blocks until the TUI approves, continues planning, or cancels.
func (r *PlanReviewer) ReviewPlan(ctx context.Context, planMarkdown string) (tools.PlanReview, error) {
	reply := make(chan planReviewResult, 1)
	req := planReviewRequest{planMarkdown: planMarkdown, reply: reply}
	select {
	case r.requests <- req:
	case <-ctx.Done():
		return tools.PlanReview{}, ctx.Err()
	}

	select {
	case result := <-reply:
		if result.cancelled {
			return tools.PlanReview{}, tools.ErrPlanReviewCancelled
		}
		return result.review, nil
	case <-ctx.Done():
		return tools.PlanReview{}, ctx.Err()
	}
}

var _ tools.PlanReviewer = (*PlanReviewer)(nil)

type planReviewMsg struct {
	req planReviewRequest
}

func listenForPlanReview(ctx context.Context, reviewer *PlanReviewer) tea.Cmd {
	if reviewer == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case req := <-reviewer.Requests():
			return planReviewMsg{req: req}
		case <-ctx.Done():
			return nil
		}
	}
}
