package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// These tests record proven pre-existing defects. They intentionally assert
// current behavior so the verification gate remains executable before a
// separately approved correction defines the new semantics.

func TestDVFEI001ApprovalCallbackIsAuthenticatedBoundedAndReachable(t *testing.T) {
	store := approval.NewStore()
	gateway := NewGateway("token", "", make(chan Task, 1), store)
	server := gateway.Server(":0")

	unauthorized := postApprovalCallback(server.Handler, "", `{"approval_id":"approval-1","approved":true}`)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorized.Code)
	}

	malformed := postApprovalCallback(server.Handler, "token", `{"approval_id":"approval-1","approved":true,"unknown":1}`)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d, want 400", malformed.Code)
	}

	tooLarge := postApprovalCallback(server.Handler, "token", `{"approval_id":"approval-1","approved":true,"reason":"`+strings.Repeat("x", 70<<10)+`"}`)
	if tooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d, want 413", tooLarge.Code)
	}

	sendEntered := make(chan struct{})
	resolved := make(chan approval.Result, 1)
	go func() {
		result, _ := store.Wait(context.Background(), approval.Request{ID: "approval-1"}, func(approval.Request) error {
			close(sendEntered)
			return nil
		})
		resolved <- result
	}()
	<-sendEntered

	success := postApprovalCallback(server.Handler, "token", `{"approval_id":"approval-1","approved":true,"reason":"reviewed"}`)
	if success.Code != http.StatusNoContent {
		t.Fatalf("success status = %d body=%q, want 204", success.Code, success.Body.String())
	}
	if result := <-resolved; !result.Approved || result.Reason != "reviewed" {
		t.Fatalf("resolved result = %#v", result)
	}

	unknown := postApprovalCallback(server.Handler, "token", `{"approval_id":"missing","approved":false}`)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown status = %d, want 404", unknown.Code)
	}

	wrongMethod := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/webhook/approval", nil)
	request.Header.Set("Authorization", "Bearer token")
	server.Handler.ServeHTTP(wrongMethod, request)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", wrongMethod.Code)
	}
}

func TestDVFEI002DuplicateMessageDeliveriesAlwaysEnqueueNewTasks(t *testing.T) {
	const concurrent = 8
	tasks := make(chan Task, 32)
	gateway := NewGateway("token", "", tasks, approval.NewStore())
	handler := gateway.Server(":0").Handler

	first := postMessageEvent(t, handler, "event-1", "message-1", true)
	second := postMessageEvent(t, handler, "event-1", "message-1", true)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("sequential duplicate acknowledgements = %d, %d", first.Code, second.Code)
	}
	assertDuplicateTasks(t, tasks, 2, "message-1")

	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(concurrent)
	statuses := make(chan int, concurrent)
	for i := 0; i < concurrent; i++ {
		go func() {
			defer wait.Done()
			<-start
			statuses <- postMessageEvent(t, handler, "event-concurrent", "message-concurrent", true).Code
		}()
	}
	close(start)
	wait.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("concurrent duplicate acknowledgement = %d", status)
		}
	}
	assertDuplicateTasks(t, tasks, concurrent, "message-concurrent")

	// Completion and restart have no durable duplicate authority either.
	postMessageEvent(t, handler, "event-after", "message-1", true)
	assertDuplicateTasks(t, tasks, 1, "message-1")
	restartedTasks := make(chan Task, 1)
	restarted := NewGateway("token", "", restartedTasks, approval.NewStore())
	postMessageEvent(t, restarted.Server(":0").Handler, "event-restart", "message-1", true)
	assertDuplicateTasks(t, restartedTasks, 1, "message-1")
}

