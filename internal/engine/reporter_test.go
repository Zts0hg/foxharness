package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type finalProvider struct{}

func (p *finalProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	return &schema.Message{Role: schema.RoleAssistant, Content: "done"}, nil
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
