package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/approval"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// These tests retain the executable evidence for pre-existing defects and are
// updated to the confirmed correction semantics only after each TDD fix turns
// Green.

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

func TestDVFEI002DuplicateMessageDeliveriesUseDurableAtMostOnceAcceptance(t *testing.T) {
	const concurrent = 8
	tasks := make(chan Task, 32)
	storePath := filepath.Join(t.TempDir(), "deliveries.json")
	deliveryStore, err := NewFileDeliveryStore(storePath)
	if err != nil {
		t.Fatalf("NewFileDeliveryStore() error = %v", err)
	}
	gateway := NewGateway("token", "", tasks, approval.NewStore()).WithDeliveryStore(deliveryStore)
	handler := gateway.Server(":0").Handler

	first := postMessageEvent(t, handler, "event-1", "message-1", true)
	second := postMessageEvent(t, handler, "event-1", "message-1", true)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("sequential duplicate acknowledgements = %d, %d", first.Code, second.Code)
	}
	assertDuplicateTasks(t, tasks, 1, "message-1")
	assertNoTask(t, tasks)

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
	assertDuplicateTasks(t, tasks, 1, "message-concurrent")
	assertNoTask(t, tasks)

	// Completion and restart have no durable duplicate authority either.
	postMessageEvent(t, handler, "event-after", "message-1", true)
	assertNoTask(t, tasks)
	restartedTasks := make(chan Task, 1)
	restartedStore, err := NewFileDeliveryStore(storePath)
	if err != nil {
		t.Fatalf("restart NewFileDeliveryStore() error = %v", err)
	}
	restarted := NewGateway("token", "", restartedTasks, approval.NewStore()).WithDeliveryStore(restartedStore)
	postMessageEvent(t, restarted.Server(":0").Handler, "event-restart", "message-1", true)
	assertNoTask(t, restartedTasks)
}

func TestDVFEI003MissingOrBlankSenderIsRejectedBeforeReservationAndEnqueue(t *testing.T) {
	missing := messageEvent("event-1", "message-1", false)
	blank := messageEvent("event-2", "message-2", true)
	*blank.Event.Sender.SenderId.OpenId = ""
	for name, event := range map[string]*larkim.P2MessageReceiveV1{"missing": missing, "blank": blank} {
		t.Run(name+" parser", func(t *testing.T) {
			if task, err := taskFromMessageEvent(event); err == nil || !strings.Contains(err.Error(), "sender") {
				t.Fatalf("taskFromMessageEvent() = %#v, %v; want sender validation", task, err)
			}
		})
	}

	tasks := make(chan Task, 2)
	deliveries := &countingDeliveryStore{}
	handler := NewGateway("token", "", tasks, approval.NewStore()).WithDeliveryStore(deliveries).Server(":0").Handler
	missingResponse := postMessageEvent(t, handler, "event-1", "message-1", false)
	blankBody := strings.Replace(messageEventJSON("event-2", "message-2", true), `"sender-1"`, `""`, 1)
	blankRequest := httptest.NewRequest(http.MethodPost, "/webhook/event", strings.NewReader(blankBody))
	blankRequest.Header.Set("Content-Type", "application/json")
	blankResponse := httptest.NewRecorder()
	handler.ServeHTTP(blankResponse, blankRequest)
	if missingResponse.Code != http.StatusOK || blankResponse.Code != http.StatusOK {
		t.Fatalf("validation acknowledgements = %d, %d; want 200", missingResponse.Code, blankResponse.Code)
	}
	if calls := deliveries.reserveCalls.Load(); calls != 0 {
		t.Fatalf("delivery reservations = %d, want zero", calls)
	}
	assertNoTask(t, tasks)
}

