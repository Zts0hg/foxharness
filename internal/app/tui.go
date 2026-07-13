package app

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/settings"
	"github.com/Zts0hg/foxharness/internal/tui"
)

// RunTUI starts an interactive terminal UI that keeps one session open across
// many user-submitted runs. The onModelChange callback is invoked whenever the
// user switches models via the /model command; it may be nil.
func RunTUI(ctx context.Context, cfg CLIConfig, onModelChange func(string) error) error {
	homeDir, _ := os.UserHomeDir()
	loadedSettings, _ := settings.Load(homeDir)
	permissionState := permission.NewState(
		permission.NormalizeMode(loadedSettings.TUI.Permissions.Mode),
		loadedSettings.TUI.Permissions.FullAccessWarningRemembered,
	)
	permissionBridge := tui.NewPermissionBridge()
	var runner *AgentRunner
	reviewer := permission.NewProviderReviewer(func() provider.LLMProvider {
		if runner == nil {
			return nil
		}
		runner.mu.Lock()
		defer runner.mu.Unlock()
		return runner.llmProvider
	})
	reviewer.OnRetry = permissionBridge.OnReviewRetry
	coordinator := permission.NewCoordinator(permission.Config{
		State:     permissionState,
		Workspace: cfg.WorkDir,
		CWD:       cfg.WorkDir,
		Source:    permission.SourceMain,
		Approver:  permissionBridge,
		Reviewer:  reviewer,
		Sink:      permissionBridge,
	})
	defer coordinator.State().ClearGrants()
	runnerCfg := agentRunnerConfigFromCLI(cfg)
	runnerCfg.OnModelChange = onModelChange
	runnerCfg.Permission = coordinator
	var err error
	runner, err = NewAgentRunner(ctx, runnerCfg)
	if err != nil {
		return err
	}
	restoreLogs := redirectTUILogs(runner.SessionDir())
	defer restoreLogs()

	asker := attachInteractiveAsker(runner)
	planReviewer := attachInteractivePlanReviewer(runner)

	return tui.Run(ctx, runner, tui.Config{
		Model:             cfg.Model,
		InitialPrompt:     cfg.Prompt,
		HomeDir:           homeDir,
		ProviderID:        cfg.ResolvedLLM.ProviderID,
		ProviderProfileID: cfg.ResolvedLLM.SettingsProviderID,
		ProviderProtocol:  cfg.ResolvedLLM.Protocol,
		Registry:          runner.SlashRegistry(),
		Executor:          runner.SlashExecutor(),
		Asker:             asker,
		PlanReviewer:      planReviewer,
		Permissions:       permissionBridge,
		Autodev: func(runCtx context.Context, backlogPath string, reporter autodev.Reporter) error {
			autodevCfg := cfg
			autodevCfg.Prompt = backlogPath
			return RunAutodev(runCtx, autodevCfg, reporter)
		},
	})
}

func attachInteractivePlanReviewer(runner *AgentRunner) *tui.PlanReviewer {
	reviewer := tui.NewPlanReviewer()
	runner.SetPlanReviewer(reviewer)
	return reviewer
}

// attachInteractiveAsker creates the interactive asker, installs it on the
// runner so the ask_user_question tool is registered for this (TUI) session, and
// returns it for the TUI model to listen on.
func attachInteractiveAsker(runner *AgentRunner) *tui.Asker {
	asker := tui.NewAsker()
	runner.SetUserAsker(asker)
	return asker
}

func redirectTUILogs(sessionDir string) func() {
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()

	restore := func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	}

	logPath := filepath.Join(sessionDir, "tui.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.SetOutput(io.Discard)
		return restore
	}

	log.SetOutput(file)
	return func() {
		restore()
		_ = file.Close()
	}
}
