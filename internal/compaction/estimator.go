package compaction

import (
	"unicode"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// ImprovedRoughEstimator approximates token cost from byte length with a
// separate, denser ratio for JSON-shaped payloads. The shared 4/3 safety
// margin biases the estimate upward to keep compaction triggers conservative.
//
// Heuristic (matches Claude Code):
//   - plain text  : len(content) / 4
//   - JSON content: len(content) / 2
//   - apply 4/3 multiplier on the raw estimate
type ImprovedRoughEstimator struct{}

// EstimateText returns the estimated token count for a single string using the
// improved heuristic. Empty input returns zero.
func (ImprovedRoughEstimator) EstimateText(text string) int {
	if text == "" {
		return 0
	}
	var raw int
	if looksLikeJSON(text) {
		raw = len(text) / 2
	} else {
		raw = len(text) / 4
	}
	return raw * 4 / 3
}

// Estimate satisfies TokenEstimator by delegating to EstimateMessages.
func (e ImprovedRoughEstimator) Estimate(messages []schema.Message) int {
	return e.EstimateMessages(messages)
}

// EstimateMessages sums the per-message token estimates across content, tool
// call payloads, and tool result identifiers.
func (e ImprovedRoughEstimator) EstimateMessages(messages []schema.Message) int {
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

// HybridEstimator combines exact API usage data carried on the most recent
// assistant message with the improved rough estimator for messages produced
// after that response. When no usage data is available the estimator falls
// back to a full rough estimate.
type HybridEstimator struct {
	rough ImprovedRoughEstimator
}

// NewHybridEstimator constructs a HybridEstimator using the supplied
// ImprovedRoughEstimator as the fallback estimator for unaccounted tail
// messages.
func NewHybridEstimator(rough ImprovedRoughEstimator) *HybridEstimator {
	return &HybridEstimator{rough: rough}
}

// Estimate returns the total estimated token count for messages.
func (h *HybridEstimator) Estimate(messages []schema.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != schema.RoleAssistant || msg.Usage == nil {
			continue
		}
		exact := totalUsageTokens(msg.Usage)
		if exact <= 0 {
			continue
		}
		tail := h.rough.EstimateMessages(messages[i+1:])
		return exact + tail
	}
	return h.rough.EstimateMessages(messages)
}

func totalUsageTokens(u *schema.Usage) int {
	if u == nil {
		return 0
	}
	return int(u.InputTokens + u.OutputTokens + u.CacheCreationTokens + u.CacheReadTokens)
}

func looksLikeJSON(text string) bool {
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		return r == '{' || r == '['
	}
	return false
}
