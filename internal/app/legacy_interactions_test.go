package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type recordingInteractionNoticeSink struct {
	notices []InteractionNotice
}

func (s *recordingInteractionNoticeSink) NotifyInteraction(_ context.Context, notice InteractionNotice) {
	s.notices = append(s.notices, notice)
}

type typedNilInteractionPort struct{}

func (*typedNilInteractionPort) AskQuestions(context.Context, QuestionRequest) (QuestionResponse, error) {
	panic("typed-nil question port was invoked")
}

func (*typedNilInteractionPort) ReviewPlan(context.Context, PlanReviewRequest) (PlanReviewResponse, error) {
	panic("typed-nil plan-review port was invoked")
}

func (*typedNilInteractionPort) RequestPermission(context.Context, PermissionRequest) (PermissionResponse, error) {
	panic("typed-nil permission port was invoked")
}

func (*typedNilInteractionPort) NotifyInteraction(context.Context, InteractionNotice) {
	panic("typed-nil interaction notice sink was invoked")
}

func TestLegacyInteractionAdaptersNormalizeTypedNilPorts(t *testing.T) {
	var port *typedNilInteractionPort
	if got := newLegacyQuestionAsker(port); got != nil {
		t.Fatalf("typed-nil question adapter = %#v, want nil", got)
	}
	if got := newLegacyPlanReviewer(port); got != nil {
		t.Fatalf("typed-nil plan-review adapter = %#v, want nil", got)
	}
	if got := newLegacyPermissionApprover(port); got != nil {
		t.Fatalf("typed-nil permission adapter = %#v, want nil", got)
	}
	events := newLegacyPermissionEventSink(port)
	events.OnReviewStart(permission.Request{ToolName: "bash"})
	events.OnPermissionStateChanged()
}

