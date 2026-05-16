package engine

import "context"

// Reporter receives human-facing lifecycle events from an engine run.
// It keeps output surfaces such as Feishu, CLI, or Web UI out of the core
// agent loop while still allowing them to stream progress to users.
type Reporter interface {
	OnThinking(ctx context.Context, turn int)
	OnToolCall(ctx context.Context, toolName string, args string)
	OnToolResult(ctx context.Context, toolName string, result string, isError bool)
	OnMessage(ctx context.Context, content string)
}
