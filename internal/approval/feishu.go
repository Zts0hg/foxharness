package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/Zts0hg/foxharness/internal/middleware"
)

// Messenger is the minimal interface required by FeishuApprover to send
// approval-request notifications to a chat.
type Messenger interface {
	SendText(ctx context.Context, chatID, text string) error
}

// FeishuApprover implements the middleware.Approver interface by sending
// approval-request messages to a Feishu chat and delegating the blocking wait
// to an approval.Store.
type FeishuApprover struct {
	chatID    string
	messenger Messenger
	store     *Store
}

// NewFeishuApprover creates a FeishuApprover that sends approval prompts to
// the specified chatID via messenger and tracks pending approvals in store.
func NewFeishuApprover(chatID string, messenger Messenger, store *Store) *FeishuApprover {
	return &FeishuApprover{chatID: chatID, messenger: messenger, store: store}
}

// Approve sends a human-readable approval prompt to the Feishu chat and
// blocks until the operator responds, the request times out, or ctx is
// cancelled.  It returns the approval decision, the operator's reason, and
// any error encountered.
func (a *FeishuApprover) Approve(ctx context.Context, req middleware.ApprovalRequest) (bool, string, error) {
	approvalReq := Request{
		ID:        newApprovalID(),
		ToolName:  req.ToolName,
		Arguments: req.Arguments,
		Risk:      req.Risk,
	}

	result, err := a.store.Wait(ctx, approvalReq, func(r Request) error {
		text := "高危工具调用等待审批\n\n" +
			"Tool: " + r.ToolName + "\n" +
			"Risk: " + r.Risk + "\n\n" +
			"Arguments:\n" + r.Arguments + "\n\n" +
			"ApprovalID: " + r.ID

		return a.messenger.SendText(ctx, a.chatID, text)
	})

	if err != nil {
		return false, "", err
	}

	return result.Approved, result.Reason, nil

}

func newApprovalID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