func TestLegacyQuestionAndPlanAdaptersPreserveCorrelationAndValues(t *testing.T) {
	ctx := toolprotocol.WithRun(context.Background(), "session-1", "run-1")
	ctx = toolprotocol.WithToolCall(ctx, "call-1")
	questionPort := questionPortFunc(func(_ context.Context, request QuestionRequest) (QuestionResponse, error) {
		if request.Correlation.SessionID != "session-1" || request.Correlation.RunID != "run-1" || request.Correlation.ToolCallID != "call-1" {
			t.Fatalf("question correlation = %#v", request.Correlation)
		}
		if len(request.Questions) != 1 || request.Questions[0].ID == "" || request.Questions[0].Options[0].Preview != "preview" {
			t.Fatalf("question request = %#v", request)
		}
		return QuestionResponse{
			CorrelationID: request.Correlation.ID,
			Answers: []QuestionAnswer{{
				QuestionID: request.Questions[0].ID, QuestionText: "Choose?", Value: "A", Preview: "preview", Notes: "note",
			}},
		}, nil
	})
	answers, err := newLegacyQuestionAsker(questionPort).Ask(ctx, []tools.Question{{
		Header: "Scope", Prompt: "Choose?", MultiSelect: true,
		Options: []tools.Option{{Label: "A", Description: "first", Preview: "preview"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	wantAnswers := []tools.Answer{{QuestionText: "Choose?", Value: "A", Preview: "preview", Notes: "note"}}
	if !reflect.DeepEqual(answers, wantAnswers) {
		t.Fatalf("answers = %#v, want %#v", answers, wantAnswers)
	}

	planPort := planReviewPortFunc(func(_ context.Context, request PlanReviewRequest) (PlanReviewResponse, error) {
		if request.Correlation.ToolCallID != "call-1" || request.PlanMarkdown != "# Plan" {
			t.Fatalf("plan request = %#v", request)
		}
		return PlanReviewResponse{
			CorrelationID: request.Correlation.ID, Decision: PlanContinuePlanning, Feedback: "revise",
		}, nil
	})
	review, err := newLegacyPlanReviewer(planPort).ReviewPlan(ctx, "# Plan")
	if err != nil || review.Decision != tools.PlanContinuePlanning || review.Feedback != "revise" {
		t.Fatalf("review/error = %#v/%v", review, err)
	}
}

func TestLegacyInteractionAdaptersMapCancellationAndRejectStaleResponses(t *testing.T) {
	question := newLegacyQuestionAsker(questionPortFunc(func(_ context.Context, request QuestionRequest) (QuestionResponse, error) {
		return QuestionResponse{CorrelationID: request.Correlation.ID}, ErrQuestionCancelled
	}))
	if _, err := question.Ask(context.Background(), nil); !errors.Is(err, tools.ErrUserCancelled) {
		t.Fatalf("question cancellation = %v", err)
	}

	plan := newLegacyPlanReviewer(planReviewPortFunc(func(_ context.Context, request PlanReviewRequest) (PlanReviewResponse, error) {
		return PlanReviewResponse{CorrelationID: request.Correlation.ID}, ErrPlanReviewCancelled
	}))
	if _, err := plan.ReviewPlan(context.Background(), "plan"); !errors.Is(err, tools.ErrPlanReviewCancelled) {
		t.Fatalf("plan cancellation = %v", err)
	}

	staleQuestion := newLegacyQuestionAsker(questionPortFunc(func(context.Context, QuestionRequest) (QuestionResponse, error) {
		return QuestionResponse{CorrelationID: "stale"}, nil
	}))
	if _, err := staleQuestion.Ask(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "correlation") {
		t.Fatalf("stale question response error = %v", err)
	}

	stalePlan := newLegacyPlanReviewer(planReviewPortFunc(func(context.Context, PlanReviewRequest) (PlanReviewResponse, error) {
		return PlanReviewResponse{CorrelationID: "stale", Decision: PlanApproved}, nil
	}))
	if _, err := stalePlan.ReviewPlan(context.Background(), "plan"); err == nil || !strings.Contains(err.Error(), "correlation") {
		t.Fatalf("stale plan response error = %v", err)
	}

	permissionAdapter := newLegacyPermissionApprover(permissionPortFunc(func(context.Context, PermissionRequest) (PermissionResponse, error) {
		return PermissionResponse{CorrelationID: "stale", Decision: PermissionAllowOnce}, nil
	}))
	if _, err := permissionAdapter.Approve(context.Background(), permission.ApprovalRequest{}); err == nil || !strings.Contains(err.Error(), "correlation") {
		t.Fatalf("stale permission response error = %v", err)
	}
}

func TestLegacyPermissionAdapterPreservesVisibleApprovalSemantics(t *testing.T) {
	ctx := toolprotocol.WithRun(context.Background(), "session-1", "run-1")
	ctx = toolprotocol.WithToolCall(ctx, "call-1")
	port := permissionPortFunc(func(_ context.Context, request PermissionRequest) (PermissionResponse, error) {
		want := PermissionRequest{
			Correlation: InteractionCorrelation{
				ID: "permission:session-1:run-1:call-1", SessionID: "session-1", RunID: "run-1", ToolCallID: "call-1",
			},
			ToolName: "bash", Arguments: `{"command":"rm file"}`, Action: "bash rm file",
			Risk: "high", Source: "main", CWD: "/work", Workspace: "/work",
			Effects: []string{"execute"}, PolicyReason: "human approval required",
			ReviewerReason: "needs review", ReviewerFailure: "offline",
		}
		if !reflect.DeepEqual(request, want) {
			t.Fatalf("permission request = %#v, want %#v", request, want)
		}
		return PermissionResponse{
			CorrelationID: request.Correlation.ID, Decision: PermissionDenyWithFeedback, Feedback: "use trash",
		}, nil
	})
	decision, err := newLegacyPermissionApprover(port).Approve(ctx, permission.ApprovalRequest{
		Request: permission.Request{
			ToolCall: schema.ToolCall{ID: "call-1", Name: "bash", Arguments: []byte(`{"command":"rm file"}`)},
			ToolName: "bash", Arguments: `{"command":"rm file"}`, Action: "bash rm file",
			Risk: permission.RiskHigh, Source: permission.SourceMain, CWD: "/work", Workspace: "/work",
			Capabilities: toolpolicy.Assessment{
				Behavior: toolpolicy.BehaviorHumanOnly, Reason: "human approval required", Effects: []toolpolicy.Effect{toolpolicy.EffectExecute},
			},
		},
		Review:          &permission.ReviewResult{Rationale: "needs review"},
		ReviewerFailure: "offline",
	})
	if err != nil || decision.Kind != permission.UserDenyFeedback || decision.Feedback != "use trash" {
		t.Fatalf("decision/error = %#v/%v", decision, err)
	}
}

func TestLegacyPermissionAdapterPreservesNilEffects(t *testing.T) {
	port := permissionPortFunc(func(_ context.Context, request PermissionRequest) (PermissionResponse, error) {
		if request.Effects != nil {
			t.Fatalf("nil effects became %#v", request.Effects)
		}
		return PermissionResponse{CorrelationID: request.Correlation.ID, Decision: PermissionDeny}, nil
	})
	_, err := newLegacyPermissionApprover(port).Approve(context.Background(), permission.ApprovalRequest{
		Review: &permission.ReviewResult{Decision: permission.ReviewEscalate},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLegacyPermissionEventsMapToApplicationInteractionNotices(t *testing.T) {
	sink := &recordingInteractionNoticeSink{}
	events := newLegacyPermissionEventSink(sink)
	request := permission.Request{
		ToolCall: schema.ToolCall{ID: "call-1"}, ToolName: "bash", Action: "bash pwd",
	}
	events.OnReviewStart(request)
	events.OnReviewRetry(request, 2)
	events.OnAutoApproved(request, permission.ReviewResult{})
	events.OnEscalated(request, permission.ReviewResult{})
	events.OnPermissionStateChanged()
	want := []InteractionNotice{
		{Kind: InteractionPermissionReviewStarted, Correlation: InteractionCorrelation{ID: "permission:::call-1", ToolCallID: "call-1"}, ToolName: "bash", Action: "bash pwd"},
		{Kind: InteractionPermissionReviewRetry, Correlation: InteractionCorrelation{ID: "permission:::call-1", ToolCallID: "call-1"}, ToolName: "bash", Action: "bash pwd", Attempt: 2},
		{Kind: InteractionPermissionAutoApproved, Correlation: InteractionCorrelation{ID: "permission:::call-1", ToolCallID: "call-1"}, ToolName: "bash", Action: "bash pwd"},
		{Kind: InteractionPermissionEscalated, Correlation: InteractionCorrelation{ID: "permission:::call-1", ToolCallID: "call-1"}, ToolName: "bash", Action: "bash pwd"},
		{Kind: InteractionPermissionStateChanged},
	}
	if !reflect.DeepEqual(sink.notices, want) {
		t.Fatalf("interaction notices = %#v, want %#v", sink.notices, want)
	}
}

func TestLegacyInteractiveApplicationMapsPermissionStateAndCommands(t *testing.T) {
	state := permission.NewState(permission.ModeApprove, false)
	state.AddGrant(permission.Grant{Key: "grant-1"})
	runner := &AgentRunner{permissionCoordinator: permission.NewCoordinator(permission.Config{State: state})}
	application := NewLegacyInteractiveApplication(runner)
	if got := application.PermissionState(); got.SelectedMode != PermissionModeApprove || got.SessionGrantCount != 1 {
		t.Fatalf("permission state = %#v", got)
	}
	got := application.UpdatePermissionMode(context.Background(), PermissionModeCommand{Mode: PermissionModeAsk})
	if got.SelectedMode != PermissionModeAsk || got.EffectiveMode != PermissionModeAsk {
		t.Fatalf("updated state = %#v", got)
	}
	got = application.ActivateFullAccess(context.Background(), FullAccessCommand{Remember: true})
	if got.EffectiveMode != PermissionModeFullAccess || !got.FullAccessRemembered {
		t.Fatalf("full-access state = %#v", got)
	}
	cleared := application.ClearPermissionGrants(context.Background())
	if cleared.Cleared != 1 || cleared.State.SessionGrantCount != 0 {
		t.Fatalf("clear outcome = %#v", cleared)
	}
}
