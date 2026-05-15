// Package recovery tracks tool execution failures and generates recovery
// prompts that guide the agent away from repeating the same failing calls.
// It maintains a sliding window of recent errors with deduplication counts
// so that the agent is forced to change strategy after repeated failures.
package recovery

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// ToolError records a single tool invocation failure together with its
// arguments, truncated output, and the number of times the same tool call
// (by fingerprint) has failed.
type ToolError struct {
	ToolName  string
	Arguments string
	Output    string
	Count     int
}

// Tracker observes tool call results, retains the most recent failures,
// and decides when to inject a recovery prompt into the agent's context.
type Tracker struct {
	recent  []ToolError
	counts  map[string]int
	pending bool
}

// NewTracker returns a Tracker ready to record failures.
func NewTracker() *Tracker {
	return &Tracker{
		counts: make(map[string]int),
	}
}

// Record registers a failed tool call result. Successful calls are
// ignored. The call is fingerprinted by tool name and arguments so that
// repeated identical failures are counted. A sliding window of the five
// most recent errors is maintained.
func (t *Tracker) Record(call schema.ToolCall, result schema.ToolResult) {
	if !result.IsError {
		return
	}

	key := fingerprint(call)
	t.counts[key]++

	item := ToolError{
		ToolName:  call.Name,
		Arguments: string(call.Arguments),
		Output:    truncate(result.Output, 2000),
		Count:     t.counts[key],
	}

	t.recent = append(t.recent, item)
	if len(t.recent) > 5 {
		t.recent = t.recent[len(t.recent)-5:]
	}

	t.pending = true
}

func fingerprint(call schema.ToolCall) string {
	h := sha1.New()
	h.Write([]byte(call.Name))
	h.Write([]byte{0})
	h.Write(call.Arguments)
	return hex.EncodeToString(h.Sum(nil))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]..."
}

// ShouldInject reports whether a recovery prompt should be injected into
// the agent's next turn. It returns true when there is at least one
// pending un-injected error.
func (t *Tracker) ShouldInject() bool {
	if !t.pending || len(t.recent) == 0 {
		return false
	}

	last := t.recent[len(t.recent)-1]
	return last.Count >= 1
}

// MarkInject clears the pending flag so the recovery prompt is not
// injected again until the next error is recorded.
func (t *Tracker) MarkInject() {
	t.pending = false
}

// BuildPrompt constructs a recovery prompt for the agent describing the
// most recent failure, its repetition count, and a set of rules the agent
// should follow to diagnose and recover. When the same call has failed
// two or more times, a stronger directive prohibiting a direct retry is
// included.
func (t *Tracker) BuildPrompt() string {
	if len(t.recent) == 0 {
		return ""
	}

	last := t.recent[len(t.recent)-1]

	var b strings.Builder
	b.WriteString("## Error Recovery Notice\n\n")
	b.WriteString("上一次工具调用失败了。你必须先诊断失败原因，再决定下一步行动。\n\n")
	b.WriteString(fmt.Sprintf("- Tool: `%s`\n", last.ToolName))
	b.WriteString(fmt.Sprintf("- Arguments: `%s`\n", last.Arguments))
	b.WriteString(fmt.Sprintf("- Failure count for same tool+arguments: %d\n", last.Count))
	b.WriteString("\n错误输出摘要: \n\n")
	b.WriteString("```plain\n")
	b.WriteString(last.Output)
	b.WriteString("\n```\n\n")

	b.WriteString("恢复规则：\n")
	b.WriteString("1. 不要盲目重复完全相同的工具调用。\n")
	b.WriteString("2. 如果是路径错误，先用 bash 或 read_file 检查真实文件结构。\n")
	b.WriteString("3. 如果是 edit_file 匹配失败，先重新读取目标文件，再提供更长、更准确的 old_string。\n")
	b.WriteString("4. 如果是 bash 命令失败，先解释失败原因，再决定是修改代码、调整命令还是读取更多上下文。\n")
	b.WriteString("5. 如果你确实要重复同一调用，必须说明新证据是什么。\n")

	if last.Count >= 2 {
		b.WriteString("\n强制要求：同一工具和同一参数已经失败多次，下一步禁止再次原样调用。必须换一种调查或修复策略。\n")
	}

	return b.String()
}
