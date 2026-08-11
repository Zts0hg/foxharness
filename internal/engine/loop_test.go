package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolresult"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/tracing"
)

type bigOutputTool struct {
	name   string
	output string
}

func (t *bigOutputTool) Name() string { return t.name }
func (t *bigOutputTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.name,
		Description: "test tool",
		InputSchema: map[string]any{"type": "object"},
	}
}
func (t *bigOutputTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.output, nil
}

type structuredFailureTool struct {
	name string
}

func (t *structuredFailureTool) Name() string { return t.name }
func (t *structuredFailureTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: t.name, Description: "structured failure test"}
}
func (t *structuredFailureTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return "legacy success", nil
}
func (t *structuredFailureTool) ExecuteResult(ctx context.Context, args json.RawMessage) (tools.ExecutionResult, error) {
	return tools.ExecutionResult{Output: "structured failure", Failed: true}, nil
}

type sequencedProvider struct {
	responses       []*provider.GenerateResponse
	call            int
	seen            [][]schema.Message
	seenTools       [][]string
	seenDefinitions [][]schema.ToolDefinition
}

func (p *sequencedProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.seen = append(p.seen, append([]schema.Message(nil), messages...))
	p.seenDefinitions = append(p.seenDefinitions, append([]schema.ToolDefinition(nil), availableTools...))
	names := make([]string, 0, len(availableTools))
	for _, definition := range availableTools {
		names = append(names, definition.Name)
	}
	p.seenTools = append(p.seenTools, names)
	idx := p.call
	if idx >= len(p.responses) {
		idx = len(p.responses) - 1
	}
	p.call++
	return p.responses[idx], nil
}

type mutatingFinalProvider struct {
	mutate func()
}

func (p *mutatingFinalProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	if p.mutate != nil {
		p.mutate()
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

type optionCapturingProvider struct {
	generateCalls int
	optionCalls   int
	options       []provider.GenerateOptions
}

func (p *optionCapturingProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.generateCalls++
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

func (p *optionCapturingProvider) GenerateWithOptions(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.optionCalls++
	p.options = append(p.options, options)
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

type streamingTestProvider struct {
	generateCalls int
	streamCalls   int
	options       []provider.GenerateOptions
	deltas        []string
	streamErr     error
	responses     []*provider.GenerateResponse
}

func (p *streamingTestProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.generateCalls++
	if len(p.responses) > 0 {
		idx := p.generateCalls - 1
		if idx >= len(p.responses) {
			idx = len(p.responses) - 1
		}
		return p.responses[idx], nil
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "fallback"}}, nil
}

func (p *streamingTestProvider) GenerateStream(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition, options provider.GenerateOptions, callbacks provider.StreamCallbacks) (*provider.GenerateResponse, error) {
	p.streamCalls++
	p.options = append(p.options, options)
	for _, delta := range p.deltas {
		callbacks.EmitTextDelta(delta)
	}
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: strings.Join(p.deltas, "")}}, nil
}

type deltaRecordingReporter struct {
	recordingReporter
	deltas []string
}

func (r *deltaRecordingReporter) OnMessageDelta(ctx context.Context, content string) {
	r.deltas = append(r.deltas, content)
}

type statusCodeError struct {
	StatusCode int
	message    string
}

func (e statusCodeError) Error() string {
	return e.message
}

type engineTurnRegistry struct {
	tools.Registry
	turns int
}

func (r *engineTurnRegistry) BeginTurn() {
	r.turns++
	r.Register(&bigOutputTool{name: "turn_tool", output: "ok"})
}

func TestEngineUsesGenerateOptionsForEffortOverride(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &optionCapturingProvider{}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1, EffortOverride: "high"})
	if _, err := eng.RunWithReporter(context.Background(), sess, "test", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if p.generateCalls != 0 {
		t.Fatalf("Generate calls = %d, want 0 when effort override is set", p.generateCalls)
	}
	if p.optionCalls != 1 || len(p.options) != 1 || p.options[0].Effort != "high" {
		t.Fatalf("GenerateWithOptions calls/options = %d/%#v, want high", p.optionCalls, p.options)
	}
}

