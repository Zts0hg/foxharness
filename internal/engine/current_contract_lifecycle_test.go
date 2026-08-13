package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/metrics"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/testsupport/runtimecontract"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/tracing"
)

func TestRunLifecycleSuccessIsOrderedFinishedAndReportedOnce(t *testing.T) {
	workDir := t.TempDir()
	sess := newLifecycleSession(t, workDir)
	registry := tools.NewRegistry()
	registry.Register(&bigOutputTool{name: "lifecycle_tool", output: "tool output"})
	call := schema.ToolCall{ID: "call-lifecycle", Name: "lifecycle_tool", Arguments: json.RawMessage(`{}`)}
	modelProvider := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{call}}, Usage: schema.Usage{InputTokens: 4, OutputTokens: 2}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "lifecycle complete"}, Usage: schema.Usage{InputTokens: 7, OutputTokens: 3}},
	}}
	reporter := &recordingReporter{}
	eng := NewAgentEngine(modelProvider, registry, workDir, staticComposer{}, Config{MaxTurns: 3})

	result, err := eng.RunWithReporter(context.Background(), sess, "run lifecycle", reporter)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	wantEvents := []string{
		"start:" + string(sess.ID) + ":" + result.RunID,
		"tool_call:lifecycle_tool",
		"tool_result:lifecycle_tool:false",
		"message:lifecycle complete",
		"complete:" + string(sess.ID) + ":" + result.RunID,
	}
	if fmt.Sprint(reporter.events) != fmt.Sprint(wantEvents) {
		t.Fatalf("reporter events = %#v, want %#v", reporter.events, wantEvents)
	}
	assertLifecycleRunFinished(t, sess, result.RunID)
	assertLifecycleResultPaths(t, sess, result)
	assertLifecycleMetricsExactlyOnce(t, result.MetricsPath, 2, 1, 0)

	records, err := session.NewMessageLog(sess).LoadRecords()
	if err != nil {
		t.Fatalf("LoadRecords() error = %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("message records = %d, want 4", len(records))
	}
	if got := records[1].Message.Usage; got == nil || got.InputTokens != 4 || got.OutputTokens != 2 {
		t.Fatalf("first response usage = %#v", got)
	}
	if got := records[3].Message.Usage; got == nil || got.InputTokens != 7 || got.OutputTokens != 3 {
		t.Fatalf("final response usage = %#v", got)
	}

	events, err := tracing.Load(result.TracePath)
	if err != nil {
		t.Fatalf("Load(trace) error = %v", err)
	}
	var runStarts, runEnds int
	for _, event := range events {
		if event.Name != "run" {
			continue
		}
		switch event.Type {
		case tracing.EventSpanStart:
			runStarts++
		case tracing.EventSpanEnd:
			runEnds++
		}
	}
	if runStarts != 1 || runEnds != 1 {
		t.Fatalf("run trace start/end = %d/%d, want 1/1", runStarts, runEnds)
	}
}

