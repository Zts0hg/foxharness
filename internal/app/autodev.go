// Package app — autodev.go provides the concrete adapters that bind the
// internal/autodev control plane to the real AgentRunner, provider, and
// git/gh processes. The dependency direction is app → autodev only
// (Decision 2): autodev never imports app.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// agentRunnerAPI is the *AgentRunner method subset the adapter needs,
// extracted as an interface so the adapter is testable without a provider.
type agentRunnerAPI interface {
	Run(ctx context.Context, prompt string, reporter engine.Reporter) (*engine.RunResult, error)
	DrainExtraction(ctx context.Context) error
	CloseExtraction(ctx context.Context) error
	SetUserAsker(asker tools.UserAsker)
	SetModel(model string) error
	WorkDir() string
	SlashRegistry() *slash.Registry
	SlashExecutor() *slash.Executor
	SessionID() string
}

var _ agentRunnerAPI = (*AgentRunner)(nil)

// coreRunnerAdapter adapts an AgentRunner to autodev.CoreRunner. Runs are
// real engine runs; StagePrompt materializes codexspec command bodies via
// the runner's slash registry and executor (REQ-009).
type coreRunnerAdapter struct {
	runner agentRunnerAPI
}

var _ autodev.CoreRunner = (*coreRunnerAdapter)(nil)

// Run implements autodev.CoreRunner with one correlated terminal outcome.
func (a *coreRunnerAdapter) Run(ctx context.Context, attempt autodev.CoreAttempt, r engine.Reporter) autodev.CoreOutcome {
	recorder, reporter := newCoreOutcomeReporter(r)
	result, runErr := a.runner.Run(ctx, attempt.Prompt, reporter)
	sessionID, runID, partial := recorder.snapshot()
	started := runID != ""
	if result != nil {
		if sessionID == "" {
			sessionID = result.SessionID
		}
		if runID == "" {
			runID = result.RunID
			started = runID != ""
		}
		if runErr == nil {
			partial = result.FinalMessage
		}
	}
	status, retryClass := autodev.ClassifyCoreError(ctx, runErr, started)
	if status == autodev.CoreOutcomeStartFailed {
		sessionID, runID, partial = "", "", ""
	}
	return autodev.CoreOutcome{
		Attempt: attempt, Status: status, SessionID: sessionID, RunID: runID,
		PartialMessage: partial, Cause: runErr, RetryClass: retryClass,
		Lifecycle: autodev.CoreLifecycleEvidence{
			RunStarted:         started,
			PostRunEstablished: result != nil && started,
		},
	}
}

// coreOutcomeReporter captures only assistant messages already committed by
// the engine while forwarding the complete reporter contract unchanged.
type coreOutcomeReporter struct {
	mu        sync.Mutex
	next      engine.Reporter
	sessionID string
	runID     string
	partial   string
}

type detailedCoreOutcomeReporter struct{ *coreOutcomeReporter }

func newCoreOutcomeReporter(next engine.Reporter) (*coreOutcomeReporter, engine.Reporter) {
	recorder := &coreOutcomeReporter{next: next}
	_, detailed := next.(engine.DetailedReporter)
	if detailed {
		return recorder, &detailedCoreOutcomeReporter{recorder}
	}
	return recorder, recorder
}