func TestEngineUsesDefaultGenerateWithoutEffortOverride(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &optionCapturingProvider{}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1})
	if _, err := eng.RunWithReporter(context.Background(), sess, "test", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if p.generateCalls != 1 {
		t.Fatalf("Generate calls = %d, want 1 without effort override", p.generateCalls)
	}
	if p.optionCalls != 0 {
		t.Fatalf("GenerateWithOptions calls = %d, want 0 without effort override", p.optionCalls)
	}
}

func TestEngineStreamsModelDeltasWhenProviderSupportsStreaming(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &streamingTestProvider{deltas: []string{"hello ", "world"}}
	reporter := &deltaRecordingReporter{}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1, EffortOverride: "high"})
	result, err := eng.RunWithReporter(context.Background(), sess, "test", reporter)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if p.generateCalls != 0 || p.streamCalls != 1 {
		t.Fatalf("provider calls Generate=%d Stream=%d, want 0/1", p.generateCalls, p.streamCalls)
	}
	if len(p.options) != 1 || p.options[0].Effort != "high" {
		t.Fatalf("stream options = %#v, want effort high", p.options)
	}
	if got := strings.Join(reporter.deltas, ""); got != "hello world" {
		t.Fatalf("streamed deltas = %q, want hello world", got)
	}
	if result.FinalMessage != "hello world" {
		t.Fatalf("FinalMessage = %q, want hello world", result.FinalMessage)
	}
}

func TestEngineFallsBackWhenStreamingUnsupportedBeforeDelta(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &streamingTestProvider{streamErr: errors.New("unsupported stream_options parameter")}
	reporter := &deltaRecordingReporter{}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1})
	result, err := eng.RunWithReporter(context.Background(), sess, "test", reporter)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if p.streamCalls != 1 || p.generateCalls != 1 {
		t.Fatalf("provider calls Stream=%d Generate=%d, want 1/1", p.streamCalls, p.generateCalls)
	}
	if len(reporter.deltas) != 0 {
		t.Fatalf("streamed deltas = %#v, want none before fallback", reporter.deltas)
	}
	if result.FinalMessage != "fallback" {
		t.Fatalf("FinalMessage = %q, want fallback", result.FinalMessage)
	}
}

func TestEngineFallsBackWhenStreamingReturnsEmptyStreamBeforeDelta(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &streamingTestProvider{streamErr: provider.ErrEmptyStream}
	reporter := &deltaRecordingReporter{}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1})
	result, err := eng.RunWithReporter(context.Background(), sess, "test", reporter)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if p.streamCalls != 1 || p.generateCalls != 1 {
		t.Fatalf("provider calls Stream=%d Generate=%d, want 1/1", p.streamCalls, p.generateCalls)
	}
	if len(reporter.deltas) != 0 {
		t.Fatalf("streamed deltas = %#v, want none before fallback", reporter.deltas)
	}
	if result.FinalMessage != "fallback" {
		t.Fatalf("FinalMessage = %q, want fallback", result.FinalMessage)
	}
}

func TestEngineDisablesStreamingAfterEmptyStreamFallback(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := tools.NewRegistry()
	registry.Register(&bigOutputTool{name: "turn_tool", output: "ok"})
	p := &streamingTestProvider{
		streamErr: provider.ErrEmptyStream,
		responses: []*provider.GenerateResponse{
			{Message: &schema.Message{
				Role: schema.RoleAssistant,
				ToolCalls: []schema.ToolCall{{
					ID:        "call-turn",
					Name:      "turn_tool",
					Arguments: json.RawMessage(`{}`),
				}},
			}},
			{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
		},
	}
	reporter := &deltaRecordingReporter{}
	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 3})
	result, err := eng.RunWithReporter(context.Background(), sess, "test", reporter)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if p.streamCalls != 1 || p.generateCalls != 2 {
		t.Fatalf("provider calls Stream=%d Generate=%d, want 1/2", p.streamCalls, p.generateCalls)
	}
	if result.FinalMessage != "done" {
		t.Fatalf("FinalMessage = %q, want done", result.FinalMessage)
	}
}

