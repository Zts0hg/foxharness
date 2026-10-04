package mcpintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Zts0hg/foxharness/internal/app"
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
