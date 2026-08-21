package interactionruntime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type questionPortFunc func(context.Context, app.QuestionRequest) (app.QuestionResponse, error)

func (f questionPortFunc) AskQuestions(ctx context.Context, request app.QuestionRequest) (app.QuestionResponse, error) {
	return f(ctx, request)
}

type planPortFunc func(context.Context, app.PlanReviewRequest) (app.PlanReviewResponse, error)

func (f planPortFunc) ReviewPlan(ctx context.Context, request app.PlanReviewRequest) (app.PlanReviewResponse, error) {
	return f(ctx, request)
}

type permissionPortFunc func(context.Context, app.PermissionRequest) (app.PermissionResponse, error)

func (f permissionPortFunc) RequestPermission(ctx context.Context, request app.PermissionRequest) (app.PermissionResponse, error) {
	return f(ctx, request)
}

type noticeSinkFunc func(context.Context, app.InteractionNotice)

func (f noticeSinkFunc) NotifyInteraction(ctx context.Context, notice app.InteractionNotice) {
	f(ctx, notice)
}

type typedNilPort struct{}

func (*typedNilPort) AskQuestions(context.Context, app.QuestionRequest) (app.QuestionResponse, error) {
	panic("typed-nil question port invoked")
}
func (*typedNilPort) ReviewPlan(context.Context, app.PlanReviewRequest) (app.PlanReviewResponse, error) {
	panic("typed-nil plan port invoked")
}
func (*typedNilPort) RequestPermission(context.Context, app.PermissionRequest) (app.PermissionResponse, error) {
	panic("typed-nil permission port invoked")
}
func (*typedNilPort) NotifyInteraction(context.Context, app.InteractionNotice) {
	panic("typed-nil notice sink invoked")
}

func TestAdaptersNormalizeTypedNilPorts(t *testing.T) {
	var port *typedNilPort
	if NewQuestionAsker(port) != nil || NewPlanReviewer(port) != nil || NewPermissionApprover(port) != nil {
		t.Fatal("typed-nil interaction port was not normalized")
	}
	events := NewPermissionEvents(port)
	events.OnReviewStart(permission.Request{ToolName: "bash"})
	events.OnPermissionStateChanged()
}

func TestQuestionAndPlanAdaptersPreserveCorrelationValuesAndCancellation(t *testing.T) {
	ctx := toolprotocol.WithToolCall(tools.WithRunContext(context.Background(), "session-1", "run-1"), "call-1")
	asker := NewQuestionAsker(questionPortFunc(func(_ context.Context, request app.QuestionRequest) (app.QuestionResponse, error) {
		if request.Correlation != (app.InteractionCorrelation{ID: "question:session-1:run-1:call-1", SessionID: "session-1", RunID: "run-1", ToolCallID: "call-1"}) {
			t.Fatalf("question correlation = %#v", request.Correlation)
		}
		if len(request.Questions) != 1 || request.Questions[0].Options[0].Preview != "preview" || !request.Questions[0].MultiSelect {
			t.Fatalf("question request = %#v", request)
		}
		return app.QuestionResponse{CorrelationID: request.Correlation.ID, Answers: []app.QuestionAnswer{{
			QuestionText: "Choose?", Value: "A", Preview: "preview", Notes: "note",
		}}}, nil
	}))
	answers, err := asker.Ask(ctx, []tools.Question{{
		Header: "Scope", Prompt: "Choose?", MultiSelect: true,
		Options: []tools.Option{{Label: "A", Description: "first", Preview: "preview"}},
	}})
	if err != nil || !reflect.DeepEqual(answers, []tools.Answer{{QuestionText: "Choose?", Value: "A", Preview: "preview", Notes: "note"}}) {
		t.Fatalf("answers/error = %#v/%v", answers, err)
	}

	reviewer := NewPlanReviewer(planPortFunc(func(_ context.Context, request app.PlanReviewRequest) (app.PlanReviewResponse, error) {
		if request.Correlation.ToolCallID != "call-1" || request.PlanMarkdown != "# Plan" {
			t.Fatalf("plan request = %#v", request)
		}
		return app.PlanReviewResponse{CorrelationID: request.Correlation.ID, Decision: app.PlanContinuePlanning, Feedback: "revise"}, nil
	}))
	review, err := reviewer.ReviewPlan(ctx, "# Plan")
	if err != nil || review.Decision != tools.PlanContinuePlanning || review.Feedback != "revise" {
		t.Fatalf("review/error = %#v/%v", review, err)
	}

	cancelled := NewQuestionAsker(questionPortFunc(func(_ context.Context, request app.QuestionRequest) (app.QuestionResponse, error) {
		return app.QuestionResponse{CorrelationID: request.Correlation.ID}, app.ErrQuestionCancelled
	}))
	if _, err := cancelled.Ask(context.Background(), nil); !errors.Is(err, tools.ErrUserCancelled) {
		t.Fatalf("question cancellation = %v", err)
	}
}

