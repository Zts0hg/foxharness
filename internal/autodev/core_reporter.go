package autodev

import "context"

// CoreRunResult is the presentation-neutral terminal runtime evidence needed by Autodev.
type CoreRunResult struct {
	SessionID    string
	RunID        string
	FinalMessage string
}

// CoreReporter receives the complete, non-delta core runtime event stream.
type CoreReporter interface {
	OnRunStart(context.Context, string, string)
	OnThinking(context.Context, int)
	OnCompaction(context.Context, string)
	OnToolCall(context.Context, string, string)
	OnToolResult(context.Context, string, string, bool)
	OnMessage(context.Context, string)
	OnRunComplete(context.Context, CoreRunResult)
	OnRunError(context.Context, string, string, error)
}
