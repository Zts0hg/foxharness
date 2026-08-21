/* Package interactionruntime adapts application interaction ports to concrete runtime capabilities. */
package interactionruntime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/tools"
)

/* NewQuestionAsker maps a correlated application question port to the tool capability. */
func NewQuestionAsker(port app.QuestionPort) tools.UserAsker {
	if isNil(port) {
		return nil
	}
	return &questionAsker{port: port}
}

type questionAsker struct{ port app.QuestionPort }

func (a *questionAsker) Ask(ctx context.Context, questions []tools.Question) ([]tools.Answer, error) {
	correlation := correlation(ctx, "question", "")
	request := app.QuestionRequest{Correlation: correlation, Questions: make([]app.Question, len(questions))}
	for index, question := range questions {
		options := make([]app.QuestionOption, len(question.Options))
		for optionIndex, option := range question.Options {
			options[optionIndex] = app.QuestionOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
		}
		request.Questions[index] = app.Question{
			ID: fmt.Sprintf("%s:question:%d", correlation.ID, index+1), Header: question.Header,
			Prompt: question.Prompt, Options: options, MultiSelect: question.MultiSelect,
		}
	}
	response, err := a.port.AskQuestions(ctx, request)
	if err != nil {
		if errors.Is(err, app.ErrQuestionCancelled) {
			return nil, tools.ErrUserCancelled
		}
		return nil, err
	}
	if err := validateCorrelation(correlation.ID, response.CorrelationID); err != nil {
		return nil, err
	}
	answers := make([]tools.Answer, len(response.Answers))
	for index, answer := range response.Answers {
		answers[index] = tools.Answer{
			QuestionText: answer.QuestionText, Value: answer.Value, Preview: answer.Preview, Notes: answer.Notes,
		}
	}
	return answers, nil
}

/* NewPlanReviewer maps a correlated application review port to the Formal Plan tool capability. */
func NewPlanReviewer(port app.PlanReviewPort) tools.PlanReviewer {
	if isNil(port) {
		return nil
	}
	return &planReviewer{port: port}
}

type planReviewer struct{ port app.PlanReviewPort }

func (r *planReviewer) ReviewPlan(ctx context.Context, planMarkdown string) (tools.PlanReview, error) {
	correlation := correlation(ctx, "plan", "")
	response, err := r.port.ReviewPlan(ctx, app.PlanReviewRequest{Correlation: correlation, PlanMarkdown: planMarkdown})
	if err != nil {
		if errors.Is(err, app.ErrPlanReviewCancelled) {
			return tools.PlanReview{}, tools.ErrPlanReviewCancelled
		}
		return tools.PlanReview{}, err
	}
	if err := validateCorrelation(correlation.ID, response.CorrelationID); err != nil {
		return tools.PlanReview{}, err
	}
	return tools.PlanReview{Decision: tools.PlanReviewDecision(response.Decision), Feedback: response.Feedback}, nil
}

/* NewPermissionApprover maps a correlated application permission port to explicit runtime approval. */
func NewPermissionApprover(port app.PermissionPort) permission.UserApprover {
	if isNil(port) {
		return nil
	}
	return &permissionApprover{port: port}
}

type permissionApprover struct{ port app.PermissionPort }

func (a *permissionApprover) Approve(ctx context.Context, approval permission.ApprovalRequest) (permission.UserDecision, error) {
	request := approval.Request
	requestCorrelation := correlation(ctx, "permission", request.ToolCall.ID)
	var effects []string
	if request.Capabilities.Effects != nil {
		effects = make([]string, len(request.Capabilities.Effects))
		for index, effect := range request.Capabilities.Effects {
			effects[index] = string(effect)
		}
	}
	policyReason := ""
	if string(request.Capabilities.Behavior) == "human_only" {
		policyReason = request.Capabilities.Reason
	}
	reviewerReason := ""
	if approval.Review != nil {
		reviewerReason = approval.Review.Rationale
	}
	response, err := a.port.RequestPermission(ctx, app.PermissionRequest{
		Correlation: requestCorrelation, ToolName: request.ToolName, Arguments: request.Arguments,
		Action: request.Action, Risk: string(request.Risk), Source: string(request.Source),
		CWD: request.CWD, Workspace: request.Workspace, Effects: effects,
		PolicyReason: policyReason, ReviewerReason: reviewerReason, ReviewerFailure: approval.ReviewerFailure,
	})
	if err != nil {
		return permission.UserDecision{}, err
	}
	if err := validateCorrelation(requestCorrelation.ID, response.CorrelationID); err != nil {
		return permission.UserDecision{}, err
	}
	return permission.UserDecision{Kind: permission.UserDecisionKind(response.Decision), Feedback: response.Feedback}, nil
}

/* NewPermissionEvents maps runtime review progress to application interaction notices. */
func NewPermissionEvents(sink app.InteractionNoticeSink) *PermissionEvents {
	if isNil(sink) {
		sink = nil
	}
	return &PermissionEvents{sink: sink}
}

