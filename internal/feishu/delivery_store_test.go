package feishu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
)

func newFileDeliveryStoreAtPath(path string) (*FileDeliveryStore, error) {
	return NewFileDeliveryStore(filepath.Dir(path), filepath.Base(path))
}

func TestFileDeliveryStorePersistsAndReleasesReservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deliveries.json")
	store, err := newFileDeliveryStoreAtPath(path)
	if err != nil {
		t.Fatalf("NewFileDeliveryStore() error = %v", err)
	}
	accepted, err := store.Reserve("message-1")
	if err != nil || !accepted {
		t.Fatalf("first Reserve() = %v, %v", accepted, err)
	}
	accepted, err = store.Reserve("message-1")
	if err != nil || accepted {
		t.Fatalf("duplicate Reserve() = %v, %v", accepted, err)
	}

	restarted, err := newFileDeliveryStoreAtPath(path)
	if err != nil {
		t.Fatalf("restart NewFileDeliveryStore() error = %v", err)
	}
	accepted, err = restarted.Reserve("message-1")
	if err != nil || accepted {
		t.Fatalf("restart Reserve() = %v, %v", accepted, err)
	}
	if err := restarted.Release("message-1"); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	accepted, err = store.Reserve("message-1")
	if err != nil || !accepted {
		t.Fatalf("Reserve() after release = %v, %v", accepted, err)
	}
}

func TestFileDeliveryStoreConcurrentReservationHasOneWinner(t *testing.T) {
	store, err := NewFileDeliveryStore(t.TempDir(), "deliveries.json")
	if err != nil {
		t.Fatalf("NewFileDeliveryStore() error = %v", err)
	}
	const callers = 16
	start := make(chan struct{})
	var wait sync.WaitGroup
	var accepted atomic.Int32
	wait.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wait.Done()
			<-start
			won, reserveErr := store.Reserve("message-1")
			if reserveErr != nil {
				t.Errorf("Reserve() error = %v", reserveErr)
			}
			if won {
				accepted.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()
	if got := accepted.Load(); got != 1 {
		t.Fatalf("accepted callers = %d, want 1", got)
	}
}

func TestFileDeliveryStoreConcurrentIndependentInstancesHaveOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deliveries.json")
	const callers = 64
	stores := make([]*FileDeliveryStore, callers)
	for i := range stores {
		store, err := newFileDeliveryStoreAtPath(path)
		if err != nil {
			t.Fatalf("NewFileDeliveryStore() error = %v", err)
		}
		stores[i] = store
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	var accepted atomic.Int32
	wait.Add(callers)
	for _, store := range stores {
		go func(store *FileDeliveryStore) {
			defer wait.Done()
			<-start
			won, reserveErr := store.Reserve("message-1")
			if reserveErr != nil {
				t.Errorf("Reserve() error = %v", reserveErr)
			}
			if won {
				accepted.Add(1)
			}
		}(store)
	}
	close(start)
	wait.Wait()
	if got := accepted.Load(); got != 1 {
		t.Fatalf("accepted callers = %d, want 1", got)
	}
}

func TestFileDeliveryStoreFailsClosedOnCorruptAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deliveries.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store, err := newFileDeliveryStoreAtPath(path)
	if err != nil {
		t.Fatalf("NewFileDeliveryStore() error = %v", err)
	}
	if accepted, err := store.Reserve("message-1"); err == nil || accepted {
		t.Fatalf("Reserve() = %v, %v; want fail-closed", accepted, err)
	}
}

func TestFileDeliveryStoreRejectsSymlinkAuthorities(t *testing.T) {
	for _, suffix := range []string{"", ".lock"} {
		t.Run("deliveries.json"+suffix, func(t *testing.T) {
			root := t.TempDir()
			outside := filepath.Join(t.TempDir(), "authority")
			if err := os.WriteFile(outside, []byte(`{"version":1,"message_ids":[]}`), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			path := filepath.Join(root, "deliveries.json")
			if err := os.Symlink(outside, path+suffix); err != nil {
				t.Skipf("Symlink() error = %v", err)
			}
			store, err := NewFileDeliveryStore(root, "deliveries.json")
			if err != nil {
				t.Fatalf("NewFileDeliveryStore() error = %v", err)
			}
			if accepted, err := store.Reserve("message-1"); err == nil || accepted {
				t.Fatalf("Reserve() = %v, %v; want symlink authority rejected", accepted, err)
			}
		})
	}
}

func TestFileDeliveryStoreRejectsSymlinkDirectoryEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "redirect")); err != nil {
		t.Skipf("Symlink() error = %v", err)
	}
	store, err := NewFileDeliveryStore(root, filepath.Join("redirect", "deliveries.json"))
	if err != nil {
		t.Fatalf("NewFileDeliveryStore() error = %v", err)
	}
	if accepted, err := store.Reserve("message-1"); err == nil || accepted {
		t.Fatalf("Reserve() = %v, %v; want outside-root directory symlink rejected", accepted, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "deliveries.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside delivery authority Stat() error = %v, want not created", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "deliveries.json.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside lock authority Stat() error = %v, want not created", err)
	}
}

