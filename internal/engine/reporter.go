package engine

import "context"

// Reporter receives human-facing lifecycle events from an engine run.
// It keeps output surfaces such as Feishu, CLI, or Web UI out of the core
// agent loop while still allowing them to stream progress to users.
type Reporter interface {
	OnRunStart(ctx context.Context, sessionID string, runID string)
	OnThinking(ctx context.Context, turn int)
	OnCompaction(ctx context.Context, scope string)
	OnToolCall(ctx context.Context, toolName string, args string)
	OnToolResult(ctx context.Context, toolName string, result string, isError bool)
	OnMessage(ctx context.Context, content string)
	OnRunComplete(ctx context.Context, result RunResult)
	OnRunError(ctx context.Context, sessionID string, runID string, err error)
}
