package app

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/settings"
)

/* LegacyTUIBindings injects presentation-owned interaction bridges and startup into the temporary TUI facade. */
type LegacyTUIBindings struct {
	Permissions        PermissionPort
	Questions          QuestionPort
	PlanReview         PlanReviewPort
	InteractionNotices InteractionNoticeSink
	Attach             func(*AgentRunner)
	Start              func(context.Context, InteractiveApplication) error
}

// RunTUI starts an interactive terminal UI that keeps one session open across
// many user-submitted runs. The onModelChange callback is invoked whenever the
// user switches models via the /model command; it may be nil.
func RunTUI(ctx context.Context, cfg CLIConfig, onModelChange func(string) error, bindings LegacyTUIBindings) error {
	homeDir, _ := os.UserHomeDir()
	loadedSettings, _ := settings.Load(homeDir)
	permissionState := permission.NewState(
		permission.NormalizeMode(loadedSettings.TUI.Permissions.Mode),
		loadedSettings.TUI.Permissions.FullAccessWarningRemembered,
	)
	permissionEvents := newLegacyPermissionEventSink(bindings.InteractionNotices)
	var runner *AgentRunner
	reviewer := permission.NewProviderReviewer(func() provider.LLMProvider {
		if runner == nil {
			return nil
		}
		runner.mu.Lock()
		defer runner.mu.Unlock()
		return runner.llmProvider
	})
	reviewer.OnRetry = permissionEvents.OnReviewRetry
	coordinator := permission.NewCoordinator(permission.Config{
		State:     permissionState,
		Workspace: cfg.WorkDir,
		CWD:       cfg.WorkDir,
		Source:    permission.SourceMain,
		Approver:  newLegacyPermissionApprover(bindings.Permissions),
		Reviewer:  reviewer,
		Sink:      permissionEvents,
	})
	defer coordinator.State().ClearGrants()
	runnerCfg := agentRunnerConfigFromCLI(cfg)
	runnerCfg.RuntimeProfile = childParentTUI
	runnerCfg.OnModelChange = onModelChange
	runnerCfg.Permission = coordinator
	var err error
	runner, err = NewAgentRunner(ctx, runnerCfg)
	if err != nil {
		return err
	}
	runner.SetUserAsker(newLegacyQuestionAsker(bindings.Questions))
	runner.SetPlanReviewer(newLegacyPlanReviewer(bindings.PlanReview))
	restoreLogs := redirectTUILogs(runner.SessionDir())
	defer restoreLogs()

	if bindings.Attach != nil {
		bindings.Attach(runner)
	}
	if bindings.Start == nil {
		return errors.New("legacy TUI start binding is required")
	}
	return bindings.Start(ctx, NewLegacyInteractiveApplication(runner))
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