func TestEngineDisablesStreamingAfterUnsupportedFallback(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := tools.NewRegistry()
	registry.Register(&bigOutputTool{name: "turn_tool", output: "ok"})
	p := &streamingTestProvider{
		streamErr: errors.New("streaming is not supported by this endpoint"),
		responses: []*provider.GenerateResponse{
			{Message: &schema.Message{
				Role: schema.RoleAssistant,
				ToolCalls: []schema.ToolCall{{
					ID:        "call-turn",
					Name:      "turn_tool",
					Arguments: json.RawMessage(`{}`),
				}},
			}},
			{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
		},
	}
	reporter := &deltaRecordingReporter{}
	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 3})
	result, err := eng.RunWithReporter(context.Background(), sess, "test", reporter)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if p.streamCalls != 1 || p.generateCalls != 2 {
		t.Fatalf("provider calls Stream=%d Generate=%d, want 1/2", p.streamCalls, p.generateCalls)
	}
	if result.FinalMessage != "done" {
		t.Fatalf("FinalMessage = %q, want done", result.FinalMessage)
	}
}

func TestEngineDoesNotFallbackWhenStreamingFailsAfterDelta(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &streamingTestProvider{deltas: []string{"partial"}, streamErr: errors.New("unsupported stream_options parameter")}
	reporter := &deltaRecordingReporter{}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1})
	_, err = eng.RunWithReporter(context.Background(), sess, "test", reporter)
	if err == nil {
		t.Fatalf("RunWithReporter() error = nil, want stream error")
	}
	if p.streamCalls != 1 || p.generateCalls != 0 {
		t.Fatalf("provider calls Stream=%d Generate=%d, want 1/0", p.streamCalls, p.generateCalls)
	}
	if got := strings.Join(reporter.deltas, ""); got != "partial" {
		t.Fatalf("streamed deltas = %q, want partial", got)
	}
}

func TestEngineFallsBackWhenStreamingFailsWithRetryableStartError(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &streamingTestProvider{streamErr: statusCodeError{StatusCode: http.StatusTooManyRequests, message: "rate limited"}}
	reporter := &deltaRecordingReporter{}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1})
	result, err := eng.RunWithReporter(context.Background(), sess, "test", reporter)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if p.streamCalls != 1 || p.generateCalls != 1 {
		t.Fatalf("provider calls Stream=%d Generate=%d, want 1/1", p.streamCalls, p.generateCalls)
	}
	if len(reporter.deltas) != 0 {
		t.Fatalf("streamed deltas = %#v, want none before retry fallback", reporter.deltas)
	}
	if result.FinalMessage != "fallback" {
		t.Fatalf("FinalMessage = %q, want fallback", result.FinalMessage)
	}
}

func TestEngineBeginsRegistryTurnBeforeToolDiscovery(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := &engineTurnRegistry{Registry: tools.NewRegistry()}
	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "call-turn",
				Name:      "turn_tool",
				Arguments: json.RawMessage(`{}`),
			}},
		}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 3})

	if _, err := eng.RunWithReporter(context.Background(), sess, "test", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if registry.turns != 2 {
		t.Fatalf("BeginTurn calls = %d, want 2", registry.turns)
	}
	if len(p.seenTools) != 2 || !containsString(p.seenTools[0], "turn_tool") {
		t.Fatalf("provider tool surfaces = %#v, want turn_tool on first call", p.seenTools)
	}
}

