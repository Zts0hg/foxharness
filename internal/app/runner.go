package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/slash/skilltool"
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
	OnModelChange   func(model string) error
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
	checkpointer   checkpoint.Checkpointer
	slashRegistry  *slash.Registry
	slashExecutor  *slash.Executor

	pendingMu          sync.Mutex
	pendingActivations []string
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
	cp := checkpoint.New(checkpoint.Config{SessionDir: sess.RootDir})
	if checkpointDisabledFromEnv() {
		cp.SetDisabled(true)
	}
	if cfg.SessionID != "" || cfg.ContinueSession {
		if err := cp.RestoreStateFromLog(); err != nil {
			return nil, fmt.Errorf("恢复 checkpoint 状态失败: %w", err)
		}
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

	slashRegistry := slash.NewRegistry(workDir)
	if err := slashRegistry.Load(); err != nil {
		log.Printf("[slash] registry load failed: %v", err)
	}

	ar := &AgentRunner{
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
		checkpointer:     cp,
		slashRegistry:    slashRegistry,
	}
	ar.slashExecutor = slash.NewExecutor(
		slash.WithWorkDir(workDir),
		slash.WithForkRunner(&subagentForkRunner{
			getManager: ar.currentSubagentManager,
			getSession: ar.currentSessionIDLocked,
		}),
	)
	slashRegistry.OnActivate(ar.recordSkillActivation)
	return ar, nil
}

// recordSkillActivation queues an activation notice that the engine
// drains via NextTurnReminders at the start of the next turn. This
// closes the REQ-010 gap where a skill activated mid-run was previously
// only visible to the model on subsequent runs because the system
// prompt was composed once before the turn loop.
func (r *AgentRunner) recordSkillActivation(cmd *slash.Command) {
	if cmd == nil || !cmd.IsModelInvocable() {
		return
	}
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	r.pendingActivations = append(r.pendingActivations, formatActivationReminder(cmd))
}

func formatActivationReminder(cmd *slash.Command) string {
	out := "A new skill became available for the rest of this session: `" + cmd.Name + "`"
	if cmd.Description != "" {
		out += "\n  Description: " + cmd.Description
	}
	if cmd.Frontmatter.WhenToUse != "" {
		out += "\n  When to use: " + cmd.Frontmatter.WhenToUse
	}
	if cmd.Frontmatter.ArgumentHint != "" {
		out += "\n  Arguments: " + cmd.Frontmatter.ArgumentHint
	} else if cmd.Frontmatter.Arguments != "" {
		out += "\n  Arguments: " + cmd.Frontmatter.Arguments
	}
	out += "\nInvoke it via the `skill` tool with name=\"" + cmd.Name + "\"."
	return out
}

// drainPendingActivations returns and clears any activation notices
// queued since the previous turn. Safe for concurrent access; the
// engine calls it once per turn via NextTurnReminders.
func (r *AgentRunner) drainPendingActivations() []string {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	if len(r.pendingActivations) == 0 {
		return nil
	}
	out := r.pendingActivations
	r.pendingActivations = nil
	return out
}

// currentSubagentManager returns a freshly-built subagent.Manager bound to
// the runner's current LLM provider. Built per-call so a /model switch is
// immediately reflected in fork-mode skills without rebuilding the
// executor or fork runner.
func (r *AgentRunner) currentSubagentManager() *subagent.Manager {
	r.mu.Lock()
	p := r.llmProvider
	wd := r.workDir
	r.mu.Unlock()
	return subagent.NewManager(p, wd)
}

// currentSessionIDLocked returns the current session id, or "" when no
// session is attached. Read under the runner mutex so it stays consistent
// across NewSession swaps.
func (r *AgentRunner) currentSessionIDLocked() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.currentSession == nil {
		return ""
	}
	return r.currentSession.ID
}

// SlashRegistry exposes the runner's slash command registry to callers
// like the TUI that need to attach it to the model.
func (r *AgentRunner) SlashRegistry() *slash.Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.slashRegistry
}

// SlashExecutor exposes the runner's slash executor, configured with the
// work directory and any fork runner wired up at construction time.
func (r *AgentRunner) SlashExecutor() *slash.Executor {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.slashExecutor
}

// Run executes one prompt as a new run in the current session.
func (r *AgentRunner) Run(ctx context.Context, userPrompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, nil, reporter)
}

// RunRestricted executes one prompt with the tool registry filtered down
// to allowedTools. Calls from prompt commands that declare an
// `allowed-tools` frontmatter use this path so the per-turn restriction
// is enforced at the registry level (NFR-002), not just advisory.
//
// allowedTools must be non-empty; pass nil/empty to Run instead.
func (r *AgentRunner) RunRestricted(ctx context.Context, userPrompt string, allowedTools []string, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, allowedTools, reporter)
}

