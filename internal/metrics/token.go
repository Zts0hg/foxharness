package metrics

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/Zts0hg/foxharness/internal/schema"
)

type TokenEstimator interface {
	EstimateMessages(messages []schema.Message) int
	EstimateText(text string) int
}

type RoughEstimator struct{}

func (RoughEstimator) EstimateText(text string) int {
	n := utf8.RuneCountInString(text)
	if n == 0 {
		return 0
	}

	return n + 1
}

func (e RoughEstimator) EstimateMessages(messages []schema.Message) int {
	total := 0
	for _, msg := range messages {
		total += e.EstimateText(msg.Content)

		for _, call := range msg.ToolCalls {
			total += e.EstimateText(call.ID)
			total += e.EstimateText(call.Name)
			total += e.EstimateText(string(call.Arguments))
		}

		if msg.ToolCallID != "" {
			total += e.EstimateText(msg.ToolCallID)
		}

	}

	return total
}

func EstimateToolDefinitions(est TokenEstimator, tools []schema.ToolDefinition) int {
	if len(tools) == 0 {
		return 0
	}
	data, _ := json.Marshal(tools)
	return est.EstimateText(string(data))
}
