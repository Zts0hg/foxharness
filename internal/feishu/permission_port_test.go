package feishu

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/approval"
)

func TestPermissionPortPreservesRemoteApprovalTextDecisionAndCorrelation(t *testing.T) {
	store := approval.NewStore()
	var sentChat string
	var sentText string
	messenger := textMessengerFunc(func(_ context.Context, chatID, text string) error {
		sentChat, sentText = chatID, text
		approvalID := strings.TrimSpace(text[strings.LastIndex(text, "ApprovalID: ")+len("ApprovalID: "):])
		if err := store.Resolve(approvalID, approval.Result{Approved: false, Reason: "narrow the target"}); err != nil {
			t.Fatalf("resolve pending approval: %v", err)
		}
		return nil
	})
	port := NewPermissionPort("chat-1", messenger, store)
	request := app.PermissionRequest{
		Correlation: app.InteractionCorrelation{ID: "interaction-1", SessionID: "session-1", RunID: "run-1", ToolCallID: "call-1"},
		ToolName:    "write_file", Arguments: `{"path":"x","content":"y"}`,
		Risk: "high", Action: "write x", ReviewerFailure: "review unavailable", ReviewerReason: "needs confirmation",
	}

	response, err := port.RequestPermission(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.CorrelationID != "interaction-1" || response.Decision != app.PermissionDenyWithFeedback || response.Feedback != "narrow the target" {
		t.Fatalf("permission response = %#v", response)
	}
	if sentChat != "chat-1" {
		t.Fatalf("approval chat = %q", sentChat)
	}
	for _, fragment := range []string{
		"工具调用等待统一权限审批", "Tool: write_file", "Risk: high", "Action: write x",
		"Reviewer failure: review unavailable", "Reviewer rationale: needs confirmation",
		"Arguments:\n{\"path\":\"x\",\"content\":\"y\"}", "ApprovalID: ",
	} {
		if !strings.Contains(sentText, fragment) {
			t.Fatalf("approval text missing %q:\n%s", fragment, sentText)
		}
	}
}

type textMessengerFunc func(context.Context, string, string) error

func (f textMessengerFunc) SendText(ctx context.Context, chatID, text string) error {
	return f(ctx, chatID, text)
}

func TestPermissionPortKeepsApprovalIDReachableForLargeArguments(t *testing.T) {
	store := approval.NewStore()
	var sentText string
	messenger := textMessengerFunc(func(_ context.Context, _ string, text string) error {
		sentText = text
		approvalID := strings.TrimSpace(text[strings.LastIndex(text, "ApprovalID: ")+len("ApprovalID: "):])
		if err := store.Resolve(approvalID, approval.Result{Approved: false, Reason: "denied"}); err != nil {
			t.Fatalf("resolve pending approval: %v", err)
		}
		return nil
	})
	port := NewPermissionPort("chat-1", messenger, store)
	request := app.PermissionRequest{
		Correlation: app.InteractionCorrelation{ID: "interaction-large", SessionID: "session-1", RunID: "run-1", ToolCallID: "call-1"},
		ToolName:    "write_file",
		Arguments:   strings.Repeat("x", 4000),
		Risk:        "high",
	}

	if _, err := port.RequestPermission(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	sentRunes := len([]rune(sentText))
	if sentRunes > maxFeishuTextRunes {
		t.Fatalf("approval text = %d runes, exceeds the messenger budget %d", sentRunes, maxFeishuTextRunes)
	}
	approvalSuffix := sentText[strings.LastIndex(sentText, "ApprovalID: ")+len("ApprovalID: "):]
	if matched, _ := regexp.MatchString(`^[0-9a-f]{16}$`, approvalSuffix); !matched {
		t.Fatalf("approval text does not end with a complete callback identifier (got %q):\n...%s", approvalSuffix, sentText[max(0, len(sentText)-200):])
	}
}
