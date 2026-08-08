package approval

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStoreWaitReturnsResolvedApproval(t *testing.T) {
	store := NewStore()
	req := Request{ID: "approval-1", ToolName: "bash", Arguments: `{}`, Risk: "high"}
	sendCalled := make(chan struct{}, 1)
	done := make(chan Result, 1)

	go func() {
		result, err := store.Wait(context.Background(), req, func(Request) error {
			sendCalled <- struct{}{}
			return nil
		})
		if err != nil {
			t.Errorf("Wait() error = %v", err)
			return
		}
		done <- result
	}()

	select {
	case <-sendCalled:
	case <-time.After(time.Second):
		t.Fatal("Wait did not call send")
	}
	if err := store.Resolve(req.ID, Result{Approved: true, Reason: "ok"}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	select {
	case got := <-done:
		if !got.Approved || got.Reason != "ok" {
			t.Fatalf("result = %+v, want approved ok", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return resolved decision")
	}
}

func TestStoreWaitReturnsSendErrorAndCleansPendingRequest(t *testing.T) {
	store := NewStore()
	req := Request{ID: "approval-1", ToolName: "bash"}
	sendErr := errors.New("send failed")

	_, err := store.Wait(context.Background(), req, func(Request) error {
		return sendErr
	})
	if !errors.Is(err, sendErr) {
		t.Fatalf("Wait() error = %v, want send error", err)
	}
	if err := store.Resolve(req.ID, Result{Approved: true}); err == nil {
		t.Fatal("Resolve() error = nil, want pending request removed")
	}
}

func TestStoreWaitReturnsContextCancellation(t *testing.T) {
	store := NewStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := store.Wait(ctx, Request{ID: "approval-1", ToolName: "bash"}, func(Request) error {
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v, want context canceled", err)
	}
}
