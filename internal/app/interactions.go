package app

import (
	"context"
	"errors"
)

/* ErrQuestionCancelled identifies a user-dismissed question interaction. */
var ErrQuestionCancelled = errors.New("user cancelled the question prompt")

/* ErrPlanReviewCancelled identifies a user-dismissed Formal Plan review. */
var ErrPlanReviewCancelled = errors.New("user cancelled plan review")

/* PermissionMode identifies the selected interactive approval policy. */
type PermissionMode string

const (
	/* PermissionModeAsk requires an explicit user decision for reviewed calls. */
	PermissionModeAsk PermissionMode = "ask"
	/* PermissionModeApprove permits automatic review before user escalation. */
	PermissionModeApprove PermissionMode = "approve"
	/* PermissionModeFullAccess runs calls without interactive approval. */
	PermissionModeFullAccess PermissionMode = "full_access"
)

/* PermissionState is the presentation-safe snapshot of interactive permission state. */
type PermissionState struct {
	SelectedMode           PermissionMode
	EffectiveMode          PermissionMode
	FullAccessRemembered   bool
	FullAccessNeedsWarning bool
	SessionGrantCount      int
}

/* PermissionModeCommand selects the future interactive permission policy. */
type PermissionModeCommand struct {
	Mode                        PermissionMode
	FullAccessWarningRemembered bool
}

/* FullAccessCommand confirms activation after the presentation warning. */
type FullAccessCommand struct {
	Remember bool
}

/* PermissionGrantClearOutcome reports the removed grants and resulting state. */
type PermissionGrantClearOutcome struct {
	Cleared int
	State   PermissionState
}

/* InteractivePermissionController exposes typed permission state and mutations. */
type InteractivePermissionController interface {
	PermissionState() PermissionState
	UpdatePermissionMode(context.Context, PermissionModeCommand) PermissionState
	ActivateFullAccess(context.Context, FullAccessCommand) PermissionState
	ClearPermissionGrants(context.Context) PermissionGrantClearOutcome
}

/* InteractionNoticeKind identifies non-blocking presentation progress around an interaction. */
type InteractionNoticeKind string

const (
	/* InteractionPermissionReviewStarted marks automatic review startup. */
	InteractionPermissionReviewStarted InteractionNoticeKind = "permission_review_started"
	/* InteractionPermissionReviewRetry marks one automatic-review retry. */
	InteractionPermissionReviewRetry InteractionNoticeKind = "permission_review_retry"
	/* InteractionPermissionAutoApproved records an automatic approval. */
	InteractionPermissionAutoApproved InteractionNoticeKind = "permission_auto_approved"
	/* InteractionPermissionEscalated marks fallback to explicit user approval. */
	InteractionPermissionEscalated InteractionNoticeKind = "permission_escalated"
	/* InteractionPermissionStateChanged requests a visible permission-state refresh. */
	InteractionPermissionStateChanged InteractionNoticeKind = "permission_state_changed"
)

/* InteractionNotice is one application-owned non-blocking interaction observation. */
type InteractionNotice struct {
	Kind        InteractionNoticeKind
	Correlation InteractionCorrelation
	ToolName    string
	Action      string
	Attempt     int
}

/* InteractionNoticeSink consumes non-blocking interaction progress separately from request/response ports. */
type InteractionNoticeSink interface {
	NotifyInteraction(context.Context, InteractionNotice)
}

/* InteractionCorrelation identifies one blocking request across runtime and presentation. */
type InteractionCorrelation struct {
	ID         string
	SessionID  string
	RunID      string
	ToolCallID string
}

/* PermissionDecision identifies one explicit user authorization response. */
type PermissionDecision string

const (
	/* PermissionAllowOnce permits only the correlated invocation. */
	PermissionAllowOnce PermissionDecision = "allow_once"
	/* PermissionAllowSession permits the current equivalent session scope. */
	PermissionAllowSession PermissionDecision = "allow_session"
	/* PermissionDeny rejects the correlated invocation without feedback. */
	PermissionDeny PermissionDecision = "deny"
	/* PermissionDenyWithFeedback rejects the invocation with user feedback. */
	PermissionDenyWithFeedback PermissionDecision = "deny_feedback"
)

/* PermissionRequest is the application-owned user approval projection. */
type PermissionRequest struct {
	Correlation     InteractionCorrelation
	ToolName        string
	Arguments       string
	Action          string
	Risk            string
	Source          string
	CWD             string
	Workspace       string
	Effects         []string
	PolicyReason    string
	ReviewerReason  string
	ReviewerFailure string
}

/* PermissionResponse is one correlated explicit user approval decision. */
type PermissionResponse struct {
	CorrelationID string
	Decision      PermissionDecision
	Feedback      string
}

/* PermissionPort blocks on one approval until response, cancellation, or deadline. */
type PermissionPort interface {
	RequestPermission(context.Context, PermissionRequest) (PermissionResponse, error)
}

/* QuestionOption is one presentation-neutral selectable answer. */
type QuestionOption struct {
	Label       string
	Description string
	Preview     string
}

/* Question is one presentation-neutral user question. */
type Question struct {
	ID          string
	Header      string
	Prompt      string
	Options     []QuestionOption
	MultiSelect bool
}

/* QuestionAnswer is one response correlated to its exact question text. */
type QuestionAnswer struct {
	QuestionID   string
	QuestionText string
	Value        string
	Preview      string
	Notes        string
}

/* QuestionRequest groups questions under one blocking interaction identity. */
type QuestionRequest struct {
	Correlation InteractionCorrelation
	Questions   []Question
}

/* QuestionResponse returns ordered answers for one correlated request. */
type QuestionResponse struct {
	CorrelationID string
	Answers       []QuestionAnswer
}

/* QuestionPort blocks on user answers until response, cancellation, or deadline. */
type QuestionPort interface {
	AskQuestions(context.Context, QuestionRequest) (QuestionResponse, error)
}

/* PlanReviewDecision identifies one explicit Formal Plan response. */
type PlanReviewDecision string

const (
	/* PlanApproved permits execution to continue with the submitted plan. */
	PlanApproved PlanReviewDecision = "approved"
	/* PlanContinuePlanning requests revision while retaining planning mode. */
	PlanContinuePlanning PlanReviewDecision = "continue_planning"
)

/* PlanReviewRequest presents one complete correlated Formal Plan proposal. */
type PlanReviewRequest struct {
	Correlation  InteractionCorrelation
	PlanMarkdown string
}

/* PlanReviewResponse returns one correlated plan decision and optional feedback. */
type PlanReviewResponse struct {
	CorrelationID string
	Decision      PlanReviewDecision
	Feedback      string
}

/* PlanReviewPort blocks on plan review until response, cancellation, or deadline. */
type PlanReviewPort interface {
	ReviewPlan(context.Context, PlanReviewRequest) (PlanReviewResponse, error)
}
