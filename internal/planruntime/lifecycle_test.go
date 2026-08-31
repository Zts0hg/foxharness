package planruntime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestLifecycleMovesFormalApprovalAndChecklistAtTurnBoundaries(t *testing.T) {
	formal := tools.NewRegistry()
	checklist := tools.NewRegistry()
	defaults := tools.NewRegistry()
	formal.Register(namedPlanTool{name: "submit_plan", execute: func() {}})
	checklist.Register(namedPlanTool{name: "update_todo", execute: func() {}})
	defaults.Register(namedPlanTool{name: "write_file", execute: func() {}})
	lifecycle := New(formal, checklist, defaults, nil)

	if !hasLifecycleTool(lifecycle, "submit_plan") || lifecycle.MemoryExtractionAllowed() {
		t.Fatalf("initial lifecycle state is not Formal Plan")
	}
	lifecycle.Approve("# Plan")
	if !hasLifecycleTool(lifecycle, "submit_plan") {
		t.Fatal("approval changed tools before the next turn")
	}
	lifecycle.BeginTurn()
	if !hasLifecycleTool(lifecycle, "update_todo") || len(lifecycle.RuntimeReminders()) != 1 {
		t.Fatal("approved lifecycle did not enter checklist phase")
	}
	lifecycle.Execute(context.Background(), schema.ToolCall{ID: "todo", Name: "update_todo", Arguments: json.RawMessage(`{}`)})
	if !hasLifecycleTool(lifecycle, "update_todo") {
		t.Fatal("checklist changed tools before the next turn")
	}
	lifecycle.BeginTurn()
	if !hasLifecycleTool(lifecycle, "write_file") || lifecycle.CompletionReminder() != "" || !lifecycle.MemoryExtractionAllowed() {
		t.Fatal("lifecycle did not enter default implementation phase")
	}
}

type namedPlanTool struct {
	name    string
	execute func()
}

func (t namedPlanTool) Name() string                      { return t.name }
func (t namedPlanTool) Definition() schema.ToolDefinition { return schema.ToolDefinition{Name: t.name} }
func (t namedPlanTool) Execute(context.Context, json.RawMessage) (string, error) {
	if t.execute != nil {
		t.execute()
	}
	return "ok", nil
}

func hasLifecycleTool(registry tools.Registry, name string) bool {
	for _, definition := range registry.GetAvailableTools() {
		if definition.Name == name {
			return true
		}
	}
	return false
}
