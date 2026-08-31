package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
)

type filteredAliasCapability struct{}

func (*filteredAliasCapability) Name() string       { return "inspect" }
func (*filteredAliasCapability) Aliases() []string  { return []string{"Inspect"} }
func (*filteredAliasCapability) ParallelSafe() bool { return true }
func (*filteredAliasCapability) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: "inspect", Description: "inspect"}
}
func (*filteredAliasCapability) Execute(context.Context, json.RawMessage) (string, error) {
	return "inspected", nil
}
func (*filteredAliasCapability) AssessPermission(toolpolicy.Context, json.RawMessage) (toolpolicy.Assessment, error) {
	return toolpolicy.Assessment{Action: "inspect", ReadOnly: true}, nil
}

type countingTurnRegistry struct {
	Registry
	turns int
}

func (r *countingTurnRegistry) BeginTurn() {
	r.turns++
}

func TestFilteredRegistryDelegatesBeginTurn(t *testing.T) {
	base := &countingTurnRegistry{Registry: NewRegistry()}
	filtered := NewFilteredRegistry(base, []string{"read_file"})
	turnAware, ok := filtered.(TurnAwareRegistry)
	if !ok {
		t.Fatalf("filtered registry type %T does not implement TurnAwareRegistry", filtered)
	}

	turnAware.BeginTurn()
	turnAware.BeginTurn()
	if base.turns != 2 {
		t.Fatalf("base BeginTurn calls = %d, want 2", base.turns)
	}
}

func TestFilteredRegistryCanonicalCeilingRetainsCapabilityAliases(t *testing.T) {
	base := NewRegistry()
	base.Register(&filteredAliasCapability{})
	filtered := NewFilteredRegistry(base, []string{"inspect"})

	definitions := filtered.GetAvailableTools()
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	if !reflect.DeepEqual(names, []string{"Inspect", "inspect"}) {
		t.Fatalf("filtered definitions = %v, want canonical capability and alias", names)
	}
	result := filtered.Execute(context.Background(), schema.ToolCall{ID: "alias", Name: "Inspect"})
	if result.IsError || result.Output != "inspected" {
		t.Fatalf("filtered alias execution = %#v", result)
	}
	if !filtered.IsParallelSafe("Inspect") {
		t.Fatal("filtered alias lost parallel-safety metadata")
	}
	permissionRegistry, ok := filtered.(PermissionRegistry)
	if !ok {
		t.Fatalf("filtered registry type %T has no permission metadata", filtered)
	}
	assessment, found, err := permissionRegistry.AssessPermission("Inspect", toolpolicy.Context{}, nil)
	if err != nil || !found || !assessment.ReadOnly {
		t.Fatalf("filtered alias assessment = %+v/%t/%v", assessment, found, err)
	}
}
