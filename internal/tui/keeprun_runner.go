// Package tui — keeprun_runner.go is the production keeprun.PhaseRunner: the
// bridge between the deterministic Go orchestrator (internal/keeprun) and the
// LLM engine. It resolves a /codexspec:* command to its prompt body via the
// slash Executor and drives the engine to run it with the keep-run tool set.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/keeprun"
	"github.com/Zts0hg/foxharness/internal/slash"
)

// phaseEngine is the subset of the engine runner the phase adapter needs: a
// tool-restricted run plus the active session id.
type phaseEngine interface {
	RunRestricted(ctx context.Context, prompt string, allowedTools []string, reporter engine.Reporter) (*engine.RunResult, error)
	SessionID() string
}

// keepRunPhaseRunner is the production keeprun.PhaseRunner. For each phase it
// resolves the /codexspec:* command body via the slash Executor, drives the
// engine to run it with the keep-run tool set, and returns the engine's final
// message as the phase outcome. It is the only place keep-run reaches the LLM.
type keepRunPhaseRunner struct {
	engine   phaseEngine
	resolve  func(ctx context.Context, command, sessionID string) (string, error)
	reporter engine.Reporter
	// onPrompt, when set, is called once per phase with the user-facing header
	// (the command name plus the code-injected instructions) before the engine
	// run, so the TUI renders the phase as a user turn mirroring an interactive
	// session. The /codexspec:* template body is intentionally omitted because it
	// is already visible in the command's .md file.
	onPrompt func(ctx context.Context, displayPrompt string)
}

// newKeepRunPhaseRunner builds the adapter from the engine runner, the slash
// registry/executor used to resolve command bodies, and a reporter used to
// stream within-phase progress.
func newKeepRunPhaseRunner(eng phaseEngine, reg *slash.Registry, exec *slash.Executor, rep engine.Reporter) *keepRunPhaseRunner {
	if rep == nil {
		rep = keepRunNopReporter{}
	}
	r := &keepRunPhaseRunner{engine: eng, reporter: rep}
	r.resolve = func(ctx context.Context, command, sessionID string) (string, error) {
		cmd, ok := reg.Lookup(command)
		if !ok {
			return "", fmt.Errorf("keep-run: command %q not found in registry", command)
		}
		res, err := exec.Execute(ctx, cmd, "", sessionID)
		if err != nil {
			return "", fmt.Errorf("keep-run: resolve %q: %w", command, err)
		}
		return res.Content, nil
	}
	return r
}

// RunPhase resolves and runs one phase via the engine's restricted run. All
// phases run inline (direct mode); the subagent review mode (isolated fork) is a
// planned refinement (plan Decision 9). Merge prohibition is enforced by the
// bash command guard (keeprun.MergeProhibited), not by the tool allow-list,
// because merges are bash commands rather than a distinct tool (FR-010).
func (r *keepRunPhaseRunner) RunPhase(ctx context.Context, req keeprun.PhaseRequest) (keeprun.PhaseOutcome, error) {
	body, err := r.resolve(ctx, req.Phase.Command, r.engine.SessionID())
	if err != nil {
		return keeprun.PhaseOutcome{}, err
	}
	if r.onPrompt != nil {
		r.onPrompt(ctx, buildKeepRunDisplayPrompt(req))
	}
	res, err := r.engine.RunRestricted(ctx, buildKeepRunPrompt(body, req), keepRunAllowedTools(), r.reporter)
	if err != nil {
		return keeprun.PhaseOutcome{}, err
	}
	if res == nil {
		return keeprun.PhaseOutcome{}, fmt.Errorf("keep-run: %q produced no result", req.Phase.Command)
	}
	return keeprun.PhaseOutcome{Output: res.FinalMessage}, nil
}

// buildKeepRunPrompt assembles the engine prompt: the resolved command body
// followed by the code-injected instructions (worktree/spec routing, the merge
// prohibition, and any verdict or fix instruction).
func buildKeepRunPrompt(body string, req keeprun.PhaseRequest) string {
	return body + keepRunInjectedInstructions(req)
}

// keepRunInjectedInstructions returns the portion of a phase prompt the
// orchestrator adds on top of the /codexspec:* template: the worktree and spec
// directories the phase must operate in, the merge prohibition, and any injected
// instruction (the review verdict contract or a review fix prompt). It is shared
// by the engine prompt and the user-facing display header so both stay in sync.
func keepRunInjectedInstructions(req keeprun.PhaseRequest) string {
	var b strings.Builder
	if req.WorktreeDir != "" {
		fmt.Fprintf(&b, "\n\n---\nRun this phase inside the git worktree: %s\n", req.WorktreeDir)
		if req.SpecDir != "" {
			fmt.Fprintf(&b, "Use this existing SDD feature directory for the task — read its artifacts from and write new ones to it; do not create a new feature directory: %s\n", req.SpecDir)
		}
		b.WriteString("Do not merge any branch into main or master.")
	}
	if strings.TrimSpace(req.Instruction) != "" {
		b.WriteString("\n\n")
		b.WriteString(req.Instruction)
	}
	return b.String()
}

// buildKeepRunDisplayPrompt renders the user-facing header shown in the TUI
// transcript for a phase: the command name plus the code-injected instructions,
// but not the /codexspec:* template body. The body is omitted on purpose — it is
// visible in the command's .md file, whereas the injected instructions live only
// in code, so surfacing them is what tells the user what the orchestrator added.
func buildKeepRunDisplayPrompt(req keeprun.PhaseRequest) string {
	return "/" + req.Phase.Command + keepRunInjectedInstructions(req)
}

// keepRunAllowedTools is the tool set granted to a keep-run phase run. bash is
// required for git and the Go toolchain; merge prohibition is enforced by the
// bash command guard (keeprun.MergeProhibited), since merges are bash commands
// rather than a distinct tool that could be withheld here.
func keepRunAllowedTools() []string {
	return []string{"read_file", "write_file", "edit_file", "bash", "subagent"}
}

// keepRunNopReporter is a no-op engine.Reporter used when no streaming reporter
// is supplied.
type keepRunNopReporter struct{}

func (keepRunNopReporter) OnRunStart(context.Context, string, string)         {}
func (keepRunNopReporter) OnThinking(context.Context, int)                    {}
func (keepRunNopReporter) OnCompaction(context.Context, string)               {}
func (keepRunNopReporter) OnToolCall(context.Context, string, string)         {}
func (keepRunNopReporter) OnToolResult(context.Context, string, string, bool) {}
func (keepRunNopReporter) OnMessage(context.Context, string)                  {}
func (keepRunNopReporter) OnRunComplete(context.Context, engine.RunResult)    {}
func (keepRunNopReporter) OnRunError(context.Context, string, string, error)  {}
