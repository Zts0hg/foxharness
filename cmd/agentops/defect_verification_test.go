package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/agentops"
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

func TestDVAOP002ProductionEntryCoordinatesShutdownAndTwoChannelDrain(t *testing.T) {
	recorder := &agentOpsShutdownRecorder{}
	gateway := &recordingAgentOpsGateway{
		recorder:  recorder,
		listening: make(chan struct{}),
		shutdown:  make(chan struct{}),
	}
	runner := &recordingAgentOpsRunner{recorder: recorder}
	feishuTasks := make(chan feishu.Task, 1)
	feishuTasks <- feishu.Task{TaskID: "accepted", ChatID: "chat", SenderID: "sender", MessageID: "message", Text: "inspect"}
	signalCtx, cancelSignal := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serve(signalCtx, gateway, runner, feishuTasks, ":0", time.Second)
	}()
	<-gateway.listening
	cancelSignal()
	if err := <-serveResult; err != nil {
		t.Fatalf("serve() error = %v", err)
	}
	if got, want := recorder.snapshot(), []string{
		"http-shutdown",
		"listener-stopped",
		"runner-cancelled",
		"task-accepted",
		"runner-input-closed",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shutdown order = %#v, want %#v", got, want)
	}

	source := readAgentOpsMain(t)
	for _, required := range []string{
		"signal.NotifyContext",
		"serve(signalCtx, gateway, runner, feishuTasks",
		"gateway.Shutdown(",
		"close(feishuTasks)",
		"bridgeDone",
		"runnerDone",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("production entry does not contain %q", required)
		}
	}
	if strings.Contains(source, "ctx := context.Background()\n\tagentTasks") {
		t.Fatal("production entry still owns an uncoordinated background lifecycle")
	}
}

func TestDVAOP002ShutdownTimeoutStillDrainsAcceptedWork(t *testing.T) {
	recorder := &agentOpsShutdownRecorder{}
	gateway := &stuckRecordingAgentOpsGateway{
		recorder:  recorder,
		listening: make(chan struct{}),
	}
	runner := &recordingAgentOpsRunner{recorder: recorder}
	feishuTasks := make(chan feishu.Task, 1)
	feishuTasks <- feishu.Task{TaskID: "accepted", ChatID: "chat", SenderID: "sender", MessageID: "message", Text: "inspect"}
	signalCtx, cancelSignal := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serve(signalCtx, gateway, runner, feishuTasks, ":0", 20*time.Millisecond)
	}()
	<-gateway.listening
	cancelSignal()
	if err := <-serveResult; !strings.Contains(fmt.Sprint(err), "wait for AgentOps listener") {
		t.Fatalf("serve() error = %v, want listener shutdown timeout", err)
	}
	if got, want := recorder.snapshot(), []string{
		"http-shutdown",
		"runner-cancelled",
		"task-accepted",
		"runner-input-closed",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shutdown-timeout order = %#v, want %#v", got, want)
	}
}

func TestDVAOP002StopAcceptingErrorStillDrainsAcceptedWork(t *testing.T) {
	recorder := &agentOpsShutdownRecorder{}
	gateway := &recordingAgentOpsGateway{
		recorder:  recorder,
		listening: make(chan struct{}),
		shutdown:  make(chan struct{}),
		stopErr:   errors.New("stop accepting AgentOps deliveries"),
	}
	runner := &recordingAgentOpsRunner{recorder: recorder}
	feishuTasks := make(chan feishu.Task, 1)
	feishuTasks <- feishu.Task{TaskID: "accepted", ChatID: "chat", SenderID: "sender", MessageID: "message", Text: "inspect"}
	signalCtx, cancelSignal := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- serve(signalCtx, gateway, runner, feishuTasks, ":0", time.Second)
	}()
	<-gateway.listening
	cancelSignal()
	if err := <-serveResult; !strings.Contains(fmt.Sprint(err), "stop accepting AgentOps deliveries") {
		t.Fatalf("serve() error = %v, want StopAccepting failure", err)
	}
	got := recorder.snapshot()
	for _, required := range []string{"http-shutdown", "listener-stopped", "runner-cancelled", "task-accepted", "runner-input-closed"} {
		if !containsAgentOpsShutdownEvent(got, required) {
			t.Fatalf("StopAccepting failure order = %#v, missing %s", got, required)
		}
	}
	if len(got) < 2 {
		t.Fatalf("StopAccepting failure order = %#v, want accepted task and input close events", got)
	}
	if got[len(got)-2] != "task-accepted" || got[len(got)-1] != "runner-input-closed" {
		t.Fatalf("StopAccepting failure terminal order = %#v, want accepted task before runner input closes", got)
	}
}

func TestDVAOP005ProductionEntryComposesDeliveryFailureObserver(t *testing.T) {
	source := readAgentOpsMain(t)
	for _, required := range []string{
		"WithDeliveryFailureObserver",
		"NewLoggingDeliveryFailureObserver(log.Default())",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("production entry does not compose %q", required)
		}
	}
}

type agentOpsShutdownRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *agentOpsShutdownRecorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *agentOpsShutdownRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type recordingAgentOpsGateway struct {
	recorder  *agentOpsShutdownRecorder
	listening chan struct{}
	shutdown  chan struct{}
	stopErr   error
}

func (g *recordingAgentOpsGateway) Listen(string) error {
	close(g.listening)
	<-g.shutdown
	g.recorder.add("listener-stopped")
	return nil
}

func (g *recordingAgentOpsGateway) Shutdown(context.Context) error {
	g.recorder.add("http-shutdown")
	close(g.shutdown)
	return nil
}

func (g *recordingAgentOpsGateway) StopAccepting(context.Context) error { return g.stopErr }

type stuckRecordingAgentOpsGateway struct {
	recorder  *agentOpsShutdownRecorder
	listening chan struct{}
}

func (g *stuckRecordingAgentOpsGateway) Listen(string) error {
	close(g.listening)
	select {}
}

func (g *stuckRecordingAgentOpsGateway) Shutdown(context.Context) error {
	g.recorder.add("http-shutdown")
	return nil
}

func (*stuckRecordingAgentOpsGateway) StopAccepting(context.Context) error { return nil }

type recordingAgentOpsRunner struct {
	recorder *agentOpsShutdownRecorder
}

func (r *recordingAgentOpsRunner) Start(ctx context.Context, tasks <-chan agentops.Task) {
	<-ctx.Done()
	r.recorder.add("runner-cancelled")
	for task := range tasks {
		if task.TaskID == "accepted" {
			r.recorder.add("task-accepted")
		}
	}
	r.recorder.add("runner-input-closed")
}

func containsAgentOpsShutdownEvent(events []string, want string) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}

func TestDVAOPApprovalReusesAuthenticatedExactlyOnceStore(t *testing.T) {
	source := readAgentOpsMain(t)
	if count := strings.Count(source, "approval.NewStore()"); count != 1 {
		t.Fatalf("approval stores = %d, want one shared authority", count)
	}
	if !strings.Contains(source, "NewGateway(verificationToken, encryptKey, feishuTasks, approvalStore)") ||
		!strings.Contains(source, "newAgentOpsTaskExecutionFactory(llmProvider, workDir, logDir, messenger, sessionStore, approvalStore)") {
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
