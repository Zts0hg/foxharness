package approval

import (
	"context"
	"fmt"

	"github.com/Zts0hg/foxharness/internal/permission"
)

// PermissionApprover adapts the remote approval store and messenger to the
// unified permission coordinator's UserApprover interface.
type PermissionApprover struct {
	chatID    string
	messenger Messenger
	store     *Store
}

// NewPermissionApprover creates a permission.UserApprover backed by the
// existing remote approval transport.
func NewPermissionApprover(chatID string, messenger Messenger, store *Store) *PermissionApprover {
	return &PermissionApprover{chatID: chatID, messenger: messenger, store: store}
}

// Approve sends a permission approval prompt and maps the human response to a
// permission.UserDecision.
func (a *PermissionApprover) Approve(ctx context.Context, req permission.ApprovalRequest) (permission.UserDecision, error) {
	if a == nil || a.messenger == nil || a.store == nil {
		return permission.UserDecision{}, fmt.Errorf("remote permission approver is not configured")
	}
	approvalReq := Request{
		ID:        newApprovalID(),
		ToolName:  req.Request.ToolName,
		Arguments: req.Request.Arguments,
		Risk:      string(req.Request.Risk),
	}
	if req.Request.Action != "" {
		approvalReq.Risk = fmt.Sprintf("%s\nAction: %s", approvalReq.Risk, req.Request.Action)
	}
	if req.ReviewerFailure != "" {
		approvalReq.Risk = fmt.Sprintf("%s\nReviewer failure: %s", approvalReq.Risk, req.ReviewerFailure)
	}
	if req.Review != nil && req.Review.Rationale != "" {
		approvalReq.Risk = fmt.Sprintf("%s\nReviewer rationale: %s", approvalReq.Risk, req.Review.Rationale)
	}

	result, err := a.store.Wait(ctx, approvalReq, func(r Request) error {
		text := "工具调用等待统一权限审批\n\n" +
			"Tool: " + r.ToolName + "\n" +
			"Risk: " + r.Risk + "\n\n" +
			"Arguments:\n" + r.Arguments + "\n\n" +
			"ApprovalID: " + r.ID

		return a.messenger.SendText(ctx, a.chatID, text)
	})
	if err != nil {
		return permission.UserDecision{}, err
	}
	if result.Approved {
		return permission.UserDecision{Kind: permission.UserAllowOnce}, nil
	}
	if result.Reason != "" {
		return permission.UserDecision{Kind: permission.UserDenyFeedback, Feedback: result.Reason}, nil
	}
	return permission.UserDecision{Kind: permission.UserDeny}, nil
}

var _ permission.UserApprover = (*PermissionApprover)(nil)
