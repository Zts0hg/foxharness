package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// ErrPlanReviewCancelled indicates that the user closed plan review without
// approving the proposal or requesting a specific revision.
var ErrPlanReviewCancelled = errors.New("user cancelled plan review")

// PlanReviewDecision identifies the user's response to a submitted proposal.
type PlanReviewDecision string

const (
	// PlanApproved permits the same task to continue in Default mode.
	PlanApproved PlanReviewDecision = "approved"
	// PlanContinuePlanning keeps the active task in Formal Plan mode.
	PlanContinuePlanning PlanReviewDecision = "continue_planning"
)

// PlanReview contains the user's decision and optional revision feedback.
type PlanReview struct {
	Decision PlanReviewDecision
	Feedback string
}

// PlanReviewer presents a persisted proposal and waits for the user's decision.
type PlanReviewer interface {
	ReviewPlan(ctx context.Context, planMarkdown string) (PlanReview, error)
}

// PlanStore atomically replaces the current session's complete plan proposal.
type PlanStore interface {
	ReplacePlan(content string) error
}

// SubmitPlanTool persists and presents complete Formal Plan proposals.
type SubmitPlanTool struct {
	store      PlanStore
	reviewer   PlanReviewer
	onApproved func(planMarkdown string)
}

// NewSubmitPlanTool creates a complete-proposal submission tool.
func NewSubmitPlanTool(store PlanStore, reviewer PlanReviewer, onApproved func(string)) *SubmitPlanTool {
	return &SubmitPlanTool{
		store:      store,
		reviewer:   reviewer,
		onApproved: onApproved,
	}
}

// Name returns the registered tool name.
func (t *SubmitPlanTool) Name() string {
	return "submit_plan"
}

// Definition returns the complete Markdown proposal schema.
func (t *SubmitPlanTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name: t.Name(),
		Description: "Submit the complete implementation plan for user review. " +
			"Call this only after read-only exploration and clarification are complete. " +
			"Each call replaces the entire proposal; do not submit incremental edits or call it alongside implementation tools.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"plan_markdown": map[string]interface{}{
					"type":        "string",
					"description": "Complete Markdown implementation proposal presented to the user exactly as submitted.",
				},
			},
			"required": []string{"plan_markdown"},
		},
	}
}

type submitPlanArgs struct {
	PlanMarkdown string `json:"plan_markdown"`
}

// Execute persists the complete proposal before opening review and returns the
// user's decision to the Agent.
func (t *SubmitPlanTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input submitPlanArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("failed to parse submit_plan arguments: %w", err)
	}
	if strings.TrimSpace(input.PlanMarkdown) == "" {
		return "", fmt.Errorf("plan_markdown cannot be empty")
	}
	if t.store == nil {
		return "", fmt.Errorf("plan store is unavailable")
	}
	if err := t.store.ReplacePlan(input.PlanMarkdown); err != nil {
		return "", fmt.Errorf("failed to persist PLAN.md: %w", err)
	}
	if t.reviewer == nil {
		return "", fmt.Errorf("interactive plan reviewer is unavailable")
	}

	review, err := t.reviewer.ReviewPlan(ctx, input.PlanMarkdown)
	if err != nil {
		if errors.Is(err, ErrPlanReviewCancelled) {
			return "The plan was not approved. The user closed plan review; continue planning without implementing.", nil
		}
		return "", fmt.Errorf("plan review failed: %w", err)
	}

	switch review.Decision {
	case PlanApproved:
		if t.onApproved != nil {
			t.onApproved(input.PlanMarkdown)
		}
		return "The user approved the plan. Continue this task in Default mode and call update_todo with an ordered, executable checklist before implementation.", nil
	case PlanContinuePlanning:
		if strings.TrimSpace(review.Feedback) == "" {
			return "The user chose to continue planning without approving this proposal. Do not implement; continue planning and submit a complete replacement when ready.", nil
		}
		return "The user requested plan changes. Continue planning without implementing, then submit a complete replacement. Feedback:\n\n" + review.Feedback, nil
	default:
		return "", fmt.Errorf("unsupported plan review decision %q", review.Decision)
	}
}
