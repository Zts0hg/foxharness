package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Zts0hg/foxharness/internal/agentops"
	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestAgentOpsTaskFactoryRunsTargetProfileWithFreshSessionAndCompatibleArtifacts(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	logDir := t.TempDir()
	homeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("AGENTOPS_TARGET_INSTRUCTION\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "payment.log"), []byte("INFO ok\nERROR timeout\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := session.NewFileStoreWithHome(workDir, homeDir)
	model := &composedAgentOpsProvider{}
	factory := newAgentOpsTaskExecutionFactory(model, workDir, logDir, discardAgentOpsMessenger{}, store, approval.NewStore())
	task := agentops.Task{
		TaskID: "task-1", ChatID: "chat-1", SenderID: "sender-1", MessageID: "message-1", Text: "/new inspect payment",
	}
	request := agentops.TaskExecutionRequest{Task: task, Prompt: agentops.BuildPrompt(task)}

	prepared, err := factory.PrepareTask(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Session.ID == "" {
		t.Fatal("prepared session identity is empty")
	}
	if _, err := os.Stat(filepath.Join(prepared.Session.Directory, "working_memory.md")); err != nil {
		t.Fatalf("working_memory.md missing after task preparation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(prepared.Session.Directory, "PLAN.md")); !os.IsNotExist(err) {
		t.Fatalf("PLAN.md exists before application start: %v", err)
	}
	application, err := prepared.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(prepared.Session.Directory, "PLAN.md")); err != nil {
		t.Fatalf("PLAN.md missing after application start: %v", err)
	}
	outcome, err := application.Run(ctx, app.RunCommand{Prompt: request.Prompt}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome == nil || outcome.FinalMessage != "incident resolved" || outcome.SessionID != prepared.Session.ID || outcome.RunID == "" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if err := application.Drain(ctx); err != nil {
		t.Fatalf("Drain() error = %v", err)
	}

	stored, err := store.Open(session.ID(prepared.Session.ID))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Source != session.SOURCEFeishu || stored.UserID != "sender-1" || stored.ChatID != "chat-1" || stored.WorkDir != workDir {
		t.Fatalf("stored session = %#v", stored)
	}
	records, err := session.NewMessageLog(stored).LoadRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 || records[0].Message.Content != request.Prompt || records[1].Message.ToolCalls[0].Name != "log_search" || !strings.Contains(records[2].Message.Content, "ERROR timeout") || records[3].Message.Content != "incident resolved" {
		t.Fatalf("persisted runtime records = %#v", records)
	}
	for _, path := range []string{prepared.Session.TranscriptPath, outcome.MetricsPath, outcome.TracePath} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("runtime artifact %s = %#v, %v", path, info, err)
		}
	}
	messages, definitions := model.firstAgentRequest()
	if model.protocolCalls.Load() != 1 || model.modelCalls.Load() != 1 {
		t.Fatalf("provider metadata calls = protocol %d/model %d, want one frozen snapshot", model.protocolCalls.Load(), model.modelCalls.Load())
	}
	if !agentOpsMessagesContain(messages, "AGENTOPS_TARGET_INSTRUCTION") || !agentOpsMessagesContain(messages, request.Prompt) {
		t.Fatalf("model context = %#v", messages)
	}
	wantTools := []string{"bash", "delegate_task", "edit_file", "log_search", "read_file", "read_todo", "update_todo", "write_file"}
	if got := agentOpsDefinitionNames(definitions); !reflect.DeepEqual(got, wantTools) {
		t.Fatalf("tool definitions = %v, want %v", got, wantTools)
	}

	second, err := factory.PrepareTask(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Session.ID == prepared.Session.ID {
		t.Fatalf("AgentOps reused task session %s", second.Session.ID)
	}
}

func TestAgentOpsApplicationPermissionApproverMapsTaskAndRunCorrelation(t *testing.T) {
	var captured app.PermissionRequest
	approver := agentOpsApplicationPermissionApprover{
		taskID: "task-1",
		port: agentOpsPermissionPortFunc(func(_ context.Context, request app.PermissionRequest) (app.PermissionResponse, error) {
			captured = request
			return app.PermissionResponse{
				CorrelationID: request.Correlation.ID, Decision: app.PermissionDenyWithFeedback, Feedback: "collect more evidence",
			}, nil
		}),
	}
	ctx := tools.WithRunContext(context.Background(), "session-1", "run-1")
	decision, err := approver.Approve(ctx, permission.ApprovalRequest{
		Request: permission.Request{
			ToolCall: schema.ToolCall{ID: "call-1", Name: "write_file"},
			ToolName: "write_file", Arguments: `{"path":"x"}`, Action: "write x", Risk: permission.RiskHigh,
			Source: permission.SourceMain, CWD: "/work", Workspace: "/work",
			Capabilities: toolpolicy.Assessment{Effects: []toolpolicy.Effect{toolpolicy.EffectMutate}},
		},
		Review: &permission.ReviewResult{Rationale: "incident change"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != permission.UserDenyFeedback || decision.Feedback != "collect more evidence" {
		t.Fatalf("permission decision = %#v", decision)
	}
	if captured.Correlation.ID != "call-1" || captured.Correlation.SessionID != "session-1" || captured.Correlation.RunID != "run-1" || captured.Correlation.ToolCallID != "call-1" {
		t.Fatalf("permission correlation = %#v", captured.Correlation)
	}
	if !reflect.DeepEqual(captured.Effects, []string{"mutate"}) || captured.ReviewerReason != "incident change" {
		t.Fatalf("permission request = %#v", captured)
	}
}

func TestSnapshotAgentOpsProviderPreservesMetadataExactly(t *testing.T) {
	model := &exactAgentOpsMetadataProvider{}
	snapshot := snapshotAgentOpsProvider(model)
	if snapshot.ProviderProtocol() != " Scripted-V2 " || snapshot.ModelName() != "model-exact" {
		t.Fatalf("provider snapshot = protocol %q/model %q", snapshot.ProviderProtocol(), snapshot.ModelName())
	}
	if model.protocolCalls.Load() != 1 || model.modelCalls.Load() != 1 {
		t.Fatalf("provider metadata calls = protocol %d/model %d", model.protocolCalls.Load(), model.modelCalls.Load())
	}
}

type composedAgentOpsProvider struct {
	mu            sync.Mutex
	agentRun      int
	requests      [][]schema.Message
	tools         [][]schema.ToolDefinition
	protocolCalls atomic.Int32
	modelCalls    atomic.Int32
}

type exactAgentOpsMetadataProvider struct {
	protocolCalls atomic.Int32
	modelCalls    atomic.Int32
}

func (*exactAgentOpsMetadataProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return nil, nil
}

func (p *exactAgentOpsMetadataProvider) ProviderProtocol() string {
	p.protocolCalls.Add(1)
	return " Scripted-V2 "
}

func (p *exactAgentOpsMetadataProvider) ModelName() string {
	p.modelCalls.Add(1)
	return "model-exact"
}

func (p *composedAgentOpsProvider) Generate(_ context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	if len(definitions) == 0 {
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: `{}`}}, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, append([]schema.Message(nil), messages...))
	p.tools = append(p.tools, append([]schema.ToolDefinition(nil), definitions...))
	p.agentRun++
	if p.agentRun == 1 {
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{
			ID: "call-1", Name: "log_search", Arguments: []byte(`{"service":"payment","query":"ERROR","limit":5}`),
		}}}}, nil
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "incident resolved"}}, nil
}

