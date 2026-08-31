package tools

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
)

// InvocationContext identifies the parent run and exact model tool call whose
// execution context reaches a tool implementation.
type InvocationContext = toolprotocol.Invocation

// WithRunContext returns a child context carrying the current runtime run.
func WithRunContext(ctx context.Context, sessionID, runID string) context.Context {
	return toolprotocol.WithRun(ctx, sessionID, runID)
}

func withToolCallContext(ctx context.Context, toolCallID string) context.Context {
	return toolprotocol.WithToolCall(ctx, toolCallID)
}

func withToolCapabilities(ctx context.Context, definitions []schema.ToolDefinition) context.Context {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	return toolprotocol.WithCapabilities(ctx, names)
}

func withDefaultToolCapabilities(ctx context.Context, definitions []schema.ToolDefinition) context.Context {
	if toolprotocol.HasCapabilities(ctx) {
		return ctx
	}
	return withToolCapabilities(ctx, definitions)
}

// InvocationContextFrom returns tool invocation lineage carried by ctx.
func InvocationContextFrom(ctx context.Context) (InvocationContext, bool) {
	return toolprotocol.FromContext(ctx)
}
