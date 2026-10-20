package mcpintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/mcpcmd"
	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"strings"
	"testing"
)

func TestManagementScopesProbeCleanupAndNoCrossProcessReconnect(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	var clients []*fakeClient
	manager := NewManager(home, work, nil)
	manager.Connector = func(context.Context, mcpconfig.Server) (Client, error) {
		f := &fakeClient{}
		clients = append(clients, f)
		return f, nil
	}
	ctx := context.Background()
	for _, scope := range []string{"user", "project"} {
		_, err := manager.ExecuteMCP(ctx, app.MCPCommand{Action: "add", Name: "browser", Scope: scope, Definition: json.RawMessage(`{"transport":"stdio","command":"fixture"}`)}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := manager.ExecuteMCP(ctx, app.MCPCommand{Action: "status"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Message, "probe") || len(result.Servers) != 1 || result.Servers[0].Scope != "project" || result.Servers[0].State != "ready" || clients[0].closed.Load() != 1 {
		t.Fatalf("probe: %#v", result)
	}
	if _, err := manager.ExecuteMCP(ctx, app.MCPCommand{Action: "reconnect", Name: "browser"}, nil); err == nil {
		t.Fatal("cross-process reconnect claimed")
	}
	if _, err := manager.ExecuteMCP(ctx, app.MCPCommand{Action: "disable", Name: "browser"}, nil); err == nil {
		t.Fatal("implicit mutation scope")
	}
	if _, err := manager.ExecuteMCP(ctx, app.MCPCommand{Action: "disable", Name: "browser", Scope: "project"}, nil); err != nil {
		t.Fatal(err)
	}
	result, err = manager.ExecuteMCP(ctx, app.MCPCommand{Action: "status"}, nil)
	if err != nil || result.Servers[0].State != "disabled" {
		t.Fatalf("disabled: %#v %v", result, err)
	}
}

func TestListRedactsEffectiveConfigurationWithoutConnecting(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	for _, scope := range []mcpconfig.Scope{mcpconfig.UserScope, mcpconfig.ProjectScope} {
		if err := mcpconfig.Add(home, work, scope, "browser", mcpconfig.Definition{Transport: "http", URL: "https://example.invalid/mcp?token=secret-query", Headers: map[string]string{"X-Key": "secret-header"}}); err != nil {
			t.Fatal(err)
		}
	}
	m := NewManager(home, work, nil)
	m.Connector = func(context.Context, mcpconfig.Server) (Client, error) {
		t.Fatal("list opened connection")
		return nil, nil
	}
	r, err := m.ExecuteMCP(context.Background(), app.MCPCommand{Action: "list"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	display := mcpcmd.Format(r)
	if !strings.Contains(display, "project") || !strings.Contains(display, "browser") || !strings.Contains(display, "https://example.invalid/mcp") || strings.Contains(display, "secret-") {
		t.Fatalf("incorrect effective list: %s", display)
	}
}

func TestNamedStatusSelectsOneServerAndReportsUnknownName(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	hostConfig(t, work, "fixture")
	m := NewManager(home, work, nil)
	calls := 0
	m.Connector = func(_ context.Context, s mcpconfig.Server) (Client, error) {
		calls++
		if s.Name != "browser" {
			t.Errorf("unrelated server probed: %s", s.Name)
		}
		return &fakeClient{}, nil
	}
	r, err := m.ExecuteMCP(context.Background(), app.MCPCommand{Action: "status", Name: "browser"}, nil)
	if err != nil || len(r.Servers) != 1 || r.Servers[0].Name != "browser" || calls != 1 {
		t.Fatalf("named status: %+v %v calls=%d", r, err, calls)
	}
	if _, err := m.ExecuteMCP(context.Background(), app.MCPCommand{Action: "status", Name: "missing"}, nil); err == nil {
		t.Fatal("unknown server hidden")
	}
}

type cleanupFailure struct{ fakeClient }

func (*cleanupFailure) Close() error { return fmt.Errorf("cleanup failed") }
func TestProbeReportsCleanupFailure(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	hostConfig(t, work, "s")
	manager := NewManager(home, work, nil)
	manager.Connector = func(context.Context, mcpconfig.Server) (Client, error) { return &cleanupFailure{}, nil }
	if _, err := manager.ExecuteMCP(context.Background(), app.MCPCommand{Action: "status"}, nil); err == nil {
		t.Fatal("probe cleanup failure hidden")
	}
}