func TestNewFileDeliveryStoreRejectsPathOutsideRoot(t *testing.T) {
	if _, err := NewFileDeliveryStore(t.TempDir(), filepath.Join("..", "deliveries.json")); err == nil {
		t.Fatal("NewFileDeliveryStore() accepted a path outside its trusted root")
	}
}

func TestFileDeliveryStoreReportsAcceptedWhenCommitIsVisibleBeforePostCommitError(t *testing.T) {
	commitErr := errors.New("sync committed delivery store")
	previous := commitDeliveryStoreFileFunc
	commitDeliveryStoreFileFunc = func(root *os.Root, temporaryPath, targetPath string) (bool, error) {
		if err := root.Rename(temporaryPath, targetPath); err != nil {
			return false, err
		}
		return true, commitErr
	}
	defer func() { commitDeliveryStoreFileFunc = previous }()

	path := filepath.Join(t.TempDir(), "deliveries.json")
	store, err := newFileDeliveryStoreAtPath(path)
	if err != nil {
		t.Fatalf("NewFileDeliveryStore() error = %v", err)
	}
	accepted, err := store.Reserve("message-1")
	if !accepted || !errors.Is(err, commitErr) {
		t.Fatalf("Reserve() = %v, %v; want accepted post-commit error %v", accepted, err, commitErr)
	}

	restarted, err := newFileDeliveryStoreAtPath(path)
	if err != nil {
		t.Fatalf("restart NewFileDeliveryStore() error = %v", err)
	}
	if accepted, err := restarted.Reserve("message-1"); err != nil || accepted {
		t.Fatalf("restart Reserve() = %v, %v; want persisted duplicate", accepted, err)
	}
}

func TestGatewayRollsBackReservationWhenEnqueueIsUnavailable(t *testing.T) {
	tasks := make(chan Task)
	store := newMemoryDeliveryStore()
	gateway := NewGateway("token", "", tasks, approval.NewStore()).WithDeliveryStore(store)
	handler := gateway.Server(":0").Handler

	request := httptest.NewRequest(http.MethodPost, "/webhook/event", nil)
	request.Body = readCloser(messageEventJSON("event-1", "message-1", true))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("unavailable enqueue status = %d, want 500", recorder.Code)
	}

	received := make(chan Task, 1)
	gateway.tasks = received
	retry := postMessageEvent(t, handler, "event-2", "message-1", true)
	if retry.Code != http.StatusOK {
		t.Fatalf("retry status = %d body=%q", retry.Code, retry.Body.String())
	}
	if task := <-received; task.MessageID != "message-1" {
		t.Fatalf("retry task = %#v", task)
	}
}

func TestGatewayStopAcceptingPreventsReservationAndEnqueue(t *testing.T) {
	tasks := make(chan Task, 1)
	store := &countingDeliveryStore{}
	gateway := NewGateway("token", "", tasks, approval.NewStore()).WithDeliveryStore(store)
	handler := gateway.Server(":0").Handler
	if err := gateway.StopAccepting(context.Background()); err != nil {
		t.Fatalf("StopAccepting() error = %v", err)
	}

	response := postMessageEvent(t, handler, "event-1", "message-1", true)
	if response.Code == http.StatusOK {
		t.Fatalf("post-shutdown status = 200, want rejected delivery")
	}
	if calls := store.reserveCalls.Load(); calls != 0 {
		t.Fatalf("Reserve() calls after StopAccepting = %d, want 0", calls)
	}
	select {
	case task := <-tasks:
		t.Fatalf("post-shutdown task enqueued: %#v", task)
	default:
	}
}

