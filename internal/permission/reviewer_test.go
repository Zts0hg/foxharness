package permission

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
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

func TestProviderReviewerParsesFencedJSONResponse(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{content: "```json\n{\"decision\":\"approve\",\"risk_level\":\"low\",\"user_authorization\":\"medium\",\"rationale\":\"read-only and scoped\"}\n```"},
		},
	}
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
	}

	result, err := reviewer.Review(context.Background(), reviewRequest(), Evidence{Text: "trusted context"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewApprove {
		t.Fatalf("decision = %q, want approve", result.Decision)
	}
	if reviewProvider.calls != 1 {
		t.Fatalf("review calls = %d, want 1", reviewProvider.calls)
	}
}

func TestProviderReviewerRequestsStructuredOutput(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{content: `{"decision":"approve","risk_level":"low","user_authorization":"medium","rationale":"read-only and scoped"}`},
		},
	}
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
	}

	result, err := reviewer.Review(context.Background(), reviewRequest(), Evidence{Text: "trusted context"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewApprove {
		t.Fatalf("decision = %q, want approve", result.Decision)
	}
	if reviewProvider.generateCalls != 0 {
		t.Fatalf("Generate calls = %d, want 0 when structured output options are supported", reviewProvider.generateCalls)
	}
	if reviewProvider.optionCalls != 1 {
		t.Fatalf("GenerateWithOptions calls = %d, want 1", reviewProvider.optionCalls)
	}
	opts := reviewProvider.options
	if opts.StructuredOutput == nil {
		t.Fatal("StructuredOutput = nil, want permission review schema")
	}
	if opts.StructuredOutput.Name != "permission_review" {
		t.Fatalf("StructuredOutput.Name = %q, want permission_review", opts.StructuredOutput.Name)
	}
	if opts.StructuredOutput.Schema["type"] != "object" {
		t.Fatalf("StructuredOutput.Schema = %#v, want object schema", opts.StructuredOutput.Schema)
	}
	if !opts.StructuredOutput.Strict {
		t.Fatal("StructuredOutput.Strict = false, want true")
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

func TestProviderReviewerCanApproveHighRiskReadOnlyFileRequest(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{content: `{"decision":"approve","risk_level":"low","user_authorization":"high","rationale":"user explicitly requested reading this external project document"}`},
		},
	}
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
	}
	request := reviewRequest()
	request.ToolName = "read_file"
	request.Action = "read_file /Users/xiaoming/code/japanese-word-game/IMPROVEMENT_ANALYSIS.md"
	request.Risk = RiskHigh

	result, err := reviewer.Review(context.Background(), request, Evidence{Text: "User asked to inspect /Users/xiaoming/code/japanese-word-game for old project notes."})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewApprove {
		t.Fatalf("decision = %q, want approve for read-only external file request", result.Decision)
	}
}

func TestProviderReviewerCanApproveHighRiskReadOnlyDelegateTask(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{content: `{"decision":"approve","risk_level":"low","user_authorization":"high","rationale":"read-only subagent directly matches the user's requested project search"}`},
		},
	}
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
	}
	request := reviewRequest()
	request.ToolName = "delegate_task"
	request.Action = `delegate_task {"read_only":true,"task":"search old project docs"}`
	request.Arguments = `{"read_only":true,"task":"search old project docs"}`
	request.Risk = RiskHigh

	result, err := reviewer.Review(context.Background(), request, Evidence{Text: "User asked to search the old project directory for documentation."})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewApprove {
		t.Fatalf("decision = %q, want approve for read-only delegate task", result.Decision)
	}
}

func TestProviderReviewerEscalatesHighRiskWritableDelegateTask(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{content: `{"decision":"approve","risk_level":"low","user_authorization":"high","rationale":"claimed authorized"}`},
		},
	}
	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
	}
	request := reviewRequest()
	request.ToolName = "delegate_task"
	request.Action = `delegate_task {"task":"change files"}`
	request.Arguments = `{"task":"change files"}`
	request.Risk = RiskHigh

	result, err := reviewer.Review(context.Background(), request, Evidence{Text: "User asked to inspect docs only."})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Decision != ReviewEscalate {
		t.Fatalf("decision = %q, want escalation for writable delegate task", result.Decision)
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

func TestProviderReviewerLogsFailureDetails(t *testing.T) {
	reviewProvider := &scriptedReviewProvider{
		responses: []reviewProviderResponse{
			{err: errors.New("network refused")},
			{content: `not json`},
			{nilMessage: true},
		},
	}
	var logs bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})

	reviewer := &ProviderReviewer{
		Lookup: func() provider.LLMProvider { return reviewProvider },
	}
	_, err := reviewer.Review(context.Background(), reviewRequest(), Evidence{Text: "trusted context"})
	if err == nil {
		t.Fatal("Review() error = nil, want failure")
	}
	got := logs.String()
	for _, want := range []string{
		"permission auto-review attempt 1/3 failed",
		"generate: network refused",
		"permission auto-review attempt 2/3 failed",
		"parse:",
		`response="not json"`,
		"permission auto-review unavailable after 3 attempts",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("logs missing %q:\n%s", want, got)
		}
	}
}

type reviewProviderResponse struct {
	content    string
	nilMessage bool
	err        error
}

type scriptedReviewProvider struct {
	responses     []reviewProviderResponse
	calls         int
	generateCalls int
	optionCalls   int
	options       provider.GenerateOptions
}

func (p *scriptedReviewProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.generateCalls++
	return p.nextResponse(availableTools)
}

func (p *scriptedReviewProvider) GenerateWithOptions(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.optionCalls++
	p.options = options
	return p.nextResponse(availableTools)
}

func (p *scriptedReviewProvider) nextResponse(availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
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
