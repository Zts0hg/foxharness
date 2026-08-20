package registryexec

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestCapabilitiesIntersectDefinitionsAndExecutionWithAllowedNames(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(registryTool{name: "alpha", output: "alpha output", parallel: true})
	registry.Register(registryTool{name: "beta", output: "beta output"})
	registry.Register(registryTool{name: "gamma", output: "gamma output"})

	capabilities := Capabilities(registry, []string{"beta", "alpha", "missing"}, nil)
	if len(capabilities) != 2 || capabilities[0].Definition.Name != "alpha" || capabilities[1].Definition.Name != "beta" {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	if !capabilities[0].ParallelSafe || capabilities[1].ParallelSafe {
		t.Fatalf("parallel flags = %v/%v", capabilities[0].ParallelSafe, capabilities[1].ParallelSafe)
	}
	result := capabilities[0].Execute(context.Background(), schema.ToolCall{ID: "call-1", Name: "alpha"})
	if result.CallID != "call-1" || result.FullContent != "alpha output" || result.ModelContent != "alpha output" || result.ObserverContent != "alpha output" || result.IsError {
		t.Fatalf("execution result = %#v", result)
	}
}

func TestCapabilitiesInvokeResultHookExactlyOnceWithDefensiveValues(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(registryTool{name: "alpha", output: "failed", fail: true})
	var calls []schema.ToolCall
	var results []schema.ToolResult
	capabilities := Capabilities(registry, []string{"alpha"}, func(call schema.ToolCall, result schema.ToolResult) {
		calls = append(calls, call)
		results = append(results, result)
		call.Arguments[0] = 'x'
	})
	original := schema.ToolCall{ID: "call-1", Name: "alpha", Arguments: []byte(`{"value":1}`)}
	got := capabilities[0].Execute(context.Background(), original)
	if got.CallID != "call-1" || !got.IsError {
		t.Fatalf("result = %#v", got)
	}
	if len(calls) != 1 || len(results) != 1 || results[0].ToolCallID != "call-1" || !results[0].IsError {
		t.Fatalf("hook values = %#v/%#v", calls, results)
	}
	if reflect.DeepEqual(calls[0].Arguments, original.Arguments) {
		t.Fatal("hook mutation did not prove a defensive call copy")
	}
	if string(original.Arguments) != `{"value":1}` {
		t.Fatalf("original arguments mutated to %q", original.Arguments)
	}
}

func TestCapabilitiesApplyRunContextBeforeRegistryExecution(t *testing.T) {
	registry := tools.NewRegistry()
	captured := make(chan tools.InvocationContext, 1)
	registry.Register(contextTool{captured: captured})
	capabilities := CapabilitiesWithContext(registry, []string{"capture"}, func(ctx context.Context) context.Context {
		return tools.WithRunContext(ctx, "session-1", "run-2")
	}, nil)
	capabilities[0].Execute(context.Background(), schema.ToolCall{ID: "call-3", Name: "capture"})
	got := <-captured
	if want := (tools.InvocationContext{SessionID: "session-1", RunID: "run-2", ToolCallID: "call-3"}); got != want {
		t.Fatalf("invocation = %#v, want %#v", got, want)
	}
}

type registryTool struct {
	name     string
	output   string
	fail     bool
	parallel bool
}

func (t registryTool) Name() string { return t.name }
func (t registryTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: t.name, InputSchema: map[string]any{"type": "object"}}
}
func (t registryTool) Execute(context.Context, json.RawMessage) (string, error) { return t.output, nil }
func (t registryTool) ExecuteResult(context.Context, json.RawMessage) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{Output: t.output, Failed: t.fail}, nil
}
func (t registryTool) ParallelSafe() bool { return t.parallel }

type contextTool struct {
	captured chan tools.InvocationContext
}

func (contextTool) Name() string { return "capture" }
func (contextTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: "capture", InputSchema: map[string]any{"type": "object"}}
}
func (t contextTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	invocation, _ := tools.InvocationContextFrom(ctx)
	t.captured <- invocation
	return "done", nil
}