func TestDVFEI003MissingSenderIsAcceptedAndSharesSessionIdentity(t *testing.T) {
	eventOne := messageEvent("event-1", "message-1", false)
	eventTwo := messageEvent("event-2", "message-2", false)
	first, err := taskFromMessageEvent(eventOne)
	if err != nil || first.SenderID != "" {
		t.Fatalf("first missing-sender event = %#v, %v", first, err)
	}
	second, err := taskFromMessageEvent(eventTwo)
	if err != nil || second.SenderID != "" {
		t.Fatalf("second missing-sender event = %#v, %v", second, err)
	}

	workDir := t.TempDir()
	runner := &Runner{workDir: workDir, sessionManager: session.NewManagerWithHome(workDir, t.TempDir())}
	firstSession, created, err := runner.resolveSession(false, first)
	if err != nil || !created {
		t.Fatalf("first resolveSession() = %#v, %v, created=%v", firstSession, err, created)
	}
	secondSession, created, err := runner.resolveSession(false, second)
	if err != nil || created || secondSession.ID != firstSession.ID {
		t.Fatalf("second resolveSession() = %#v, %v, created=%v; want shared session %s", secondSession, err, created, firstSession.ID)
	}
}

func TestDVFEI004CancelledWaiterStillAcquiresSessionLockLater(t *testing.T) {
	runner := &Runner{locks: make(map[string]*sessionLock)}
	releaseFirst := runner.acquireSessionLock("chat:sender")

	waiterEntered := make(chan struct{})
	waiterAcquired := make(chan struct{})
	waiterRelease := make(chan struct{})
	go func() {
		close(waiterEntered)
		release := runner.acquireSessionLock("chat:sender")
		close(waiterAcquired)
		<-waiterRelease
		release()
	}()
	<-waiterEntered
	waitForLockRefs(t, runner, "chat:sender", 2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error = %v", ctx.Err())
	}
	select {
	case <-waiterAcquired:
		t.Fatal("waiter acquired a lock that is still held")
	default:
	}

	releaseFirst()
	<-waiterAcquired
	close(waiterRelease)
}

func TestDVFEI005SessionWaitersConsumeGlobalCapacity(t *testing.T) {
	runner := &Runner{maxConcurrentTasks: 2, locks: make(map[string]*sessionLock)}
	releases := map[string]chan struct{}{
		"same-1": make(chan struct{}),
		"same-2": make(chan struct{}),
		"other":  make(chan struct{}),
	}
	started := make(chan string, 3)
	runner.runTask = func(_ context.Context, task Task) {
		releaseLock := runner.acquireSessionLock(task.ChatID + ":" + task.SenderID)
		defer releaseLock()
		started <- task.TaskID
		<-releases[task.TaskID]
	}

	tasks := make(chan Task)
	startReturned := make(chan struct{})
	go func() {
		runner.Start(context.Background(), tasks)
		close(startReturned)
	}()
	tasks <- Task{TaskID: "same-1", ChatID: "chat", SenderID: "sender"}
	if got := <-started; got != "same-1" {
		t.Fatalf("first started task = %q", got)
	}
	tasks <- Task{TaskID: "same-2", ChatID: "chat", SenderID: "sender"}
	waitForLockRefs(t, runner, "chat:sender", 2)

	acceptedOther := make(chan struct{})
	go func() {
		tasks <- Task{TaskID: "other", ChatID: "other-chat", SenderID: "other-sender"}
		close(acceptedOther)
	}()
	<-acceptedOther
	select {
	case got := <-started:
		t.Fatalf("unrelated task %q started despite both permits being held by one session", got)
	default:
	}

	close(releases["same-1"])
	close(releases["same-2"])
	close(releases["other"])
	close(tasks)
	<-startReturned
}

func TestDVFEI006RunnerReturnsWithoutDrainingAcceptedTask(t *testing.T) {
	for _, test := range []struct {
		name string
		stop func(context.CancelFunc, chan Task)
	}{
		{name: "task channel closed", stop: func(_ context.CancelFunc, tasks chan Task) { close(tasks) }},
		{name: "context cancelled", stop: func(cancel context.CancelFunc, _ chan Task) { cancel() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			finished := make(chan struct{})
			runner := &Runner{runTask: func(context.Context, Task) {
				close(started)
				<-release
				close(finished)
			}}
			tasks := make(chan Task)
			ctx, cancel := context.WithCancel(context.Background())
			returned := make(chan struct{})
			go func() {
				runner.Start(ctx, tasks)
				close(returned)
			}()
			tasks <- Task{TaskID: "accepted"}
			<-started
			test.stop(cancel, tasks)
			<-returned
			select {
			case <-finished:
				t.Fatal("in-flight task unexpectedly finished before Runner.Start returned")
			default:
			}
			close(release)
			<-finished
			cancel()
		})
	}
}