func TestGatewayStopAcceptingTimeoutReturnsBeforeUncooperativeDeliveryExits(t *testing.T) {
	tasks := make(chan Task, 1)
	store := &blockingReserveDeliveryStore{
		enteredReserve: make(chan struct{}),
		releaseReserve: make(chan struct{}),
		released:       make(chan string, 1),
	}
	gateway := NewGateway("token", "", tasks, approval.NewStore()).WithDeliveryStore(store)
	handler := gateway.Server(":0").Handler

	responseDone := make(chan int, 1)
	go func() {
		responseDone <- postMessageEvent(t, handler, "event-1", "message-1", true).Code
	}()
	<-store.enteredReserve

	stopDone := make(chan error, 1)
	go func() {
		stopCtx, cancelStop := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancelStop()
		stopDone <- gateway.StopAccepting(stopCtx)
	}()

	select {
	case err := <-stopDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("StopAccepting() error = %v, want deadline evidence", err)
		}
	case <-time.After(200 * time.Millisecond):
		close(store.releaseReserve)
		t.Fatal("StopAccepting() remained blocked after its context deadline")
	}
	close(store.releaseReserve)
	if status := <-responseDone; status == http.StatusOK {
		t.Fatalf("active delivery status = 200, want shutdown rejection after rollback")
	}
	select {
	case messageID := <-store.released:
		if messageID != "message-1" {
			t.Fatalf("released message ID = %q, want message-1", messageID)
		}
	default:
		t.Fatal("active delivery was not rolled back after StopAccepting timeout")
	}
	select {
	case task := <-tasks:
		t.Fatalf("active delivery enqueued after StopAccepting timeout: %#v", task)
	default:
	}
}

func TestGatewayStopAcceptingTimeoutCancelsContextAwareReservation(t *testing.T) {
	tasks := make(chan Task, 1)
	store := &contextAwareBlockingDeliveryStore{
		enteredReserve: make(chan struct{}),
		releaseReserve: make(chan struct{}),
		released:       make(chan string, 1),
	}
	t.Cleanup(func() {
		store.releaseOnce.Do(func() { close(store.releaseReserve) })
	})
	gateway := NewGateway("token", "", tasks, approval.NewStore()).WithDeliveryStore(store)
	handler := gateway.Server(":0").Handler

	responseDone := make(chan int, 1)
	go func() {
		responseDone <- postMessageEvent(t, handler, "event-1", "message-1", true).Code
	}()
	<-store.enteredReserve

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelStop()
	stopDone := make(chan error, 1)
	go func() { stopDone <- gateway.StopAccepting(stopCtx) }()

	select {
	case err := <-stopDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("StopAccepting() error = %v, want deadline evidence", err)
		}
	case <-time.After(200 * time.Millisecond):
		store.releaseOnce.Do(func() { close(store.releaseReserve) })
		t.Fatal("StopAccepting() did not return after cancelling a context-aware reservation")
	}
	if status := <-responseDone; status == http.StatusOK {
		t.Fatalf("active delivery status = 200, want shutdown rejection after reservation cancellation")
	}
	select {
	case messageID := <-store.released:
		if messageID != "message-1" {
			t.Fatalf("released message ID = %q, want message-1", messageID)
		}
	case <-time.After(time.Second):
		t.Fatal("context-aware cancelled delivery did not roll back")
	}
	select {
	case task := <-tasks:
		t.Fatalf("cancelled delivery enqueued after StopAccepting timeout: %#v", task)
	default:
	}
}

func TestGatewayQueueUnavailableRollbackUsesContextAwareRelease(t *testing.T) {
	tasks := make(chan Task)
	store := &blockingLegacyReleaseDeliveryStore{
		releaseCalled:        make(chan struct{}),
		releaseUnblock:       make(chan struct{}),
		releaseContextCalled: make(chan context.Context, 1),
	}
	t.Cleanup(func() {
		store.releaseOnce.Do(func() { close(store.releaseUnblock) })
	})
	gateway := NewGateway("token", "", tasks, approval.NewStore()).WithDeliveryStore(store)
	handler := gateway.Server(":0").Handler

	responseDone := make(chan int, 1)
	go func() {
		responseDone <- postMessageEvent(t, handler, "event-1", "message-1", true).Code
	}()

	select {
	case <-store.releaseContextCalled:
	case <-store.releaseCalled:
		store.releaseOnce.Do(func() { close(store.releaseUnblock) })
		t.Fatal("queue-unavailable rollback used blocking Release instead of context-aware ReleaseContext")
	case <-time.After(200 * time.Millisecond):
		store.releaseOnce.Do(func() { close(store.releaseUnblock) })
		t.Fatal("queue-unavailable rollback did not reach a release path")
	}
	select {
	case status := <-responseDone:
		if status == http.StatusOK {
			t.Fatalf("queue-unavailable status = 200, want rejection")
		}
	case <-time.After(200 * time.Millisecond):
		store.releaseOnce.Do(func() { close(store.releaseUnblock) })
		t.Fatal("queue-unavailable rollback did not complete")
	}
}

