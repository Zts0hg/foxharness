package slash

import (
	"context"
	"errors"
	"time"
)

// ForkRunner is the dependency the Executor uses to delegate fork-mode
// command execution to a sub-agent. The slash package keeps this interface
// minimal so it does not pull in internal/subagent or internal/engine.
//
// Implementations should treat task as the fully-processed user prompt and
// agentType as the optional agent identifier from the command's frontmatter.
type ForkRunner interface {
	Run(ctx context.Context, task string, agentType string) (string, error)
}

// Executor orchestrates the per-command pipeline: argument substitution,
// shell embedding, variable replacement, before/after hooks, and dispatch
// to either inline (return the processed content) or fork (delegate to a
// sub-agent) modes.
//
// All cross-cutting dependencies are injected through Option values so
// tests can construct executors that do not depend on the subagent
// package or the file system.
type Executor struct {
	forkRunner   ForkRunner
	workDir      string
	shellTimeout time.Duration
	hookTimeout  time.Duration
}

// ExecutorOption configures an Executor at construction time.
type ExecutorOption func(*Executor)

// WithForkRunner installs the ForkRunner used by fork-mode commands. When
// no ForkRunner is supplied, fork-mode commands return an error.
func WithForkRunner(r ForkRunner) ExecutorOption {
	return func(e *Executor) { e.forkRunner = r }
}

// WithWorkDir scopes shell embeddings and hooks to the given working
// directory.
func WithWorkDir(dir string) ExecutorOption {
	return func(e *Executor) { e.workDir = dir }
}

// WithShellTimeout overrides the default per-embedding shell timeout.
func WithShellTimeout(d time.Duration) ExecutorOption {
	return func(e *Executor) { e.shellTimeout = d }
}

// WithHookTimeout overrides the default per-hook execution timeout.
func WithHookTimeout(d time.Duration) ExecutorOption {
	return func(e *Executor) { e.hookTimeout = d }
}

// NewExecutor returns an Executor with the supplied options applied. The
// zero-value Executor has no ForkRunner; fork-mode commands will fail
// until WithForkRunner is supplied.
func NewExecutor(opts ...ExecutorOption) *Executor {
	e := &Executor{
		shellTimeout: DefaultShellTimeout,
		hookTimeout:  DefaultHookTimeout,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ExecutionResult bundles everything an inline-mode caller needs to start
// the next agent turn: the processed prompt content and any per-turn
// restrictions declared by the command's frontmatter. Fork-mode results
// populate Content with the sub-agent's report and leave restriction
// fields empty — the sub-agent runs in its own sandbox.
type ExecutionResult struct {
	// Content is the processed prompt body (inline mode) or the
	// sub-agent's report (fork mode).
	Content string

	// AllowedTools mirrors the command's `allowed-tools` frontmatter.
	// When non-empty, the caller must restrict the next agent turn to
	// these tools — typically by wrapping the tool registry in
	// NewFilteredRegistry. nil means "no restriction".
	AllowedTools []string

	// Fork is true when the result came from a fork-mode sub-agent.
	// Callers can use it to decide how to display the result
	// (e.g. as a tool report rather than a regular assistant reply).
	Fork bool
}

// Execute processes cmd through the pipeline and returns an
// ExecutionResult.
//
// For inline-mode commands the result's Content is the processed prompt
// the caller should feed back into the conversation, and AllowedTools
// surfaces the per-turn tool restriction. For fork-mode commands the
// Content is whatever the ForkRunner produced and AllowedTools is empty
// (the sub-agent enforces its own constraints).
//
// rawArgs is the un-parsed argument string typed after the command name
// (or supplied by the model's tool call). sessionID is used for
// ${FOXHARNESS_SESSION_ID} substitution and may be empty.
func (e *Executor) Execute(ctx context.Context, cmd *Command, rawArgs, sessionID string) (ExecutionResult, error) {
	if cmd == nil {
		return ExecutionResult{}, errors.New("slash: nil command")
	}

	args := ParseArguments(rawArgs)
	argNames := SplitArgumentNames(cmd.Frontmatter.Arguments)
	processed := SubstituteArguments(cmd.Content, args, argNames)

	shellWorkDir := e.workDir
	processed, err := ExecuteEmbeddedShell(processed, shellWorkDir, e.shellTimeout)
	if err != nil {
		return ExecutionResult{}, err
	}

	vars := map[string]string{
		VarSkillDir:  cmd.SkillDir,
		VarSessionID: sessionID,
	}
	processed = ReplaceVariables(processed, vars)

	_ = ExecuteHooks(ctx, cmd.Frontmatter.Hooks, shellWorkDir, e.hookTimeout)
	defer func() {
		_ = ExecuteAfterHook(ctx, cmd.Frontmatter.Hooks, shellWorkDir, e.hookTimeout)
	}()

	if isForkMode(cmd) {
		if e.forkRunner == nil {
			return ExecutionResult{}, errors.New("fork mode unavailable: no runner configured")
		}
		out, err := e.forkRunner.Run(ctx, processed, cmd.Frontmatter.Agent)
		if err != nil {
			return ExecutionResult{}, err
		}
		return ExecutionResult{Content: out, Fork: true}, nil
	}
	return ExecutionResult{
		Content:      processed,
		AllowedTools: append([]string(nil), cmd.Frontmatter.AllowedTools...),
	}, nil
}

func isForkMode(cmd *Command) bool {
	return cmd.Frontmatter.Context == "fork"
}