func TestEngineMakesStructuredToolFailuresVisibleToRecoveryMetricsAndTracing(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := tools.NewRegistry()
	registry.Register(&structuredFailureTool{name: "fail_structured"})
	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "call-fail",
				Name:      "fail_structured",
				Arguments: json.RawMessage(`{}`),
			}},
		}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	var observed schema.ToolResult
	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{
		MaxTurns: 3,
		OnToolCalled: func(call schema.ToolCall, result schema.ToolResult) {
			if call.ID == "call-fail" {
				observed = result
			}
		},
	})

	result, err := eng.RunWithReporter(context.Background(), sess, "test", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if !observed.IsError {
		t.Fatalf("OnToolCalled IsError = false, want true")
	}
	if len(p.seen) < 2 || !messagesContain(p.seen[1], "Error Recovery Notice") {
		t.Fatalf("second model call missing recovery prompt: %#v", p.seen)
	}

	metricsData, err := os.ReadFile(result.MetricsPath)
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	if !strings.Contains(string(metricsData), `"is_error":true`) || !strings.Contains(string(metricsData), `"error_count":1`) {
		t.Fatalf("metrics did not record failed tool result:\n%s", metricsData)
	}

	events, err := tracing.Load(result.TracePath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, event := range events {
		if event.Type == tracing.EventSpanEnd && event.Name == "tool_call" && event.Status == "error" && event.Attrs["is_error"] == true {
			return
		}
	}
	t.Fatalf("trace did not contain an error tool_call span: %#v", events)
}

func TestRunWithReporterSurfacesTranscriptWriteWarnings(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := os.Mkdir(sess.TranscriptPath(), 0o755); err != nil {
		t.Fatalf("Mkdir transcript path error = %v", err)
	}

	prov := &sequencedProvider{responses: []*provider.GenerateResponse{{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"},
	}}}
	eng := NewAgentEngine(prov, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1})

	result, err := eng.RunWithReporter(context.Background(), sess, "hello", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result == nil {
		t.Fatalf("RunWithReporter() result = nil")
	}
	if !telemetryWarningsContain(result.TelemetryWarnings, "transcript") {
		t.Fatalf("TelemetryWarnings = %+v, want transcript warning", result.TelemetryWarnings)
	}
}

func TestRunWithReporterSurfacesMetricsWriteWarnings(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	prov := &mutatingFinalProvider{mutate: func() {
		replaceCurrentRunFileWithDirectory(t, sess, "metrics.jsonl")
	}}
	eng := NewAgentEngine(prov, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1})

	result, err := eng.RunWithReporter(context.Background(), sess, "hello", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result == nil {
		t.Fatalf("RunWithReporter() result = nil")
	}
	if !telemetryWarningsContain(result.TelemetryWarnings, "metrics") {
		t.Fatalf("TelemetryWarnings = %+v, want metrics warning", result.TelemetryWarnings)
	}
}

func TestRunWithReporterSurfacesTraceWriteWarnings(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	prov := &mutatingFinalProvider{mutate: func() {
		replaceCurrentRunFileWithDirectory(t, sess, "trace.jsonl")
	}}
	eng := NewAgentEngine(prov, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1})

	result, err := eng.RunWithReporter(context.Background(), sess, "hello", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result == nil {
		t.Fatalf("RunWithReporter() result = nil")
	}
	if !telemetryWarningsContain(result.TelemetryWarnings, "trace") {
		t.Fatalf("TelemetryWarnings = %+v, want trace warning", result.TelemetryWarnings)
	}
}

func TestMessagesEqualDoesNotDependOnBackingArray(t *testing.T) {
	original := []schema.Message{{Role: schema.RoleUser, Content: "same"}}
	copied := append([]schema.Message(nil), original...)

	if !messagesEqual(original, copied) {
		t.Fatalf("copied unchanged messages should compare equal")
	}

	changed := append([]schema.Message(nil), original...)
	changed[0].Content = "changed"
	if messagesEqual(original, changed) {
		t.Fatalf("same-length changed messages should compare different")
	}
}

func telemetryWarningsContain(warnings []TelemetryWarning, sink string) bool {
	for _, warning := range warnings {
		if warning.Sink == sink {
			return true
		}
	}
	return false
}

func replaceCurrentRunFileWithDirectory(t *testing.T, sess *session.Session, name string) {
	t.Helper()
	entries, err := os.ReadDir(sess.RunsDir())
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", sess.RunsDir(), err)
	}
	if len(entries) != 1 {
		t.Fatalf("run directories = %d, want 1", len(entries))
	}
	path := filepath.Join(sess.RunsDir(), entries[0].Name(), name)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Remove(%s) error = %v", path, err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir(%s) error = %v", path, err)
	}
}

func TestEngineCompletionGateInjectsReminderThenAllowsCompletion(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "premature"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	gateCalls := 0
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{
		MaxTurns: 3,
		CompletionGate: func() string {
			gateCalls++
			if gateCalls == 1 {
				return "submit_plan is still required"
			}
			return ""
		},
	})

	result, err := eng.RunWithReporter(context.Background(), sess, "test", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result.FinalMessage != "done" || p.call != 2 {
		t.Fatalf("result = %#v, provider calls = %d, want done after 2", result, p.call)
	}
	if len(p.seen) < 2 || !messagesContain(p.seen[1], "submit_plan is still required") {
		t.Fatalf("second call missing completion reminder: %#v", p.seen)
	}
}