func TestRunLifecycleFailuresFinishAndReportCurrentOutcome(t *testing.T) {
	tests := []struct {
		name       string
		composer   func(*session.Session) PromptComposer
		provider   func(*session.Session) provider.LLMProvider
		registry   func(*session.Session) tools.Registry
		wantError  string
		wantFacts  []string
		wantModels int
		wantTools  int
		wantErrors int
	}{
		{
			name: "composer",
			composer: func(*session.Session) PromptComposer {
				return lifecycleComposerFunc(func(string) (string, error) { return "", errors.New("composer unavailable") })
			},
			provider:  func(*session.Session) provider.LLMProvider { return &finalProvider{} },
			wantError: "组装系统提示词失败: composer unavailable",
		},
		{
			name:     "model",
			composer: func(*session.Session) PromptComposer { return staticComposer{} },
			provider: func(*session.Session) provider.LLMProvider {
				return lifecycleProviderFunc(func(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
					return nil, errors.New("model unavailable")
				})
			},
			wantError:  "模型生成失败: model unavailable",
			wantModels: 1,
			wantErrors: 1,
		},
		{
			name: "user_message_persistence",
			composer: func(sess *session.Session) PromptComposer {
				return lifecycleComposerFunc(func(string) (string, error) {
					replaceLifecycleFileWithDirectory(t, sess.MessagesPath())
					return "system", nil
				})
			},
			provider:  func(*session.Session) provider.LLMProvider { return &finalProvider{} },
			wantError: "写入 Session 用户消息失败",
		},
		{
			name:     "assistant_message_persistence",
			composer: func(*session.Session) PromptComposer { return staticComposer{} },
			provider: func(sess *session.Session) provider.LLMProvider {
				return lifecycleProviderFunc(func(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
					replaceLifecycleFileWithDirectory(t, sess.MessagesPath())
					return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "uncommitted"}}, nil
				})
			},
			wantError:  "写入 Session 助手消息失败",
			wantModels: 1,
		},
		{
			name:     "tool_result_message_persistence",
			composer: func(*session.Session) PromptComposer { return staticComposer{} },
			provider: func(*session.Session) provider.LLMProvider {
				return &sequencedProvider{responses: []*provider.GenerateResponse{{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "call-break-log", Name: "break_message_log", Arguments: json.RawMessage(`{}`)}}}}}}
			},
			registry: func(sess *session.Session) tools.Registry {
				registry := tools.NewRegistry()
				registry.Register(&lifecycleMutatingTool{path: sess.MessagesPath()})
				return registry
			},
			wantError:  "写入 Session 工具结果失败",
			wantFacts:  []string{"tool_call:break_message_log", "tool_result:break_message_log:false"},
			wantModels: 1,
			wantTools:  1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			workDir := t.TempDir()
			sess := newLifecycleSession(t, workDir)
			registry := tools.Registry(tools.NewRegistry())
			if testCase.registry != nil {
				registry = testCase.registry(sess)
			}
			reporter := &recordingReporter{}
			eng := NewAgentEngine(testCase.provider(sess), registry, workDir, testCase.composer(sess), Config{MaxTurns: 2})
			result, runErr := eng.RunWithReporter(context.Background(), sess, "fail lifecycle", reporter)
			if runErr == nil || !strings.Contains(runErr.Error(), testCase.wantError) {
				t.Fatalf("RunWithReporter() result/error = %#v, %v; want %q", result, runErr, testCase.wantError)
			}
			if result != nil {
				t.Fatalf("RunWithReporter() result = %#v, want nil", result)
			}
			if len(reporter.events) < 2 || !strings.HasPrefix(reporter.events[0], "start:") || !strings.HasPrefix(reporter.events[len(reporter.events)-1], "error:") {
				t.Fatalf("reporter events = %#v, want start ... error", reporter.events)
			}
			for _, want := range testCase.wantFacts {
				if !containsString(reporter.events, want) {
					t.Fatalf("reporter events = %#v, missing %q", reporter.events, want)
				}
			}
			run := assertLifecycleRunFinished(t, sess, "")
			assertLifecycleFailedRunArtifacts(t, run, testCase.wantModels, testCase.wantTools, testCase.wantErrors)
		})
	}
}

func TestToolResultArtifactPersistenceFailureKeepsCurrentNonFatalFallback(t *testing.T) {
	workDir := t.TempDir()
	sess := newLifecycleSession(t, workDir)
	registry := tools.NewRegistry()
	fullOutput := strings.Repeat("P", 60_000)
	registry.Register(&bigOutputTool{name: "large_lifecycle", output: fullOutput})
	modelProvider := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "call-persist-fail", Name: "large_lifecycle", Arguments: json.RawMessage(`{}`)}}}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "continued after artifact failure"}},
	}}
	reporter := &recordingReporter{}
	eng := NewAgentEngine(modelProvider, registry, workDir, staticComposer{}, Config{MaxTurns: 3})
	eng.WithFileSystem(lifecycleFailingFileSystem{})

	result, err := eng.RunWithReporter(context.Background(), sess, "persist large result", reporter)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result == nil || result.FinalMessage != "continued after artifact failure" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.TelemetryWarnings) != 0 {
		t.Fatalf("TelemetryWarnings = %#v, want current empty surface", result.TelemetryWarnings)
	}
	if len(modelProvider.seen) != 2 || !messagesContain(modelProvider.seen[1], fullOutput) {
		t.Fatalf("fallback model context did not retain full output")
	}
	if _, err := os.Stat(filepath.Join(sess.ToolResultsDir(), "call-persist-fail.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("persisted artifact stat error = %v, want not exist", err)
	}
	assertLifecycleRunFinished(t, sess, result.RunID)
}

