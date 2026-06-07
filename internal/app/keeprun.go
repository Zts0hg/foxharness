package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Zts0hg/foxharness/internal/tui"
)

// RunKeepRun executes the keep-run SDD pipeline headlessly (no TUI), building the
// shared runtime exactly as the interactive and one-shot paths do and streaming
// progress to stdout. It reads BACKLOG.md from the working directory and processes
// pending tasks until none remain or ctx is canceled.
func RunKeepRun(ctx context.Context, cfg CLIConfig) error {
	runner, err := NewAgentRunner(ctx, agentRunnerConfigFromCLI(cfg))
	if err != nil {
		return err
	}
	repoDir, err := filepath.Abs(cfg.WorkDir)
	if err != nil {
		return err
	}
	return tui.RunKeepRunHeadless(ctx, runner, repoDir, runner.SlashRegistry(), runner.SlashExecutor(), os.Stdout)
}