func TestDVFEI007DuplicateApprovalCanBlockAndResolveTwice(t *testing.T) {
	store := approval.NewStore()
	sendEntered := make(chan struct{})
	releaseSend := make(chan struct{})
	waitResult := make(chan approval.Result, 1)
	go func() {
		result, _ := store.Wait(context.Background(), approval.Request{ID: "approval-1"}, func(approval.Request) error {
			close(sendEntered)
			<-releaseSend
			return nil
		})
		waitResult <- result
	}()
	<-sendEntered
	if err := store.Resolve("approval-1", approval.Result{Approved: true, Reason: "first"}); err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	secondStarted := make(chan struct{})
	secondReturned := make(chan error, 1)
	go func() {
		close(secondStarted)
		secondReturned <- store.Resolve("approval-1", approval.Result{Reason: "duplicate"})
	}()
	<-secondStarted
	select {
	case err := <-secondReturned:
		t.Fatalf("duplicate Resolve() returned before the first result drained: %v", err)
	default:
	}

	close(releaseSend)
	if result := <-waitResult; result.Reason != "first" {
		t.Fatalf("Wait() result = %#v", result)
	}
	if err := <-secondReturned; err != nil {
		t.Fatalf("duplicate Resolve() error after unblock = %v, want current second success", err)
	}
	if err := store.Resolve("approval-1", approval.Result{}); err == nil {
		t.Fatal("late Resolve() error = nil, want removed pending request")
	}
}

func TestDVFEI008CompactorFallsBackInsteadOfUsingProviderModel(t *testing.T) {
	providerWithModel := namedProvider{model: "claude-4-sonnet"}
	config := compaction.DefaultCompactionConfig()
	compactor, err := compaction.NewCompactor(providerWithModel, config)
	if err != nil {
		t.Fatalf("NewCompactor() error = %v", err)
	}
	selectedWindow := compaction.NewModelRegistry().Lookup(providerWithModel.model)
	if compactor.ContextWindow() != compaction.DefaultContextWindow || compactor.ContextWindow() == selectedWindow {
		t.Fatalf("current compactor window = %d, selected model window = %d", compactor.ContextWindow(), selectedWindow)
	}

	source := readPackageSource(t, "runner.go")
	if strings.Contains(source, "compCfg.Model =") {
		t.Fatal("runner now propagates compactor model; update DV-FEI-008 classification")
	}
}

func TestDVFEI009PanicRecoveryEmitsNoTerminalReply(t *testing.T) {
	httpClient := &countingHTTPClient{err: errors.New("unexpected delivery")}
	runner := &Runner{
		messenger: newTestMessenger(httpClient),
		runTask: func(_ context.Context, task Task) {
			if task.TaskID == "panic" {
				panic("task panic")
			}
		},
		maxConcurrentTasks: 1,
	}
	tasks := make(chan Task, 2)
	tasks <- Task{TaskID: "panic", ChatID: "chat"}
	tasks <- Task{TaskID: "after", ChatID: "chat"}
	close(tasks)

	var logs bytes.Buffer
	logWritten := make(chan struct{}, 1)
	oldWriter := log.Writer()
	log.SetOutput(io.MultiWriter(&logs, writerFunc(func(data []byte) (int, error) {
		select {
		case logWritten <- struct{}{}:
		default:
		}
		return len(data), nil
	})))
	t.Cleanup(func() { log.SetOutput(oldWriter) })
	runner.Start(context.Background(), tasks)
	<-logWritten
	if !strings.Contains(logs.String(), "panic recovered") {
		t.Fatalf("panic log missing: %s", logs.String())
	}
	if calls := httpClient.calls.Load(); calls != 0 {
		t.Fatalf("panic recovery sent %d terminal messages, want current zero", calls)
	}
}