func TestGatewayEnqueuesAcceptedDeliveryWhenStoreReportsPostCommitError(t *testing.T) {
	tasks := make(chan Task, 1)
	storeErr := errors.New("unlock delivery store after commit")
	gateway := NewGateway("token", "", tasks, approval.NewStore()).WithDeliveryStore(postCommitErrorDeliveryStore{err: storeErr})
	handler := gateway.Server(":0").Handler

	response := postMessageEvent(t, handler, "event-1", "message-1", true)
	if response.Code != http.StatusOK {
		t.Fatalf("post-commit reserve error status = %d body=%q, want 200", response.Code, response.Body.String())
	}
	select {
	case task := <-tasks:
		if task.MessageID != "message-1" {
			t.Fatalf("task = %#v, want message-1", task)
		}
	default:
		t.Fatal("accepted delivery was not enqueued after post-commit store error")
	}
}

type postCommitErrorDeliveryStore struct {
	err error
}

func (s postCommitErrorDeliveryStore) Reserve(string) (bool, error) { return true, s.err }
func (postCommitErrorDeliveryStore) Release(string) error           { return nil }

type blockingReserveDeliveryStore struct {
	enteredReserve chan struct{}
	releaseReserve chan struct{}
	released       chan string
}

func (s *blockingReserveDeliveryStore) Reserve(string) (bool, error) {
	close(s.enteredReserve)
	<-s.releaseReserve
	return true, nil
}

func (s *blockingReserveDeliveryStore) Release(messageID string) error {
	s.released <- messageID
	return nil
}

type contextAwareBlockingDeliveryStore struct {
	enteredReserve chan struct{}
	releaseReserve chan struct{}
	releaseOnce    sync.Once
	released       chan string
	enterOnce      sync.Once
}

func (s *contextAwareBlockingDeliveryStore) Reserve(string) (bool, error) {
	s.enterOnce.Do(func() { close(s.enteredReserve) })
	<-s.releaseReserve
	return true, nil
}

func (s *contextAwareBlockingDeliveryStore) ReserveContext(ctx context.Context, messageID string) (bool, error) {
	s.enterOnce.Do(func() { close(s.enteredReserve) })
	select {
	case <-s.releaseReserve:
		return true, nil
	case <-ctx.Done():
		return true, ctx.Err()
	}
}

func (s *contextAwareBlockingDeliveryStore) Release(string) error {
	s.releaseOnce.Do(func() { close(s.releaseReserve) })
	return nil
}

func (s *contextAwareBlockingDeliveryStore) ReleaseContext(_ context.Context, messageID string) error {
	s.released <- messageID
	return nil
}

type blockingLegacyReleaseDeliveryStore struct {
	releaseOnce          sync.Once
	releaseCalled        chan struct{}
	releaseUnblock       chan struct{}
	releaseContextCalled chan context.Context
}

func (*blockingLegacyReleaseDeliveryStore) Reserve(string) (bool, error) {
	return true, nil
}

func (s *blockingLegacyReleaseDeliveryStore) ReserveContext(context.Context, string) (bool, error) {
	return true, nil
}

func (s *blockingLegacyReleaseDeliveryStore) Release(string) error {
	s.releaseOnce.Do(func() { close(s.releaseCalled) })
	<-s.releaseUnblock
	return nil
}

func (s *blockingLegacyReleaseDeliveryStore) ReleaseContext(ctx context.Context, _ string) error {
	s.releaseContextCalled <- ctx
	return nil
}

type stringReadCloser struct{ *strings.Reader }

func (stringReadCloser) Close() error { return nil }

func readCloser(value string) stringReadCloser {
	return stringReadCloser{Reader: strings.NewReader(value)}
}
