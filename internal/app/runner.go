package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// AgentRunnerConfig contains the engine and session options shared by the
// one-shot CLI and the interactive TUI.
type AgentRunnerConfig struct {
	WorkDir         string
	Model           string
	EnableThinking  bool
	EnablePlanMode  bool
	MaxTurns        int
	SessionID       string
	ContinueSession bool
	NewSession      bool
}

// AgentRunner owns one long-lived session and can execute many user prompts
// as separate runs inside that session.
type AgentRunner struct {
	mu sync.Mutex

	workDir string
	model   string

	enableThinking bool
	enablePlanMode bool
	maxTurns       int

	store          *memory.Store
	manager        *session.Manager
	llmProvider    provider.LLMProvider
	currentSession *session.Session
}

func agentRunnerConfigFromCLI(cfg CLIConfig) AgentRunnerConfig {
	return AgentRunnerConfig{
		WorkDir:         cfg.WorkDir,
		Model:           cfg.Model,
		EnableThinking:  cfg.EnableThinking,
		EnablePlanMode:  cfg.EnablePlanMode,
		MaxTurns:        cfg.MaxTurns,
		SessionID:       cfg.SessionID,
		ContinueSession: cfg.ContinueSession,
		NewSession:      cfg.NewSession,
	}
}

// NewAgentRunner initializes the shared runtime for one-shot and interactive
// execution. The selected session remains attached to the runner until
// NewSession is called.
func NewAgentRunner(ctx context.Context, cfg AgentRunnerConfig) (*AgentRunner, error) {
	_ = ctx

	workDir, err := filepath.Abs(cfg.WorkDir)
	if err != nil {
		return nil, err
	}

	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		return nil, fmt.Errorf("初始化文件记忆失败: %w", err)
	}

	manager := session.NewManager(workDir)
	sess, err := resolveRunnerSession(manager, workDir, cfg)
	if err != nil {
		return nil, err
	}

	return &AgentRunner{
		workDir:        workDir,
		model:          cfg.Model,
		enableThinking: cfg.EnableThinking,
		enablePlanMode: cfg.EnablePlanMode,
		maxTurns:       cfg.MaxTurns,
		store:          store,
		manager:        manager,
		llmProvider:    provider.NewZhipuOpenAIProvider(cfg.Model),
		currentSession: sess,
	}, nil
}

// Run executes one prompt as a new run in the current session.
func (r *AgentRunner) Run(ctx context.Context, userPrompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess := r.currentSession
	enableThinking := r.enableThinking
	if r.enablePlanMode {
		planner := memory.NewPlanner(r.llmProvider, r.store)
		if err := planner.BuildPlan(ctx, userPrompt); err != nil {
			log.Printf("[PlanMode] 生成计划失败，将回退到旧版本每轮 Thinking: %v", err)
			enableThinking = true
		} else {
			log.Printf("[PlanMode] 计划已生成，本次任务关闭每轮 Thinking")
			enableThinking = false
		}
	}

	composer := prompt.NewComposer(r.workDir).WithMemory(sess.MemoryPath())
	eng := engine.NewAgentEngine(
		r.llmProvider,
		r.buildRegistry(sess),
		r.workDir,
		composer,
		engine.Config{
			EnableThinking: enableThinking,
			MaxTurns:       r.maxTurns,
		},
	)
	eng.WithCompactor(compaction.NewCompactor(
		r.llmProvider,
		compaction.RoughEstimator{},
		compaction.DefaultConfig(),
	))

	return eng.RunWithReporter(ctx, sess, userPrompt, reporter)
}

// NewSession switches the runner to a fresh CLI session.
func (r *AgentRunner) NewSession(ctx context.Context) (string, error) {
	_ = ctx

	r.mu.Lock()
	defer r.mu.Unlock()

	sess, err := r.manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: r.workDir,
	})
	if err != nil {
		return "", err
	}
	r.currentSession = sess
	return sess.ID, nil
}

func (r *AgentRunner) SessionID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentSession.ID
}

func (r *AgentRunner) SessionDir() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentSession.RootDir
}

func (r *AgentRunner) TranscriptPath() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentSession.TranscriptPath()
}

func (r *AgentRunner) WorkDir() string {
	return r.workDir
}

func (r *AgentRunner) Model() string {
	return r.model
}

func (r *AgentRunner) buildRegistry(sess *session.Session) tools.Registry {
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(r.workDir))
	registry.Register(tools.NewWriteFileTool(r.workDir))
	registry.Register(tools.NewBashTool(r.workDir))
	registry.Register(tools.NewEditFileTool(r.workDir))

	subManager := subagent.NewManager(r.llmProvider, r.workDir)
	registry.Register(subagent.NewTool(subManager, sess.ID))
	return registry
}
