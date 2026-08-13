package subagent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
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

func TestIACHD001DelegateSchemaAndInputDomainRemainNarrow(t *testing.T) {
	tool := NewTool(childManagerWithTestPermission(NewManager(&finalReportProvider{}, t.TempDir())), "parent")
	definition := tool.Definition()
	if definition.Name != "delegate_task" || !strings.Contains(definition.Description, "Subagent") {
		t.Fatalf("delegate definition identity = %#v", definition)
	}
	schemaMap, ok := definition.InputSchema.(map[string]interface{})
	if !ok || schemaMap["type"] != "object" {
		t.Fatalf("delegate schema = %#v", definition.InputSchema)
	}
	properties, ok := schemaMap["properties"].(map[string]interface{})
	if !ok || len(properties) != 2 {
		t.Fatalf("delegate properties = %#v", schemaMap["properties"])
	}
	if properties["task"].(map[string]interface{})["type"] != "string" || properties["read_only"].(map[string]interface{})["type"] != "boolean" {
		t.Fatalf("delegate property types = %#v", properties)
	}
	if required := schemaMap["required"].([]string); !reflect.DeepEqual(required, []string{"task"}) {
		t.Fatalf("delegate required fields = %v", required)
	}
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{`),
		json.RawMessage(`{}`),
		json.RawMessage(`{"task":false}`),
		json.RawMessage(`{"task":"   "}`),
		json.RawMessage(`{"task":"inspect","read_only":"yes"}`),
	} {
		if _, err := tool.Execute(context.Background(), raw); err == nil {
			t.Fatalf("delegate accepted invalid input %s", raw)
		}
	}
}

func TestIACHD002And003AssessmentMatchesNormalizedExecution(t *testing.T) {
	tests := []struct {
		name           string
		raw            json.RawMessage
		wantReadOnly   bool
		wantRisk       toolpolicy.Risk
		wantTools      []string
		wantActionPart string
	}{
		{
			name:         "default read only and ignored expansion fields",
			raw:          json.RawMessage(`{"task":"  inspect files  ","model":"other","depth":2,"max_turns":999,"allowed_tools":["delegate_task"]}`),
			wantReadOnly: true, wantRisk: toolpolicy.RiskLow, wantTools: []string{"bash", "read_file"}, wantActionPart: "task=inspect files",
		},
		{
			name:         "explicit writable",
			raw:          json.RawMessage(`{"task":"  edit one file  ","read_only":false}`),
			wantReadOnly: false, wantRisk: toolpolicy.RiskMedium, wantTools: []string{"bash", "edit_file", "read_file", "write_file"}, wantActionPart: "task=edit one file",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			provider := &childDefinitionCaptureProvider{}
			tool := NewTool(childManagerWithTestPermission(NewManager(provider, t.TempDir())), "parent")
			assessment, err := tool.AssessPermission(toolpolicy.Context{}, test.raw)
			if err != nil {
				t.Fatal(err)
			}
			if assessment.Behavior != toolpolicy.BehaviorReviewable || assessment.ReadOnly != test.wantReadOnly || assessment.RiskHint != test.wantRisk || !assessment.NestedEnforcement || !strings.Contains(assessment.Action, test.wantActionPart) {
				t.Fatalf("delegate assessment = %#v", assessment)
			}
			output, err := tool.Execute(context.Background(), test.raw)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(output, "Subagent Session: ") || !strings.HasSuffix(output, "\n\nReport:\ndone") {
				t.Fatalf("delegate success output = %q", output)
			}
			if !reflect.DeepEqual(provider.toolNames, test.wantTools) {
				t.Fatalf("delegate executable tools = %v, want %v", provider.toolNames, test.wantTools)
			}
		})
	}
}

func TestIACHD004DelegateCarriesParentRunAndToolCallLineage(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	childManager := childManagerWithTestPermission(NewManager(&finalReportProvider{}, workDir))
	var childOptions session.CreateOptions
	createCalls := 0
	childManager.createSession = func(options session.CreateOptions) (*session.Session, error) {
		createCalls++
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
	parent := engine.NewLegacyEngine(
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
	if childOptions.ParentRunID != session.RunID(result.RunID) || childOptions.DelegationID != "delegate-call" {
		t.Fatalf("delegate child lineage = parent run %q delegation %q, want %q/%q", childOptions.ParentRunID, childOptions.DelegationID, result.RunID, "delegate-call")
	}
	if createCalls != 1 {
		t.Fatalf("delegate child session creations = %d, want exactly one", createCalls)
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