func TestCancellationDuringModelStreamingStopsObservationAndFinishesRun(t *testing.T) {
	workDir := t.TempDir()
	sess := newLifecycleSession(t, workDir)
	modelProvider := &lifecycleBlockingStreamProvider{entered: make(chan struct{}), stopped: make(chan struct{})}
	reporter := &deltaRecordingReporter{}
	eng := NewAgentEngine(modelProvider, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 2})
	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		result *RunResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := eng.RunWithReporter(ctx, sess, "cancel stream", reporter)
		done <- outcome{result: result, err: err}
	}()

	select {
	case <-modelProvider.entered:
	case <-time.After(time.Second):
		t.Fatal("stream did not start")
	}
	cancel()
	select {
	case <-modelProvider.stopped:
	case <-time.After(time.Second):
		t.Fatal("stream provider did not stop after cancellation")
	}
	var got outcome
	select {
	case got = <-done:
	case <-time.After(time.Second):
		t.Fatal("run did not terminate after stream cancellation")
	}
	if got.result != nil || !errors.Is(got.err, context.Canceled) {
		t.Fatalf("cancelled result/error = %#v, %v", got.result, got.err)
	}
	if fmt.Sprint(reporter.deltas) != fmt.Sprint([]string{"partial delta"}) {
		t.Fatalf("deltas = %#v", reporter.deltas)
	}
	if len(reporter.events) != 2 || !strings.HasPrefix(reporter.events[0], "start:") || !strings.HasPrefix(reporter.events[1], "error:") {
		t.Fatalf("events = %#v, want start/error only", reporter.events)
	}
	assertLifecycleRunFinished(t, sess, "")
}

func TestCurrentAdapterReportsUsageFinishReasonAndTurnCountOnce(t *testing.T) {
	call := runtimecontract.ToolCall{ID: "call-outcome", Name: "outcome_tool", Arguments: `{}`}
	observed, err := newCurrentProductionContractAdapter(t).Run(context.Background(), runtimecontract.RunInput{
		Profile: "CLIExec", Prompt: "report outcome", DisplayPrompt: "report outcome", SessionChoice: "new", Model: "scripted", Provider: "scripted", MaxTurns: 3,
	}, runtimecontract.Script{
		Tools: []runtimecontract.ToolBehavior{{Call: call, Definition: runtimecontract.ToolDefinition{Name: call.Name, InputSchema: `{}`}, Result: runtimecontract.ToolResult{Output: "ok"}}},
		ModelSteps: []runtimecontract.ModelStep{
			{Response: runtimecontract.ModelResponse{ToolCalls: []runtimecontract.ToolCall{call}, FinishReason: "tool_calls", Usage: runtimecontract.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}}},
			{Response: runtimecontract.ModelResponse{Content: "outcome complete", FinishReason: "stop", Usage: runtimecontract.Usage{InputTokens: 7, OutputTokens: 3, TotalTokens: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("adapter Run() error = %v", err)
	}
	wantOutcome := runtimecontract.Outcome{FinalMessage: "outcome complete", FinishReason: "stop", TurnCount: 2, Usage: runtimecontract.Usage{InputTokens: 11, OutputTokens: 5, TotalTokens: 16}}
	if observed.Outcome != wantOutcome {
		t.Fatalf("outcome = %#v, want %#v", observed.Outcome, wantOutcome)
	}
	var completions, summaries int
	for _, fact := range observed.Facts {
		if fact.Kind == "run_completed" {
			completions++
		}
	}
	for _, metric := range observed.Metrics {
		if metric.Kind == string(metrics.EventRunSummary) {
			summaries++
		}
	}
	if completions != 1 || summaries != 1 {
		t.Fatalf("completion/summary count = %d/%d, want 1/1", completions, summaries)
	}
}

func newLifecycleSession(t *testing.T, workDir string) *session.Session {
	t.Helper()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return sess
}

type lifecycleComposerFunc func(string) (string, error)

func (f lifecycleComposerFunc) Compose(prompt string) (string, error) { return f(prompt) }

type lifecycleProviderFunc func(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error)

func (f lifecycleProviderFunc) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return f(ctx, messages, definitions)
}

type lifecycleMutatingTool struct {
	path string
}

func (t *lifecycleMutatingTool) Name() string { return "break_message_log" }
func (t *lifecycleMutatingTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: t.Name(), InputSchema: map[string]any{}}
}
func (t *lifecycleMutatingTool) Execute(context.Context, json.RawMessage) (string, error) {
	if err := os.Remove(t.path); err != nil {
		return "", err
	}
	if err := os.Mkdir(t.path, 0o755); err != nil {
		return "", err
	}
	return "message log replaced", nil
}

type lifecycleFailingFileSystem struct{}

func (lifecycleFailingFileSystem) WriteFile(string, []byte, os.FileMode) error {
	return errors.New("fixture write failure")
}
func (lifecycleFailingFileSystem) Stat(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
func (lifecycleFailingFileSystem) MkdirAll(string, os.FileMode) error {
	return errors.New("fixture mkdir failure")
}

type lifecycleBlockingStreamProvider struct {
	once    sync.Once
	entered chan struct{}
	stopped chan struct{}
}

func (p *lifecycleBlockingStreamProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return nil, errors.New("unexpected non-stream fallback")
}

func (p *lifecycleBlockingStreamProvider) GenerateStream(ctx context.Context, _ []schema.Message, _ []schema.ToolDefinition, _ provider.GenerateOptions, callbacks provider.StreamCallbacks) (*provider.GenerateResponse, error) {
	p.once.Do(func() { close(p.entered) })
	callbacks.EmitTextDelta("partial delta")
	<-ctx.Done()
	close(p.stopped)
	return nil, ctx.Err()
}

func replaceLifecycleFileWithDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Remove(%s) error = %v", path, err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir(%s) error = %v", path, err)
	}
}

