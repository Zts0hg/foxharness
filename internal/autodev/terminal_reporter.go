package autodev

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// TerminalReporter streams the full autodev interaction — engineer
// messages, core LLM output, tool calls, and every control-plane action —
// as a readable line-oriented log, normally to stdout (REQ-024).
type TerminalReporter struct {
	mu  sync.Mutex
	out io.Writer
}

// NewTerminalReporter creates a TerminalReporter writing to out.
func NewTerminalReporter(out io.Writer) *TerminalReporter {
	return &TerminalReporter{out: out}
}

var _ Reporter = (*TerminalReporter)(nil)

func (r *TerminalReporter) printf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.out, format+"\n", args...)
}

// OnRunStart implements engine.Reporter.
func (r *TerminalReporter) OnRunStart(ctx context.Context, sessionID, runID string) {
	r.printf("  core   → run %s started", runID)
}

// OnThinking implements engine.Reporter.
func (r *TerminalReporter) OnThinking(ctx context.Context, turn int) {
	r.printf("  core   → thinking (turn %d)", turn)
}

// OnCompaction implements engine.Reporter.
func (r *TerminalReporter) OnCompaction(ctx context.Context, scope string) {
	r.printf("  core   → context compacted (%s)", scope)
}

// OnToolCall implements engine.Reporter.
func (r *TerminalReporter) OnToolCall(ctx context.Context, toolName, args string) {
	r.printf("  core   → tool %s %s", toolName, oneLine(args, 160))
}

// OnToolResult implements engine.Reporter.
func (r *TerminalReporter) OnToolResult(ctx context.Context, toolName, result string, isError bool) {
	marker := "✓"
	if isError {
		marker = "✗"
	}
	r.printf("  core   ← tool %s %s %s", toolName, marker, oneLine(result, 160))
}

// OnMessage implements engine.Reporter.
func (r *TerminalReporter) OnMessage(ctx context.Context, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	r.printf("  core   → %s", indentContinuations(content))
}

// OnRunComplete implements engine.Reporter.
func (r *TerminalReporter) OnRunComplete(ctx context.Context, result engine.RunResult) {
	r.printf("  core   → run %s complete", result.RunID)
}

// OnRunError implements engine.Reporter.
func (r *TerminalReporter) OnRunError(ctx context.Context, sessionID, runID string, err error) {
	r.printf("  core   ✗ run %s error: %v", runID, err)
}

// OnItemStart implements Reporter.
func (r *TerminalReporter) OnItemStart(ctx context.Context, index, total int, item LedgerItem) {
	r.printf("[autodev] item %d/%d  %s  %s", index, total, item.Priority, item.Slug)
}

// OnWorktree implements Reporter.
func (r *TerminalReporter) OnWorktree(ctx context.Context, wt Worktree) {
	r.printf("[autodev] worktree %s  branch %s", wt.Path, wt.Branch)
}

// OnStageStart implements Reporter.
func (r *TerminalReporter) OnStageStart(ctx context.Context, slug, stage string) {
	r.printf("[stage] %s", stage)
}

// OnEngineerDecision implements Reporter.
func (r *TerminalReporter) OnEngineerDecision(ctx context.Context, questions []tools.Question, answers []tools.Answer) {
	for _, q := range questions {
		r.printf("  core   → asks: %s", oneLine(q.Prompt, 160))
	}
	for _, a := range answers {
		r.printf("  engineer → %s", oneLine(a.Value, 160))
	}
}

// OnEngineerReview implements Reporter.
func (r *TerminalReporter) OnEngineerReview(ctx context.Context, stage, instruction string) {
	r.printf("  engineer → %s", indentContinuations(instruction))
}

// OnVerify implements Reporter.
func (r *TerminalReporter) OnVerify(ctx context.Context, stage string, ok bool, gap string) {
	if ok {
		r.printf("[stage] %s  DONE", stage)
		return
	}
	r.printf("[stage] %s  NOT DONE: %s", stage, oneLine(gap, 200))
}

// OnGate implements Reporter.
func (r *TerminalReporter) OnGate(ctx context.Context, result GateResult) {
	parts := make([]string, 0, len(result.Steps))
	for _, s := range result.Steps {
		switch {
		case s.Skipped:
			parts = append(parts, s.Name+" skipped")
		case s.Passed:
			parts = append(parts, s.Name+" ✓")
		default:
			parts = append(parts, s.Name+" ✗")
		}
	}
	r.printf("[gate] %s", strings.Join(parts, "  "))
}

// OnIssue implements Reporter.
func (r *TerminalReporter) OnIssue(ctx context.Context, number int) {
	r.printf("[remote] issue #%d", number)
}

// OnPR implements Reporter.
func (r *TerminalReporter) OnPR(ctx context.Context, number int) {
	r.printf("[remote] PR #%d", number)
}

// OnItemDone implements Reporter.
func (r *TerminalReporter) OnItemDone(ctx context.Context, item LedgerItem) {
	r.printf("[ledger] %s = done (issue #%d, pr #%d)", item.Slug, item.Issue, item.PR)
}

// OnInfo implements Reporter.
func (r *TerminalReporter) OnInfo(ctx context.Context, msg string) {
	r.printf("[autodev] %s", msg)
}

func oneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit-3]) + "..."
}

func indentContinuations(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n           ")
}