func TestEngineCompletionGateFailsAfterRepeatedUnsatisfiedFinal(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "premature"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "still premature"}},
	}}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{
		MaxTurns:       3,
		CompletionGate: func() string { return "update_todo is still required" },
	})

	result, err := eng.RunWithReporter(context.Background(), sess, "test", nil)
	if err == nil || !strings.Contains(err.Error(), "completion gate remained unsatisfied") {
		t.Fatalf("RunWithReporter() error = %v, want unsatisfied completion gate", err)
	}
	if result == nil || p.call != 2 {
		t.Fatalf("result = %#v, provider calls = %d, want partial result after 2", result, p.call)
	}
}

func TestEngineCompletionGateGrantsRetryWhenReminderChanges(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "plan missing"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "checklist missing"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	reminders := []string{
		"submit_plan is still required",
		"update_todo is now required",
		"",
	}
	gateCalls := 0
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{
		MaxTurns: 4,
		CompletionGate: func() string {
			reminder := reminders[gateCalls]
			gateCalls++
			return reminder
		},
	})

	result, err := eng.RunWithReporter(context.Background(), sess, "test", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result == nil || result.FinalMessage != "done" || p.call != 3 {
		t.Fatalf("result = %#v, provider calls = %d, want done after both gate retries", result, p.call)
	}
	if len(p.seen) < 3 || !messagesContain(p.seen[1], reminders[0]) || !messagesContain(p.seen[2], reminders[1]) {
		t.Fatalf("provider history missing phase-specific reminders: %#v", p.seen)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func messagesContain(messages []schema.Message, want string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, want) {
			return true
		}
	}
	return false
}

type usageReportingProvider struct {
	usage   schema.Usage
	content string
}

func (p *usageReportingProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	content := p.content
	if content == "" {
		content = "done"
	}
	return &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: content},
		Usage:   p.usage,
	}, nil
}

func TestCallModelUsesGenerateResponse(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &usageReportingProvider{
		usage: schema.Usage{InputTokens: 1234, OutputTokens: 56},
	}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 2})

	result, err := eng.RunWithReporter(context.Background(), sess, "hello", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result.FinalMessage != "done" {
		t.Fatalf("FinalMessage = %q, want done", result.FinalMessage)
	}

	messages, err := session.NewMessageLog(sess).LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages() error = %v", err)
	}
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(messages))
	}
	assistant := messages[len(messages)-1]
	if assistant.Role != schema.RoleAssistant {
		t.Fatalf("last message role = %q, want assistant", assistant.Role)
	}
	if assistant.Usage == nil {
		t.Fatalf("assistant.Usage = nil, want populated usage")
	}
	if assistant.Usage.InputTokens != 1234 || assistant.Usage.OutputTokens != 56 {
		t.Fatalf("assistant.Usage = %#v, want {InputTokens:1234, OutputTokens:56}", assistant.Usage)
	}
}

func TestEngineRequiresTodoUpdateBeforeFinalResponse(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	todoPath := filepath.Join(sess.RootDir, "TODO.md")
	if err := os.WriteFile(todoPath, []byte("# TODO\n\n- [ ] Finish report\n"), 0644); err != nil {
		t.Fatalf("seed TODO.md: %v", err)
	}

	registry := tools.NewRegistry()
	registry.Register(tools.NewUpdateTodoTool(sess.RootDir))
	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "analysis complete"}},
		{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "call_update_todo",
				Name:      "update_todo",
				Arguments: json.RawMessage(`{"content":"# TODO\n\n- [x] Finish report\n"}`),
			}},
		}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}

	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 5})
	result, err := eng.RunWithReporter(context.Background(), sess, "finish report", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result.FinalMessage != "done" {
		t.Fatalf("FinalMessage = %q, want done", result.FinalMessage)
	}
	if p.call != 3 {
		t.Fatalf("provider calls = %d, want 3", p.call)
	}
	data, err := os.ReadFile(todoPath)
	if err != nil {
		t.Fatalf("read TODO.md: %v", err)
	}
	if strings.Contains(string(data), "- [ ] Finish report") || !strings.Contains(string(data), "- [x] Finish report") {
		t.Fatalf("TODO.md was not marked complete:\n%s", data)
	}

	var foundReminder bool
	if len(p.seen) < 2 {
		t.Fatalf("provider saw %d calls, want at least 2", len(p.seen))
	}
	for _, msg := range p.seen[1] {
		if strings.Contains(msg.Content, "TODO.md still has incomplete checklist items") {
			foundReminder = true
			break
		}
	}
	if !foundReminder {
		t.Fatalf("second model call missing TODO completion reminder: %#v", p.seen[1])
	}
}