func TestDVFEI004CancelledWaiterLeavesSessionLockWithoutLaterExecution(t *testing.T) {
	runner := &Runner{locks: make(map[string]*sessionLock)}
	releaseFirst, err := runner.acquireSessionLock(context.Background(), "chat:sender")
	if err != nil {
		t.Fatalf("first acquireSessionLock() error = %v", err)
	}

	waiterEntered := make(chan struct{})
	waiterResult := make(chan error, 1)
	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	go func() {
		close(waiterEntered)
		release, acquireErr := runner.acquireSessionLock(waiterCtx, "chat:sender")
		if acquireErr == nil {
			release()
		}
		waiterResult <- acquireErr
	}()
	<-waiterEntered
	waitForLockRefs(t, runner, "chat:sender", 2)
	cancelWaiter()
	if err := <-waiterResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled acquireSessionLock() error = %v", err)
	}

	releaseFirst()
	releaseNext, err := runner.acquireSessionLock(context.Background(), "chat:sender")
	if err != nil {
		t.Fatalf("next acquireSessionLock() error = %v", err)
	}
	releaseNext()
}

func TestDVFEI004AcceptedTaskTimeoutIncludesGlobalPermitWait(t *testing.T) {
	createdContexts := make(chan context.CancelFunc, 3)
	runner := &Runner{
		maxConcurrentTasks: 1,
		newTaskContext: func(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
			taskCtx, cancel := context.WithCancel(parent)
			createdContexts <- cancel
			return taskCtx, cancel
		},
	}
	firstRelease := make(chan struct{})
	firstFinished := make(chan struct{})
	started := make(chan string, 2)
	runner.runTask = func(_ context.Context, task Task) {
		if task.TaskID == "first" {
			<-firstRelease
			close(firstFinished)
			return
		}
		started <- task.TaskID
	}

	tasks := make(chan Task)
	startReturned := make(chan struct{})
	go func() {
		runner.Start(context.Background(), tasks)
		close(startReturned)
	}()
	tasks <- Task{TaskID: "first"}
	cancelFirst := <-createdContexts
	defer cancelFirst()

	secondAccepted := make(chan struct{})
	go func() {
		tasks <- Task{TaskID: "second"}
		close(secondAccepted)
	}()
	<-secondAccepted
	cancelSecond := <-createdContexts
	cancelSecond()
	close(firstRelease)
	<-firstFinished
	tasks <- Task{TaskID: "third", ChatID: "other", SenderID: "sender"}
	cancelThird := <-createdContexts
	defer cancelThird()
	select {
	case got := <-started:
		if got != "third" {
			t.Fatalf("started task = %q, want expired second task skipped before third", got)
		}
	case <-time.After(time.Second):
		t.Fatal("third task did not start after the global permit became available")
	}
	close(tasks)
	<-startReturned
}

