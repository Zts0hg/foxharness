package skilltool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func newRegistryWithSkill(t *testing.T, c *slash.Command) *slash.Registry {
	t.Helper()
	r := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	r.Register(c)
	return r
}

// forkRunnerStub is a no-op slash.ForkRunner used by tests that need a
// fork-mode skill to flow through the executor without spinning up a
// real subagent.Manager.
type forkRunnerStub struct {
	report       string
	allowedTools []string
	err          error
}

func (s *forkRunnerStub) Run(ctx context.Context, task string, agentType string, allowedTools []string) (string, error) {
	s.allowedTools = append([]string(nil), allowedTools...)
	return s.report, s.err
}

func TestSkillToolExecuteRetainsForkPartialOutcomeWithError(t *testing.T) {
	cmd := &slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "inspect",
		Content: "Inspect",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			Context:       "fork",
		},
	}
	registry := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	registry.Register(cmd)
	terminalErr := errors.New("child failed")
	runner := &forkRunnerStub{report: "typed partial outcome", err: terminalErr}
	tool := NewSkillTool(registry, slash.NewExecutor(slash.WithForkRunner(runner)), func() string { return "parent" })
	raw, _ := json.Marshal(map[string]string{"name": "inspect"})

	output, err := tool.Execute(context.Background(), raw)
	if !errors.Is(err, terminalErr) || output != "typed partial outcome" {
		t.Fatalf("skill fork output/error = %q/%v, want retained partial and terminal error", output, err)
	}
}

func TestSkillToolRegistryRetainsForkPartialOutcomeAndFailure(t *testing.T) {
	cmd := &slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "inspect",
		Content: "Inspect",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			Context:       "fork",
		},
	}
	skillRegistry := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	skillRegistry.Register(cmd)
	runner := &forkRunnerStub{report: "typed partial outcome", err: errors.New("child failed")}
	tool := NewSkillTool(skillRegistry, slash.NewExecutor(slash.WithForkRunner(runner)), func() string { return "parent" })
	registry := tools.NewRegistry()
	registry.Register(tool)
	raw, _ := json.Marshal(map[string]string{"name": "inspect"})

	result := registry.Execute(context.Background(), schema.ToolCall{ID: "fork-skill", Name: "skill", Arguments: raw})
	if !result.IsError || !strings.Contains(result.Output, "typed partial outcome") || !strings.Contains(result.Output, "child failed") {
		t.Fatalf("fork skill registry result = %#v, want partial outcome and terminal failure", result)
	}
}

func statFile(p string) (os.FileInfo, error) { return os.Stat(p) }

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

func TestSkillTool_AssessPermissionUsesPureExecutionPlan(t *testing.T) {
	wd := t.TempDir()
	marker := wd + "/must-not-exist"
	cmd := &slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "inspect",
		Content: "Inspect !`touch $ARGUMENTS`",
		Frontmatter: slash.Frontmatter{
			Hooks: &slash.FrontmatterHooks{After: "rm -rf " + marker},
		},
	}
	tool := NewSkillTool(newRegistryWithSkill(t, cmd), slash.NewExecutor(slash.WithWorkDir(wd)), func() string { return "session" })
	args, _ := json.Marshal(map[string]string{"name": "inspect", "arguments": marker})

	assessment, err := tool.AssessPermission(toolpolicy.Context{Workspace: wd, CWD: wd}, args)
	if err != nil {
		t.Fatalf("AssessPermission() error = %v", err)
	}
	if assessment.Behavior != toolpolicy.BehaviorReviewable || assessment.RiskHint != toolpolicy.RiskCritical || assessment.ReadOnly {
		t.Fatalf("assessment = %+v", assessment)
	}
	if len(assessment.Commands) != 2 || assessment.Commands[0] != "touch "+marker || assessment.Commands[1] != "rm -rf "+marker {
		t.Fatalf("commands = %#v", assessment.Commands)
	}
	if !strings.Contains(assessment.Action, "touch "+marker) || !strings.Contains(assessment.Action, "rm -rf "+marker) {
		t.Fatalf("action does not expose planned commands: %q", assessment.Action)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatal("permission assessment executed a planned shell command")
	}
}

