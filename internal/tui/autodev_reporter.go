// Package tui — autodev_reporter.go bridges autodev orchestration events
// into the Bubble Tea update loop. It mirrors the asker.go pattern: the
// orchestrator goroutine sends messages over the model's events channel and
// the update loop renders them in the session area (REQ-025, TC-017).
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
)

// TUIReporter implements autodev.Reporter over the TUI event channel. The
// embedded channelReporter handles the engine.Reporter half so core-Agent
// output renders exactly like a normal interactive run; the orchestration
// events render as system entries. Sends never block past context
// cancellation.
type TUIReporter struct {
	channelReporter
}

// NewTUIReporter creates a TUIReporter sending into events.
func NewTUIReporter(events chan<- tea.Msg) *TUIReporter {
	return &TUIReporter{channelReporter{events: events}}
}

var _ autodev.Reporter = (*TUIReporter)(nil)

func (r *TUIReporter) sendSystem(ctx context.Context, title, body, status string) {
	r.send(ctx, runEventMsg{role: "system", title: title, body: body, status: status})
}

// OnItemStart implements autodev.Reporter.
func (r *TUIReporter) OnItemStart(ctx context.Context, index, total int, item autodev.LedgerItem) {
	r.sendSystem(ctx, "autodev",
		fmt.Sprintf("item %d/%d  %s  %s", index, total, item.Priority, item.Slug),
		"autodev: "+item.Slug)
}

// OnWorktree implements autodev.Reporter.
func (r *TUIReporter) OnWorktree(ctx context.Context, wt autodev.Worktree) {
	r.sendSystem(ctx, "autodev", fmt.Sprintf("worktree %s  branch %s", wt.Path, wt.Branch), "")
}

// OnStageStart implements autodev.Reporter.
func (r *TUIReporter) OnStageStart(ctx context.Context, slug, stage string) {
	r.sendSystem(ctx, "autodev stage", stage, "autodev stage: "+stage)
}

// OnEngineerDecision implements autodev.Reporter.
func (r *TUIReporter) OnEngineerDecision(ctx context.Context, questions []tools.Question, answers []tools.Answer) {
	var b strings.Builder
	for _, q := range questions {
		b.WriteString("core asks: " + q.Prompt + "\n")
	}
	for _, a := range answers {
		b.WriteString("engineer → " + a.Value + "\n")
	}
	r.sendSystem(ctx, "engineer decision", strings.TrimSpace(b.String()), "")
}

// OnEngineerReview implements autodev.Reporter.
func (r *TUIReporter) OnEngineerReview(ctx context.Context, stage, instruction string) {
	r.sendSystem(ctx, "engineer review", instruction, "engineer steering "+stage)
}

// OnVerify implements autodev.Reporter.
func (r *TUIReporter) OnVerify(ctx context.Context, stage string, ok bool, gap string) {
	if ok {
		r.sendSystem(ctx, "autodev verify", stage+"  DONE", "")
		return
	}
	r.sendSystem(ctx, "autodev verify", stage+"  NOT DONE: "+gap, "")
}

// OnGate implements autodev.Reporter.
func (r *TUIReporter) OnGate(ctx context.Context, result autodev.GateResult) {
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
	r.sendSystem(ctx, "autodev gate", strings.Join(parts, "  "), "")
}

// OnIssue implements autodev.Reporter.
func (r *TUIReporter) OnIssue(ctx context.Context, number int) {
	r.sendSystem(ctx, "autodev remote", fmt.Sprintf("issue #%d", number), "")
}

// OnPR implements autodev.Reporter.
func (r *TUIReporter) OnPR(ctx context.Context, number int) {
	r.sendSystem(ctx, "autodev remote", fmt.Sprintf("PR #%d", number), "")
}

// OnItemDone implements autodev.Reporter.
func (r *TUIReporter) OnItemDone(ctx context.Context, item autodev.LedgerItem) {
	r.sendSystem(ctx, "autodev",
		fmt.Sprintf("%s = done (issue #%d, pr #%d)", item.Slug, item.Issue, item.PR),
		"autodev: "+item.Slug+" done")
}

// OnInfo implements autodev.Reporter.
func (r *TUIReporter) OnInfo(ctx context.Context, msg string) {
	r.sendSystem(ctx, "autodev", msg, "")
}
