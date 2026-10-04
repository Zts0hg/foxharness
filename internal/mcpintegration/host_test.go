package mcpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type fakeClient struct {
	calls, closed    atomic.Int32
	started, release chan struct{}
	once             sync.Once
	result           toolprotocol.ExecutionResult
}

func (f *fakeClient) Tools() []schema.ToolDefinition {
	return []schema.ToolDefinition{{Name: "click", InputSchema: map[string]any{"type": "object"}}}
}
func (f *fakeClient) ProtocolVersion() string { return "2025-11-25" }
func (f *fakeClient) Call(ctx context.Context, _ string, _ json.RawMessage) (toolprotocol.ExecutionResult, error) {
	f.calls.Add(1)
	if f.started != nil {
		f.once.Do(func() { close(f.started) })
		select {
		case <-f.release:
		case <-ctx.Done():
			return toolprotocol.ExecutionResult{}, ctx.Err()
		}
	}
	return f.result, nil
}
func (f *fakeClient) ReadResource(context.Context, string) (toolprotocol.ExecutionResult, error) {
	return toolprotocol.ExecutionResult{Output: "resource"}, nil
}
func (f *fakeClient) Close() error { f.closed.Add(1); return nil }

func hostConfig(t *testing.T, work string, command string) {
	t.Helper()
	dir := filepath.Join(work, ".foxharness")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := `{"mcpServers":{"browser":{"transport":"stdio","command":"` + command + `","call_timeout_seconds":0.1},"bad":{"transport":"stdio","command":"bad"}}}`
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestHostOwnsIsolatedSessionsBorrowersAndGeneration(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	hostConfig(t, work, "old")
	var clients []*fakeClient
	connector := func(ctx context.Context, s mcpconfig.Server) (Client, error) {
		if s.Name == "bad" {
			return nil, errors.New("failed")
		}
		f := &fakeClient{}
		clients = append(clients, f)
		return f, nil
	}
	h, err := New(context.Background(), home, work, connector)
	if err != nil {
		t.Fatal(err)
	}
	other, err := New(context.Background(), home, work, connector)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close(context.Background())
	if len(clients) != 2 || len(h.Capabilities()) != 2 {
		t.Fatalf("isolation/catalog: %d %#v", len(clients), h.Capabilities())
	}
	borrowed := h.Borrow()
	if err := borrowed.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if clients[0].closed.Load() != 0 {
		t.Fatal("borrower closed owner")
	}
	if err := borrowed.Reconnect(context.Background(), "browser"); err == nil {
		t.Fatal("borrower can reconnect")
	}
	r := tools.NewRegistry()
	if err := Mount(r, h); err != nil {
		t.Fatal(err)
	}
	name := ToolName("browser", "click")
	if got := r.Execute(context.Background(), schema.ToolCall{ID: "c", Name: name, Arguments: json.RawMessage(`{}`)}); got.IsError {
		t.Fatalf("call: %#v", got)
	}
	hostConfig(t, work, "new")
	if len(clients) != 2 {
		t.Fatal("config edit reconnected")
	}
	if err := h.Reconnect(context.Background(), "browser"); err != nil {
		t.Fatal(err)
	}
	if len(clients) != 3 || clients[0].closed.Load() != 1 {
		t.Fatal("reconnect did not replace owner")
	}
	if got := r.Execute(context.Background(), schema.ToolCall{ID: "old", Name: name}); !got.IsError || !strings.Contains(got.Output, "stale") {
		t.Fatalf("old adapter dispatched: %#v", got)
	}
	if err := h.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.Close(context.Background())
	if clients[2].closed.Load() != 1 {
		t.Fatal("root close not idempotent")
	}
}

func TestServerQueueCancellationDoesNotDispatchAndReconnectDrains(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	hostConfig(t, work, "s")
	f := &fakeClient{started: make(chan struct{}), release: make(chan struct{})}
	h, err := New(context.Background(), home, work, func(context.Context, mcpconfig.Server) (Client, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close(context.Background())
	r := tools.NewRegistry()
	if err := Mount(r, h); err != nil {
		t.Fatal(err)
	}
	name := ToolName("browser", "click")
	done := make(chan struct{})
	go func() { defer close(done); r.Execute(context.Background(), schema.ToolCall{Name: name, ID: "1"}) }()
	<-f.started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := r.Execute(ctx, schema.ToolCall{Name: name, ID: "2"})
	if !got.IsError || f.calls.Load() != 1 {
		t.Fatal("cancelled queued call dispatched")
	}
	reconnected := make(chan error, 1)
	go func() { reconnected <- h.Reconnect(context.Background(), "browser") }()
	select {
	case <-reconnected:
		t.Fatal("reconnect skipped active call")
	case <-time.After(10 * time.Millisecond):
	}
	close(f.release)
	<-done
	if err := <-reconnected; err != nil {
		t.Fatal(err)
	}
}

func TestKnownLinksRequireOriginCapabilityAndUnknownMetadataNeverGrantsPermission(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	hostConfig(t, work, "s")
	f := &fakeClient{result: toolprotocol.ExecutionResult{ContentParts: []schema.ContentPart{{Type: "resource_link", URI: "test://page"}}}}
	h, err := New(context.Background(), home, work, func(context.Context, mcpconfig.Server) (Client, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close(context.Background())
	r := tools.NewRegistry()
	if err := Mount(r, h); err != nil {
		t.Fatal(err)
	}
	name := ToolName("browser", "click")
	helper := ResourceToolName("browser")
	call := schema.ToolCall{Name: helper, Arguments: json.RawMessage(`{"uri":"test://page"}`)}
	if got := r.Execute(context.Background(), call); !got.IsError {
		t.Fatal("unknown link accepted")
	}
	r.Execute(context.Background(), schema.ToolCall{Name: name})
	if got := r.Execute(toolprotocol.WithCapabilities(context.Background(), []string{helper}), call); !got.IsError {
		t.Fatal("read bypassed origin capability")
	}
	if got := r.Execute(toolprotocol.WithCapabilities(context.Background(), []string{name, helper}), call); got.IsError || got.Output != "resource" {
		t.Fatalf("authorized resource failed: %#v", got)
	}
	cross := schema.ToolCall{Name: ResourceToolName("bad"), Arguments: call.Arguments}
	if got := r.Execute(context.Background(), cross); !got.IsError {
		t.Fatal("cross-server link accepted")
	}
	if err := Mount(r, h); err == nil {
		t.Fatal("collision overwrote existing tools")
	}
}

func TestSlowReconnectDoesNotBlockHealthyServer(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	hostConfig(t, work, "s")
	started, release := make(chan struct{}), make(chan struct{})
	var reconnect atomic.Bool
	h, err := New(context.Background(), home, work, func(_ context.Context, s mcpconfig.Server) (Client, error) {
		if s.Name == "browser" && reconnect.Load() {
			close(started)
			<-release
		}
		return &fakeClient{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close(context.Background())
	r := tools.NewRegistry()
	if err := Mount(r, h); err != nil {
		t.Fatal(err)
	}
	reconnect.Store(true)
	done := make(chan error, 1)
	go func() { done <- h.Reconnect(context.Background(), "browser") }()
	<-started
	called := make(chan schema.ToolResult, 1)
	go func() { called <- r.Execute(context.Background(), schema.ToolCall{Name: ToolName("bad", "click")}) }()
	select {
	case result := <-called:
		if result.IsError {
			t.Error(result.Output)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("healthy server blocked by unrelated reconnect")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCloseCancelsActiveCallsAndCleansAllServers(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	hostConfig(t, work, "s")
	var clients []*fakeClient
	h, err := New(context.Background(), home, work, func(context.Context, mcpconfig.Server) (Client, error) {
		f := &fakeClient{started: make(chan struct{}), release: make(chan struct{})}
		clients = append(clients, f)
		return f, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r := tools.NewRegistry()
	if err := Mount(r, h); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Execute(context.Background(), schema.ToolCall{Name: ToolName("browser", "click")})
	}()
	<-clients[1].started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := h.Close(ctx); err != nil {
		t.Fatal(err)
	}
	<-done
	for _, f := range clients {
		if f.closed.Load() != 1 {
			t.Fatal("connection leaked")
		}
	}
}

func TestNamesAndExternalPermissionAssessment(t *testing.T) {
	names := map[string]bool{}
	for _, server := range []string{"a-b", "a_b", "中文", "a"} {
		for _, remote := range []string{"x", "x-y", "x_y", strings.Repeat("z", 200)} {
			name := ToolName(server, remote)
			if len(name) > 64 || names[name] {
				t.Fatal("name collision or overflow")
			}
			names[name] = true
			for _, r := range name {
				if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
					t.Fatal("unsafe provider name")
				}
			}
		}
	}
	a := &adapter{definition: schema.ToolDefinition{Name: "external"}}
	assessment, err := a.AssessPermission(toolpolicy.Context{}, nil)
	if err != nil || assessment.ReadOnly || assessment.Behavior != toolpolicy.BehaviorHumanOnly {
		t.Fatalf("metadata granted permission: %#v %v", assessment, err)
	}
}
func TestUnattendedOAuthStatusIsActionable(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	if err := mcpconfig.Add(home, work, mcpconfig.ProjectScope, "auth", mcpconfig.Definition{Transport: "streamable-http", URL: "http://127.0.0.1:1", Auth: mcpconfig.Auth{Type: "oauth"}}); err != nil {
		t.Fatal(err)
	}
	h, err := New(context.Background(), home, work, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close(context.Background())
	if s := h.Statuses()[0]; s.State != "authorization-required" || !strings.Contains(s.Reason, "login") {
		t.Fatalf("unactionable: %#v", s)
	}
}

type disconnectedClient struct{ fakeClient }

func (f *disconnectedClient) Call(context.Context, string, json.RawMessage) (toolprotocol.ExecutionResult, error) {
	return toolprotocol.ExecutionResult{}, errors.New("response lost")
}
func TestTransportFailureUpdatesVisibleStatusWithoutRetry(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	hostConfig(t, work, "s")
	h, err := New(context.Background(), home, work, func(context.Context, mcpconfig.Server) (Client, error) { return &disconnectedClient{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close(context.Background())
	r := tools.NewRegistry()
	Mount(r, h)
	result := r.Execute(context.Background(), schema.ToolCall{Name: ToolName("browser", "click")})
	if !result.IsError {
		t.Fatal("transport error advertised as success")
	}
	for _, s := range h.Statuses() {
		if s.Name == "browser" && (s.State != "failed" || !strings.Contains(s.Reason, "reconnect")) {
			t.Fatalf("failure invisible: %#v", s)
		}
	}
}
