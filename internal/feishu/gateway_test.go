package feishu

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
)

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
	gateway.server = gateway.Server(":0")
	if err := gateway.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := gateway.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}
