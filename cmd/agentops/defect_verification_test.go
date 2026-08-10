package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/feishu"
)

func TestDVAOP001AgentOpsUsesGatewayDurableAcceptanceAuthority(t *testing.T) {
	home := t.TempDir()
	store, err := newDeliveryStore(home)
	if err != nil {
		t.Fatalf("newDeliveryStore() error = %v", err)
	}
	tasks := make(chan feishu.Task, 2)
	handler := feishu.NewGateway("token", "", tasks, approval.NewStore()).WithDeliveryStore(store).Server(":0").Handler

	first := postAgentOpsEvent(handler, agentOpsMessageEventJSON("first", "message-1"))
	duplicate := postAgentOpsEvent(handler, agentOpsMessageEventJSON("duplicate", "message-1"))
	if first.Code != http.StatusOK || duplicate.Code != http.StatusOK {
		t.Fatalf("delivery statuses = %d, %d; want 200, 200", first.Code, duplicate.Code)
	}
	if got := len(tasks); got != 1 {
		t.Fatalf("enqueued tasks = %d, want one", got)
	}

	restartedStore, err := newDeliveryStore(home)
	if err != nil {
		t.Fatalf("restart newDeliveryStore() error = %v", err)
	}
	restartedTasks := make(chan feishu.Task, 1)
	restarted := feishu.NewGateway("token", "", restartedTasks, approval.NewStore()).WithDeliveryStore(restartedStore).Server(":0").Handler
	restartDuplicate := postAgentOpsEvent(restarted, agentOpsMessageEventJSON("restart", "message-1"))
	if restartDuplicate.Code != http.StatusOK || len(restartedTasks) != 0 {
		t.Fatalf("restart duplicate status/tasks = %d/%d, want 200/0", restartDuplicate.Code, len(restartedTasks))
	}

	source := readAgentOpsMain(t)
	for _, forbidden := range []string{"type Deduper struct", "NewDeduper()", "deduper.Mark("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("production entry retains second acceptance authority %q", forbidden)
		}
	}
	if !strings.Contains(source, "WithDeliveryStore(deliveryStore)") {
		t.Fatal("AgentOps Gateway does not compose the durable DeliveryStore")
	}
}

func TestDVAOP001GatewayRejectsMissingAndEmptyMessageIDsBeforeAgentOps(t *testing.T) {
	tasks := make(chan feishu.Task, 1)
	handler := feishu.NewGateway("token", "", tasks, approval.NewStore()).Server(":0").Handler
	missing := strings.Replace(agentOpsMessageEventJSON("missing", "message-1"), `"message_id":"message-1",`, "", 1)
	empty := agentOpsMessageEventJSON("empty", "")
	missingResponse := postAgentOpsEvent(handler, missing)
	emptyResponse := postAgentOpsEvent(handler, empty)
	if missingResponse.Code != http.StatusOK {
		t.Fatalf("missing message ID status = %d, want acknowledged rejection 200", missingResponse.Code)
	}
	if emptyResponse.Code != http.StatusInternalServerError {
		t.Fatalf("empty message ID status = %d, want visible rejection 500", emptyResponse.Code)
	}
	select {
	case task := <-tasks:
		t.Fatalf("invalid message ID reached AgentOps: %#v", task)
	default:
	}
}

func TestDVAOP002ProductionEntryHasNoCoordinatedShutdown(t *testing.T) {
	source := readAgentOpsMain(t)
	for _, current := range []string{
		"ctx := context.Background()",
		"go runner.Start(ctx, agentTasks)",
		"go func()",
		"gateway.Listen(\":7777\")",
	} {
		if !strings.Contains(source, current) {
			t.Fatalf("production entry no longer contains %q", current)
		}
	}
	for _, absent := range []string{"signal.NotifyContext", "gateway.Shutdown(", "close(feishuTasks)", "runnerDone"} {
		if strings.Contains(source, absent) {
			t.Fatalf("production entry now contains %q; update DV-AOP-002 classification", absent)
		}
	}
}

func TestDVAOPApprovalReusesAuthenticatedExactlyOnceStore(t *testing.T) {
	source := readAgentOpsMain(t)
	if count := strings.Count(source, "approval.NewStore()"); count != 1 {
		t.Fatalf("approval stores = %d, want one shared authority", count)
	}
	if !strings.Contains(source, "NewGateway(verificationToken, encryptKey, feishuTasks, approvalStore)") ||
		!strings.Contains(source, "NewRunner(llmProvider, workDir, logDir, messenger, approvalStore)") {
		t.Fatal("Gateway and AgentOps Runner do not share the approval Store")
	}

	store := approval.NewStore()
	releaseSend := make(chan struct{})
	sent := make(chan struct{})
	waitDone := make(chan struct{})
	go func() {
		_, _ = store.Wait(context.Background(), approval.Request{ID: "aop-approval"}, func(approval.Request) error {
			close(sent)
			<-releaseSend
			return nil
		})
		close(waitDone)
	}()
	<-sent
	handler := feishu.NewGateway("token", "", make(chan feishu.Task), store).Server(":0").Handler
	first := postAgentOpsApproval(handler, `{"approval_id":"aop-approval","approved":true}`)
	duplicate := postAgentOpsApproval(handler, `{"approval_id":"aop-approval","approved":false}`)
	if first.Code != http.StatusNoContent || duplicate.Code != http.StatusConflict {
		t.Fatalf("approval statuses = %d, %d; want 204, 409", first.Code, duplicate.Code)
	}
	close(releaseSend)
	<-waitDone
	late := postAgentOpsApproval(handler, `{"approval_id":"aop-approval","approved":false}`)
	if late.Code != http.StatusNotFound {
		t.Fatalf("late approval status = %d, want 404", late.Code)
	}
}

func postAgentOpsApproval(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/webhook/approval", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func postAgentOpsEvent(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/webhook/event", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func agentOpsMessageEventJSON(eventID, messageID string) string {
	return fmt.Sprintf(`{"schema":"2.0","header":{"event_id":%q,"event_type":"im.message.receive_v1","app_id":"app","tenant_key":"tenant","create_time":"1","token":"token"},"event":{"sender":{"sender_id":{"open_id":"sender-1"}},"message":{"message_id":%q,"chat_id":"chat-1","message_type":"text","content":"{\"text\":\"run task\"}"}}}`, eventID, messageID)
}

func readAgentOpsMain(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(current), "main.go"))
	if err != nil {
		t.Fatalf("ReadFile(main.go) error = %v", err)
	}
	return string(data)
}
