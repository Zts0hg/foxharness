package approval

import (
	"context"
	"errors"
	"sync"
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

func TestStoreTimeoutRemovesPendingRequest(t *testing.T) {
	timeout := make(chan time.Time, 1)
	store := NewStore()
	store.newTimeout = func() (<-chan time.Time, func()) {
		return timeout, func() {}
	}
	sent := make(chan struct{})
	waitResult := make(chan Result, 1)
	go func() {
		result, err := store.Wait(context.Background(), Request{ID: "approval-timeout"}, func(Request) error {
			close(sent)
			return nil
		})
		if err != nil {
			t.Errorf("Wait() error = %v", err)
		}
		waitResult <- result
	}()
	<-sent
	timeout <- time.Unix(1, 0)
	result := <-waitResult
	if result.Approved || result.Reason != "审批超时" {
		t.Fatalf("timeout result = %#v", result)
	}
	if err := store.Resolve("approval-timeout", Result{Approved: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("post-timeout Resolve() error = %v, want ErrNotFound", err)
	}
}

func TestStoreConcurrentResolveHasExactlyOneWinner(t *testing.T) {
	store := NewStore()
	sent := make(chan struct{})
	releaseSend := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSend) }) }
	t.Cleanup(release)
	waitResult := make(chan Result, 1)
	go func() {
		result, _ := store.Wait(context.Background(), Request{ID: "approval-concurrent"}, func(Request) error {
			close(sent)
			<-releaseSend
			return nil
		})
		waitResult <- result
	}()
	<-sent

	const resolvers = 32
	start := make(chan struct{})
	results := make(chan error, resolvers)
	for i := 0; i < resolvers; i++ {
		go func(index int) {
			<-start
			results <- store.Resolve("approval-concurrent", Result{Approved: true, Reason: string(rune('A' + index))})
		}(i)
	}
	close(start)
	winners := 0
	conflicts := 0
	for i := 0; i < resolvers; i++ {
		select {
		case err := <-results:
			switch {
			case err == nil:
				winners++
			case errors.Is(err, ErrConflict):
				conflicts++
			default:
				t.Fatalf("Resolve() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent Resolve() blocked")
		}
	}
	if winners != 1 || conflicts != resolvers-1 {
		t.Fatalf("resolve outcomes = %d winner, %d conflicts", winners, conflicts)
	}
	release()
	result := <-waitResult
	if !result.Approved || result.Reason == "" {
		t.Fatalf("winning result = %#v", result)
	}
}
