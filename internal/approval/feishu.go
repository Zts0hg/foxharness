package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/Zts0hg/foxharness/internal/middleware"
)

type Messenger interface {
	SendText(ctx context.Context, chatID, text string) error
}

type FeishuApprover struct {
	chatID    string
	messenger Messenger
	store     *Store
}

func NewFeishuApprover(chatID string, messenger Messenger, store *Store) *FeishuApprover {
	return &FeishuApprover{chatID: chatID, messenger: messenger, store: store}
}

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