func TestEngine_FullCompactionFlow(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "")
	t.Setenv("FOXHARNESS_DISABLE_AUTO_COMPACT", "")

	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := tools.NewRegistry()

	responses := []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "<summary>auto summary</summary>"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}
	p := &sequencedProvider{responses: responses}

	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 5})

	compCfg := compaction.DefaultCompactionConfig()
	compCfg.Model = "test-model"
	compCfg.RecentKeep = 1
	compCfg.SessionDir = sess.RootDir
	compCfg.TranscriptPath = sess.TranscriptPath()
	compCfg.Estimator = compaction.RoughEstimator{}
	compCfg.AutoCompactThreshold = 1
	compactor, err := compaction.NewCompactor(p, compCfg)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}
	eng.WithCompactor(compactor)

	log := session.NewMessageLog(sess)
	for i := 0; i < 6; i++ {
		_, err := log.Append("seed-run", schema.Message{
			Role:    schema.RoleAssistant,
			Content: strings.Repeat("legacy ", 200),
		})
		if err != nil {
			t.Fatalf("seed Append: %v", err)
		}
	}

	if _, err := eng.RunWithReporter(context.Background(), sess, "hello", nil); err != nil {
		t.Fatalf("RunWithReporter: %v", err)
	}

	if p.call < 1 {
		t.Fatalf("expected provider to be called at least once, got %d", p.call)
	}
}

// inMemoryFS records all WriteFile calls so a test can prove the engine
// routed tool-result persistence through the injected FileSystem instead
// of touching the real disk.
type inMemoryFS struct {
	writes map[string][]byte
}

func newInMemoryFS() *inMemoryFS { return &inMemoryFS{writes: map[string][]byte{}} }

func (f *inMemoryFS) WriteFile(path string, data []byte, _ os.FileMode) error {
	buf := make([]byte, len(data))
	copy(buf, data)
	f.writes[path] = buf
	return nil
}
func (f *inMemoryFS) Stat(path string) (os.FileInfo, error) {
	if _, ok := f.writes[path]; ok {
		return nil, nil
	}
	return nil, os.ErrNotExist
}
func (f *inMemoryFS) MkdirAll(_ string, _ os.FileMode) error { return nil }

func TestEngine_ToolResultsUseInjectedFileSystem(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := tools.NewRegistry()
	largeOutput := strings.Repeat("Z", 60000)
	registry.Register(&bigOutputTool{name: "big_dump", output: largeOutput})

	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "call_mem",
				Name:      "big_dump",
				Arguments: json.RawMessage(`{}`),
			}},
		}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}

	fs := newInMemoryFS()
	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 4})
	eng.WithFileSystem(fs)

	if _, err := eng.RunWithReporter(context.Background(), sess, "go", nil); err != nil {
		t.Fatalf("RunWithReporter: %v", err)
	}

	if len(fs.writes) == 0 {
		t.Fatalf("expected at least one write to the injected filesystem")
	}
	if _, err := os.Stat(filepath.Join(sess.ToolResultsDir(), "call_mem.txt")); err == nil {
		t.Fatalf("engine wrote to disk despite injected in-memory FileSystem")
	}
}

