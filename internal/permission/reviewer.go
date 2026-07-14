package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

const reviewerAttempts = 3

// Reviewer evaluates whether an exact request may be auto-approved.
type Reviewer interface {
	Review(ctx context.Context, request Request, evidence Evidence) (ReviewResult, error)
}

// ProviderLookup returns the currently active model provider.
type ProviderLookup func() provider.LLMProvider

// ReviewResult is the strict reviewer outcome.
type ReviewResult struct {
	Decision          ReviewDecision    `json:"decision"`
	Risk              Risk              `json:"risk_level"`
	UserAuthorization UserAuthorization `json:"user_authorization"`
	Rationale         string            `json:"rationale"`
}

// ReviewDecision is the reviewer authority surface.
type ReviewDecision string

const (
	ReviewApprove  ReviewDecision = "approve"
	ReviewEscalate ReviewDecision = "escalate"
)

// UserAuthorization is the reviewer estimate of explicit user authorization.
type UserAuthorization string

const (
	AuthorizationHigh    UserAuthorization = "high"
	AuthorizationMedium  UserAuthorization = "medium"
	AuthorizationLow     UserAuthorization = "low"
	AuthorizationUnknown UserAuthorization = "unknown"
)

// ProviderReviewer runs isolated, tool-free approval review through the active provider.
type ProviderReviewer struct {
	Lookup  ProviderLookup
	Timeout time.Duration
	OnRetry func(request Request, attempt int)
}

// NewProviderReviewer creates an active-provider reviewer.
func NewProviderReviewer(lookup ProviderLookup) *ProviderReviewer {
	return &ProviderReviewer{Lookup: lookup, Timeout: 90 * time.Second}
}

// Review applies bounded retry to technical failures and fail-closed fallback.
func (r *ProviderReviewer) Review(ctx context.Context, request Request, evidence Evidence) (ReviewResult, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < reviewerAttempts; attempt++ {
		if attempt > 0 && r.OnRetry != nil {
			r.OnRetry(request, attempt+1)
		}
		p := r.Lookup()
		if p == nil {
			lastErr = fmt.Errorf("review provider unavailable")
			continue
		}
		resp, err := p.Generate(ctx, reviewerMessages(request, evidence), nil)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return ReviewResult{}, ctx.Err()
			}
			continue
		}
		result, err := parseReviewResult(resp)
		if err != nil {
			lastErr = err
			continue
		}
		if result.Decision == ReviewApprove && reviewerMayApprove(request.Risk) && reviewerMayApprove(result.Risk) && reviewerAuthorizationSufficient(result.UserAuthorization) {
			return result, nil
		}
		result.Decision = ReviewEscalate
		return result, nil
	}
	return ReviewResult{Decision: ReviewEscalate, Risk: request.Risk, UserAuthorization: AuthorizationUnknown, Rationale: "Auto-review was unavailable after three attempts."}, lastErr
}

func reviewerMessages(request Request, evidence Evidence) []schema.Message {
	system := `You are a tool-call approval reviewer. Return exactly one JSON object with fields decision, risk_level, user_authorization, and rationale. decision must be "approve" or "escalate". risk_level must be "low", "medium", "high", or "critical". user_authorization must be "high", "medium", "low", or "unknown". Approve only the exact invocation when context is sufficient, task-relevant, and narrowly scoped. Escalate critical, suspicious, unclear, unrelated, or insufficiently authorized calls.`
	user := fmt.Sprintf("Request:\nTool: %s\nAction: %s\nCWD: %s\nWorkspace: %s\nRisk: %s\n\nEvidence:\n%s", request.ToolName, request.Action, request.CWD, request.Workspace, request.Risk, evidence.Text)
	return []schema.Message{
		{Role: schema.RoleSystem, Content: system},
		{Role: schema.RoleUser, Content: user},
	}
}

func parseReviewResult(resp *provider.GenerateResponse) (ReviewResult, error) {
	if resp == nil {
		return ReviewResult{}, fmt.Errorf("nil review response")
	}
	if resp.Message == nil {
		return ReviewResult{}, fmt.Errorf("nil review message")
	}
	content := strings.TrimSpace(resp.Message.Content)
	var result ReviewResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ReviewResult{}, err
	}
	if result.Decision != ReviewApprove && result.Decision != ReviewEscalate {
		return ReviewResult{}, fmt.Errorf("invalid review decision %q", result.Decision)
	}
	switch result.Risk {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
	default:
		return ReviewResult{}, fmt.Errorf("invalid review risk %q", result.Risk)
	}
	switch result.UserAuthorization {
	case AuthorizationHigh, AuthorizationMedium, AuthorizationLow, AuthorizationUnknown:
	default:
		return ReviewResult{}, fmt.Errorf("invalid review user authorization %q", result.UserAuthorization)
	}
	if strings.TrimSpace(result.Rationale) == "" {
		return ReviewResult{}, fmt.Errorf("missing review rationale")
	}
	return result, nil
}

func reviewerMayApprove(risk Risk) bool {
	return risk == RiskLow || risk == RiskMedium
}

func reviewerAuthorizationSufficient(authorization UserAuthorization) bool {
	return authorization == AuthorizationHigh || authorization == AuthorizationMedium
}