func TestDVFEI005PerSessionFIFOLeavesCapacityForOtherSessions(t *testing.T) {
	runner := &Runner{maxConcurrentTasks: 2}
	releases := map[string]chan struct{}{
		"same-1":  make(chan struct{}, 1),
		"same-2":  make(chan struct{}, 1),
		"same-3":  make(chan struct{}, 1),
		"other-1": make(chan struct{}, 1),
		"other-2": make(chan struct{}, 1),
	}
	started := make(chan string, len(releases))
	finished := make(chan string, len(releases))
	runner.runTask = func(_ context.Context, task Task) {
		started <- task.TaskID
		<-releases[task.TaskID]
		finished <- task.TaskID
	}

	tasks := make(chan Task)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		for _, release := range releases {
			select {
			case release <- struct{}{}:
			default:
			}
		}
	})
	startReturned := make(chan struct{})
	go func() {
		runner.Start(ctx, tasks)
		close(startReturned)
	}()
	wantStart := func(want string) {
		t.Helper()
		select {
		case got := <-started:
			if got != want {
				t.Fatalf("started task = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for task %q", want)
		}
	}
	assertNoStart := func() {
		t.Helper()
		select {
		case got := <-started:
			t.Fatalf("same-session queued task %q started before its predecessor", got)
		case <-time.After(50 * time.Millisecond):
		}
	}
	releaseAndWait := func(taskID string) {
		t.Helper()
		releases[taskID] <- struct{}{}
		select {
		case got := <-finished:
			if got != taskID {
				t.Fatalf("finished task = %q, want %q", got, taskID)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for task %q to finish", taskID)
		}
	}

	tasks <- Task{TaskID: "same-1", ChatID: "chat", SenderID: "sender"}
	wantStart("same-1")
	tasks <- Task{TaskID: "same-2", ChatID: "chat", SenderID: "sender"}
	assertNoStart()
	tasks <- Task{TaskID: "same-3", ChatID: "chat", SenderID: "sender"}
	tasks <- Task{TaskID: "other-1", ChatID: "other-chat", SenderID: "other-sender"}
	wantStart("other-1")
	releaseAndWait("other-1")
	assertNoStart()
	tasks <- Task{TaskID: "other-2", ChatID: "third-chat", SenderID: "third-sender"}
	wantStart("other-2")
	releaseAndWait("other-2")
	releaseAndWait("same-1")
	wantStart("same-2")
	releaseAndWait("same-2")
	wantStart("same-3")
	releaseAndWait("same-3")
	close(tasks)
	<-startReturned
}

func TestDVFEI006RunnerDrainsAcceptedTasksOnChannelClose(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{}, 2)
	runner := &Runner{maxConcurrentTasks: 1, runTask: func(_ context.Context, task Task) {
		started <- task.TaskID
		<-release
	}}
	tasks := make(chan Task)
	returned := make(chan struct{})
	go func() {
		runner.Start(context.Background(), tasks)
		close(returned)
	}()

	tasks <- Task{TaskID: "first", ChatID: "chat", SenderID: "sender"}
	if got := <-started; got != "first" {
		t.Fatalf("started task = %q, want first", got)
	}
	tasks <- Task{TaskID: "second", ChatID: "chat", SenderID: "sender"}
	close(tasks)
	select {
	case <-returned:
		t.Fatal("Runner.Start returned before draining accepted work")
	case <-time.After(50 * time.Millisecond):
	}

	release <- struct{}{}
	if got := <-started; got != "second" {
		t.Fatalf("started task = %q, want second", got)
	}
	release <- struct{}{}
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Runner.Start did not return after accepted work drained")
	}
}

func TestDVFEI006RunnerCancelsQueuedAndInflightTasksBeforeReturning(t *testing.T) {
	started := make(chan string, 2)
	inflightCancelled := make(chan struct{})
	allowInflightFinish := make(chan struct{})
	runner := &Runner{maxConcurrentTasks: 1, runTask: func(ctx context.Context, task Task) {
		started <- task.TaskID
		<-ctx.Done()
		close(inflightCancelled)
		<-allowInflightFinish
	}}
	tasks := make(chan Task)
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan struct{})
	go func() {
		runner.Start(ctx, tasks)
		close(returned)
	}()

	tasks <- Task{TaskID: "first", ChatID: "chat", SenderID: "sender"}
	if got := <-started; got != "first" {
		t.Fatalf("started task = %q, want first", got)
	}
	tasks <- Task{TaskID: "second", ChatID: "chat", SenderID: "sender"}
	cancel()
	<-inflightCancelled
	select {
	case <-returned:
		t.Fatal("Runner.Start returned before in-flight cancellation reached a terminal state")
	default:
	}
	close(allowInflightFinish)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Runner.Start did not return after cancellation completed")
	}
	select {
	case got := <-started:
		t.Fatalf("queued task %q started during cancellation", got)
	default:
	}
}

func TestDVFEI006RunnerCancelsEveryBufferedAcceptedTask(t *testing.T) {
	messenger := &recordingTextMessenger{}
	var started atomic.Int32
	runner := &Runner{messenger: messenger, runTask: func(context.Context, Task) {
		started.Add(1)
	}}
	tasks := make(chan Task, 3)
	for _, id := range []string{"buffered-1", "buffered-2", "buffered-3"} {
		tasks <- Task{TaskID: id, ChatID: "chat", SenderID: "sender"}
	}
	close(tasks)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner.Start(ctx, tasks)

	if got := started.Load(); got != 0 {
		t.Fatalf("buffered tasks started after cancellation = %d, want 0", got)
	}
	if got := strings.Join(messenger.texts, "\n"); !strings.Contains(got, "buffered-1") || !strings.Contains(got, "buffered-2") || !strings.Contains(got, "buffered-3") {
		t.Fatalf("buffered cancellation messages = %q, want one correlated terminal message per accepted task", got)
	}
}

