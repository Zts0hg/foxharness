package tools

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
)

func TestRegistryExecutionCarriesRunAndExactToolCallIdentity(t *testing.T) {
	tool := &invocationCaptureTool{name: "capture", contexts: make(chan InvocationContext, 1)}
	registry := NewRegistry()
	registry.Register(tool)
	ctx := WithRunContext(context.Background(), "session-1", "run-1")

	result := registry.Execute(ctx, schema.ToolCall{ID: "call-1", Name: "capture", Arguments: json.RawMessage(`{}`)})
	if result.IsError {
		t.Fatalf("Execute() = %#v", result)
	}
	got := <-tool.contexts
	want := InvocationContext{SessionID: "session-1", RunID: "run-1", ToolCallID: "call-1"}
	if got != want {
		t.Fatalf("invocation context = %#v, want %#v", got, want)
	}
	parent, ok := InvocationContextFrom(ctx)
	if !ok || parent.ToolCallID != "" {
		t.Fatalf("registry mutated parent context = %#v/%t", parent, ok)
	}
}

func TestFilteredRegistryCarriesItsNarrowCapabilitySnapshot(t *testing.T) {
	capabilities := make(chan []string, 1)
	base := NewRegistry()
	base.Register(&invocationCaptureTool{name: "capture", contexts: make(chan InvocationContext, 1), capabilities: capabilities})
	base.Register(&invocationCaptureTool{name: "other", contexts: make(chan InvocationContext, 1)})
	filtered := NewFilteredRegistry(base, []string{"capture"})
	result := filtered.Execute(context.Background(), schema.ToolCall{ID: "call", Name: "capture", Arguments: json.RawMessage(`{}`)})
	if result.IsError {
		t.Fatalf("Execute() = %#v", result)
	}
	got := <-capabilities
	if len(got) != 1 || got[0] != "capture" {
		t.Fatalf("effective tool snapshot = %v, want [capture]", got)
	}
}

func TestParallelRegistryExecutionsKeepToolCallIdentityIsolated(t *testing.T) {
	tool := &invocationCaptureTool{name: "capture", contexts: make(chan InvocationContext, 2)}
	registry := NewRegistry()
	registry.Register(tool)
	ctx := WithRunContext(context.Background(), "session-1", "run-1")
	var wait sync.WaitGroup
	for _, id := range []string{"call-a", "call-b"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			registry.Execute(ctx, schema.ToolCall{ID: id, Name: "capture", Arguments: json.RawMessage(`{}`)})
		}()
	}
	wait.Wait()
	seen := map[string]bool{}
	for range 2 {
		got := <-tool.contexts
		if got.SessionID != "session-1" || got.RunID != "run-1" {
			t.Fatalf("parallel invocation lost run lineage: %#v", got)
		}
		seen[got.ToolCallID] = true
	}
	if !seen["call-a"] || !seen["call-b"] || len(seen) != 2 {
		t.Fatalf("parallel call identities = %v", seen)
	}
}

type invocationCaptureTool struct {
	name         string
	contexts     chan InvocationContext
	capabilities chan []string
}

func (t *invocationCaptureTool) Name() string { return t.name }

func (t *invocationCaptureTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: t.name, InputSchema: map[string]interface{}{"type": "object"}}
}

func (t *invocationCaptureTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	invocation, _ := InvocationContextFrom(ctx)
	t.contexts <- invocation
	if t.capabilities != nil {
		t.capabilities <- toolprotocol.CapabilitiesFromContext(ctx)
	}
	return "ok", nil
}
