package subagent

import (
	"context"
	"encoding/json"
	"testing"

	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestIACHD004DelegateCarriesParentRunAndToolCallLineage(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	childManager := childManagerWithTestPermission(NewManager(&finalReportProvider{}, workDir))
	var childOptions session.CreateOptions
	childManager.createSession = func(options session.CreateOptions) (*session.Session, error) {
		childOptions = options
		return session.NewManagerWithHome(workDir, homeDir).Create(options)
	}
	registry := tools.NewRegistry()
	registry.Register(NewTool(childManager, "parent-session"))
	parentSession, err := session.NewManagerWithHome(workDir, homeDir).Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	parent := engine.NewAgentEngine(
		&delegateOnceProvider{},
		registry,
		workDir,
		prompt.NewComposer(workDir),
		engine.Config{MaxTurns: 2},
	)
	result, err := parent.Run(context.Background(), parentSession, "delegate once")
	if err != nil {
		t.Fatal(err)
	}
	if childOptions.ParentRunID != result.RunID || childOptions.DelegationID != "delegate-call" {
		t.Fatalf("delegate child lineage = parent run %q delegation %q, want %q/%q", childOptions.ParentRunID, childOptions.DelegationID, result.RunID, "delegate-call")
	}
}

func childManagerWithTestPermission(manager *Manager) *Manager {
	return manager.WithPermission(permission.NewCoordinator(permission.Config{
		State: permission.NewState(permission.ModeFullAccess, true),
	}))
}

func TestIACHD003DelegateExecutionFailsClosedWithoutChildPermissionCoordinator(t *testing.T) {
	manager := NewManager(&finalReportProvider{}, t.TempDir())
	createCalled := false
	manager.createSession = func(session.CreateOptions) (*session.Session, error) {
		createCalled = true
		return nil, nil
	}
	tool := NewTool(manager, "parent")
	raw := json.RawMessage(`{"task":"inspect","read_only":true}`)
	assessment, err := tool.AssessPermission(toolpolicy.Context{}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Behavior != toolpolicy.BehaviorHumanOnly {
		t.Fatalf("assessment behavior = %q, want human_only", assessment.Behavior)
	}
	if _, err := tool.Execute(context.Background(), raw); err == nil {
		t.Fatal("delegate execution without child permission coordinator succeeded")
	}
	if createCalled {
		t.Fatal("fail-closed delegate created a child session")
	}
}

type delegateOnceProvider struct {
	calls int
}

func (p *delegateOnceProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	if p.calls == 1 {
		return &provider.GenerateResponse{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "delegate-call",
				Name:      "delegate_task",
				Arguments: json.RawMessage(`{"task":"inspect","read_only":true}`),
			}},
		}}, nil
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "parent done"}}, nil
}
