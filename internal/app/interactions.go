package app

import "context"

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
