package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

type permissionPortFunc func(context.Context, PermissionRequest) (PermissionResponse, error)

func (f permissionPortFunc) RequestPermission(ctx context.Context, request PermissionRequest) (PermissionResponse, error) {
	return f(ctx, request)
}

type questionPortFunc func(context.Context, QuestionRequest) (QuestionResponse, error)

func (f questionPortFunc) AskQuestions(ctx context.Context, request QuestionRequest) (QuestionResponse, error) {
	return f(ctx, request)
}

type planReviewPortFunc func(context.Context, PlanReviewRequest) (PlanReviewResponse, error)

func (f planReviewPortFunc) ReviewPlan(ctx context.Context, request PlanReviewRequest) (PlanReviewResponse, error) {
	return f(ctx, request)
}

func TestApplicationInteractionPortsCarryCorrelationAndContextLifetime(t *testing.T) {
	var _ PermissionPort = permissionPortFunc(nil)
	var _ QuestionPort = questionPortFunc(nil)
	var _ PlanReviewPort = planReviewPortFunc(nil)

	correlation := InteractionCorrelation{
		ID: "interaction-1", SessionID: "session-1", RunID: "run-1", ToolCallID: "call-1",
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()

	port := questionPortFunc(func(ctx context.Context, request QuestionRequest) (QuestionResponse, error) {
		if request.Correlation != correlation {
			t.Fatalf("correlation = %#v, want %#v", request.Correlation, correlation)
		}
		if len(request.Questions) != 1 || request.Questions[0].ID != "question-1" {
			t.Fatalf("questions = %#v, want stable question identity", request.Questions)
		}
		return QuestionResponse{CorrelationID: request.Correlation.ID}, ctx.Err()
	})
	response, err := port.AskQuestions(ctx, QuestionRequest{
		Correlation: correlation,
		Questions: []Question{{
			ID: "question-1", Header: "Scope", Prompt: "Select scope", MultiSelect: false,
			Options: []QuestionOption{{Label: "Current", Description: "Use current module"}},
		}},
	})
	if !errors.Is(err, context.DeadlineExceeded) || response.CorrelationID != correlation.ID {
		t.Fatalf("response/error = %#v/%v", response, err)
	}
}

func TestApplicationInteractionDecisionValuesRemainPresentationNeutral(t *testing.T) {
	if PermissionAllowOnce != "allow_once" || PermissionAllowSession != "allow_session" ||
		PermissionDeny != "deny" || PermissionDenyWithFeedback != "deny_feedback" {
		t.Fatal("permission decision wire values changed")
	}
	if PlanApproved != "approved" || PlanContinuePlanning != "continue_planning" {
		t.Fatal("plan decision wire values changed")
	}
}
