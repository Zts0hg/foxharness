package skilltool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/slash"
)

func newRegistryWithSkill(t *testing.T, c *slash.Command) *slash.Registry {
	t.Helper()
	r := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	r.Register(c)
	return r
}

func TestSkillTool_Name(t *testing.T) {
	tool := NewSkillTool(slash.NewRegistry(t.TempDir()).WithoutDiscovery(), slash.NewExecutor(), func() string { return "" })
	if tool.Name() != "skill" {
		t.Errorf("Name() = %q", tool.Name())
	}
}

func TestSkillTool_Definition(t *testing.T) {
	tool := NewSkillTool(slash.NewRegistry(t.TempDir()).WithoutDiscovery(), slash.NewExecutor(), func() string { return "" })
	def := tool.Definition()
	if def.Name != "skill" {
		t.Errorf("Definition.Name = %q", def.Name)
	}
	if def.Description == "" {
		t.Error("Definition.Description should be non-empty")
	}
	schema, ok := def.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatalf("InputSchema = %T, want map", def.InputSchema)
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok || props["name"] == nil || props["arguments"] == nil {
		t.Errorf("schema properties missing name/arguments: %+v", schema)
	}
}

func TestSkillTool_Execute_Valid(t *testing.T) {
	cmd := &slash.Command{
		Type:        slash.CommandPrompt,
		Name:        "review",
		Content:     "Review: $ARGUMENTS",
		Frontmatter: slash.Frontmatter{UserInvocable: true},
	}
	tool := NewSkillTool(newRegistryWithSkill(t, cmd), slash.NewExecutor(), func() string { return "sess-1" })
	args, _ := json.Marshal(map[string]string{"name": "review", "arguments": "pr-9"})
	got, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "Review: pr-9" {
		t.Errorf("got %q", got)
	}
}

func TestSkillTool_Execute_UnknownSkill(t *testing.T) {
	tool := NewSkillTool(slash.NewRegistry(t.TempDir()).WithoutDiscovery(), slash.NewExecutor(), func() string { return "" })
	args, _ := json.Marshal(map[string]string{"name": "nope", "arguments": ""})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
}

func TestSkillTool_Execute_DisableModelInvocation(t *testing.T) {
	cmd := &slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "internal",
		Content: "x",
		Frontmatter: slash.Frontmatter{
			UserInvocable:          true,
			DisableModelInvocation: true,
		},
	}
	tool := NewSkillTool(newRegistryWithSkill(t, cmd), slash.NewExecutor(), func() string { return "" })
	args, _ := json.Marshal(map[string]string{"name": "internal", "arguments": ""})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for disable-model-invocation skill")
	}
	if !strings.Contains(err.Error(), "model-invocable") {
		t.Errorf("error should mention model-invocable: %v", err)
	}
}

func TestSkillTool_Execute_UserInvocableFalseStillModelInvocable(t *testing.T) {
	cmd := &slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "helper",
		Content: "helper body",
		Frontmatter: slash.Frontmatter{
			UserInvocable:          false,
			DisableModelInvocation: false,
		},
	}
	tool := NewSkillTool(newRegistryWithSkill(t, cmd), slash.NewExecutor(), func() string { return "" })
	args, _ := json.Marshal(map[string]string{"name": "helper", "arguments": ""})
	got, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "helper body") {
		t.Errorf("got %q", got)
	}
}

func TestSkillTool_Execute_BuiltinNotInvocable(t *testing.T) {
	cmd := &slash.Command{
		Type:        slash.CommandBuiltin,
		Name:        "help",
		Frontmatter: slash.Frontmatter{UserInvocable: true},
	}
	tool := NewSkillTool(newRegistryWithSkill(t, cmd), slash.NewExecutor(), func() string { return "" })
	args, _ := json.Marshal(map[string]string{"name": "help", "arguments": ""})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("builtins must not be model-invocable")
	}
}
