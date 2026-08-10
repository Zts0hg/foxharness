package benchmark

import (
	"context"
	"sync"

	"github.com/Zts0hg/foxharness/internal/engine"
)

type runIdentityReporter struct {
	mu    sync.RWMutex
	runID string
}

func (reporter *runIdentityReporter) OnRunStart(_ context.Context, _ string, runID string) {
	reporter.mu.Lock()
	reporter.runID = runID
	reporter.mu.Unlock()
}

func (*runIdentityReporter) OnThinking(context.Context, int)                    {}
func (*runIdentityReporter) OnCompaction(context.Context, string)               {}
func (*runIdentityReporter) OnToolCall(context.Context, string, string)         {}
func (*runIdentityReporter) OnToolResult(context.Context, string, string, bool) {}
func (*runIdentityReporter) OnMessage(context.Context, string)                  {}
func (*runIdentityReporter) OnRunComplete(context.Context, engine.RunResult)    {}
func (*runIdentityReporter) OnRunError(context.Context, string, string, error)  {}

func (reporter *runIdentityReporter) RunID() string {
	reporter.mu.RLock()
	defer reporter.mu.RUnlock()
	return reporter.runID
}
