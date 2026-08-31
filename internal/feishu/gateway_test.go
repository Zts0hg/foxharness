package feishu

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
)

func TestGatewayWithDeliveryStoreTreatsTypedNilAsAbsent(t *testing.T) {
	gateway := NewGateway("token", "encrypt", make(chan Task, 1), approval.NewStore())
	var store *memoryDeliveryStore
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("WithDeliveryStore() installed typed-nil store: %v", recovered)
		}
	}()
	gateway.WithDeliveryStore(store)
	accepted, err := gateway.reserveDelivery(context.Background(), "message-1")
	if err != nil || !accepted {
		t.Fatalf("reserveDelivery() = %v/%v, want default in-memory acceptance", accepted, err)
	}
}

func TestGatewayServerUsesDefensiveTimeoutsAndPrivateMux(t *testing.T) {
	gateway := NewGateway("token", "encrypt", make(chan Task), approval.NewStore())
	server := gateway.Server(":0")

	if server == nil {
		t.Fatalf("Server() returned nil")
	}
	if server.Handler == nil || server.Handler == http.DefaultServeMux {
		t.Fatalf("Server() must use a gateway-owned handler")
	}
	if server.ReadHeaderTimeout <= 0 {
		t.Fatalf("ReadHeaderTimeout = %s, want non-zero", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout <= 0 {
		t.Fatalf("ReadTimeout = %s, want non-zero", server.ReadTimeout)
	}
	if server.WriteTimeout <= 0 {
		t.Fatalf("WriteTimeout = %s, want non-zero", server.WriteTimeout)
	}
	if server.IdleTimeout <= 0 {
		t.Fatalf("IdleTimeout = %s, want non-zero", server.IdleTimeout)
	}
}

func TestGatewayShutdownIsControlledAndIdempotent(t *testing.T) {
	gateway := NewGateway("token", "encrypt", make(chan Task), approval.NewStore())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := gateway.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() before Listen error = %v", err)
	}
	if err := gateway.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}

func TestGatewayShutdownBeforeListenPreventsLateStart(t *testing.T) {
	gateway := NewGateway("token", "encrypt", make(chan Task), approval.NewStore())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := gateway.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() before Listen error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- gateway.Listen("127.0.0.1:0")
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Listen() after Shutdown error = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_ = gateway.Shutdown(cleanupCtx)
		<-done
		t.Fatal("Listen() started after an earlier Shutdown()")
	}
}

func TestGatewayConcurrentListenAndShutdownIsRaceFree(t *testing.T) {
	for range 20 {
		gateway := NewGateway("token", "encrypt", make(chan Task), approval.NewStore())
		start := make(chan struct{})
		var calls sync.WaitGroup
		calls.Add(2)
		go func() {
			defer calls.Done()
			<-start
			_ = gateway.Listen("invalid address")
		}()
		go func() {
			defer calls.Done()
			<-start
			_ = gateway.Shutdown(context.Background())
		}()
		close(start)
		calls.Wait()
	}
}