func TestDVFEI010DeliveryFailureIsOnlyLoggedAndNotReturned(t *testing.T) {
	deliveryErr := errors.New("delivery failed")
	httpClient := &countingHTTPClient{err: deliveryErr}
	messenger := newTestMessenger(httpClient)
	if err := messenger.SendText(context.Background(), "chat", "receipt"); err == nil || !strings.Contains(err.Error(), deliveryErr.Error()) {
		t.Fatalf("Messenger.SendText() error = %v", err)
	}

	var logs bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(oldWriter) })
	reporter := NewReporter(messenger, "chat", "task")
	reporter.OnMessage(context.Background(), strings.Repeat("x", 5000))
	if !strings.Contains(logs.String(), deliveryErr.Error()) {
		t.Fatalf("reporter did not log delivery failure: %s", logs.String())
	}
	if calls := httpClient.calls.Load(); calls != 2 {
		t.Fatalf("HTTP calls = %d, want direct send plus reporter send", calls)
	}

	source := readPackageSource(t, "runner.go")
	if count := strings.Count(source, "_ = r.messenger.SendText"); count < 6 {
		t.Fatalf("ignored runner delivery sites = %d, want current behavior evidence", count)
	}
}

func postMessageEvent(t *testing.T, handler http.Handler, eventID, messageID string, withSender bool) *httptest.ResponseRecorder {
	t.Helper()
	payload := messageEventJSON(eventID, messageID, withSender)
	request := httptest.NewRequest(http.MethodPost, "/webhook/event", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func postApprovalCallback(handler http.Handler, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/webhook/approval", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func messageEvent(eventID, messageID string, withSender bool) *larkim.P2MessageReceiveV1 {
	var event larkim.P2MessageReceiveV1
	if err := json.Unmarshal([]byte(messageEventJSON(eventID, messageID, withSender)), &event); err != nil {
		panic(err)
	}
	return &event
}

func messageEventJSON(eventID, messageID string, withSender bool) string {
	sender := ""
	if withSender {
		sender = `"sender":{"sender_id":{"open_id":"sender-1"}},`
	}
	return fmt.Sprintf(`{"schema":"2.0","header":{"event_id":%q,"event_type":"im.message.receive_v1","app_id":"app","tenant_key":"tenant","create_time":"1","token":"token"},"event":{%s"message":{"message_id":%q,"chat_id":"chat-1","message_type":"text","content":"{\"text\":\"run task\"}"}}}`, eventID, sender, messageID)
}

func assertDuplicateTasks(t *testing.T, tasks <-chan Task, count int, messageID string) {
	t.Helper()
	for i := 0; i < count; i++ {
		task := <-tasks
		if task.MessageID != messageID {
			t.Fatalf("task %d MessageID = %q", i, task.MessageID)
		}
	}
}

func waitForLockRefs(t *testing.T, runner *Runner, key string, want int) {
	t.Helper()
	for i := 0; i < 10000; i++ {
		runner.locksMu.Lock()
		refs := runner.locks[key].refs
		runner.locksMu.Unlock()
		if refs == want {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("session lock refs did not reach %d", want)
}

type namedProvider struct{ model string }

func (p namedProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return nil, errors.New("not called")
}

func (p namedProvider) ModelName() string      { return p.model }
func (namedProvider) ProviderProtocol() string { return "scripted" }

type countingHTTPClient struct {
	calls atomic.Int32
	err   error
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(data []byte) (int, error) { return f(data) }

func (c *countingHTTPClient) Do(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	if c.err != nil {
		return nil, c.err
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":0}`))}, nil
}

func newTestMessenger(client *countingHTTPClient) *Messenger {
	return &Messenger{client: lark.NewClient(
		"test-app",
		"test-secret",
		lark.WithEnableTokenCache(false),
		lark.WithHttpClient(client),
	)}
}

func readPackageSource(t *testing.T, name string) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(current), name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	return string(data)
}