func assertLifecycleRunFinished(t *testing.T, sess *session.Session, wantRunID string) *session.Run {
	t.Helper()
	entries, err := os.ReadDir(sess.RunsDir())
	if err != nil {
		t.Fatalf("ReadDir(runs) error = %v", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("run entries = %#v, want one directory", entries)
	}
	runPath := filepath.Join(sess.RunsDir(), entries[0].Name(), "run.json")
	data, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatalf("ReadFile(run.json) error = %v", err)
	}
	var run session.Run
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatalf("Unmarshal(run.json) error = %v", err)
	}
	if run.SessionID != sess.ID || run.EndedAt == nil {
		t.Fatalf("finished run = %#v, want session %q and ended_at", run, sess.ID)
	}
	if wantRunID != "" && run.ID != session.RunID(wantRunID) {
		t.Fatalf("run ID = %q, want %q", run.ID, wantRunID)
	}
	return &run
}

func assertLifecycleResultPaths(t *testing.T, sess *session.Session, result *RunResult) {
	t.Helper()
	if result == nil {
		t.Fatal("result = nil")
	}
	runRoot := filepath.Join(sess.RunsDir(), result.RunID)
	if result.SessionID != string(sess.ID) || result.MetricsPath != filepath.Join(runRoot, "metrics.jsonl") || result.TracePath != filepath.Join(runRoot, "trace.jsonl") {
		t.Fatalf("result identity/paths = %#v, want session %q under %s", result, sess.ID, runRoot)
	}
	for _, path := range []string{result.MetricsPath, result.TracePath, filepath.Join(runRoot, "artifacts")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Stat(%s) error = %v", path, err)
		}
	}
}

func assertLifecycleMetricsExactlyOnce(t *testing.T, path string, wantModels, wantTools, wantErrors int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(metrics) error = %v", err)
	}
	var modelCount, toolCount, summaryCount int
	var summary metrics.RunSummary
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var header struct {
			Type metrics.EventType `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &header); err != nil {
			t.Fatalf("Unmarshal(metric header) error = %v", err)
		}
		switch header.Type {
		case metrics.EventModelCall:
			modelCount++
		case metrics.EventToolCall:
			toolCount++
		case metrics.EventRunSummary:
			summaryCount++
			if err := json.Unmarshal([]byte(line), &summary); err != nil {
				t.Fatalf("Unmarshal(run summary) error = %v", err)
			}
		}
	}
	if modelCount != wantModels || toolCount != wantTools || summaryCount != 1 {
		t.Fatalf("model/tool/summary metric count = %d/%d/%d, want %d/%d/1", modelCount, toolCount, summaryCount, wantModels, wantTools)
	}
	if summary.TotalModelCalls != wantModels || summary.TotalToolCalls != wantTools || summary.ErrorCount != wantErrors {
		t.Fatalf("run summary = %#v, want model/tool/errors %d/%d/%d", summary, wantModels, wantTools, wantErrors)
	}
}

func assertLifecycleFailedRunArtifacts(t *testing.T, run *session.Run, wantModels, wantTools, wantErrors int) {
	t.Helper()
	assertLifecycleMetricsExactlyOnce(t, run.MetricsPath(), wantModels, wantTools, wantErrors)
	events, err := tracing.Load(run.TracePath())
	if err != nil {
		t.Fatalf("Load(trace) error = %v", err)
	}
	var runEnds int
	for _, event := range events {
		if event.Type == tracing.EventSpanEnd && event.Name == "run" {
			runEnds++
			if event.Status != "error" || strings.TrimSpace(fmt.Sprint(event.Attrs["error"])) == "" {
				t.Fatalf("run terminal trace = %#v, want error with cause", event)
			}
		}
	}
	if runEnds != 1 {
		t.Fatalf("run trace end count = %d, want 1", runEnds)
	}
}
