package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/engine"
	tea "github.com/charmbracelet/bubbletea"
)

type channelReporter struct {
	events chan<- tea.Msg
}

func (r *channelReporter) OnRunStart(ctx context.Context, sessionID string, runID string) {
	r.send(ctx, runEventMsg{
		role:   "system",
		title:  "run started",
		body:   fmt.Sprintf("Session: %s\nRun: %s", sessionID, runID),
		status: fmt.Sprintf("Run started: %s", runID),
	})
}

func (r *channelReporter) OnThinking(ctx context.Context, turn int) {
	r.send(ctx, runEventMsg{
		role:   "system",
		title:  "thinking",
		body:   fmt.Sprintf("Planning turn %d.", turn),
		status: fmt.Sprintf("Thinking turn %d", turn),
	})
}

func (r *channelReporter) OnCompaction(ctx context.Context, scope string) {
	r.send(ctx, runEventMsg{
		role:   "system",
		title:  "context compacted",
		body:   fmt.Sprintf("Compacted context scope: %s", scope),
		status: "Context compacted",
	})
}

func (r *channelReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	r.send(ctx, runEventMsg{
		role:   "tool",
		title:  "call " + toolName,
		body:   strings.TrimSpace(args),
		status: "Calling tool: " + toolName,
	})
}

func (r *channelReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	status := "Tool complete: " + toolName
	if isError {
		status = "Tool failed: " + toolName
	}
	r.send(ctx, runEventMsg{
		role:   "tool",
		title:  "result " + toolName,
		body:   strings.TrimSpace(result),
		status: status,
		err:    isError,
	})
}

func (r *channelReporter) OnMessage(ctx context.Context, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	r.send(ctx, runEventMsg{
		role:   "assistant",
		title:  "foxharness",
		body:   content,
		status: "Assistant responded",
	})
}

func (r *channelReporter) OnRunComplete(ctx context.Context, result engine.RunResult) {
	r.send(ctx, runEventMsg{
		status: fmt.Sprintf("Run complete: %s", result.RunID),
	})
}

func (r *channelReporter) OnRunError(ctx context.Context, sessionID string, runID string, err error) {
	r.send(ctx, runEventMsg{
		role:   "error",
		title:  "run error",
		body:   fmt.Sprintf("Session: %s\nRun: %s\nError: %v", sessionID, runID, err),
		status: "Run failed",
		err:    true,
	})
}

func (r *channelReporter) send(ctx context.Context, msg tea.Msg) {
	if r == nil || r.events == nil {
		return
	}
	select {
	case r.events <- msg:
	case <-ctx.Done():
	}
}

var _ engine.Reporter = (*channelReporter)(nil)
