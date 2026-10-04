package mcpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPostConnectAuthorizationFailureIsActionableWithoutReplay(t *testing.T) {
	for _, code := range []int{401, 403} {
		for _, resource := range []bool{false, true} {
			t.Run(http.StatusText(code)+map[bool]string{true: "/resource", false: "/tool"}[resource], func(t *testing.T) {
				var denied atomic.Bool
				var attempts atomic.Int32
				s := mcp.NewServer(&mcp.Implementation{Name: "auth", Version: "1"}, nil)
				s.AddTool(&mcp.Tool{Name: "link", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ResourceLink{URI: "test://page", Name: "page"}}}, nil
				})
				handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil)
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if denied.Load() && r.Method == http.MethodPost {
						attempts.Add(1)
						w.WriteHeader(code)
						return
					}
					handler.ServeHTTP(w, r)
				}))
				defer srv.Close()
				home, work := t.TempDir(), t.TempDir()
				if err := mcpconfig.Add(home, work, mcpconfig.ProjectScope, "auth", mcpconfig.Definition{Transport: "streamable-http", URL: srv.URL}); err != nil {
					t.Fatal(err)
				}
				h, err := New(context.Background(), home, work, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer h.Close(context.Background())
				entry := h.servers["auth"]
				name := ToolName("auth", "link")
				if _, err := h.invoke(context.Background(), entry, entry.generation, name, "link", false, json.RawMessage(`{}`)); err != nil {
					t.Fatal(err)
				}
				denied.Store(true)
				_, err = h.invoke(context.Background(), entry, entry.generation, name, "link", resource, json.RawMessage(`{"uri":"test://page"}`))
				status := h.Statuses()[0]
				if err == nil || !strings.Contains(err.Error(), "login") || status.State != "authorization-required" || !strings.Contains(status.Reason, "login") || attempts.Load() != 1 {
					t.Fatalf("authorization lost or replayed: err=%v status=%+v attempts=%d", err, status, attempts.Load())
				}
			})
		}
	}
}

type resourceFailureClient struct{ fakeClient }

func (*resourceFailureClient) ReadResource(context.Context, string) (toolprotocol.ExecutionResult, error) {
	return toolprotocol.ExecutionResult{}, errors.New("connection lost with secret-detail")
}

func TestActiveTimeoutAndResourceFailureChangeStatus(t *testing.T) {
	for _, resource := range []bool{false, true} {
		t.Run(map[bool]string{true: "resource", false: "timeout"}[resource], func(t *testing.T) {
			home, work := t.TempDir(), t.TempDir()
			hostConfig(t, work, "fixture")
			var client Client = &fakeClient{started: make(chan struct{}), release: make(chan struct{})}
			if resource {
				client = &resourceFailureClient{}
			}
			h, err := New(context.Background(), home, work, func(context.Context, mcpconfig.Server) (Client, error) { return client, nil })
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close(context.Background())
			entry := h.servers["browser"]
			name := ToolName("browser", "click")
			entry.links["test://page"] = map[string]bool{name: true}
			_, err = h.invoke(context.Background(), entry, entry.generation, name, "click", resource, json.RawMessage(`{"uri":"test://page"}`))
			if err == nil {
				t.Fatal("failure hidden")
			}
			for _, s := range h.Statuses() {
				if s.Name == "browser" && (s.State != "failed" || !strings.Contains(s.Reason, "reconnect") || strings.Contains(s.Reason, "secret-detail")) {
					t.Fatalf("incorrect status: %+v", s)
				}
			}
			for _, c := range h.Capabilities() {
				if c.Name == name {
					t.Fatal("failed connection still advertised")
				}
			}
		})
	}
}

func TestActiveCallerCancellationKeepsHealthyConnection(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	hostConfig(t, work, "fixture")
	f := &fakeClient{started: make(chan struct{}), release: make(chan struct{})}
	h, err := New(context.Background(), home, work, func(context.Context, mcpconfig.Server) (Client, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close(context.Background())
	e := h.servers["browser"]
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := h.invoke(ctx, e, e.generation, ToolName("browser", "click"), "click", false, nil)
		done <- err
	}()
	<-f.started
	cancel()
	if <-done == nil {
		t.Fatal("cancel not observed")
	}
	if e.status.State != "ready" {
		t.Fatalf("caller cancellation invalidated connection: %+v", e.status)
	}
	close(f.release)
	if _, err := h.invoke(context.Background(), e, e.generation, ToolName("browser", "click"), "click", false, nil); err != nil {
		t.Fatal(err)
	}
}