func (r *AgentRunner) runInternal(ctx context.Context, userPrompt string, allowedTools []string, reporter engine.Reporter) (*engine.RunResult, error) {
	r.runMu.Lock()
	defer r.runMu.Unlock()

	r.mu.Lock()
	sess := r.currentSession
	store := r.store
	enableThinking := r.enableThinking
	enablePlanMode := r.enablePlanMode
	llmProvider := r.llmProvider
	cp := r.checkpointer
	providerProtocol := r.providerProtocol
	model := r.model
	maxTurns := r.maxTurns
	r.mu.Unlock()

	nextSeq, err := session.NewMessageLog(sess).NextSeq()
	if err != nil {
		return nil, fmt.Errorf("读取下一条消息序号失败: %w", err)
	}
	if err := memory.NewStateHistory(store).SnapshotBeforeMessage(nextSeq); err != nil {
		return nil, fmt.Errorf("创建 session state 快照失败: %w", err)
	}

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

	r.mu.Lock()
	registry := r.slashRegistry
	r.mu.Unlock()

	composer := prompt.NewComposer(r.workDir).WithMemory(sess.MemoryPath())
	if registry != nil {
		contextWindow := compaction.NewModelRegistry().Lookup(model)
		tokens := contextWindow
		composer = composer.WithSkillList(func() string {
			return skilltool.FormatSkillsWithinBudget(registry.ModelInvocable(), tokens)
		})
	}
	var messageIDMu sync.Mutex
	currentMessageID := ""
	setCurrentMessageID := func(messageID string) {
		messageIDMu.Lock()
		currentMessageID = messageID
		messageIDMu.Unlock()
	}
	getCurrentMessageID := func() string {
		messageIDMu.Lock()
		defer messageIDMu.Unlock()
		return currentMessageID
	}

	toolRegistry := r.buildRegistry(sess, llmProvider, cp, getCurrentMessageID)
	if len(allowedTools) > 0 {
		toolRegistry = slash.NewFilteredRegistry(toolRegistry, allowedTools)
		log.Printf("[slash] restricting next run to allowed tools: %v", allowedTools)
	}

	eng := engine.NewAgentEngine(
		llmProvider,
		toolRegistry,
		r.workDir,
		composer,
		engine.Config{
			EnableThinking:    enableThinking,
			MaxTurns:          maxTurns,
			ProviderProtocol:  providerProtocol,
			Model:             model,
			Checkpointer:      cp,
			OnUserMessageID:   setCurrentMessageID,
			OnToolCalled:      r.conditionalActivationHook(),
			NextTurnReminders: r.drainPendingActivations,
		},
	)
	compCfg := compaction.DefaultCompactionConfig()
	compCfg.Model = model
	compCfg.SessionDir = sess.RootDir
	compCfg.TranscriptPath = sess.TranscriptPath()
	compactor, err := compaction.NewCompactor(llmProvider, compCfg)
	if err != nil {
		return nil, fmt.Errorf("初始化 Compactor 失败: %w", err)
	}
	eng.WithCompactor(compactor)

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
	r.checkpointer = checkpoint.New(checkpoint.Config{SessionDir: sess.RootDir})
	if checkpointDisabledFromEnv() {
		r.checkpointer.SetDisabled(true)
	}
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
	used := compaction.ImprovedRoughEstimator{}.Estimate(messages)
	contextWindow := compaction.NewModelRegistry().Lookup(r.model)
	return formatContextUsage(used, contextWindow)
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

// ProjectInputHistory returns recent real user prompts from CLI sessions in
// this runner's project, ordered for the TUI's chronological history storage.
func (r *AgentRunner) ProjectInputHistory(limit int) ([]string, error) {
	r.mu.Lock()
	manager := r.manager
	current := r.currentSession
	r.mu.Unlock()
	if manager == nil || current == nil {
		return nil, nil
	}

	sessions, err := manager.List(session.LookupOptions{Source: session.SOURCECLI})
	if errors.Is(err, session.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	type promptRecord struct {
		text      string
		when      time.Time
		seq       int64
		sessionID string
		current   bool
	}
	var prompts []promptRecord
	for _, sess := range sessions {
		records, err := session.NewMessageLog(sess).LoadRecords()
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			msg := record.Message
			if msg.Role != schema.RoleUser || msg.ToolCallID != "" {
				continue
			}
			text := strings.TrimSpace(msg.Content)
			if text == "" || isCompactionSummaryPrompt(text) {
				continue
			}
			prompts = append(prompts, promptRecord{
				text:      text,
				when:      record.Time,
				seq:       record.Seq,
				sessionID: sess.ID,
				current:   sess.ID == current.ID,
			})
		}
	}

	sort.SliceStable(prompts, func(i, j int) bool {
		if !prompts[i].when.Equal(prompts[j].when) {
			return prompts[i].when.After(prompts[j].when)
		}
		if prompts[i].sessionID != prompts[j].sessionID {
			return prompts[i].sessionID > prompts[j].sessionID
		}
		return prompts[i].seq > prompts[j].seq
	})
	if limit > 0 && len(prompts) > limit {
		prompts = prompts[:limit]
	}

	sort.SliceStable(prompts, func(i, j int) bool {
		if prompts[i].current != prompts[j].current {
			return !prompts[i].current
		}
		if !prompts[i].when.Equal(prompts[j].when) {
			return prompts[i].when.Before(prompts[j].when)
		}
		if prompts[i].sessionID != prompts[j].sessionID {
			return prompts[i].sessionID < prompts[j].sessionID
		}
		return prompts[i].seq < prompts[j].seq
	})

	history := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		if len(history) > 0 && history[len(history)-1] == prompt.text {
			continue
		}
		history = append(history, prompt.text)
	}
	return history, nil
}

