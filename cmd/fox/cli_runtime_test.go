package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/cli"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestCLIExecTargetCompositionPreservesProfileArtifactsAndPresentation(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("TARGET_CLI_PROJECT_INSTRUCTION\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := &targetCLIProvider{}
	cfg := foxConfig{
		WorkDir: workDir, Prompt: "/help remains ordinary input", Model: "cli-target-model",
		ResolvedLLM: llmconfig.ResolvedConfig{
			Protocol: "openai", BaseURL: "https://example.test", Model: "cli-target-model",
		},
		EffortOverride: "high", MaxTurns: 5,
	}
	application, err := newCLIApplicationWithProvider(context.Background(), cfg, model)
	if err != nil {
		t.Fatal(err)
	}
	stdout := new(bytes.Buffer)
	logs := new(bytes.Buffer)
	if err := cli.Run(context.Background(), cli.Config{
		Prompt: cfg.Prompt, Application: application, Stdout: stdout, Logger: log.New(logs, "", 0),
	}); err != nil {
		t.Fatal(err)
	}

	info := application.Session()
	if info.ID == "" || info.Directory == "" || info.TranscriptPath != filepath.Join(info.Directory, "transcript.jsonl") {
		t.Fatalf("session info = %#v", info)
	}
	output := stdout.String()
	for _, fragment := range []string{
		"done:/help remains ordinary input", "Session:  " + info.ID,
		"Transcript:  " + info.TranscriptPath, "Run:  ", "Metrics:  ", "Trace:  ",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("CLI output missing %q:\n%s", fragment, output)
		}
	}
	if !strings.Contains(logs.String(), "[CLI] Session: "+info.ID) || !strings.Contains(logs.String(), "[CLI] Session dir: "+info.Directory) {
		t.Fatalf("CLI logs = %q", logs.String())
	}

	observations := model.snapshot()
	if len(observations) < 1 {
		t.Fatal("provider received no main runtime invocation")
	}
	mainCall := observations[0]
	if got := lastDirectUserMessage(mainCall.messages); got != cfg.Prompt {
		t.Fatalf("direct user prompt = %q, want %q", got, cfg.Prompt)
	}
	if !strings.Contains(mainCall.messages[0].Content, "TARGET_CLI_PROJECT_INSTRUCTION") {
		t.Fatalf("system prompt omitted project instruction:\n%s", mainCall.messages[0].Content)
	}
	if mainCall.options.Effort != "high" {
		t.Fatalf("effort = %q, want high", mainCall.options.Effort)
	}
	var names []string
	for _, definition := range mainCall.definitions {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	if got, want := strings.Join(names, ","), "bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file"; got != want {
		t.Fatalf("tool surface = %q, want %q", got, want)
	}

	stored := &session.StoredSession{ID: session.ID(info.ID), RootDir: info.Directory}
	records, err := session.NewMessageLog(stored).LoadRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Message.Content != cfg.Prompt || records[1].Message.Content != "done:"+cfg.Prompt || records[1].Message.Usage == nil || records[1].Message.Usage.InputTokens != 7 || records[1].Message.Usage.OutputTokens != 3 {
		t.Fatalf("persisted messages = %#v", records)
	}
	for _, path := range []string{
		info.TranscriptPath,
		filepath.Join(info.Directory, "runs", string(records[0].RunID), "metrics.jsonl"),
		filepath.Join(info.Directory, "runs", string(records[0].RunID), "trace.jsonl"),
		filepath.Join(info.Directory, "checkpoints.jsonl"),
		filepath.Join(info.Directory, "state_history.jsonl"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected CLI artifact %q: %v", path, err)
		}
	}
}

func TestCLIExecTargetSessionSelectionPreservesExactErrorsAndLatestCLISource(t *testing.T) {
	workDir := t.TempDir()
	store := session.NewManagerWithHome(workDir, t.TempDir())
	existing, err := store.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(session.CreateOptions{Source: session.SOURCEFeishu, WorkDir: workDir}); err != nil {
		t.Fatal(err)
	}
	selected, err := selectCLIStoredSession(store, workDir, foxConfig{ContinueSession: true})
	if err != nil || selected.ID != existing.ID {
		t.Fatalf("continue selection = %#v/%v, want %s", selected, err, existing.ID)
	}
	for _, check := range []struct {
		config foxConfig
		want   string
	}{
		{config: foxConfig{NewSession: true, SessionID: string(existing.ID)}, want: "-new 不能和 -session 或 -continue 同时使用"},
		{config: foxConfig{SessionID: string(existing.ID), ContinueSession: true}, want: "-session 不能和 -continue 同时使用"},
		{config: foxConfig{SessionID: "missing"}, want: "Session missing 不存在"},
	} {
		if _, err := selectCLIStoredSession(store, workDir, check.config); err == nil || err.Error() != check.want {
			t.Fatalf("selection error = %v, want %q", err, check.want)
		}
	}
}

