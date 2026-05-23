// Package toolresult provides persistence and budget enforcement for tool
// execution results. Large outputs that would otherwise consume the entire
// model context are written to disk with a short inline preview so the
// agent can keep reasoning without paying the full token cost.
package toolresult

import "fmt"

// MaxToolResultBytes is the absolute cap (in bytes) applied to any individual
// tool result before persistence or context insertion. 400KB matches Claude
// Code's MAX_TOOL_RESULT_BYTES — equivalent to roughly 100K tokens at the
// standard 4 bytes-per-token heuristic.
const MaxToolResultBytes = 400_000

// TruncateToCap returns content unchanged when it fits inside the absolute
// cap. Inputs that exceed the cap are truncated to the first
// MaxToolResultBytes bytes with a trailing notice that records the original
// size in kilobytes (rounded down).
func TruncateToCap(content string) string {
	if len(content) <= MaxToolResultBytes {
		return content
	}
	originalKB := len(content) / 1024
	return content[:MaxToolResultBytes] +
		fmt.Sprintf("\n...[truncated at 400KB, original size: %d KB]", originalKB)
}