func (r *coreOutcomeReporter) OnRunStart(ctx context.Context, sessionID, runID string) {
	r.mu.Lock()
	r.sessionID, r.runID = sessionID, runID
	r.mu.Unlock()
	if r.next != nil {
		r.next.OnRunStart(ctx, sessionID, runID)
	}
}
func (r *coreOutcomeReporter) OnThinking(ctx context.Context, turn int) {
	if r.next != nil {
		r.next.OnThinking(ctx, turn)
	}
}
func (r *coreOutcomeReporter) OnCompaction(ctx context.Context, scope string) {
	if r.next != nil {
		r.next.OnCompaction(ctx, scope)
	}
}
func (r *coreOutcomeReporter) OnToolCall(ctx context.Context, name, args string) {
	if r.next != nil {
		r.next.OnToolCall(ctx, name, args)
	}
}
func (r *coreOutcomeReporter) OnToolResult(ctx context.Context, name, result string, isError bool) {
	if r.next != nil {
		r.next.OnToolResult(ctx, name, result, isError)
	}
}
func (r *coreOutcomeReporter) OnMessage(ctx context.Context, content string) {
	r.mu.Lock()
	r.partial = content
	r.mu.Unlock()
	if r.next != nil {
		r.next.OnMessage(ctx, content)
	}
}
func (r *coreOutcomeReporter) OnRunComplete(ctx context.Context, result engine.RunResult) {
	if r.next != nil {
		r.next.OnRunComplete(ctx, result)
	}
}
func (r *coreOutcomeReporter) OnRunError(ctx context.Context, sessionID, runID string, err error) {
	if r.next != nil {
		r.next.OnRunError(ctx, sessionID, runID, err)
	}
}
func (r *detailedCoreOutcomeReporter) OnToolCallDetail(ctx context.Context, call schema.ToolCall) {
	if next, ok := r.next.(engine.DetailedReporter); ok {
		next.OnToolCallDetail(ctx, call)
	}
}
func (r *detailedCoreOutcomeReporter) OnToolResultDetail(ctx context.Context, call schema.ToolCall, result schema.ToolResult) {
	if next, ok := r.next.(engine.DetailedReporter); ok {
		next.OnToolResultDetail(ctx, call, result)
	}
}
func (r *coreOutcomeReporter) snapshot() (string, string, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionID, r.runID, r.partial
}

// Drain implements autodev.CoreRunner.
func (a *coreRunnerAdapter) Drain(ctx context.Context) error {
	return a.runner.DrainExtraction(ctx)
}

// Close implements autodev.CoreRunner.
func (a *coreRunnerAdapter) Close(ctx context.Context) error {
	return a.runner.CloseExtraction(ctx)
}

// SetUserAsker implements autodev.CoreRunner; installing the EngineerAsker
// here both registers the ask_user_question tool and routes it to the
// simulated engineer (REQ-013).
func (a *coreRunnerAdapter) SetUserAsker(asker tools.UserAsker) {
	a.runner.SetUserAsker(asker)
}

// SetModel implements autodev.CoreRunner.
func (a *coreRunnerAdapter) SetModel(model string) error {
	return a.runner.SetModel(model)
}

// WorkDir implements autodev.CoreRunner.
func (a *coreRunnerAdapter) WorkDir() string {
	return a.runner.WorkDir()
}

// StagePrompt implements autodev.CoreRunner by looking the command up in
// the runner's slash registry and processing it through the executor
// pipeline (argument substitution, shell embedding, variables). ctx bounds
// the embedded-shell processing so a cancelled run stops promptly.
func (a *coreRunnerAdapter) StagePrompt(ctx context.Context, command, args string) (string, error) {
	registry := a.runner.SlashRegistry()
	if registry == nil {
		return "", fmt.Errorf("no slash registry available to materialize %q", command)
	}
	cmd, ok := registry.Lookup(command)
	if !ok {
		return "", fmt.Errorf("slash command %q not found", command)
	}
	executor := a.runner.SlashExecutor()
	if executor == nil {
		executor = slash.NewExecutor(slash.WithWorkDir(a.runner.WorkDir()))
	}
	result, err := executor.Execute(ctx, cmd, args, a.runner.SessionID())
	if err != nil {
		return "", fmt.Errorf("materialize %q: %w", command, err)
	}
	return result.Content, nil
}

// appCoreRunnerFactory creates one real AgentRunner per item, scoped to the
// item's worktree. The SDD pipeline supplies its own staged structure.
type appCoreRunnerFactory struct {
	llmConfig      llmconfig.ResolvedConfig
	maxTurns       int
	newChildRunner ChildRunnerFactory
}

var _ autodev.CoreRunnerFactory = (*appCoreRunnerFactory)(nil)

