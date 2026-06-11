package app

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/tui"
)

// RunTUI starts an interactive terminal UI that keeps one session open across
// many user-submitted runs. The onModelChange callback is invoked whenever the
// user switches models via the /model command; it may be nil.
func RunTUI(ctx context.Context, cfg CLIConfig, onModelChange func(string) error) error {
	runnerCfg := agentRunnerConfigFromCLI(cfg)
	runnerCfg.OnModelChange = onModelChange
	runner, err := NewAgentRunner(ctx, runnerCfg)
	if err != nil {
		return err
	}
	restoreLogs := redirectTUILogs(runner.SessionDir())
	defer restoreLogs()

	asker := attachInteractiveAsker(runner)

	return tui.Run(ctx, runner, tui.Config{
		Model:         cfg.Model,
		InitialPrompt: cfg.Prompt,
		Registry:      runner.SlashRegistry(),
		Executor:      runner.SlashExecutor(),
		Asker:         asker,
		Autodev: func(runCtx context.Context, backlogPath string, reporter autodev.Reporter) error {
			autodevCfg := cfg
			autodevCfg.Prompt = backlogPath
			return RunAutodev(runCtx, autodevCfg, reporter)
		},
	})
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
