package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
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
	Provider        string
	EnableThinking  bool
	EnablePlanMode  bool
	MaxTurns        int
	SessionID       string
	ContinueSession bool
	NewSession      bool
	OnModelChange  func(model string) error
}

// AgentRunner owns one long-lived session and can execute many user prompts
// as separate runs inside that session.
type AgentRunner struct {
	mu    sync.Mutex
	runMu sync.Mutex

	workDir          string
	model            string
	providerProtocol string

	enableThinking bool
	enablePlanMode bool
	maxTurns       int

	onModelChange func(model string) error

	store          *memory.Store
	manager        *session.Manager
	llmProvider    provider.LLMProvider
	currentSession *session.Session
}

func agentRunnerConfigFromCLI(cfg CLIConfig) AgentRunnerConfig {
	return AgentRunnerConfig{
		WorkDir:         cfg.WorkDir,
		Model:           cfg.Model,
		Provider:        cfg.Provider,
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

	manager := session.NewManager(workDir)
	sess, err := resolveRunnerSession(manager, workDir, cfg)
	if err != nil {
		return nil, err
	}

	store := memory.NewSessionStore(workDir, sess.RootDir)
	if err := store.EnsureFiles(); err != nil {
		return nil, fmt.Errorf("初始化文件记忆失败: %w", err)
	}

	llmProvider, err := provider.NewZhipuProvider(cfg.Provider, cfg.Model)
	if err != nil {
		return nil, err
	}

	providerProtocol := cfg.Provider
	if providerProtocol == "" {
		providerProtocol = provider.ProviderProtocolOpenAI
	} else {
		providerProtocol = strings.ToLower(strings.TrimSpace(providerProtocol))
	}

	return &AgentRunner{
		workDir:          workDir,
		model:            cfg.Model,
		providerProtocol: providerProtocol,
		enableThinking:   cfg.EnableThinking,
		enablePlanMode:   cfg.EnablePlanMode,
		maxTurns:         cfg.MaxTurns,
		onModelChange:    cfg.OnModelChange,
		store:            store,
		manager:          manager,
		llmProvider:      llmProvider,
		currentSession:   sess,
	}, nil
}

// Run executes one prompt as a new run in the current session.
func (r *AgentRunner) Run(ctx context.Context, userPrompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	r.runMu.Lock()
	defer r.runMu.Unlock()

	r.mu.Lock()
	sess := r.currentSession
	store := r.store
	enableThinking := r.enableThinking
	enablePlanMode := r.enablePlanMode
	llmProvider := r.llmProvider
	providerProtocol := r.providerProtocol
	model := r.model
	maxTurns := r.maxTurns
	r.mu.Unlock()

	if enablePlanMode {
		planner := memory.NewPlanner(llmProvider, store)
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
		llmProvider,
		r.buildRegistry(sess, llmProvider),
		r.workDir,
		composer,
		engine.Config{
			EnableThinking:   enableThinking,
			MaxTurns:         maxTurns,
			ProviderProtocol: providerProtocol,
			Model:            model,
		},
	)
	eng.WithCompactor(compaction.NewCompactor(
		llmProvider,
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
	store := memory.NewSessionStore(r.workDir, sess.RootDir)
	if err := store.EnsureFiles(); err != nil {
		return "", err
	}
	r.currentSession = sess
	r.store = store
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
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.model
}

// SetModel switches the model used by future runs while preserving the current
// provider protocol. If a run is active, the switch waits until that run ends.
func (r *AgentRunner) SetModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	r.runMu.Lock()
	defer r.runMu.Unlock()

	r.mu.Lock()
	protocol := r.providerProtocol
	r.mu.Unlock()

	llmProvider, err := provider.NewZhipuProvider(protocol, model)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.model = model
	r.llmProvider = llmProvider
	r.mu.Unlock()

	if r.onModelChange != nil {
		if err := r.onModelChange(model); err != nil {
			log.Printf("[Runner] onModelChange callback failed: %v", err)
		}
	}
	return nil
}

func (r *AgentRunner) ContextUsage() string {
	r.mu.Lock()
	sess := r.currentSession
	r.mu.Unlock()
	if sess == nil {
		return "unknown"
	}

	messages, err := session.NewMessageLog(sess).LoadMessages()
	if err != nil {
		log.Printf("[Runner] 读取 Session 上下文使用量失败: %v", err)
		return "unknown"
	}
	used := compaction.RoughEstimator{}.Estimate(messages)
	return formatContextUsage(used, compaction.DefaultConfig().MaxTokens)
}

func (r *AgentRunner) MessageHistory() ([]session.MessageRecord, error) {
	r.mu.Lock()
	sess := r.currentSession
	r.mu.Unlock()
	if sess == nil {
		return nil, nil
	}
	return session.NewMessageLog(sess).LoadRecords()
}

func (r *AgentRunner) PlanMode() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enablePlanMode
}

func (r *AgentRunner) SetPlanMode(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enablePlanMode = enabled
}

func (r *AgentRunner) buildRegistry(sess *session.Session, llmProvider provider.LLMProvider) tools.Registry {
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(r.workDir))
	registry.Register(tools.NewWriteFileTool(r.workDir))
	registry.Register(tools.NewBashTool(r.workDir))
	registry.Register(tools.NewEditFileTool(r.workDir))

	subManager := subagent.NewManager(llmProvider, r.workDir)
	registry.Register(subagent.NewTool(subManager, sess.ID))
	return registry
}

func formatContextUsage(used int, maxTokens int) string {
	if maxTokens <= 0 {
		return "unknown"
	}
	if used <= 0 {
		return "0%"
	}
	if used*100 < maxTokens {
		return "<1%"
	}
	percent := (used*100 + maxTokens - 1) / maxTokens
	return fmt.Sprintf("%d%%", percent)
}
