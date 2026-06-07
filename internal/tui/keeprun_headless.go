package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/keeprun"
	"github.com/Zts0hg/foxharness/internal/slash"
)

// RunKeepRunHeadless runs the keep-run SDD pipeline to completion without the
// bubbletea TUI, driving the same orchestrator, phase runner, and merge guard as
// the interactive /keep-run command. Orchestrator progress and within-phase model
// output are written to out as plain text, so the pipeline can run unattended (a
// non-interactive shell, CI, or a smoke test). It returns an error when runner
// cannot drive tool-restricted runs, or the orchestrator's terminal error.
func RunKeepRunHeadless(ctx context.Context, runner Runner, repoDir string, reg *slash.Registry, exec *slash.Executor, out io.Writer) error {
	eng, ok := runner.(phaseEngine)
	if !ok {
		return fmt.Errorf("keep-run: runner does not support restricted runs (RunRestricted)")
	}
	if out == nil {
		out = io.Discard
	}
	if exec == nil {
		exec = slash.NewExecutor()
	}
	if guard, ok := runner.(middlewareInstaller); ok {
		guard.AddMiddleware(mergeGuard{})
		defer guard.ClearMiddleware()
	}

	pr := newKeepRunPhaseRunner(eng, reg, exec, writerReporter{out: out})
	pr.onPrompt = func(_ context.Context, display string) {
		fmt.Fprintf(out, "\n▌ %s\n", display)
	}
	orch := keeprun.NewOrchestrator(repoDir, pr, keeprun.WithProgressSink(writerSink{out: out}))
	return orch.Run(ctx)
}

// writerSink renders orchestrator progress events to a writer for headless runs.
type writerSink struct{ out io.Writer }

// Event implements keeprun.ProgressSink.
func (s writerSink) Event(ev keeprun.ProgressEvent) {
	fmt.Fprintln(s.out, formatKeepRunEvent(ev))
}

// writerReporter streams within-phase engine activity (tool calls, results, and
// messages) to a writer. It is the non-TUI counterpart of channelReporter.
type writerReporter struct{ out io.Writer }

func (writerReporter) OnRunStart(context.Context, string, string) {}
func (writerReporter) OnThinking(context.Context, int)            {}

func (r writerReporter) OnCompaction(_ context.Context, scope string) {
	fmt.Fprintf(r.out, "  [context compacted: %s]\n", scope)
}

func (r writerReporter) OnToolCall(_ context.Context, name, args string) {
	fmt.Fprintf(r.out, "  → %s\n", formatToolInvocation(name, args))
}

func (r writerReporter) OnToolResult(_ context.Context, name, result string, isErr bool) {
	status := "ok"
	if isErr {
		status = "error"
	}
	fmt.Fprintf(r.out, "  ← %s (%s): %s\n", name, status, truncateInline(strings.TrimSpace(result), 200))
}

func (r writerReporter) OnMessage(_ context.Context, content string) {
	if c := strings.TrimSpace(content); c != "" {
		fmt.Fprintln(r.out, c)
	}
}

func (writerReporter) OnRunComplete(context.Context, engine.RunResult) {}

func (r writerReporter) OnRunError(_ context.Context, _, _ string, err error) {
	fmt.Fprintf(r.out, "  [run error: %v]\n", err)
}

var _ engine.Reporter = writerReporter{}
