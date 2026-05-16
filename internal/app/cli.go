// Package app wires together the foxharness CLI entry point. It orchestrates
// session creation, plan-mode planning, tool registration, subagent setup,
// compaction, and the engine run for a single user prompt.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"

	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tools"
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
}

// RunCLI executes a single agent session from prompt to final output. It
// initializes file-based memory, creates a session, optionally generates a
// plan when plan mode is enabled, registers tools and subagent support, and
// runs the engine. Session metadata (transcript, metrics, trace) is printed
// on completion.
func RunCLI(ctx context.Context, cfg CLIConfig) error {
	workDir, err := filepath.Abs(cfg.WorkDir)
	if err != nil {
		return err
	}

	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		return fmt.Errorf("初始化文件记忆失败: %w", err)
	}

	manager := session.NewManager(workDir)
	sess, err := resolveCLISession(manager, workDir, cfg)
	if err != nil {
		return err
	}

	log.Printf("[CLI] Session: %s", sess.ID)
	log.Printf("[CLI] Session dir: %s", sess.RootDir)

	llmProvider := provider.NewZhipuOpenAIProvider(cfg.Model)
	enableThinking := cfg.EnableThinking
	if cfg.EnablePlanMode {
		planner := memory.NewPlanner(llmProvider, store)
		if err := planner.BuildPlan(ctx, cfg.Prompt); err != nil {
			log.Printf("[PlanMode] 生成计划失败，将回退到旧版本每轮 Thinking: %v", err)
			enableThinking = true
		} else {
			log.Printf("[PlanMode] 计划已生成，本次任务关闭每轮 Thinking")
			enableThinking = false
		}
	}

	composer := prompt.NewComposer(workDir).WithMemory(sess.MemoryPath())
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	subManager := subagent.NewManager(llmProvider, workDir)
	registry.Register(subagent.NewTool(subManager, sess.ID))

	eng := engine.NewAgentEngine(
		llmProvider,
		registry,
		workDir,
		composer,
		engine.Config{
			EnableThinking: enableThinking,
			MaxTurns:       cfg.MaxTurns,
		},
	)

	eng.WithCompactor(compaction.NewCompactor(
		llmProvider,
		compaction.RoughEstimator{},
		compaction.DefaultConfig(),
	))

	result, err := eng.Run(ctx, sess, cfg.Prompt)
	if err != nil {
		log.Printf("[CLI] 任务失败: %v", err)
	}

	if result != nil && result.FinalMessage != "" {
		fmt.Println(result.FinalMessage)
	}

	fmt.Println()
	fmt.Println("Session: ", sess.ID)
	fmt.Println("Transcript: ", sess.TranscriptPath())
	if result != nil {
		fmt.Println("Run: ", result.RunID)
		fmt.Println("Metrics: ", result.MetricsPath)
		fmt.Println("Trace: ", result.TracePath)
	}
	return err
}

func resolveCLISession(manager *session.Manager, workDir string, cfg CLIConfig) (*session.Session, error) {
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