func TestAdaptersRejectStaleResponses(t *testing.T) {
	question := NewQuestionAsker(questionPortFunc(func(context.Context, app.QuestionRequest) (app.QuestionResponse, error) {
		return app.QuestionResponse{CorrelationID: "stale"}, nil
	}))
	if _, err := question.Ask(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "correlation") {
		t.Fatalf("stale question error = %v", err)
	}
	plan := NewPlanReviewer(planPortFunc(func(context.Context, app.PlanReviewRequest) (app.PlanReviewResponse, error) {
		return app.PlanReviewResponse{CorrelationID: "stale"}, nil
	}))
	if _, err := plan.ReviewPlan(context.Background(), "plan"); err == nil || !strings.Contains(err.Error(), "correlation") {
		t.Fatalf("stale plan error = %v", err)
	}
	approver := NewPermissionApprover(permissionPortFunc(func(context.Context, app.PermissionRequest) (app.PermissionResponse, error) {
		return app.PermissionResponse{CorrelationID: "stale"}, nil
	}))
	if _, err := approver.Approve(context.Background(), permission.ApprovalRequest{}); err == nil || !strings.Contains(err.Error(), "correlation") {
		t.Fatalf("stale permission error = %v", err)
	}
}

func TestPermissionAdapterPreservesApprovalSemantics(t *testing.T) {
	ctx := toolprotocol.WithToolCall(tools.WithRunContext(context.Background(), "session-1", "run-1"), "call-1")
	approver := NewPermissionApprover(permissionPortFunc(func(_ context.Context, request app.PermissionRequest) (app.PermissionResponse, error) {
		want := app.PermissionRequest{
			Correlation: app.InteractionCorrelation{ID: "permission:session-1:run-1:call-1", SessionID: "session-1", RunID: "run-1", ToolCallID: "call-1"},
			ToolName:    "bash", Arguments: `{"command":"rm file"}`, Action: "bash rm file", Risk: "high", Source: "main",
			CWD: "/work", Workspace: "/work", Effects: []string{"execute"}, PolicyReason: "human approval required",
			ReviewerReason: "needs review", ReviewerFailure: "offline",
		}
		if !reflect.DeepEqual(request, want) {
			t.Fatalf("permission request = %#v, want %#v", request, want)
		}
		return app.PermissionResponse{CorrelationID: request.Correlation.ID, Decision: app.PermissionDenyWithFeedback, Feedback: "use trash"}, nil
	}))
	decision, err := approver.Approve(ctx, permission.ApprovalRequest{
		Request: permission.Request{
			ToolCall: schema.ToolCall{ID: "call-1", Name: "bash", Arguments: []byte(`{"command":"rm file"}`)},
			ToolName: "bash", Arguments: `{"command":"rm file"}`, Action: "bash rm file", Risk: permission.RiskHigh,
			Source: permission.SourceMain, CWD: "/work", Workspace: "/work",
			Capabilities: toolpolicy.Assessment{Behavior: toolpolicy.BehaviorHumanOnly, Reason: "human approval required", Effects: []toolpolicy.Effect{toolpolicy.EffectExecute}},
		},
		Review: &permission.ReviewResult{Rationale: "needs review"}, ReviewerFailure: "offline",
	})
	if err != nil || decision.Kind != permission.UserDenyFeedback || decision.Feedback != "use trash" {
		t.Fatalf("decision/error = %#v/%v", decision, err)
	}
}

func TestPermissionEventsAndControllerPreserveApplicationState(t *testing.T) {
	var notices []app.InteractionNotice
	events := NewPermissionEvents(noticeSinkFunc(func(_ context.Context, notice app.InteractionNotice) {
		notices = append(notices, notice)
	}))
	request := permission.Request{ToolCall: schema.ToolCall{ID: "call-1"}, ToolName: "bash", Action: "bash pwd"}
	events.OnReviewStart(request)
	events.OnReviewRetry(request, 2)
	events.OnAutoApproved(request, permission.ReviewResult{})
	events.OnEscalated(request, permission.ReviewResult{})
	events.OnPermissionStateChanged()
	if len(notices) != 5 || notices[1].Attempt != 2 || notices[4].Kind != app.InteractionPermissionStateChanged {
		t.Fatalf("notices = %#v", notices)
	}

	state := permission.NewState(permission.ModeApprove, false)
	state.AddGrant(permission.Grant{Key: "grant-1"})
	controller := NewPermissionController(state)
	if got := controller.UpdatePermissionMode(context.Background(), app.PermissionModeCommand{Mode: app.PermissionModeAsk}); got.EffectiveMode != app.PermissionModeAsk {
		t.Fatalf("updated permission state = %#v", got)
	}
	if got := controller.ActivateFullAccess(context.Background(), app.FullAccessCommand{Remember: true}); got.EffectiveMode != app.PermissionModeFullAccess || !got.FullAccessRemembered {
		t.Fatalf("full-access state = %#v", got)
	}
	if got := controller.ClearPermissionGrants(context.Background()); got.Cleared != 1 || got.State.SessionGrantCount != 0 {
		t.Fatalf("clear outcome = %#v", got)
	}
}
