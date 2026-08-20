package feishu

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/approval"
)

/* PermissionPort adapts application approval requests to Feishu's callback-backed transport. */
type PermissionPort struct {
	chatID    string
	messenger TextMessenger
	store     *approval.Store
}

/* NewPermissionPort binds one task chat to the shared remote approval store. */
func NewPermissionPort(chatID string, messenger TextMessenger, store *approval.Store) *PermissionPort {
	return &PermissionPort{chatID: chatID, messenger: messenger, store: store}
}

/* RequestPermission sends one approval prompt and waits for its exactly-once terminal decision. */
func (p *PermissionPort) RequestPermission(ctx context.Context, request app.PermissionRequest) (app.PermissionResponse, error) {
	if p == nil || isNilDependency(p.messenger) || p.store == nil {
		return app.PermissionResponse{}, errors.New("remote permission approver is not configured")
	}
	pending := approval.Request{
		ID: newPermissionApprovalID(), ToolName: request.ToolName,
		Arguments: request.Arguments, Risk: permissionRiskText(request),
	}
	result, err := p.store.Wait(ctx, pending, func(value approval.Request) error {
		text := "工具调用等待统一权限审批\n\n" +
			"Tool: " + value.ToolName + "\n" +
			"Risk: " + value.Risk + "\n\n" +
			"Arguments:\n" + value.Arguments + "\n\n" +
			"ApprovalID: " + value.ID
		return p.messenger.SendText(ctx, p.chatID, text)
	})
	if err != nil {
		return app.PermissionResponse{}, err
	}
	response := app.PermissionResponse{CorrelationID: request.Correlation.ID}
	switch {
	case result.Approved:
		response.Decision = app.PermissionAllowOnce
	case result.Reason != "":
		response.Decision = app.PermissionDenyWithFeedback
		response.Feedback = result.Reason
	default:
		response.Decision = app.PermissionDeny
	}
	return response, nil
}

func permissionRiskText(request app.PermissionRequest) string {
	risk := request.Risk
	if request.Action != "" {
		risk = fmt.Sprintf("%s\nAction: %s", risk, request.Action)
	}
	if request.ReviewerFailure != "" {
		risk = fmt.Sprintf("%s\nReviewer failure: %s", risk, request.ReviewerFailure)
	}
	if request.ReviewerReason != "" {
		risk = fmt.Sprintf("%s\nReviewer rationale: %s", risk, request.ReviewerReason)
	}
	return risk
}

func newPermissionApprovalID() string {
	var value [8]byte
	_, _ = rand.Read(value[:])
	return hex.EncodeToString(value[:])
}

var _ app.PermissionPort = (*PermissionPort)(nil)
