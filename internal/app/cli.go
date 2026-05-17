// Package app wires together the foxharness CLI entry point. It orchestrates
// session creation, plan-mode planning, tool registration, subagent setup,
// compaction, and the engine run for a single user prompt.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/Zts0hg/foxharness/internal/session"
)

// CLIConfig holds the configuration for a CLI agent run, including the
// workspace directory, user prompt, model identifier, and feature flags for
// thinking and plan mode.
type CLIConfig struct {
	WorkDir         string
	Prompt          string
	Model           string
	EnableThinking  bool
	EnablePlanMode  bool
	MaxTurns        int
	SessionID       string
	ContinueSession bool
	NewSession      bool
	Interactive     bool
}

// RunCLI executes a single agent session from prompt to final output. It
// initializes file-based memory, creates a session, optionally generates a
// plan when plan mode is enabled, registers tools and subagent support, and
// runs the engine. Session metadata (transcript, metrics, trace) is printed
// on completion.
func RunCLI(ctx context.Context, cfg CLIConfig) error {
	runner, err := NewAgentRunner(ctx, agentRunnerConfigFromCLI(cfg))
	if err != nil {
		return err
	}

	log.Printf("[CLI] Session: %s", runner.SessionID())
	log.Printf("[CLI] Session dir: %s", runner.SessionDir())

	result, err := runner.Run(ctx, cfg.Prompt, nil)
	if err != nil {
		log.Printf("[CLI] 任务失败: %v", err)
	}

	if result != nil && result.FinalMessage != "" {
		fmt.Println(result.FinalMessage)
	}

	fmt.Println()
	fmt.Println("Session: ", runner.SessionID())
	fmt.Println("Transcript: ", runner.TranscriptPath())
	if result != nil {
		fmt.Println("Run: ", result.RunID)
		fmt.Println("Metrics: ", result.MetricsPath)
		fmt.Println("Trace: ", result.TracePath)
	}
	return err
}

func resolveCLISession(manager *session.Manager, workDir string, cfg CLIConfig) (*session.Session, error) {
	return resolveRunnerSession(manager, workDir, agentRunnerConfigFromCLI(cfg))
}

func resolveRunnerSession(manager *session.Manager, workDir string, cfg AgentRunnerConfig) (*session.Session, error) {
	if cfg.NewSession && (cfg.SessionID != "" || cfg.ContinueSession) {
		return nil, fmt.Errorf("-new 不能和 -session 或 -continue 同时使用")
	}
	if cfg.SessionID != "" && cfg.ContinueSession {
		return nil, fmt.Errorf("-session 不能和 -continue 同时使用")
	}

	if cfg.SessionID != "" {
		sess, err := manager.Open(cfg.SessionID)
		if err != nil {
			if errors.Is(err, session.ErrNotFound) {
				return nil, fmt.Errorf("Session %s 不存在", cfg.SessionID)
			}
			return nil, err
		}
		return sess, nil
	}

	if cfg.ContinueSession {
		sess, err := manager.Latest(session.LookupOptions{Source: session.SOURCECLI})
		if err != nil {
			if errors.Is(err, session.ErrNotFound) {
				return nil, fmt.Errorf("没有可继续的 CLI Session")
			}
			return nil, err
		}
		return sess, nil
	}

	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 Session 失败: %w", err)
	}
	return sess, nil
}
