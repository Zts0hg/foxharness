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
	"time"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/feishu"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestFeishuTaskFactoryRunsTargetProfileWithCompatibleSessionAndArtifacts(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	homeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("FEISHU_TARGET_INSTRUCTION\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "fixture.txt"), []byte("fixture body"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := session.NewFileStoreWithHome(workDir, homeDir)
	model := &composedFeishuProvider{}
	factory := newFeishuTaskExecutionFactory(model, workDir, discardFeishuMessenger{}, store, approval.NewStore())
	request := feishu.TaskExecutionRequest{
		Task:   feishu.Task{TaskID: "task-1", ChatID: "chat-1", SenderID: "sender-1", MessageID: "message-1"},
		Prompt: "以下任务来自飞书用户 sender-1，消息 ID 为 message-1。\n\ninspect",
	}

	prepared, err := factory.PrepareTask(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.Created || prepared.Session.ID == "" {
		t.Fatalf("first prepared session = %#v", prepared)
	}
	if _, err := os.Stat(filepath.Join(prepared.Session.Directory, "working_memory.md")); err != nil {
		t.Fatalf("working_memory.md missing after task preparation: %v", err)
	}
	outcome, err := prepared.Application.Run(ctx, app.RunCommand{Prompt: request.Prompt}, &recordingNotificationSink{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome == nil || outcome.FinalMessage != "target complete" || outcome.SessionID != prepared.Session.ID || outcome.RunID == "" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if err := prepared.Drain(ctx); err != nil {
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
	if len(records) != 4 || records[0].Message.Content != request.Prompt || records[1].Message.ToolCalls[0].Name != "read_file" || records[2].Message.ToolCallID != "call-1" || records[3].Message.Content != "target complete" {
		t.Fatalf("persisted runtime records = %#v", records)
	}
	for _, path := range []string{prepared.Session.TranscriptPath, outcome.MetricsPath, outcome.TracePath} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("runtime artifact %s = %#v, %v", path, info, err)
		}
	}
	requestMessages, definitions := model.firstAgentRequest()
	if model.protocolCalls.Load() != 1 || model.modelCalls.Load() != 1 {
		t.Fatalf("provider metadata calls = protocol %d/model %d, want one frozen snapshot", model.protocolCalls.Load(), model.modelCalls.Load())
	}
	if !messagesContain(requestMessages, "FEISHU_TARGET_INSTRUCTION") || !messagesContain(requestMessages, request.Prompt) {
		t.Fatalf("model context = %#v", requestMessages)
	}
	if got, want := definitionNames(definitions), []string{"bash", "delegate_task", "edit_file", "read_file", "read_todo", "update_todo", "write_file"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tool definitions = %v, want %v", got, want)
	}

	continued, err := factory.PrepareTask(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Created || continued.Session.ID != prepared.Session.ID {
		t.Fatalf("continued session = %#v, first %s", continued, prepared.Session.ID)
	}
	if _, err := continued.Application.Run(ctx, app.RunCommand{Prompt: request.Prompt}, nil); err != nil {
		t.Fatal(err)
	}
	if err := continued.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	request.ForceNewSession = true
	forced, err := factory.PrepareTask(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !forced.Created || forced.Session.ID == prepared.Session.ID {
		t.Fatalf("forced session = %#v, first %s", forced, prepared.Session.ID)
	}
	if err := forced.Drain(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationPermissionApproverMapsRunCorrelationAndDecision(t *testing.T) {
	var captured app.PermissionRequest
	approver := applicationPermissionApprover{
		taskID: "task-1",
		port: appPermissionPortFunc(func(_ context.Context, request app.PermissionRequest) (app.PermissionResponse, error) {
			captured = request
			return app.PermissionResponse{
				CorrelationID: request.Correlation.ID, Decision: app.PermissionDenyWithFeedback, Feedback: "use another file",
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
		Review: &permission.ReviewResult{Rationale: "needs review"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != permission.UserDenyFeedback || decision.Feedback != "use another file" {
		t.Fatalf("permission decision = %#v", decision)
	}
	if captured.Correlation.ID != "call-1" || captured.Correlation.SessionID != "session-1" || captured.Correlation.RunID != "run-1" || captured.Correlation.ToolCallID != "call-1" {
		t.Fatalf("permission correlation = %#v", captured.Correlation)
	}
	if !reflect.DeepEqual(captured.Effects, []string{"mutate"}) || captured.ReviewerReason != "needs review" {
		t.Fatalf("permission request = %#v", captured)
	}
}

func TestFeishuRuntimeExtractionRemainsFireAndForget(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	store := session.NewFileStoreWithHome(workDir, t.TempDir())
	model := &feishuExtractionBarrierProvider{
		started: make(chan struct{}), release: make(chan struct{}), completed: make(chan struct{}),
	}
	factory := newFeishuTaskExecutionFactory(model, workDir, discardFeishuMessenger{}, store, approval.NewStore())
	request := feishu.TaskExecutionRequest{
		Task:   feishu.Task{TaskID: "task", ChatID: "chat", SenderID: "sender", MessageID: "message"},
		Prompt: "extract after completion",
	}
	prepared, err := factory.PrepareTask(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	type runResult struct {
		outcome *app.RunOutcome
		err     error
	}
	runDone := make(chan runResult, 1)
	go func() {
		outcome, runErr := prepared.Application.Run(ctx, app.RunCommand{Prompt: request.Prompt}, nil)
		runDone <- runResult{outcome: outcome, err: runErr}
	}()
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("runtime run did not launch extraction")
	}
	select {
	case result := <-runDone:
		if result.err != nil || result.outcome == nil || result.outcome.FinalMessage != "done" {
			t.Fatalf("Run() = %#v, %v", result.outcome, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime run waited for blocked extraction")
	}
	close(model.release)
	<-model.completed
	if err := prepared.Drain(ctx); err != nil {
		t.Fatal(err)
	}
}

type composedFeishuProvider struct {
	mu            sync.Mutex
	agentRun      int
	requests      [][]schema.Message
	tools         [][]schema.ToolDefinition
	protocolCalls atomic.Int32
	modelCalls    atomic.Int32
}

type feishuExtractionBarrierProvider struct {
	started   chan struct{}
	release   chan struct{}
	completed chan struct{}
	once      sync.Once
}

func (p *feishuExtractionBarrierProvider) Generate(ctx context.Context, _ []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	if len(definitions) == 7 {
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
	}
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
		close(p.completed)
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: `{}`}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (*feishuExtractionBarrierProvider) ProviderProtocol() string { return "scripted" }
func (*feishuExtractionBarrierProvider) ModelName() string        { return "claude-4-sonnet" }

func (p *composedFeishuProvider) Generate(_ context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
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
			ID: "call-1", Name: "read_file", Arguments: []byte(`{"path":"fixture.txt"}`),
		}}}}, nil
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "target complete"}}, nil
}

func (p *composedFeishuProvider) ProviderProtocol() string {
	p.protocolCalls.Add(1)
	return "scripted"
}

func (p *composedFeishuProvider) ModelName() string {
	p.modelCalls.Add(1)
	return "claude-4-sonnet"
}

func (p *composedFeishuProvider) firstAgentRequest() ([]schema.Message, []schema.ToolDefinition) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]schema.Message(nil), p.requests[0]...), append([]schema.ToolDefinition(nil), p.tools[0]...)
}

type discardFeishuMessenger struct{}

func (discardFeishuMessenger) SendText(context.Context, string, string) error { return nil }

type recordingNotificationSink struct {
	notifications []app.Notification
}

type appPermissionPortFunc func(context.Context, app.PermissionRequest) (app.PermissionResponse, error)

func (f appPermissionPortFunc) RequestPermission(ctx context.Context, request app.PermissionRequest) (app.PermissionResponse, error) {
	return f(ctx, request)
}

func (s *recordingNotificationSink) Notify(_ context.Context, notification app.Notification) {
	s.notifications = append(s.notifications, notification)
}

func definitionNames(definitions []schema.ToolDefinition) []string {
	names := make([]string, len(definitions))
	for index, definition := range definitions {
		names[index] = definition.Name
	}
	sort.Strings(names)
	return names
}

func messagesContain(messages []schema.Message, fragment string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, fragment) {
			return true
		}
	}
	return false
}
