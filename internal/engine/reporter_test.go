package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/tracing"
)

type finalProvider struct{}

func (p *finalProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"},
	}, nil
}

type loopingProvider struct {
	calls      int
	finalAfter int
}

func (p *loopingProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	if p.calls >= p.finalAfter {
		return &provider.GenerateResponse{
			Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"},
		}, nil
	}
	return &provider.GenerateResponse{
		Message: &schema.Message{
			Role:    schema.RoleAssistant,
			Content: fmt.Sprintf("turn %d", p.calls),
			ToolCalls: []schema.ToolCall{
				{ID: fmt.Sprintf("call-%d", p.calls), Name: "missing_tool"},
			},
		},
	}, nil
}

func TestDefaultConfigLeavesMaxTurnsUnlimited(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxTurns != 0 {
		t.Fatalf("DefaultConfig().MaxTurns = %d, want 0 for unlimited", cfg.MaxTurns)
	}
}

func TestRunWithZeroMaxTurnsIsUnlimited(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	provider := &loopingProvider{finalAfter: 22}
	eng := NewAgentEngine(
		provider,
		tools.NewRegistry(),
		workDir,
		staticComposer{},
		Config{},
	)

	result, err := eng.RunWithReporter(context.Background(), sess, "hello", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result.FinalMessage != "done" {
		t.Fatalf("FinalMessage = %q, want done", result.FinalMessage)
	}
	if provider.calls != 22 {
		t.Fatalf("provider calls = %d, want 22", provider.calls)
	}
}

func TestRunWithPositiveMaxTurnsStillLimits(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	provider := &loopingProvider{finalAfter: 4}
	eng := NewAgentEngine(
		provider,
		tools.NewRegistry(),
		workDir,
		staticComposer{},
		Config{MaxTurns: 2},
	)

	result, err := eng.RunWithReporter(context.Background(), sess, "hello", nil)
	if err == nil {
		t.Fatal("RunWithReporter() error = nil, want max turn limit error")
	}
	if !strings.Contains(err.Error(), "超过最大 Turn 数限制: 2") {
		t.Fatalf("error = %q, want max turn limit", err.Error())
	}
	if result == nil {
		t.Fatal("RunWithReporter() result = nil, want partial result")
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}
}

type snapshotRecorder struct {
	messageIDs []string
}

func (r *snapshotRecorder) TrackEdit(filePath, messageID string) error { return nil }
func (r *snapshotRecorder) MakeSnapshot(messageID string) error {
	r.messageIDs = append(r.messageIDs, messageID)
	return nil
}
func (r *snapshotRecorder) Rewind(messageID string) ([]string, error) { return nil, nil }
func (r *snapshotRecorder) GetDiffStats(messageID string) (*checkpoint.DiffStats, error) {
	return nil, nil
}
func (r *snapshotRecorder) HasAnyChanges(messageID string) (bool, error) { return false, nil }
func (r *snapshotRecorder) SetDisabled(disabled bool)                    {}
func (r *snapshotRecorder) IsDisabled() bool                             { return false }
func (r *snapshotRecorder) RestoreStateFromLog() error                   { return nil }

func TestEngineMakeSnapshotOnUserMessage(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	cp := &snapshotRecorder{}
	var currentMessageID string
	eng := NewAgentEngine(
		&finalProvider{},
		tools.NewRegistry(),
		workDir,
		staticComposer{},
		Config{
			MaxTurns:        3,
			Checkpointer:    cp,
			OnUserMessageID: func(id string) { currentMessageID = id },
		},
	)
	if _, err := eng.RunWithReporter(context.Background(), sess, "hello", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if len(cp.messageIDs) != 1 || cp.messageIDs[0] != "0" {
		t.Fatalf("MakeSnapshot calls = %#v, want [0]", cp.messageIDs)
	}
	if currentMessageID != "0" {
		t.Fatalf("currentMessageID = %q, want 0", currentMessageID)
	}
}

func TestModelCallTraceRecordsProviderAndModel(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	eng := NewAgentEngine(
		&finalProvider{},
		tools.NewRegistry(),
		workDir,
		staticComposer{},
		Config{
			MaxTurns:         3,
			ProviderProtocol: "openai",
			Model:            "trace-model",
		},
	)

	result, err := eng.RunWithReporter(context.Background(), sess, "hello", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}

	events, err := tracing.Load(result.TracePath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, event := range events {
		if event.Type != tracing.EventSpanStart || event.Name != "model_call" {
			continue
		}
		if event.Attrs["provider_protocol"] != "openai" {
			t.Fatalf("provider_protocol attr = %#v, want openai; attrs = %#v", event.Attrs["provider_protocol"], event.Attrs)
		}
		if event.Attrs["model"] != "trace-model" {
			t.Fatalf("model attr = %#v, want trace-model; attrs = %#v", event.Attrs["model"], event.Attrs)
		}
		return
	}
	t.Fatalf("trace did not contain model_call span_start: %#v", events)
}

type staticComposer struct{}

func (c staticComposer) Compose(userPrompt string) (string, error) {
	return "system", nil
}

type recordingReporter struct {
	events []string
}

func (r *recordingReporter) OnRunStart(ctx context.Context, sessionID string, runID string) {
	r.events = append(r.events, fmt.Sprintf("start:%s:%s", sessionID, runID))
}

func (r *recordingReporter) OnThinking(ctx context.Context, turn int) {
	r.events = append(r.events, fmt.Sprintf("thinking:%d", turn))
}

func (r *recordingReporter) OnCompaction(ctx context.Context, scope string) {
	r.events = append(r.events, fmt.Sprintf("compaction:%s", scope))
}

func (r *recordingReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	r.events = append(r.events, fmt.Sprintf("tool_call:%s", toolName))
}

func (r *recordingReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	r.events = append(r.events, fmt.Sprintf("tool_result:%s:%t", toolName, isError))
}

func (r *recordingReporter) OnMessage(ctx context.Context, content string) {
	r.events = append(r.events, fmt.Sprintf("message:%s", content))
}

func (r *recordingReporter) OnRunComplete(ctx context.Context, result RunResult) {
	r.events = append(r.events, fmt.Sprintf("complete:%s:%s", result.SessionID, result.RunID))
}

func (r *recordingReporter) OnRunError(ctx context.Context, sessionID string, runID string, err error) {
	r.events = append(r.events, fmt.Sprintf("error:%s:%s", sessionID, runID))
}

func TestRunWithReporterEmitsLifecycleAndPersistsMessages(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	eng := NewAgentEngine(
		&finalProvider{},
		tools.NewRegistry(),
		workDir,
		staticComposer{},
		Config{MaxTurns: 3},
	)
	reporter := &recordingReporter{}

	result, err := eng.RunWithReporter(context.Background(), sess, "hello", reporter)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result == nil {
		t.Fatalf("RunWithReporter() result = nil")
	}
	if result.SessionID != sess.ID {
		t.Fatalf("result.SessionID = %q, want %q", result.SessionID, sess.ID)
	}
	if result.RunID == "" {
		t.Fatalf("result.RunID is empty")
	}

	wantEvents := []string{
		fmt.Sprintf("start:%s:%s", sess.ID, result.RunID),
		"message:done",
		fmt.Sprintf("complete:%s:%s", sess.ID, result.RunID),
	}
	if len(reporter.events) != len(wantEvents) {
		t.Fatalf("events = %#v, want %#v", reporter.events, wantEvents)
	}
	for i, want := range wantEvents {
		if reporter.events[i] != want {
			t.Fatalf("events[%d] = %q, want %q; all events = %#v", i, reporter.events[i], want, reporter.events)
		}
	}

	messages, err := session.NewMessageLog(sess).LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[0].Role != schema.RoleUser || messages[0].Content != "hello" {
		t.Fatalf("first message = %#v, want user hello", messages[0])
	}
	if messages[1].Role != schema.RoleAssistant || messages[1].Content != "done" {
		t.Fatalf("second message = %#v, want assistant done", messages[1])
	}
}

func TestRunWithoutReporterDoesNotWriteStdout(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	eng := NewAgentEngine(
		&finalProvider{},
		tools.NewRegistry(),
		workDir,
		staticComposer{},
		Config{MaxTurns: 3},
	)

	stdout := captureStdout(t, func() {
		result, err := eng.RunWithReporter(context.Background(), sess, "hello", nil)
		if err != nil {
			t.Fatalf("RunWithReporter() error = %v", err)
		}
		if result == nil || result.FinalMessage != "done" {
			t.Fatalf("RunWithReporter() result = %#v, want final message done", result)
		}
	})

	if stdout != "" {
		t.Fatalf("RunWithReporter() wrote stdout %q, want empty", stdout)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = previous
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("reader Close() error = %v", err)
	}

	return string(out)
}
