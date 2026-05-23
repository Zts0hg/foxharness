package tui

import (
	"context"
	"encoding/json"
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
		status: fmt.Sprintf("Run started: %s", runID),
	})
}

func (r *channelReporter) OnThinking(ctx context.Context, turn int) {
	r.send(ctx, runEventMsg{
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
		body:   formatToolInvocation(toolName, args),
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

func formatToolInvocation(toolName string, args string) string {
	fields := parseToolArgs(args)
	switch toolName {
	case "bash":
		if command := fields["command"]; command != "" {
			return "Bash (" + truncateInline(command, 120) + ")"
		}
	case "read_file":
		if path := fields["path"]; path != "" {
			return "Read (" + truncateInline(path, 120) + ")"
		}
	case "write_file":
		if path := fields["path"]; path != "" {
			return "Write (" + truncateInline(path, 120) + ")"
		}
	case "edit_file":
		if path := fields["path"]; path != "" {
			return "Edit (" + truncateInline(path, 120) + ")"
		}
	case "delegate_task":
		if task := fields["task"]; task != "" {
			return "Task (" + truncateInline(task, 80) + ")"
		}
	}

	args = strings.TrimSpace(args)
	if args == "" {
		return toolName
	}
	return fmt.Sprintf("%s(%s)", toolName, truncateInline(args, 120))
}

func parseToolArgs(args string) map[string]string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return nil
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return out
}

func truncateInline(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
