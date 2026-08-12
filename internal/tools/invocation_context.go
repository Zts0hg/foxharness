package tools

import "context"

type invocationContextKey struct{}

// InvocationContext identifies the parent run and exact model tool call whose
// execution context reaches a tool implementation.
type InvocationContext struct {
	SessionID  string
	RunID      string
	ToolCallID string
}

// WithRunContext returns a child context carrying the current runtime run.
func WithRunContext(ctx context.Context, sessionID, runID string) context.Context {
	invocation, _ := InvocationContextFrom(ctx)
	invocation.SessionID = sessionID
	invocation.RunID = runID
	return context.WithValue(ctx, invocationContextKey{}, invocation)
}

func withToolCallContext(ctx context.Context, toolCallID string) context.Context {
	invocation, _ := InvocationContextFrom(ctx)
	invocation.ToolCallID = toolCallID
	return context.WithValue(ctx, invocationContextKey{}, invocation)
}

// InvocationContextFrom returns tool invocation lineage carried by ctx.
func InvocationContextFrom(ctx context.Context) (InvocationContext, bool) {
	if ctx == nil {
		return InvocationContext{}, false
	}
	invocation, ok := ctx.Value(invocationContextKey{}).(InvocationContext)
	return invocation, ok
}