type cancelOnSecondDoneContext struct {
	mu        sync.Mutex
	done      chan struct{}
	doneCalls int
	cancelled bool
}

func newCancelOnSecondDoneContext() *cancelOnSecondDoneContext {
	return &cancelOnSecondDoneContext{done: make(chan struct{})}
}

func (*cancelOnSecondDoneContext) Deadline() (time.Time, bool) { return time.Time{}, false }

func (c *cancelOnSecondDoneContext) Done() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.doneCalls++
	if c.doneCalls == 2 && !c.cancelled {
		close(c.done)
		c.cancelled = true
	}
	return c.done
}

func (c *cancelOnSecondDoneContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelled {
		return context.Canceled
	}
	return nil
}

func (*cancelOnSecondDoneContext) Value(any) any { return nil }

func TestDVFEI006RunnerDrainsBufferedTasksWhenCancellationRacesWithReceive(t *testing.T) {
	messenger := &recordingTextMessenger{}
	var started atomic.Int32
	runner := &Runner{messenger: messenger, runTask: func(context.Context, Task) {
		started.Add(1)
	}}
	tasks := make(chan Task, 3)
	for _, id := range []string{"race-1", "race-2", "race-3"} {
		tasks <- Task{TaskID: id, ChatID: "chat", SenderID: "sender"}
	}
	close(tasks)

	runner.Start(newCancelOnSecondDoneContext(), tasks)

	if got := started.Load(); got != 0 {
		t.Fatalf("buffered tasks started during cancellation race = %d, want 0", got)
	}
	got := strings.Join(messenger.texts, "\n")
	for _, id := range []string{"race-1", "race-2", "race-3"} {
		if !strings.Contains(got, id) {
			t.Fatalf("buffered cancellation messages = %q, missing correlated terminal message for %s", got, id)
		}
	}
}

func TestDVFEI007ApprovalResolutionIsNonBlockingAndExactlyOnce(t *testing.T) {
	store := approval.NewStore()
	sendEntered := make(chan struct{})
	releaseSend := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSend) }) }
	t.Cleanup(release)
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
		secondReturned <- store.Resolve("approval-1", approval.Result{Reason: "duplicate"})
		close(secondStarted)
	}()
	select {
	case err := <-secondReturned:
		if !errors.Is(err, approval.ErrConflict) {
			t.Fatalf("duplicate Resolve() error = %v, want ErrConflict", err)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("duplicate Resolve() blocked behind the first result")
	}
	<-secondStarted

	release()
	if result := <-waitResult; result.Reason != "first" {
		t.Fatalf("Wait() result = %#v", result)
	}
	if err := store.Resolve("approval-1", approval.Result{}); !errors.Is(err, approval.ErrNotFound) {
		t.Fatalf("late Resolve() error = %v, want ErrNotFound", err)
	}
}

func TestDVFEI007CancelledApprovalCannotResolveLater(t *testing.T) {
	store := approval.NewStore()
	ctx, cancel := context.WithCancel(context.Background())
	sent := make(chan struct{})
	waitResult := make(chan error, 1)
	go func() {
		_, err := store.Wait(ctx, approval.Request{ID: "approval-cancelled"}, func(approval.Request) error {
			close(sent)
			return nil
		})
		waitResult <- err
	}()
	<-sent
	cancel()
	if err := <-waitResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v, want context.Canceled", err)
	}
	if err := store.Resolve("approval-cancelled", approval.Result{Approved: true}); !errors.Is(err, approval.ErrNotFound) {
		t.Fatalf("post-cancellation Resolve() error = %v, want ErrNotFound", err)
	}
}