func TestEngine_ToolResultPersistence(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := tools.NewRegistry()
	largeOutput := strings.Repeat("X", 60000)
	registry.Register(&bigOutputTool{name: "big_dump", output: largeOutput})

	responses := []*provider.GenerateResponse{
		{
			Message: &schema.Message{
				Role: schema.RoleAssistant,
				ToolCalls: []schema.ToolCall{{
					ID:        "call_big_1",
					Name:      "big_dump",
					Arguments: json.RawMessage(`{}`),
				}},
			},
		},
		{
			Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"},
		},
	}
	p := &sequencedProvider{responses: responses}

	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 4})

	if _, err := eng.RunWithReporter(context.Background(), sess, "fetch big", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}

	persistedPath := filepath.Join(sess.ToolResultsDir(), "call_big_1.txt")
	data, err := os.ReadFile(persistedPath)
	if err != nil {
		t.Fatalf("expected persisted tool result at %s: %v", persistedPath, err)
	}
	if len(data) != len(largeOutput) {
		t.Fatalf("persisted file size = %d, want %d", len(data), len(largeOutput))
	}

	messages, err := session.NewMessageLog(sess).LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	var toolMsg *schema.Message
	for i := range messages {
		if messages[i].ToolCallID == "call_big_1" {
			toolMsg = &messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatalf("expected tool result message in history")
	}
	if !strings.Contains(toolMsg.Content, "<persisted-output>") {
		t.Fatalf("tool result in context should be preview, got: %q", toolMsg.Content[:200])
	}
	if len(toolMsg.Content) >= len(largeOutput) {
		t.Fatalf("preview should be smaller than full output: got %d, full %d", len(toolMsg.Content), len(largeOutput))
	}
}

func TestEngine_ReadFileLargeResultUsesUnifiedPersistence(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	content := strings.Repeat("R", toolresult.PersistenceThreshold+1000)
	if err := os.WriteFile(filepath.Join(workDir, "large.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))

	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "call_read_large",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"large.txt"}`),
			}},
		}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}

	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 4})
	if _, err := eng.RunWithReporter(context.Background(), sess, "read large", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}

	persistedPath := filepath.Join(sess.ToolResultsDir(), "call_read_large.txt")
	data, err := os.ReadFile(persistedPath)
	if err != nil {
		t.Fatalf("expected persisted read_file result at %s: %v", persistedPath, err)
	}
	if string(data) != content {
		t.Fatalf("persisted read_file result length = %d, want complete %d-byte file", len(data), len(content))
	}

	messages, err := session.NewMessageLog(sess).LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	var toolMsg *schema.Message
	for i := range messages {
		if messages[i].ToolCallID == "call_read_large" {
			toolMsg = &messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatalf("expected read_file result message in history")
	}
	if !strings.Contains(toolMsg.Content, "<persisted-output>") {
		t.Fatalf("read_file result should use persisted preview, got: %q", toolMsg.Content[:200])
	}
	if strings.Contains(toolMsg.Content, "截断至前 8000 字节") {
		t.Fatalf("read_file result used the old 8000-byte truncation path")
	}
}

func TestRun_BlocksWhenContextExceedsBlockingThreshold(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("session.Create: %v", err)
	}

	prov := &usageReportingProvider{content: "done"}
	reg := tools.NewRegistry()
	eng := NewAgentEngine(prov, reg, workDir, staticComposer{}, DefaultConfig())

	cfg := compaction.DefaultCompactionConfig()
	cfg.Model = "test"
	cfg.ContextWindow = 100
	cfg.Estimator = compaction.ImprovedRoughEstimator{}
	c, err := compaction.NewCompactor(prov, cfg)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}
	eng.WithCompactor(c)

	longPrompt := strings.Repeat("x", 2000)
	_, runErr := eng.Run(context.Background(), sess, longPrompt)
	if runErr == nil {
		t.Fatalf("expected error when context exceeds blocking threshold")
	}
	if !strings.Contains(runErr.Error(), "阻塞阈值") {
		t.Fatalf("unexpected error message: %v", runErr)
	}
}

func TestRun_ReportsContextEstimate(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("session.Create: %v", err)
	}

	prov := &usageReportingProvider{content: "done"}
	reg := tools.NewRegistry()

	var gotUsed, gotWindow int
	cfg := DefaultConfig()
	cfg.OnContextEstimate = func(usedTokens, contextWindow int) {
		gotUsed = usedTokens
		gotWindow = contextWindow
	}

	eng := NewAgentEngine(prov, reg, workDir, staticComposer{}, cfg)
	compCfg := compaction.DefaultCompactionConfig()
	compCfg.Model = "test"
	compCfg.ContextWindow = 200000
	c, err := compaction.NewCompactor(prov, compCfg)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}
	eng.WithCompactor(c)

	_, runErr := eng.Run(context.Background(), sess, "hello")
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if gotUsed == 0 {
		t.Fatalf("OnContextEstimate was not called; gotUsed = 0")
	}
	if gotWindow != 200000 {
		t.Fatalf("OnContextEstimate got contextWindow = %d, want 200000", gotWindow)
	}
}
