package slash

import (
	"context"
	"errors"
	"time"

	"github.com/Zts0hg/foxharness/internal/toolprotocol"
)

// ForkRunner is the dependency the Executor uses to delegate fork-mode
// command execution to a sub-agent. The slash package keeps this interface
// minimal so it does not pull in internal/subagent or internal/engine.
//
// Implementations should treat task as the fully-processed user prompt,
// agentType as the optional agent identifier from the command's
// frontmatter, and allowedTools as the per-call tool allow-list copied
// from the command's `allowed-tools` frontmatter. nil means no restriction;
// a non-nil empty slice means the command explicitly declared deny-all. The
// runner is expected to enforce the allow-list by constructing the sub-agent's
// tool registry with the same filter the TUI path uses; otherwise fork-mode
// skills with `allowed-tools` would silently escape the restriction.
type ForkRunner interface {
	Run(ctx context.Context, task string, agentType string, allowedTools []string) (string, error)
}

// PermissionAwareForkRunner reports whether forked tool calls inherit the
// active permission coordinator.
type PermissionAwareForkRunner interface {
	PermissionEnforced() bool
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

// ForkPermissionEnforced reports whether the configured fork runner proves a
// nested permission boundary.
func (e *Executor) ForkPermissionEnforced() bool {
	if e == nil || e.forkRunner == nil {
		return false
	}
	reporter, ok := e.forkRunner.(PermissionAwareForkRunner)
	return ok && reporter.PermissionEnforced()
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
	// When non-nil, the caller must restrict the next agent turn to these
	// tools — typically by wrapping the tool registry in NewFilteredRegistry.
	// nil means "no restriction"; a non-nil empty slice means explicit deny-all.
	AllowedTools []string

	// Fork is true when the result came from a fork-mode sub-agent.
	// Callers can use it to decide how to display the result
	// (e.g. as a tool report rather than a regular assistant reply).
	Fork bool

	// Effort mirrors the command's frontmatter effort for inline prompt
	// commands. Callers validate it against the active provider protocol before
	// starting the model run.
	Effort string

	// AfterHook is set for inline-mode commands that declare a
	// frontmatter `hooks.after`. The caller is responsible for invoking
	// AfterHook once the command's execution truly completes — i.e.,
	// after the model has finished running on Content. Fork-mode
	// commands set AfterHook to nil because Execute already fired the
	// hook synchronously after the sub-agent returned.
	//
	// The closure is idempotent only in the sense that the underlying
	// shell command can be invoked multiple times safely; the executor
	// will not deduplicate calls. AfterHook is nil when no after-hook
	// is declared.
	AfterHook func(ctx context.Context)
}

// ExecutionPlan is a side-effect-free description shared by permission
// assessment and execution.
type ExecutionPlan struct {
	Template     string
	Commands     []string
	Fork         bool
	AllowedTools []string
	Effort       string
	Hooks        *FrontmatterHooks
	variables    map[string]string
}

// Plan applies pure substitutions and records every shell command that the
// execution pipeline may run.
func (e *Executor) Plan(cmd *Command, rawArgs, sessionID string) (ExecutionPlan, error) {
	if cmd == nil {
		return ExecutionPlan{}, errors.New("slash: nil command")
	}
	args := ParseArguments(rawArgs)
	argNames := SplitArgumentNames(cmd.Frontmatter.Arguments)
	template := SubstituteArguments(cmd.Content, args, argNames)
	commands := EmbeddedShellCommands(template)
	var hooks *FrontmatterHooks
	if cmd.Frontmatter.Hooks != nil {
		copy := *cmd.Frontmatter.Hooks
		hooks = &copy
		if copy.Before != "" {
			commands = append(commands, copy.Before)
		}
		if copy.After != "" {
			commands = append(commands, copy.After)
		}
	}
	return ExecutionPlan{
		Template:     template,
		Commands:     commands,
		Fork:         isForkMode(cmd),
		AllowedTools: cloneAllowedTools(cmd.Frontmatter.AllowedTools),
		Effort:       cmd.Frontmatter.Effort,
		Hooks:        hooks,
		variables: map[string]string{
			VarSkillDir:        cmd.SkillDir,
			VarSessionID:       sessionID,
			VarClaudeSkillDir:  cmd.SkillDir,
			VarClaudeSessionID: sessionID,
		},
	}, nil
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
	plan, err := e.Plan(cmd, rawArgs, sessionID)
	if err != nil {
		return ExecutionResult{}, err
	}

	shellWorkDir := e.workDir
	processed, err := ExecuteEmbeddedShell(ctx, plan.Template, shellWorkDir, e.shellTimeout)
	if err != nil {
		return ExecutionResult{}, err
	}
	processed = ReplaceVariables(processed, plan.variables)

	_ = ExecuteHooks(ctx, plan.Hooks, shellWorkDir, e.hookTimeout)

	if plan.Fork {
		if e.forkRunner == nil {
			return ExecutionResult{}, errors.New("fork mode unavailable: no runner configured")
		}
		allowedCopy := forkAllowedTools(ctx, plan.AllowedTools)
		out, forkErr := e.forkRunner.Run(ctx, processed, cmd.Frontmatter.Agent, allowedCopy)
		// Fork mode completes synchronously inside Execute, so the
		// after-hook runs here — regardless of forkErr — to mirror
		// "after the command's execution completes".
		_ = ExecuteAfterHook(ctx, plan.Hooks, shellWorkDir, e.hookTimeout)
		if forkErr != nil {
			return ExecutionResult{Content: out, Fork: true}, forkErr
		}
		return ExecutionResult{Content: out, Fork: true}, nil
	}

	// Inline mode does NOT defer the after-hook here. The caller (TUI or
	// SkillTool) starts the actual agent run on the returned prompt and
	// is responsible for invoking AfterHook when that run completes.
	// Deferring inside Execute would fire the hook before the model has
	// touched the content, defeating REQ-012.
	hooks := plan.Hooks
	timeout := e.hookTimeout
	var afterHook func(context.Context)
	if hooks != nil && hooks.After != "" {
		afterHook = func(finishCtx context.Context) {
			_ = ExecuteAfterHook(finishCtx, hooks, shellWorkDir, timeout)
		}
	}
	return ExecutionResult{
		Content:      processed,
		AllowedTools: cloneAllowedTools(plan.AllowedTools),
		Effort:       plan.Effort,
		AfterHook:    afterHook,
	}, nil
}

func isForkMode(cmd *Command) bool {
	return cmd.Frontmatter.Context == "fork"
}

func cloneAllowedTools(tools []string) []string {
	if tools == nil {
		return nil
	}
	return append([]string{}, tools...)
}

func forkAllowedTools(ctx context.Context, commandAllowed []string) []string {
	if !toolprotocol.HasCapabilities(ctx) {
		return cloneAllowedTools(commandAllowed)
	}
	parentAllowed := toolprotocol.CapabilitiesFromContext(ctx)
	if commandAllowed == nil {
		return parentAllowed
	}
	allowed := make(map[string]struct{}, len(commandAllowed))
	for _, name := range commandAllowed {
		allowed[name] = struct{}{}
	}
	result := make([]string, 0, len(parentAllowed))
	for _, name := range parentAllowed {
		if _, ok := allowed[name]; ok {
			result = append(result, name)
		}
	}
	return result
}