func TestCLIExecResultHooksPreserveSkillThenMemoryOrder(t *testing.T) {
	var order []string
	hook := combineResultHooks(
		func(schema.ToolCall, schema.ToolResult) { order = append(order, "skill") },
		func(schema.ToolCall, schema.ToolResult) { order = append(order, "memory") },
	)
	hook(schema.ToolCall{ID: "call-1"}, schema.ToolResult{ToolCallID: "call-1"})
	if got, want := strings.Join(order, ","), "skill,memory"; got != want {
		t.Fatalf("result hook order = %q, want %q", got, want)
	}
}

func TestCLIExecTargetToolRunReturnsBeforeTrackedExtractionAndDrainJoins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "fixture.txt"), []byte("fixture content"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := &targetCLIToolProvider{extractionStarted: make(chan struct{}), releaseExtraction: make(chan struct{})}
	config := foxConfig{
		WorkDir: workDir, Model: "model", MaxTurns: 4,
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "model"},
	}
	application, err := newCLIApplicationWithProvider(context.Background(), config, model)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := application.Run(context.Background(), app.RunCommand{Prompt: "read fixture"}, nil)
	if err != nil || outcome == nil || outcome.FinalMessage != "read complete" {
		t.Fatalf("Run() = %#v/%v", outcome, err)
	}
	select {
	case <-model.extractionStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("target CLI extraction did not start after the run result")
	}
	drained := make(chan error, 1)
	go func() { drained <- application.Drain(context.Background()) }()
	select {
	case err := <-drained:
		t.Fatalf("Drain returned before extraction completion: %v", err)
	default:
	}
	close(model.releaseExtraction)
	if err := <-drained; err != nil {
		t.Fatal(err)
	}

	stored := &session.StoredSession{ID: session.ID(application.Session().ID), RootDir: application.Session().Directory}
	records, err := session.NewMessageLog(stored).LoadRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 || len(records[1].Message.ToolCalls) != 1 || records[1].Message.ToolCalls[0].Name != "read_file" || !strings.Contains(records[2].Message.Content, "fixture content") || records[3].Message.Content != "read complete" {
		t.Fatalf("tool run records = %#v", records)
	}
}

type targetCLIObservation struct {
	messages    []schema.Message
	definitions []schema.ToolDefinition
	options     provider.GenerateOptions
}

type targetCLIProvider struct {
	mu           sync.Mutex
	observations []targetCLIObservation
}

func (p *targetCLIProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{})
}

func (p *targetCLIProvider) GenerateWithOptions(_ context.Context, messages []schema.Message, definitions []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.observations = append(p.observations, targetCLIObservation{
		messages: append([]schema.Message(nil), messages...), definitions: append([]schema.ToolDefinition(nil), definitions...), options: options,
	})
	call := len(p.observations)
	p.mu.Unlock()
	if call > 1 {
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: `{}`}}, nil
	}
	prompt := lastDirectUserMessage(messages)
	return &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "done:" + prompt},
		Usage:   schema.Usage{InputTokens: 7, OutputTokens: 3},
	}, nil
}

func (p *targetCLIProvider) snapshot() []targetCLIObservation {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]targetCLIObservation(nil), p.observations...)
}

type targetCLIToolProvider struct {
	mu                sync.Mutex
	calls             int
	extractionStarted chan struct{}
	releaseExtraction chan struct{}
	once              sync.Once
}

func (p *targetCLIToolProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{})
}

func (p *targetCLIToolProvider) GenerateWithOptions(ctx context.Context, _ []schema.Message, _ []schema.ToolDefinition, _ provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	switch call {
	case 1:
		return &provider.GenerateResponse{Message: &schema.Message{
			Role:      schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{ID: "read-1", Name: "read_file", Arguments: []byte(`{"path":"fixture.txt"}`)}},
		}}, nil
	case 2:
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "read complete"}}, nil
	default:
		p.once.Do(func() { close(p.extractionStarted) })
		select {
		case <-p.releaseExtraction:
			return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: `{}`}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func lastDirectUserMessage(messages []schema.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == schema.RoleUser && messages[index].ToolCallID == "" {
			return messages[index].Content
		}
	}
	return ""
}
