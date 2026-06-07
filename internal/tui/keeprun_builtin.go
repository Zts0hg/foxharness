package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Zts0hg/foxharness/internal/keeprun"
	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/slash"
)

// keepRunProgressMsg carries an orchestrator progress event into the bubbletea
// update loop for display.
type keepRunProgressMsg struct{ event keeprun.ProgressEvent }

// keepRunDoneMsg signals the keep-run orchestrator finished (err is nil on a
// clean exit, or the failure/cancellation reason otherwise).
type keepRunDoneMsg struct{ err error }

// eventSink adapts keeprun.ProgressSink to the TUI event channel so orchestrator
// progress streams into the bubbletea loop.
type eventSink struct{ events chan<- tea.Msg }

// Event implements keeprun.ProgressSink.
func (s eventSink) Event(ev keeprun.ProgressEvent) {
	s.events <- keepRunProgressMsg{event: ev}
}

// startKeepRun builds the phase runner and orchestrator and runs the pipeline on
// a background goroutine, streaming progress (and a final keepRunDoneMsg) to the
// events channel. The provided ctx should be cancelable so Ctrl+C stops the run;
// the state file lets a later invocation resume.
//
// It returns an error without launching when runner cannot drive restricted
// engine runs (so allowed-tools / merge restriction is never silently dropped).
// middlewareInstaller is the optional capability a runner exposes to install a
// per-run tool middleware. The production *app.AgentRunner satisfies it; keep-run
// uses it to install the merge guard for the duration of the run (FR-010).
type middlewareInstaller interface {
	AddMiddleware(middleware.Middleware)
	ClearMiddleware()
}

func startKeepRun(ctx context.Context, repoDir string, runner Runner, reg *slash.Registry, exec *slash.Executor, events chan tea.Msg) error {
	eng, ok := runner.(phaseEngine)
	if !ok {
		return fmt.Errorf("keep-run: runner does not support restricted runs (RunRestricted)")
	}
	if exec == nil {
		exec = slash.NewExecutor()
	}

	// Detect if running from a worktree and use the main repository path
	mainRepo, isWorktree, err := keeprun.DetectRepoEnvironment(repoDir)
	if err != nil {
		return fmt.Errorf("keep-run: detect repo environment: %w", err)
	}

	if isWorktree {
		// When running from a worktree, copy BACKLOG.md to the main repo
		// so the orchestrator can read it from there
		worktreeBacklog := filepath.Join(repoDir, "BACKLOG.md")
		mainBacklog := filepath.Join(mainRepo, "BACKLOG.md")
		if data, err := os.ReadFile(worktreeBacklog); err == nil {
			if err := os.WriteFile(mainBacklog, data, 0o644); err != nil {
				return fmt.Errorf("copy BACKLOG.md to main repo: %w", err)
			}
		}
		repoDir = mainRepo
		// Notify the user that we're using the main repository
		select {
		case events <- runEventMsg{
			role:  "system",
			title: "keep-run",
			body:  fmt.Sprintf("检测到从 worktree 中运行，将从主仓库创建 worktree: %s", mainRepo),
		}:
		case <-ctx.Done():
		}
	} else {
		repoDir = mainRepo
	}

	// Install the merge guard so no phase can merge into an integration branch
	// (FR-010); it is removed when keep-run finishes.
	guard, _ := runner.(middlewareInstaller)
	if guard != nil {
		guard.AddMiddleware(mergeGuard{})
	}
	pr := newTUIPhaseRunner(eng, reg, exec, events)
	orch := keeprun.NewOrchestrator(repoDir, pr, keeprun.WithProgressSink(eventSink{events: events}))
	go func() {
		err := orch.Run(ctx)
		if guard != nil {
			guard.ClearMiddleware()
		}
		events <- keepRunDoneMsg{err: err}
	}()
	return nil
}

// startKeepRunCmd is the tea.Cmd that launches the keep-run orchestrator. On a
// launch failure it surfaces a keepRunDoneMsg; on success it returns nil and the
// orchestrator streams progress via the events channel.
func startKeepRunCmd(ctx context.Context, repoDir string, runner Runner, reg *slash.Registry, exec *slash.Executor, events chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if err := startKeepRun(ctx, repoDir, runner, reg, exec, events); err != nil {
			return keepRunDoneMsg{err: err}
		}
		return nil
	}
}

// newTUIPhaseRunner builds the production keeprun.PhaseRunner wired for live TUI
// display. It streams within-phase model output (thinking, tool calls, results,
// and messages) into the transcript via a channelReporter exactly as an
// interactive turn does, and posts a user-style header (the command name plus
// the code-injected instructions) before each phase run via onPrompt.
func newTUIPhaseRunner(eng phaseEngine, reg *slash.Registry, exec *slash.Executor, events chan<- tea.Msg) *keepRunPhaseRunner {
	pr := newKeepRunPhaseRunner(eng, reg, exec, &channelReporter{events: events})
	pr.onPrompt = func(ctx context.Context, displayPrompt string) {
		select {
		case events <- runEventMsg{role: "user", title: "you", body: displayPrompt}:
		case <-ctx.Done():
		}
	}
	return pr
}

// formatKeepRunEvent renders a progress event as a one-line status string for
// the TUI transcript (FR-012).
func formatKeepRunEvent(ev keeprun.ProgressEvent) string {
	switch ev.Kind {
	case keeprun.EventTaskStart:
		return fmt.Sprintf("🗂️  Task %q (slug %s) — starting at phase %d/%d", ev.Task, ev.Slug, ev.Phase, ev.Total)
	case keeprun.EventPhaseStart:
		return fmt.Sprintf("▶️  Phase %d/%d: %s", ev.Phase, ev.Total, ev.Command)
	case keeprun.EventPhaseComplete:
		return fmt.Sprintf("✅ Phase %d/%d complete: %s", ev.Phase, ev.Total, ev.Command)
	case keeprun.EventPhaseRetry:
		return fmt.Sprintf("🔁 Retrying %s (attempt %d): %s", ev.Command, ev.Attempt, ev.Message)
	case keeprun.EventReviewFix:
		return fmt.Sprintf("🛠️  Fixing review findings: %s", ev.Command)
	case keeprun.EventTaskComplete:
		s := fmt.Sprintf("🎉 Task %q complete", ev.Task)
		if ev.Message != "" {
			s += " (" + ev.Message + ")"
		}
		return s
	case keeprun.EventExit:
		return "🏁 keep-run: " + ev.Message
	default:
		return ev.Message
	}
}
