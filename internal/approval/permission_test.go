package approval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type recordingMessenger struct {
	texts chan string
}

func newRecordingMessenger() *recordingMessenger {
	return &recordingMessenger{texts: make(chan string, 1)}
}

func (m *recordingMessenger) SendText(ctx context.Context, chatID, text string) error {
	m.texts <- text
	return nil
}

func TestPermissionApproverMapsApprovalToAllowOnce(t *testing.T) {
	store := NewStore()
	messenger := newRecordingMessenger()
	approver := NewPermissionApprover("chat-1", messenger, store)
	done := make(chan permission.UserDecision, 1)

	go func() {
		decision, err := approver.Approve(context.Background(), permissionApprovalRequest())
		if err != nil {
			t.Errorf("Approve() error = %v", err)
			return
		}
		done <- decision
	}()

	id := receiveApprovalID(t, messenger)
	if err := store.Resolve(id, Result{Approved: true, Reason: "ok"}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	decision := receiveDecision(t, done)
	if decision.Kind != permission.UserAllowOnce {
		t.Fatalf("decision = %+v, want allow once", decision)
	}
}

func TestPermissionApproverMapsDenialReasonToFeedback(t *testing.T) {
	store := NewStore()
	messenger := newRecordingMessenger()
	approver := NewPermissionApprover("chat-1", messenger, store)
	done := make(chan permission.UserDecision, 1)

	go func() {
		decision, err := approver.Approve(context.Background(), permissionApprovalRequest())
		if err != nil {
			t.Errorf("Approve() error = %v", err)
			return
		}
		done <- decision
	}()

	id := receiveApprovalID(t, messenger)
	if err := store.Resolve(id, Result{Approved: false, Reason: "too broad"}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	decision := receiveDecision(t, done)
	if decision.Kind != permission.UserDenyFeedback || decision.Feedback != "too broad" {
		t.Fatalf("decision = %+v, want denial feedback", decision)
	}
}

func permissionApprovalRequest() permission.ApprovalRequest {
	args, _ := json.Marshal(map[string]string{"command": "go test ./..."})
	return permission.ApprovalRequest{
		Request: permission.Request{
			ToolCall:  schema.ToolCall{ID: "call-1", Name: "bash", Arguments: args},
			ToolName:  "bash",
			Arguments: string(args),
			Action:    "bash go test ./...",
			Risk:      permission.RiskMedium,
		},
	}
}

func receiveApprovalID(t *testing.T, messenger *recordingMessenger) string {
	t.Helper()
	select {
	case text := <-messenger.texts:
		const marker = "ApprovalID: "
		idx := strings.LastIndex(text, marker)
		if idx < 0 {
			t.Fatalf("approval text missing ID:\n%s", text)
		}
		return strings.TrimSpace(text[idx+len(marker):])
	case <-time.After(time.Second):
		t.Fatal("approval prompt was not sent")
	}
	return ""
}

func receiveDecision(t *testing.T, done <-chan permission.UserDecision) permission.UserDecision {
	t.Helper()
	select {
	case decision := <-done:
		return decision
	case <-time.After(time.Second):
		t.Fatal("Approve did not return")
	}
	return permission.UserDecision{}
}