func TestDVFEI007ApprovalCallbackMapsConflictAndNotFound(t *testing.T) {
	store := approval.NewStore()
	releaseSend := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSend) }) }
	t.Cleanup(release)
	sent := make(chan struct{})
	waitDone := make(chan struct{})
	go func() {
		_, _ = store.Wait(context.Background(), approval.Request{ID: "approval-http"}, func(approval.Request) error {
			close(sent)
			<-releaseSend
			return nil
		})
		close(waitDone)
	}()
	<-sent
	handler := NewGateway("token", "", make(chan Task), store).Server(":0").Handler
	first := postApprovalCallback(handler, "token", `{"approval_id":"approval-http","approved":true}`)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first callback status = %d, want 204", first.Code)
	}
	duplicate := postApprovalCallback(handler, "token", `{"approval_id":"approval-http","approved":false}`)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate callback status = %d, want 409", duplicate.Code)
	}
	release()
	<-waitDone
	late := postApprovalCallback(handler, "token", `{"approval_id":"approval-http","approved":false}`)
	if late.Code != http.StatusNotFound {
		t.Fatalf("late callback status = %d, want 404", late.Code)
	}
}

func TestDVFEI009PanicRecoveryEmitsOneOutcomeAndBoundedTerminalReply(t *testing.T) {
	httpClient := &countingHTTPClient{
		contextErrors:    make(chan error, 1),
		contextDeadlines: make(chan bool, 1),
	}
	observer := &recordingTaskOutcomeObserver{}
	afterStarted := make(chan struct{})
	runner := &Runner{
		messenger:           newTestMessenger(httpClient),
		taskOutcomeObserver: observer,
		runTask: func(_ context.Context, task Task) {
			if task.TaskID == "panic" {
				panic("task panic")
			}
			close(afterStarted)
		},
		maxConcurrentTasks: 1,
	}
	tasks := make(chan Task, 2)
	tasks <- Task{TaskID: "panic", ChatID: "chat", SenderID: "sender"}
	tasks <- Task{TaskID: "after", ChatID: "chat", SenderID: "sender"}
	close(tasks)

	runner.Start(context.Background(), tasks)
	<-afterStarted
	outcomes := observer.snapshot()
	if len(outcomes) != 1 {
		t.Fatalf("task outcomes = %#v, want exactly one", outcomes)
	}
	if outcome := outcomes[0]; outcome.TaskID != "panic" || outcome.ChatID != "chat" || outcome.Status != TaskOutcomeFailed || !strings.Contains(outcome.Error, "task panic") {
		t.Fatalf("panic outcome = %#v", outcome)
	}
	if calls := httpClient.calls.Load(); calls != 1 {
		t.Fatalf("panic terminal delivery calls = %d, want one", calls)
	}
	if contextErr := <-httpClient.contextErrors; contextErr != nil {
		t.Fatalf("panic terminal delivery context error = %v, want fresh bounded context", contextErr)
	}
	if hasDeadline := <-httpClient.contextDeadlines; !hasDeadline {
		t.Fatal("panic terminal delivery context has no deadline")
	}
}

