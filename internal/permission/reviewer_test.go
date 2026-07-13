package permission

import (
	"context"
	"errors"
	"testing"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestProviderReviewerReportsRetriesAfterFirstAttempt(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{err: errors.New("temporary network error")},
			{content: `{"decision":"approve","risk_level":"low","user_authorization":"medium","rationale":"read-only and scoped"}`},
		},
	}
	var attempts []int
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
		OnRetry: func(request Request, attempt int) {
			attempts = append(attempts, attempt)
		},
	}

	result, err := reviewer.Review(context.Background(), reviewRequest(), Evidence{Text: "trusted context"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewApprove {
		t.Fatalf("decision = %q, want approve", result.Decision)
	}
	if len(attempts) != 1 || attempts[0] != 2 {
		t.Fatalf("retry attempts = %#v, want [2]", attempts)
	}
}

func TestProviderReviewerDoesNotReportRetryForValidEscalation(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{content: `{"decision":"escalate","risk_level":"high","user_authorization":"low","rationale":"requires user authorization"}`},
		},
	}
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
		OnRetry: func(request Request, attempt int) {
			t.Fatalf("OnRetry called for valid escalation attempt %d", attempt)
		},
	}

	result, err := reviewer.Review(context.Background(), reviewRequest(), Evidence{Text: "trusted context"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewEscalate {
		t.Fatalf("decision = %q, want escalate", result.Decision)
	}
}

func TestProviderReviewerRequiresStructuredAuthorizationField(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{content: `{"decision":"approve","risk_level":"low","rationale":"missing authorization"}`},
			{content: `{"decision":"approve","risk_level":"low","user_authorization":"medium","rationale":"valid retry"}`},
		},
	}
	var attempts []int
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
		OnRetry: func(request Request, attempt int) {
			attempts = append(attempts, attempt)
		},
	}

	result, err := reviewer.Review(context.Background(), reviewRequest(), Evidence{Text: "trusted context"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewApprove || result.UserAuthorization != AuthorizationMedium {
		t.Fatalf("result = %+v, want approve with medium authorization", result)
	}
	if len(attempts) != 1 || attempts[0] != 2 {
		t.Fatalf("retry attempts = %#v, want [2]", attempts)
	}
}

func TestProviderReviewerCannotDowngradeDeterministicRisk(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{content: `{"decision":"approve","risk_level":"low","user_authorization":"high","rationale":"claimed safe"}`},
		},
	}
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
	}
	request := reviewRequest()
	request.Risk = RiskHigh

	result, err := reviewer.Review(context.Background(), request, Evidence{Text: "trusted context"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewEscalate {
		t.Fatalf("decision = %q, want escalate for high deterministic risk", result.Decision)
	}
}

func TestProviderReviewerEscalatesUnknownAuthorization(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{content: `{"decision":"approve","risk_level":"low","user_authorization":"unknown","rationale":"not enough authorization"}`},
		},
	}
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
	}

	result, err := reviewer.Review(context.Background(), reviewRequest(), Evidence{Text: "trusted context"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewEscalate {
		t.Fatalf("decision = %q, want escalate for unknown authorization", result.Decision)
	}
}

func TestProviderReviewerRetriesNilMessageResponse(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{nilMessage: true},
			{content: `{"decision":"approve","risk_level":"low","user_authorization":"medium","rationale":"valid retry"}`},
		},
	}
	var attempts []int
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
		OnRetry: func(request Request, attempt int) {
			attempts = append(attempts, attempt)
		},
	}

	result, err := reviewer.Review(context.Background(), reviewRequest(), Evidence{Text: "trusted context"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewApprove {
		t.Fatalf("decision = %q, want approve after retry", result.Decision)
	}
	if len(attempts) != 1 || attempts[0] != 2 {
		t.Fatalf("retry attempts = %#v, want [2]", attempts)
	}
}

type reviewProviderResponse struct {
	content    string
	nilMessage bool
	err        error
}

type scriptedReviewProvider struct {
	responses []reviewProviderResponse
	calls     int
}

func (p *scriptedReviewProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	if len(availableTools) != 0 {
		return nil, errors.New("reviewer must not receive tools")
	}
	if p.calls >= len(p.responses) {
		return nil, errors.New("unexpected review call")
	}
	resp := p.responses[p.calls]
	p.calls++
	if resp.err != nil {
		return nil, resp.err
	}
	if resp.nilMessage {
		return &provider.GenerateResponse{}, nil
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: resp.content}}, nil
}

func reviewRequest() Request {
	return Request{
		ToolName:  "bash",
		Action:    "bash git status --short",
		CWD:       "/tmp/work",
		Workspace: "/tmp/work",
		Risk:      RiskLow,
		Source:    SourceMain,
	}
}