func (p *composedAgentOpsProvider) ProviderProtocol() string {
	p.protocolCalls.Add(1)
	return "scripted"
}

func (p *composedAgentOpsProvider) ModelName() string {
	p.modelCalls.Add(1)
	return "claude-4-sonnet"
}

func (p *composedAgentOpsProvider) firstAgentRequest() ([]schema.Message, []schema.ToolDefinition) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]schema.Message(nil), p.requests[0]...), append([]schema.ToolDefinition(nil), p.tools[0]...)
}

type discardAgentOpsMessenger struct{}

func (discardAgentOpsMessenger) SendText(context.Context, string, string) error { return nil }

type agentOpsPermissionPortFunc func(context.Context, app.PermissionRequest) (app.PermissionResponse, error)

func (f agentOpsPermissionPortFunc) RequestPermission(ctx context.Context, request app.PermissionRequest) (app.PermissionResponse, error) {
	return f(ctx, request)
}

func agentOpsDefinitionNames(definitions []schema.ToolDefinition) []string {
	names := make([]string, len(definitions))
	for index, definition := range definitions {
		names[index] = definition.Name
	}
	sort.Strings(names)
	return names
}

func agentOpsMessagesContain(messages []schema.Message, fragment string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, fragment) {
			return true
		}
	}
	return false
}