func (r *AgentRunner) TruncateMessageHistory(seq int64) error {
	r.mu.Lock()
	sess := r.currentSession
	r.mu.Unlock()
	if sess == nil {
		return nil
	}
	return session.NewMessageLog(sess).TruncateBeforeSeq(seq)
}

func isCompactionSummaryPrompt(content string) bool {
	return strings.HasPrefix(strings.TrimSpace(content), "## Compacted Context Summary")
}

func (r *AgentRunner) RestoreSessionStateBeforeMessage(seq int64) (bool, error) {
	r.mu.Lock()
	store := r.store
	r.mu.Unlock()
	if store == nil {
		return false, nil
	}
	err := memory.NewStateHistory(store).RestoreBeforeMessage(seq)
	if errors.Is(err, memory.ErrStateSnapshotNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *AgentRunner) Checkpointer() checkpoint.Checkpointer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.checkpointer
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

// conditionalActivationHook returns an engine.OnToolCalled callback that
// extracts a file path from read_file/write_file/edit_file tool calls and
// notifies the slash registry so it can activate any conditional skills
// whose `paths` globs match.
func (r *AgentRunner) conditionalActivationHook() func(schema.ToolCall, schema.ToolResult) {
	r.mu.Lock()
	registry := r.slashRegistry
	r.mu.Unlock()
	if registry == nil {
		return nil
	}
	return func(call schema.ToolCall, result schema.ToolResult) {
		// A failed tool call did not actually operate on the file the
		// model named (middleware denial, missing path, permission
		// error, etc.). Activating conditional skills on a failed
		// attempt would (a) violate REQ-010 ("operates on" implies
		// success) and (b) leak skill metadata about paths the user
		// never successfully touched. Gate on IsError.
		if result.IsError {
			return
		}
		switch call.Name {
		case "read_file", "write_file", "edit_file":
		default:
			return
		}
		path := extractFilePath(call.Arguments)
		if path == "" {
			return
		}
		registry.CheckConditional(path)
	}
}

// subagentForkRunner implements slash.ForkRunner by delegating to a
// subagent.Manager built on demand. Both the manager and the parent
// session id are read through getters so that /new (new session) and
// /model (provider swap) are reflected immediately — keeping snapshots
// here would leave fork-mode skills pinned to the original session and
// model. The agentType parameter is currently advisory; the underlying
// manager does not yet differentiate personas.
type subagentForkRunner struct {
	getManager func() *subagent.Manager
	getSession func() string
}

func (s *subagentForkRunner) Run(ctx context.Context, task string, agentType string, allowedTools []string) (string, error) {
	_ = agentType
	mgr := s.getManager()
	if mgr == nil {
		return "", fmt.Errorf("fork runner: subagent manager unavailable")
	}
	res, err := mgr.Run(ctx, subagent.Request{
		ParentSessionID: s.getSession(),
		Task:            task,
		ReadOnly:        false,
		AllowedTools:    allowedTools,
	})
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", nil
	}
	return res.Report, nil
}

func extractFilePath(raw []byte) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return ""
	}
	return args.Path
}

func (r *AgentRunner) buildRegistry(sess *session.Session, llmProvider provider.LLMProvider, cp checkpoint.Checkpointer, getMessageID func() string) tools.Registry {
	registry := tools.NewRegistry()
	registry.Use(middleware.NewCheckpointMiddleware(cp, getMessageID, r.workDir))
	registry.Register(tools.NewReadFileTool(r.workDir))
	registry.Register(tools.NewWriteFileTool(r.workDir))
	registry.Register(tools.NewBashTool(r.workDir))
	registry.Register(tools.NewEditFileTool(r.workDir))
	registry.Register(tools.NewReadTodoTool(sess.RootDir))
	registry.Register(tools.NewUpdateTodoTool(sess.RootDir))

	subManager := subagent.NewManager(llmProvider, r.workDir)
	registry.Register(subagent.NewTool(subManager, sess.ID))

	r.mu.Lock()
	slashReg := r.slashRegistry
	slashExec := r.slashExecutor
	r.mu.Unlock()
	if slashReg != nil && slashExec != nil {
		registry.Register(skilltool.NewSkillTool(slashReg, slashExec, func() string { return sess.ID }))
	}
	return registry
}

func checkpointDisabledFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FOXHARNESS_DISABLE_FILE_CHECKPOINTING"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
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