func TestDVFEI010DeliveryFailuresAreTypedAndObservedByStage(t *testing.T) {
	deliveryErr := errors.New("delivery failed")
	httpClient := &countingHTTPClient{err: deliveryErr}
	observer := &recordingDeliveryFailureObserver{}
	task := Task{TaskID: "task", ChatID: "chat"}
	runner := &Runner{
		messenger:               newTestMessenger(httpClient),
		deliveryFailureObserver: observer,
	}
	runner.deliverTaskText(context.Background(), task, DeliveryStageReceipt, "receipt")
	runner.deliverTaskText(context.Background(), task, DeliveryStageSession, "session")
	runner.deliverTaskText(context.Background(), task, DeliveryStageFinal, "final")
	runner.deliverTaskText(context.Background(), task, DeliveryStageFailure, "failure")
	reporter := NewReporter(runner.messenger, task.ChatID, task.TaskID).WithDeliveryFailureObserver(observer)
	reporter.Notify(context.Background(), app.Notification{Kind: app.NotificationMessage, Content: "lifecycle"})
	runner.handleTaskPanic(task, "panic")
	runner.deliverCancellation(task, context.Canceled)

	failures := observer.snapshot()
	wantStages := []DeliveryStage{
		DeliveryStageReceipt,
		DeliveryStageSession,
		DeliveryStageFinal,
		DeliveryStageFailure,
		DeliveryStageLifecycle,
		DeliveryStagePanicFailure,
		DeliveryStageCancellation,
	}
	if len(failures) != len(wantStages) {
		t.Fatalf("delivery failures = %#v, want %d", failures, len(wantStages))
	}
	for i, failure := range failures {
		if failure.TaskID != task.TaskID || failure.ChatID != task.ChatID || failure.Stage != wantStages[i] || !errors.Is(failure.Cause, deliveryErr) {
			t.Fatalf("delivery failure %d = %#v, want stage %q and correlated cause", i, failure, wantStages[i])
		}
	}
	if calls := httpClient.calls.Load(); calls != int32(len(wantStages)) {
		t.Fatalf("HTTP calls = %d, want one per observed stage", calls)
	}
	if source := readPackageSource(t, "runner.go"); strings.Contains(source, "_ = r.messenger.SendText") {
		t.Fatal("Runner still contains a hidden direct delivery error")
	}
}

func TestDVFEI010MessengerBoundsTextBeforeTransport(t *testing.T) {
	httpClient := &countingHTTPClient{texts: make(chan string, 1)}
	messenger := newTestMessenger(httpClient)
	if err := messenger.SendText(context.Background(), "chat", strings.Repeat("x", 5000)); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	text := <-httpClient.texts
	if runes := len([]rune(text)); runes > maxFeishuTextRunes {
		t.Fatalf("transport text runes = %d, want <= %d", runes, maxFeishuTextRunes)
	}
	if !strings.Contains(text, "已截断") {
		t.Fatalf("bounded transport text lacks truncation marker: %q", text)
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

func assertNoTask(t *testing.T, tasks <-chan Task) {
	t.Helper()
	select {
	case task := <-tasks:
		t.Fatalf("unexpected duplicate task: %#v", task)
	default:
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

type countingHTTPClient struct {
	calls            atomic.Int32
	err              error
	contextErrors    chan error
	contextDeadlines chan bool
	texts            chan string
}

type countingDeliveryStore struct {
	reserveCalls atomic.Int32
}

func (s *countingDeliveryStore) Reserve(string) (bool, error) {
	s.reserveCalls.Add(1)
	return true, nil
}

func (*countingDeliveryStore) Release(string) error { return nil }

func (c *countingHTTPClient) Do(request *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	if c.contextErrors != nil {
		c.contextErrors <- request.Context().Err()
	}
	if c.contextDeadlines != nil {
		_, hasDeadline := request.Context().Deadline()
		c.contextDeadlines <- hasDeadline
	}
	if c.texts != nil {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		var envelope struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, err
		}
		var content struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(envelope.Content), &content); err != nil {
			return nil, err
		}
		c.texts <- content.Text
	}
	if c.err != nil {
		return nil, c.err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"code":0}`)),
	}, nil
}

type recordingTaskOutcomeObserver struct {
	mu       sync.Mutex
	outcomes []TaskOutcome
}

func (o *recordingTaskOutcomeObserver) ObserveTaskOutcome(outcome TaskOutcome) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.outcomes = append(o.outcomes, outcome)
}

func (o *recordingTaskOutcomeObserver) snapshot() []TaskOutcome {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]TaskOutcome(nil), o.outcomes...)
}

type recordingDeliveryFailureObserver struct {
	mu       sync.Mutex
	failures []DeliveryFailure
}

func (o *recordingDeliveryFailureObserver) ObserveDeliveryFailure(failure DeliveryFailure) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.failures = append(o.failures, failure)
}

func (o *recordingDeliveryFailureObserver) snapshot() []DeliveryFailure {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]DeliveryFailure(nil), o.failures...)
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
