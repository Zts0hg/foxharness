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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/feishu"
)

func TestDVAOP001DeduperIsProcessLocalAndClaimsBeforeTerminalWork(t *testing.T) {
	now := time.Unix(1000, 0)
	deduper := NewDeduperWithTTL(time.Minute)
	deduper.now = func() time.Time { return now }
	if !deduper.Mark("") || deduper.Mark("") {
		t.Fatal("empty message IDs currently share one accepted dedupe key")
	}
	if !deduper.Mark("sequential") || deduper.Mark("sequential") {
		t.Fatal("sequential duplicate classification changed")
	}

	const contenders = 32
	start := make(chan struct{})
	var winners atomic.Int32
	var wait sync.WaitGroup
	wait.Add(contenders)
	for i := 0; i < contenders; i++ {
		go func() {
			defer wait.Done()
			<-start
			if deduper.Mark("concurrent") {
				winners.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()
	if got := winners.Load(); got != 1 {
		t.Fatalf("concurrent winners = %d, want one", got)
	}

	if !deduper.Mark("ttl") {
		t.Fatal("first TTL message rejected")
	}
	now = now.Add(time.Minute)
	if deduper.Mark("ttl") {
		t.Fatal("message expired at the exact TTL boundary; current comparison is strictly greater")
	}
	now = now.Add(time.Nanosecond)
	if !deduper.Mark("ttl") {
		t.Fatal("message did not expire immediately after the TTL boundary")
	}

	if !deduper.Mark("bridge-failure") || deduper.Mark("bridge-failure") {
		t.Fatal("claimed message unexpectedly became retryable after simulated bridge failure")
	}
	if !NewDeduperWithTTL(time.Minute).Mark("bridge-failure") {
		t.Fatal("new process retained process-local dedupe state")
	}

	source := readAgentOpsMain(t)
	markIndex := strings.Index(source, "deduper.Mark(task.MessageID)")
	bridgeIndex := strings.Index(source, "agentTasks <- agentTask")
	if markIndex < 0 || bridgeIndex < 0 || markIndex > bridgeIndex {
		t.Fatal("AgentOps no longer claims dedupe before bridge delivery")
	}
	if strings.Contains(source, "WithDeliveryStore") || strings.Contains(source, "deduper.Release") {
		t.Fatal("AgentOps now composes durable acceptance or rollback; update DV-AOP-001 classification")
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
