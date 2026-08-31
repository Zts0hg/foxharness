/* Package toolprotocol defines narrow values shared by tool adapters and executors. */
package toolprotocol

import "context"

type invocationContextKey struct{}
type capabilityContextKey struct{}

/* Invocation identifies the parent run and exact model tool call. */
type Invocation struct {
	SessionID  string
	RunID      string
	ToolCallID string
}

/* WithCapabilities returns a context carrying one immutable effective tool snapshot. */
func WithCapabilities(ctx context.Context, allowed []string) context.Context {
	return context.WithValue(ctx, capabilityContextKey{}, cloneStrings(allowed))
}

/* CapabilitiesFromContext returns a defensive copy of the effective tool snapshot. */
func CapabilitiesFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	allowed, _ := ctx.Value(capabilityContextKey{}).([]string)
	return cloneStrings(allowed)
}

/* HasCapabilities reports whether an outer executor already supplied an authoritative snapshot. */
func HasCapabilities(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Value(capabilityContextKey{}).([]string)
	return ok
}

/* ExecutionResult carries output together with a model-visible failure flag. */
type ExecutionResult struct {
	Output string
	Failed bool
}

/* WithRun returns a context carrying the current runtime run identity. */
func WithRun(ctx context.Context, sessionID, runID string) context.Context {
	invocation, _ := FromContext(ctx)
	invocation.SessionID = sessionID
	invocation.RunID = runID
	return context.WithValue(ctx, invocationContextKey{}, invocation)
}

/* WithToolCall returns a context carrying the exact tool-call identity. */
func WithToolCall(ctx context.Context, toolCallID string) context.Context {
	invocation, _ := FromContext(ctx)
	invocation.ToolCallID = toolCallID
	return context.WithValue(ctx, invocationContextKey{}, invocation)
}

/* FromContext returns the invocation lineage carried by ctx. */
func FromContext(ctx context.Context) (Invocation, bool) {
	if ctx == nil {
		return Invocation{}, false
	}
	invocation, ok := ctx.Value(invocationContextKey{}).(Invocation)
	return invocation, ok
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}
