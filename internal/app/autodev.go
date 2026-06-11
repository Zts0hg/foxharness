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

	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// agentRunnerAPI is the *AgentRunner method subset the adapter needs,
// extracted as an interface so the adapter is testable without a provider.
type agentRunnerAPI interface {
	Run(ctx context.Context, prompt string, reporter engine.Reporter) (*engine.RunResult, error)
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

// Run implements autodev.CoreRunner.
func (a *coreRunnerAdapter) Run(ctx context.Context, prompt string, r engine.Reporter) (*engine.RunResult, error) {
	return a.runner.Run(ctx, prompt, r)
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
// item's worktree. Plan mode is disabled: the SDD pipeline supplies its own
// structure, and per-run planning would fight the staged prompts.
type appCoreRunnerFactory struct {
	providerProtocol string
	model            string
	maxTurns         int
}

var _ autodev.CoreRunnerFactory = (*appCoreRunnerFactory)(nil)

// New implements autodev.CoreRunnerFactory.
func (f *appCoreRunnerFactory) New(ctx context.Context, workDir, model string) (autodev.CoreRunner, error) {
	if model == "" {
		model = f.model
	}
	runner, err := NewAgentRunner(ctx, AgentRunnerConfig{
		WorkDir:        workDir,
		Model:          model,
		Provider:       f.providerProtocol,
		EnablePlanMode: false,
		MaxTurns:       f.maxTurns,
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
	llm, err := provider.NewZhipuProvider(cfg.Provider, adCfg.Model)
	if err != nil {
		return autodev.Deps{}, err
	}

	return autodev.Deps{
		Config:   adCfg,
		RepoRoot: repoRoot,
		CoreFactory: &appCoreRunnerFactory{
			providerProtocol: cfg.Provider,
			model:            adCfg.Model,
			maxTurns:         cfg.MaxTurns,
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
