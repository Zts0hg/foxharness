package metrics

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// TokenEstimator estimates token counts for messages and raw text.
// Implementations may use exact tokenization or heuristic approximation.
type TokenEstimator interface {
	// EstimateMessages returns the estimated token count for a slice of
	// messages, including their content, tool calls, and tool result IDs.
	EstimateMessages(messages []schema.Message) int
	// EstimateText returns the estimated token count for a plain string.
	EstimateText(text string) int
}

// RoughEstimator provides a fast, approximate token estimate based on
// Unicode rune count. It is suitable for deciding when to trigger context
// compaction but should not be used when exact billing-level counts are
// required.
type RoughEstimator struct{}

// EstimateText returns the rune count plus one as a rough token estimate.
// An empty string returns zero.
func (RoughEstimator) EstimateText(text string) int {
	n := utf8.RuneCountInString(text)
	if n == 0 {
		return 0
	}

	return n + 1
}

// EstimateMessages sums estimated tokens across all message content,
// tool call fields (ID, name, arguments), and tool result IDs.
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

// EstimateToolDefinitions serializes the tool definitions to JSON and
// returns the estimated token count of the resulting string.
func EstimateToolDefinitions(est TokenEstimator, tools []schema.ToolDefinition) int {
	if len(tools) == 0 {
		return 0
	}
	data, _ := json.Marshal(tools)
	return est.EstimateText(string(data))
}