func TestSkillTool_AssessPermissionFailsClosedForUnknownSkill(t *testing.T) {
	tool := NewSkillTool(slash.NewRegistry(t.TempDir()).WithoutDiscovery(), slash.NewExecutor(), func() string { return "" })
	assessment, err := tool.AssessPermission(toolpolicy.Context{}, json.RawMessage(`{"name":"missing"}`))
	if err != nil {
		t.Fatalf("AssessPermission() error = %v", err)
	}
	if assessment.Behavior != toolpolicy.BehaviorHumanOnly {
		t.Fatalf("behavior = %q, want human_only", assessment.Behavior)
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

func TestSkillTool_Execute_InlineAllowedToolsRefused(t *testing.T) {
	cmd := &slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "scan",
		Content: "Scan body",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			AllowedTools:  []string{"read_file"},
			// Context defaults to inline — and that's the unsafe combo.
		},
	}
	tool := NewSkillTool(newRegistryWithSkill(t, cmd), slash.NewExecutor(), func() string { return "" })
	args, _ := json.Marshal(map[string]string{"name": "scan", "arguments": ""})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected inline+allowed-tools to be refused for model invocation")
	}
	if !strings.Contains(err.Error(), "context: fork") {
		t.Errorf("refusal must hint at fork mode: %v", err)
	}
}

func TestSkillTool_Execute_InlineExplicitEmptyAllowedToolsRefused(t *testing.T) {
	cmd := &slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "scan",
		Content: "Scan body",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			AllowedTools:  []string{},
			// Context defaults to inline; explicit empty still means a restriction.
		},
	}
	tool := NewSkillTool(newRegistryWithSkill(t, cmd), slash.NewExecutor(), func() string { return "" })
	args, _ := json.Marshal(map[string]string{"name": "scan", "arguments": ""})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected inline+explicit-empty allowed-tools to be refused for model invocation")
	}
	if !strings.Contains(err.Error(), "context: fork") {
		t.Errorf("refusal must hint at fork mode: %v", err)
	}
}

func TestSkillTool_Execute_InlineAllowedToolsRefusedBeforePipeline(t *testing.T) {
	wd := t.TempDir()
	beforeMarker := wd + "/before.touched"
	afterMarker := wd + "/after.touched"
	shellMarker := wd + "/shell.touched"
	cmd := &slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "scan",
		Content: "Scan !`touch " + shellMarker + "`",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			AllowedTools:  []string{"read_file"},
			Hooks: &slash.FrontmatterHooks{
				Before: "touch " + beforeMarker,
				After:  "touch " + afterMarker,
			},
		},
	}
	tool := NewSkillTool(newRegistryWithSkill(t, cmd), slash.NewExecutor(slash.WithWorkDir(wd)), func() string { return "" })
	args, _ := json.Marshal(map[string]string{"name": "scan", "arguments": ""})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected inline+allowed-tools to be refused")
	}
	for _, marker := range []string{beforeMarker, afterMarker, shellMarker} {
		if _, statErr := os.Stat(marker); statErr == nil {
			t.Fatalf("refused inline+allowed-tools skill executed pipeline side effect: %s", marker)
		}
	}
}

func TestSkillTool_Execute_ForkAllowedToolsAccepted(t *testing.T) {
	// Fork-mode skills get enforced inside the sub-agent, so inline-mode
	// refusal does NOT apply.
	cmd := &slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "deploy",
		Content: "Deploy",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			Context:       "fork",
			Agent:         "general-purpose",
			AllowedTools:  []string{"bash"},
		},
	}
	r := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	r.Register(cmd)
	exec := slash.NewExecutor(slash.WithForkRunner(&forkRunnerStub{report: "ok"}))
	tool := NewSkillTool(r, exec, func() string { return "" })
	args, _ := json.Marshal(map[string]string{"name": "deploy", "arguments": ""})
	got, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("fork-mode allowed-tools should be accepted: %v", err)
	}
	if got != "ok" {
		t.Errorf("got %q", got)
	}
}

func TestSkillTool_Execute_AfterHookFiresOnSuccess(t *testing.T) {
	wd := t.TempDir()
	marker := wd + "/skill-after.touched"
	cmd := &slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "ping",
		Content: "ping body",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			Hooks:         &slash.FrontmatterHooks{After: "touch " + marker},
		},
	}
	r := slash.NewRegistry(wd).WithoutDiscovery()
	r.Register(cmd)
	exec := slash.NewExecutor(slash.WithWorkDir(wd))
	tool := NewSkillTool(r, exec, func() string { return "" })
	args, _ := json.Marshal(map[string]string{"name": "ping", "arguments": ""})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, err := statFile(marker); err != nil {
		t.Errorf("after-hook did not fire on accepted model invocation: %v", err)
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