/* PermissionEvents forwards non-blocking permission review progress. */
type PermissionEvents struct{ sink app.InteractionNoticeSink }

/* OnReviewStart reports automatic review startup. */
func (s *PermissionEvents) OnReviewStart(request permission.Request) {
	s.notify(request, app.InteractionPermissionReviewStarted, 0)
}

/* OnReviewRetry reports one automatic review retry. */
func (s *PermissionEvents) OnReviewRetry(request permission.Request, attempt int) {
	s.notify(request, app.InteractionPermissionReviewRetry, attempt)
}

/* OnAutoApproved reports successful automatic approval. */
func (s *PermissionEvents) OnAutoApproved(request permission.Request, _ permission.ReviewResult) {
	s.notify(request, app.InteractionPermissionAutoApproved, 0)
}

/* OnEscalated reports fallback to explicit user approval. */
func (s *PermissionEvents) OnEscalated(request permission.Request, _ permission.ReviewResult) {
	s.notify(request, app.InteractionPermissionEscalated, 0)
}

/* OnPermissionStateChanged asks presentation to refresh the permission snapshot. */
func (s *PermissionEvents) OnPermissionStateChanged() {
	if s == nil || s.sink == nil {
		return
	}
	s.sink.NotifyInteraction(context.Background(), app.InteractionNotice{Kind: app.InteractionPermissionStateChanged})
}

func (s *PermissionEvents) notify(request permission.Request, kind app.InteractionNoticeKind, attempt int) {
	if s == nil || s.sink == nil {
		return
	}
	s.sink.NotifyInteraction(context.Background(), app.InteractionNotice{
		Kind: kind, Correlation: correlation(context.Background(), "permission", request.ToolCall.ID),
		ToolName: request.ToolName, Action: request.Action, Attempt: attempt,
	})
}

/* PermissionController exposes one permission state through application-owned DTOs. */
type PermissionController struct{ state *permission.State }

/* NewPermissionController constructs the application permission capability around process state. */
func NewPermissionController(state *permission.State) *PermissionController {
	if state == nil {
		state = permission.NewState(permission.ModeAsk, false)
	}
	return &PermissionController{state: state}
}

/* PermissionState returns the current presentation-safe snapshot. */
func (c *PermissionController) PermissionState() app.PermissionState {
	return mapPermissionState(c.state.Snapshot())
}

/* UpdatePermissionMode changes the selected future-run mode. */
func (c *PermissionController) UpdatePermissionMode(_ context.Context, command app.PermissionModeCommand) app.PermissionState {
	c.state.SetSelected(permission.Mode(command.Mode), command.FullAccessWarningRemembered)
	return c.PermissionState()
}

/* ActivateFullAccess confirms unrestricted operation. */
func (c *PermissionController) ActivateFullAccess(_ context.Context, command app.FullAccessCommand) app.PermissionState {
	c.state.ActivateFullAccess(command.Remember)
	return c.PermissionState()
}

/* ClearPermissionGrants removes all process-local session grants. */
func (c *PermissionController) ClearPermissionGrants(context.Context) app.PermissionGrantClearOutcome {
	cleared := c.state.ClearGrants()
	return app.PermissionGrantClearOutcome{Cleared: cleared, State: c.PermissionState()}
}

func mapPermissionState(snapshot permission.Snapshot) app.PermissionState {
	return app.PermissionState{
		SelectedMode: app.PermissionMode(snapshot.SelectedMode), EffectiveMode: app.PermissionMode(snapshot.EffectiveMode),
		FullAccessRemembered: snapshot.FullAccessRemembered, FullAccessNeedsWarning: snapshot.FullAccessNeedsWarning,
		SessionGrantCount: snapshot.SessionGrantCount,
	}
}

var interactionSequence uint64

func correlation(ctx context.Context, kind string, fallbackToolCallID string) app.InteractionCorrelation {
	invocation, _ := tools.InvocationContextFrom(ctx)
	toolCallID := invocation.ToolCallID
	if toolCallID == "" {
		toolCallID = fallbackToolCallID
	}
	id := strings.Join([]string{kind, invocation.SessionID, invocation.RunID, toolCallID}, ":")
	if invocation.SessionID == "" && invocation.RunID == "" && toolCallID == "" {
		id = fmt.Sprintf("%s:%d", kind, atomic.AddUint64(&interactionSequence, 1))
	}
	return app.InteractionCorrelation{ID: id, SessionID: invocation.SessionID, RunID: invocation.RunID, ToolCallID: toolCallID}
}

func validateCorrelation(requestID string, responseID string) error {
	if responseID != requestID {
		return fmt.Errorf("interaction response correlation %q does not match request %q", responseID, requestID)
	}
	return nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var _ permission.EventSink = (*PermissionEvents)(nil)
var _ permission.StateChangeSink = (*PermissionEvents)(nil)
var _ app.InteractivePermissionController = (*PermissionController)(nil)
