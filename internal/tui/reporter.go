package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

type channelNotificationSink struct {
	events      chan<- tea.Msg
	operationID uint64
	mu          sync.Mutex
	streaming   bool
}

func (s *channelNotificationSink) OnRunStart(ctx context.Context, sessionID string, runID string) {
	s.Notify(ctx, app.Notification{Kind: app.NotificationRunStarted, SessionID: sessionID, RunID: runID})
}

func (s *channelNotificationSink) OnThinking(ctx context.Context, turn int) {
	s.Notify(ctx, app.Notification{Kind: app.NotificationThinking, Turn: turn})
}

func (s *channelNotificationSink) OnCompaction(ctx context.Context, scope string) {
	s.Notify(ctx, app.Notification{Kind: app.NotificationContextCompacted, Phase: scope})
}

func (s *channelNotificationSink) OnToolCall(ctx context.Context, name string, arguments string) {
	s.Notify(ctx, app.Notification{Kind: app.NotificationToolCall, Name: name, Content: arguments})
}

func (s *channelNotificationSink) OnToolResult(ctx context.Context, name string, result string, isError bool) {
	s.Notify(ctx, app.Notification{Kind: app.NotificationToolResult, Name: name, Content: result, IsError: isError})
}

func (s *channelNotificationSink) OnMessage(ctx context.Context, content string) {
	s.Notify(ctx, app.Notification{Kind: app.NotificationMessage, Content: content})
}

func (s *channelNotificationSink) OnMessageDelta(ctx context.Context, content string) {
	s.Notify(ctx, app.Notification{Kind: app.NotificationMessageDelta, Content: content})
}

func (s *channelNotificationSink) OnRunError(ctx context.Context, sessionID string, runID string, err error) {
	content := ""
	if err != nil {
		content = err.Error()
	}
	s.Notify(ctx, app.Notification{
		Kind: app.NotificationRunError, SessionID: sessionID, RunID: runID,
		Content: content, IsError: true,
	})
}

func (s *channelNotificationSink) Notify(ctx context.Context, notification app.Notification) {
	event := runEventMsg{operationID: s.operationID}
	switch notification.Kind {
	case app.NotificationRunStarted:
		event.status = fmt.Sprintf("Run started: %s", notification.RunID)
	case app.NotificationThinking:
		event.status = fmt.Sprintf("Thinking turn %d", notification.Turn)
	case app.NotificationContextCompacted:
		event.role = "system"
		event.title = "context compacted"
		event.body = fmt.Sprintf("Compacted context scope: %s", notification.Phase)
		event.status = "Context compacted"
	case app.NotificationToolCall:
		event.role = "tool"
		event.title = "call " + notification.Name
		event.body = formatToolInvocation(notification.Name, notification.Content)
		event.status = "Calling tool: " + notification.Name
	case app.NotificationToolResult:
		event.role = "tool"
		event.title = "result " + notification.Name
		event.body = strings.TrimSpace(notification.Content)
		event.status = "Tool complete: " + notification.Name
		event.err = notification.IsError
		if notification.IsError {
			event.status = "Tool failed: " + notification.Name
		}
	case app.NotificationMessageDelta:
		if notification.Content == "" {
			return
		}
		s.mu.Lock()
		s.streaming = true
		s.mu.Unlock()
		event.role = "assistant"
		event.title = "stream"
		event.body = notification.Content
		event.status = "Assistant responding"
		event.delta = true
	case app.NotificationMessage:
		content := strings.TrimSpace(notification.Content)
		if content == "" {
			return
		}
		s.mu.Lock()
		streaming := s.streaming
		s.streaming = false
		s.mu.Unlock()
		event.role = "assistant"
		event.title = "foxharness"
		event.body = content
		event.status = "Assistant responded"
		event.streamFinal = streaming
	case app.NotificationRunCompleted:
		event.status = fmt.Sprintf("Run complete: %s", notification.RunID)
	case app.NotificationRunError:
		event.role = "error"
		event.title = "run error"
		event.body = fmt.Sprintf("Session: %s\nRun: %s\nError: %s", notification.SessionID, notification.RunID, notification.Content)
		event.status = "Run failed"
		event.err = true
	default:
		return
	}
	s.send(ctx, event)
}

func (s *channelNotificationSink) send(ctx context.Context, msg tea.Msg) {
	if s == nil || s.events == nil {
		return
	}
	if event, ok := msg.(runEventMsg); ok {
		if event.operationID == 0 {
			event.operationID = s.operationID
		}
		msg = event
	}
	select {
	case s.events <- msg:
	case <-ctx.Done():
	}
}

var _ app.NotificationSink = (*channelNotificationSink)(nil)

func formatToolInvocation(toolName string, args string) string {
	fields := parseToolArgs(args)
	switch toolName {
	case "bash":
		if command := fields["command"]; command != "" {
			return "Bash (" + normalizeInline(command) + ")"
		}
	case "read_file":
		if path := fields["path"]; path != "" {
			return "Read (" + normalizeInline(path) + ")"
		}
	case "write_file":
		if path := fields["path"]; path != "" {
			return "Write (" + normalizeInline(path) + ")"
		}
	case "edit_file":
		if path := fields["path"]; path != "" {
			return "Edit (" + normalizeInline(path) + ")"
		}
	case "read_todo":
		return "Read TODO"
	case "update_todo":
		return "Update TODO"
	case "submit_plan":
		return "Submit plan"
	case "delegate_task":
		if task := fields["task"]; task != "" {
			return "Task (" + normalizeInline(task) + ")"
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
	s = normalizeInline(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func normalizeInline(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
