package slash

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type fakeTool struct {
	name string
}

func (f *fakeTool) Name() string { return f.name }
func (f *fakeTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: f.name, Description: f.name + " desc"}
}
func (f *fakeTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return f.name + ":executed", nil
}

func newBaseRegistry(toolNames ...string) tools.Registry {
	r := tools.NewRegistry()
	for _, n := range toolNames {
		r.Register(&fakeTool{name: n})
	}
	return r
}

func TestFilteredRegistry_GetAvailableTools(t *testing.T) {
	base := newBaseRegistry("a", "b", "c")
	filtered := NewFilteredRegistry(base, []string{"a", "c"})
	defs := filtered.GetAvailableTools()
	if len(defs) != 2 {
		t.Errorf("expected 2 defs, got %d", len(defs))
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["a"] || !names["c"] || names["b"] {
		t.Errorf("filtered defs = %v", names)
	}
}

func TestFilteredRegistry_ExecuteAllowed(t *testing.T) {
	base := newBaseRegistry("a", "b")
	filtered := NewFilteredRegistry(base, []string{"a"})
	res := filtered.Execute(context.Background(), schema.ToolCall{ID: "1", Name: "a", Arguments: json.RawMessage(`{}`)})
	if res.IsError {
		t.Errorf("a should succeed, got error: %s", res.Output)
	}
}

func TestFilteredRegistry_ExecuteBlocked(t *testing.T) {
	base := newBaseRegistry("a", "b")
	filtered := NewFilteredRegistry(base, []string{"a"})
	res := filtered.Execute(context.Background(), schema.ToolCall{ID: "1", Name: "b", Arguments: json.RawMessage(`{}`)})
	if !res.IsError {
		t.Error("b should be blocked")
	}
}

func TestFilteredRegistry_EmptyAllowedList(t *testing.T) {
	base := newBaseRegistry("a", "b")
	filtered := NewFilteredRegistry(base, []string{})
	defs := filtered.GetAvailableTools()
	if len(defs) != 0 {
		t.Errorf("empty allow list should expose 0 tools, got %d", len(defs))
	}
}

func TestFilteredRegistry_NoFilterUsesBase(t *testing.T) {
	base := newBaseRegistry("a", "b")
	filtered := NewFilteredRegistry(base, nil)
	defs := filtered.GetAvailableTools()
	if len(defs) != 2 {
		t.Errorf("nil filter must pass through, got %d", len(defs))
	}
}

func TestFilteredRegistry_IsParallelSafeDelegates(t *testing.T) {
	base := newBaseRegistry("a")
	filtered := NewFilteredRegistry(base, []string{"a"})
	// fakeTool does not implement ParallelSafeTool, so this should be false.
	if filtered.IsParallelSafe("a") {
		t.Error("fakeTool is not parallel-safe")
	}
	// For a filtered-out tool, IsParallelSafe should also be false.
	if filtered.IsParallelSafe("notreal") {
		t.Error("missing tool must not be parallel-safe")
	}
}
