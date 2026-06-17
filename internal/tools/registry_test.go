package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// aliasStubTool is a minimal BaseTool plus AliasableTool used to exercise the
// registry's alias handling without depending on a real tool implementation.
type aliasStubTool struct {
	name    string
	aliases []string
	out     string
}

func (t *aliasStubTool) Name() string { return t.name }
func (t *aliasStubTool) Aliases() []string {
	return t.aliases
}
func (t *aliasStubTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.name,
		Description: "alias stub",
		InputSchema: map[string]interface{}{"type": "object"},
	}
}
func (t *aliasStubTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.out, nil
}

// availableToolNames collects the advertised tool names into a set for
// membership assertions that do not depend on map iteration order.
func availableToolNames(t *testing.T, r Registry) map[string]bool {
	t.Helper()
	names := make(map[string]bool)
	for _, d := range r.GetAvailableTools() {
		names[d.Name] = true
	}
	return names
}

func TestRegistryAdvertisesAliasInAvailableTools(t *testing.T) {
	r := NewRegistry()
	r.Register(&aliasStubTool{name: "ask_user_question", aliases: []string{"AskUserQuestion"}, out: "ok"})

	names := availableToolNames(t, r)
	if !names["ask_user_question"] {
		t.Errorf("canonical name missing from available tools: %v", names)
	}
	if !names["AskUserQuestion"] {
		t.Errorf("alias name missing from available tools: %v", names)
	}
}

func TestRegistryExecutesViaAliasAndCanonical(t *testing.T) {
	r := NewRegistry()
	r.Register(&aliasStubTool{name: "ask_user_question", aliases: []string{"AskUserQuestion"}, out: "answered"})

	for _, name := range []string{"ask_user_question", "AskUserQuestion"} {
		res := r.Execute(context.Background(), schema.ToolCall{ID: "1", Name: name})
		if res.IsError {
			t.Fatalf("Execute(%q) returned error: %s", name, res.Output)
		}
		if res.Output != "answered" {
			t.Fatalf("Execute(%q) = %q, want %q", name, res.Output, "answered")
		}
	}
}

func TestRegistryUnknownToolStillErrors(t *testing.T) {
	r := NewRegistry()
	res := r.Execute(context.Background(), schema.ToolCall{ID: "1", Name: "no_such_tool"})
	if !res.IsError {
		t.Fatal("expected error result for an unknown tool name")
	}
}

func TestRegistryIsParallelSafeViaAlias(t *testing.T) {
	r := NewRegistry()
	r.Register(&parallelAliasStub{name: "p", aliases: []string{"P"}})

	if !r.IsParallelSafe("p") {
		t.Fatal("canonical name should report parallel-safe")
	}
	if !r.IsParallelSafe("P") {
		t.Fatal("alias name should report parallel-safe")
	}
}

// parallelAliasStub implements both AliasableTool and ParallelSafeTool so the
// registry's alias resolution can be exercised through IsParallelSafe.
type parallelAliasStub struct {
	name    string
	aliases []string
}

func (t *parallelAliasStub) Name() string { return t.name }
func (t *parallelAliasStub) Aliases() []string {
	return t.aliases
}
func (t *parallelAliasStub) ParallelSafe() bool { return true }
func (t *parallelAliasStub) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: t.name, Description: "parallel alias stub"}
}
func (t *parallelAliasStub) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return "", nil
}
