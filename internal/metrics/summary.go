package metrics

import "time"

type Aggregator struct {
	startedAt    time.Time
	modelCalls   int
	toolCalls    int
	inputTokens  int
	outputTokens int
	errorCount   int
}

func NewAggregator() *Aggregator {
	return &Aggregator{startedAt: time.Now()}
}

func (a *Aggregator) AddModel(inputTokens, outputTokens int, hasError bool) {
	a.modelCalls++
	a.inputTokens += inputTokens
	a.outputTokens += outputTokens
	if hasError {
		a.errorCount++
	}
}

func (a *Aggregator) AddTool(hasError bool) {
	a.toolCalls++
	if hasError {
		a.errorCount++
	}
}

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