// New implements autodev.CoreRunnerFactory.
func (f *appCoreRunnerFactory) New(ctx context.Context, workDir, model string) (autodev.CoreRunner, error) {
	if model == "" {
		model = f.llmConfig.Model
	}
	llmConfig := f.llmConfig.WithModel(model)
	permissions := permission.NewCoordinator(permission.Config{
		State:     permission.NewState(permission.ModeFullAccess, true),
		Workspace: workDir,
		CWD:       workDir,
	})
	runner, err := NewAgentRunner(ctx, AgentRunnerConfig{
		WorkDir:           workDir,
		Model:             llmConfig.Model,
		LLM:               llmConfig,
		MaxTurns:          f.maxTurns,
		ExtractionContext: ctx,
		Permission:        permissions,
		RuntimeProfile:    childParentAutodev,
		NewChildRunner:    f.newChildRunner,
	})
	if err != nil {
		return nil, err
	}
	return &coreRunnerAdapter{runner: runner}, nil
}

// resolveAutodevModel picks the model shared by the engineer and core
// Agents: .foxharness/autodev.yml wins, otherwise the CLI-resolved model
// (REQ-016).
func resolveAutodevModel(cliModel string, cfg autodev.AutodevConfig) string {
	if cfg.Model != "" {
		return cfg.Model
	}
	return cliModel
}

// resolveEngineerPersona returns the configured engineer persona: the
// inline engineer_prompt wins, then engineer_prompt_file (relative paths
// resolve against the repo root); empty means the autodev default persona
// applies (REQ-016).
func resolveEngineerPersona(cfg autodev.AutodevConfig, repoRoot string) (string, error) {
	if strings.TrimSpace(cfg.EngineerPrompt) != "" {
		return cfg.EngineerPrompt, nil
	}
	if cfg.EngineerPromptFile == "" {
		return "", nil
	}
	path := cfg.EngineerPromptFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read engineer_prompt_file: %w", err)
	}
	return string(data), nil
}

// buildAutodevDeps assembles the orchestrator dependencies from the CLI
// config: the autodev config, the shared model, the provider-backed
// engineer Agent, the AgentRunner-backed core factory, and the os/exec
// git/gh runners.
func buildAutodevDeps(ctx context.Context, cfg CLIConfig, reporter autodev.Reporter) (autodev.Deps, error) {
	repoRoot, err := filepath.Abs(cfg.WorkDir)
	if err != nil {
		return autodev.Deps{}, err
	}
	adCfg, err := autodev.Load(repoRoot)
	if err != nil {
		return autodev.Deps{}, err
	}
	// A positional backlog path on the CLI overrides the configured file.
	if strings.TrimSpace(cfg.Prompt) != "" {
		adCfg.BacklogFile = strings.TrimSpace(cfg.Prompt)
	}
	adCfg.Model = resolveAutodevModel(cfg.Model, adCfg)

	persona, err := resolveEngineerPersona(adCfg, repoRoot)
	if err != nil {
		return autodev.Deps{}, err
	}
	if cfg.ResolvedLLM.Protocol == "" || cfg.ResolvedLLM.BaseURL == "" || cfg.ResolvedLLM.Model == "" {
		return autodev.Deps{}, fmt.Errorf("missing LLM configuration: protocol, base_url, and model are required")
	}
	llmConfig := cfg.ResolvedLLM.WithModel(adCfg.Model)
	llm, err := provider.NewProvider(llmConfig)
	if err != nil {
		return autodev.Deps{}, err
	}

	return autodev.Deps{
		Config:   adCfg,
		RepoRoot: repoRoot,
		CoreFactory: &appCoreRunnerFactory{
			llmConfig:      llmConfig,
			maxTurns:       cfg.MaxTurns,
			newChildRunner: cfg.NewChildRunner,
		},
		Engineer: autodev.NewEngineerAgent(llm, adCfg.Model, persona),
		Git:      autodev.NewExecGitRunner(),
		Exec:     autodev.NewExecCommandRunner(),
		Reporter: reporter,
	}, nil
}

// RunAutodev runs the backlog autopilot: it builds the orchestrator with
// the real adapters and drains the backlog, streaming every event through
// reporter (REQ-024, REQ-026).
func RunAutodev(ctx context.Context, cfg CLIConfig, reporter autodev.Reporter) error {
	deps, err := buildAutodevDeps(ctx, cfg, reporter)
	if err != nil {
		return err
	}
	return autodev.New(deps).Run(ctx)
}
