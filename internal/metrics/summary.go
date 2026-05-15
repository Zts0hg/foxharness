package metrics

import "time"

// Aggregator accumulates model and tool call counts during a run and
// produces a RunSummary on demand.
type Aggregator struct {
	startedAt    time.Time
	modelCalls   int
	toolCalls    int
	inputTokens  int
	outputTokens int
	errorCount   int
}

// NewAggregator returns an Aggregator initialized with the current time
// as the start of the run.
func NewAggregator() *Aggregator {
	return &Aggregator{startedAt: time.Now()}
}

// AddModel records one model invocation, incrementing token counters and
// optionally the error count.
func (a *Aggregator) AddModel(inputTokens, outputTokens int, hasError bool) {
	a.modelCalls++
	a.inputTokens += inputTokens
	a.outputTokens += outputTokens
	if hasError {
		a.errorCount++
	}
}

// AddTool records one tool invocation, optionally incrementing the error
// count.
func (a *Aggregator) AddTool(hasError bool) {
	a.toolCalls++
	if hasError {
		a.errorCount++
	}
}

// Summary returns a RunSummary snapshot containing all accumulated
// counters and the wall-clock duration since the aggregator was created.
func (a *Aggregator) Summary(sessionID string) RunSummary {
	return RunSummary{
		Time:              time.Now(),
		Type:              EventRunSummary,
		SessionID:         sessionID,
		TotalModelCalls:   a.modelCalls,
		TotalToolCalls:    a.toolCalls,
		TotalInputTokens:  a.inputTokens,
		TotalOutputTokens: a.outputTokens,
		TotalDurationMS:   time.Since(a.startedAt).Milliseconds(),
		ErrorCount:        a.errorCount,
	}
}
