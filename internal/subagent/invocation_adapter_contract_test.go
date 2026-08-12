package subagent

import (
	"context"
	"encoding/json"
	"testing"

	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestIACHD004DelegateCarriesParentRunAndToolCallLineage(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	childManager := NewManager(&finalReportProvider{}, workDir)
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
