package feishu

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Zts0hg/foxharness/internal/approval"
)

func TestFileDeliveryStorePersistsAndReleasesReservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deliveries.json")
	store, err := NewFileDeliveryStore(path)
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

	restarted, err := NewFileDeliveryStore(path)
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
	store, err := NewFileDeliveryStore(filepath.Join(t.TempDir(), "deliveries.json"))
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

func TestFileDeliveryStoreFailsClosedOnCorruptAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deliveries.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store, err := NewFileDeliveryStore(path)
	if err != nil {
		t.Fatalf("NewFileDeliveryStore() error = %v", err)
	}
	if accepted, err := store.Reserve("message-1"); err == nil || accepted {
		t.Fatalf("Reserve() = %v, %v; want fail-closed", accepted, err)
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

type stringReadCloser struct{ *strings.Reader }

func (stringReadCloser) Close() error { return nil }

func readCloser(value string) stringReadCloser {
	return stringReadCloser{Reader: strings.NewReader(value)}
}
