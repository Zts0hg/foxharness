package tools

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakePlanStore struct {
	content string
	err     error
	events  *[]string
}

func (s *fakePlanStore) ReplacePlan(content string) error {
	if s.events != nil {
		*s.events = append(*s.events, "store:"+content)
	}
	if s.err != nil {
		return s.err
	}
	s.content = content
	return nil
}

type fakePlanReviewer struct {
	decision PlanReview
	err      error
	plans    []string
	events   *[]string
}

func (r *fakePlanReviewer) ReviewPlan(ctx context.Context, planMarkdown string) (PlanReview, error) {
	r.plans = append(r.plans, planMarkdown)
	if r.events != nil {
		*r.events = append(*r.events, "review:"+planMarkdown)
	}
	return r.decision, r.err
}

func mustSubmitPlanArgs(t *testing.T, plan string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"plan_markdown": plan})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return raw
}

func TestSubmitPlanDefinition(t *testing.T) {
	tool := NewSubmitPlanTool(&fakePlanStore{}, &fakePlanReviewer{}, nil)
	if got := tool.Name(); got != "submit_plan" {
		t.Fatalf("Name() = %q, want submit_plan", got)
	}
	def := tool.Definition()
	if def.Name != "submit_plan" {
		t.Fatalf("Definition().Name = %q, want submit_plan", def.Name)
	}
	schema, ok := def.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatalf("InputSchema type = %T, want map", def.InputSchema)
	}
	if got := schema["required"]; !reflect.DeepEqual(got, []string{"plan_markdown"}) {
		t.Fatalf("required = %#v, want plan_markdown", got)
	}
}

func TestSubmitPlanPersistsBeforeReviewAndApprovesExactPayload(t *testing.T) {
	events := []string{}
	store := &fakePlanStore{content: "old", events: &events}
	reviewer := &fakePlanReviewer{
		decision: PlanReview{Decision: PlanApproved},
		events:   &events,
	}
	var approved string
	tool := NewSubmitPlanTool(store, reviewer, func(plan string) {
		events = append(events, "approved:"+plan)
		approved = plan
	})
	plan := "\n# Plan\n\nExact bytes without trailing newline"

	out, err := tool.Execute(context.Background(), mustSubmitPlanArgs(t, plan))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if store.content != plan || approved != plan {
		t.Fatalf("payload drift: stored=%q approved=%q want=%q", store.content, approved, plan)
	}
	if !reflect.DeepEqual(reviewer.plans, []string{plan}) {
		t.Fatalf("reviewed plans = %#v, want exact payload", reviewer.plans)
	}
	wantEvents := []string{"store:" + plan, "review:" + plan, "approved:" + plan}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
	if !strings.Contains(strings.ToLower(out), "approved") || !strings.Contains(out, "update_todo") {
		t.Fatalf("output = %q, want approval and update_todo guidance", out)
	}
}

func TestSubmitPlanContinuePlanningReturnsFeedbackWithoutApproval(t *testing.T) {
	tests := []struct {
		name     string
		feedback string
		want     string
	}{
		{name: "revision", feedback: "Split database migration into two steps.", want: "Split database migration"},
		{name: "no feedback", feedback: "", want: "continue planning"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakePlanStore{}
			reviewer := &fakePlanReviewer{decision: PlanReview{
				Decision: PlanContinuePlanning,
				Feedback: tt.feedback,
			}}
			approved := false
			tool := NewSubmitPlanTool(store, reviewer, func(string) { approved = true })

			out, err := tool.Execute(context.Background(), mustSubmitPlanArgs(t, "proposal"))
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if approved {
				t.Fatal("approval callback invoked for continue planning")
			}
			if !strings.Contains(strings.ToLower(out), strings.ToLower(tt.want)) {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
		})
	}
}

func TestSubmitPlanRejectsBlankBeforePersistence(t *testing.T) {
	events := []string{}
	store := &fakePlanStore{content: "old", events: &events}
	reviewer := &fakePlanReviewer{events: &events}
	tool := NewSubmitPlanTool(store, reviewer, nil)

	if _, err := tool.Execute(context.Background(), mustSubmitPlanArgs(t, " \n\t ")); err == nil {
		t.Fatal("Execute() error = nil, want blank-plan error")
	}
	if store.content != "old" || len(events) != 0 || len(reviewer.plans) != 0 {
		t.Fatalf("blank input caused side effects: store=%q events=%#v plans=%#v", store.content, events, reviewer.plans)
	}
}

func TestSubmitPlanWriteFailurePreservesPriorProposalAndSkipsReview(t *testing.T) {
	writeErr := errors.New("disk full")
	store := &fakePlanStore{content: "last successful", err: writeErr}
	reviewer := &fakePlanReviewer{decision: PlanReview{Decision: PlanApproved}}
	approved := false
	tool := NewSubmitPlanTool(store, reviewer, func(string) { approved = true })

	_, err := tool.Execute(context.Background(), mustSubmitPlanArgs(t, "new proposal"))
	if err == nil || !strings.Contains(err.Error(), writeErr.Error()) {
		t.Fatalf("Execute() error = %v, want %v", err, writeErr)
	}
	if store.content != "last successful" || len(reviewer.plans) != 0 || approved {
		t.Fatalf("write failure changed state: store=%q plans=%#v approved=%v", store.content, reviewer.plans, approved)
	}
}

func TestSubmitPlanReviewCancellationStaysUnapproved(t *testing.T) {
	reviewer := &fakePlanReviewer{err: ErrPlanReviewCancelled}
	approved := false
	tool := NewSubmitPlanTool(&fakePlanStore{}, reviewer, func(string) { approved = true })

	out, err := tool.Execute(context.Background(), mustSubmitPlanArgs(t, "proposal"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if approved || !strings.Contains(strings.ToLower(out), "not approved") {
		t.Fatalf("cancel result = %q, approved=%v", out, approved)
	}
}

func TestSubmitPlanReviewerErrorIsReturned(t *testing.T) {
	reviewErr := errors.New("review UI failed")
	reviewer := &fakePlanReviewer{err: reviewErr}
	approved := false
	tool := NewSubmitPlanTool(&fakePlanStore{}, reviewer, func(string) { approved = true })

	_, err := tool.Execute(context.Background(), mustSubmitPlanArgs(t, "proposal"))
	if err == nil || !strings.Contains(err.Error(), reviewErr.Error()) {
		t.Fatalf("Execute() error = %v, want %v", err, reviewErr)
	}
	if approved {
		t.Fatal("approval callback invoked after reviewer error")
	}
}

func TestSubmitPlanRejectsMalformedJSON(t *testing.T) {
	tool := NewSubmitPlanTool(&fakePlanStore{}, &fakePlanReviewer{}, nil)
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{bad`)); err == nil {
		t.Fatal("Execute() error = nil, want malformed JSON error")
	}
}
