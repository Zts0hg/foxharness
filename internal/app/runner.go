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
	"sync/atomic"
	"time"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/permission"
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
	LLM             llmconfig.ResolvedConfig
	EnableThinking  bool
	MaxTurns        int
	SessionID       string
	ContinueSession bool
	NewSession      bool
	OnModelChange   func(model string) error
	Permission      *permission.Coordinator
}

// AgentRunner owns one long-lived session and can execute many user prompts
// as separate runs inside that session.
type AgentRunner struct {
	mu    sync.Mutex
	runMu sync.Mutex

	workDir          string
	model            string
	providerProtocol string
	llmConfig        llmconfig.ResolvedConfig

	enableThinking    bool
	collaborationMode collaboration.Mode
	maxTurns          int

	onModelChange func(model string) error

	store          *memory.Store
	autoMemory     *automemory.Store
	manager        *session.Manager
	llmProvider    provider.LLMProvider
	currentSession *session.Session

	// extractionFire overrides the default post-run memory extraction launcher.
	// It is nil in production (which uses automemory.PerRunHooks.Fire); tests set
	// it to observe the hook synchronously.
	extractionFire func(sess *session.Session, runID string, tracker *automemory.Tracker)
	checkpointer   checkpoint.Checkpointer
	slashRegistry  *slash.Registry
	slashExecutor  *slash.Executor

	userAsker             tools.UserAsker
	planReviewer          tools.PlanReviewer
	permissionCoordinator *permission.Coordinator

	pendingMu          sync.Mutex
	pendingActivations []string

	contextUsedTokens   int64
	contextWindowTokens int64

	// extractWG tracks in-flight post-run memory extraction goroutines so a
	// short-lived runner (the one-shot CLI) can await them before exiting.
	extractWG sync.WaitGroup
}

func agentRunnerConfigFromCLI(cfg CLIConfig) AgentRunnerConfig {
	return AgentRunnerConfig{
		WorkDir:         cfg.WorkDir,
		Model:           cfg.Model,
		LLM:             cfg.ResolvedLLM,
		EnableThinking:  cfg.EnableThinking,
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
	autoMem := automemory.NewStore(manager.HomeDir(), workDir)
	cp := checkpoint.New(checkpoint.Config{SessionDir: sess.RootDir})
	if checkpointDisabledFromEnv() {
		cp.SetDisabled(true)
	}
	if cfg.SessionID != "" || cfg.ContinueSession {
		if err := cp.RestoreStateFromLog(); err != nil {
			return nil, fmt.Errorf("恢复 checkpoint 状态失败: %w", err)
		}
	}

	if cfg.LLM.Protocol == "" || cfg.LLM.BaseURL == "" || cfg.LLM.Model == "" {
		return nil, fmt.Errorf("missing LLM configuration: protocol, base_url, and model are required")
	}

	llmProvider, err := provider.NewProvider(cfg.LLM)
	if err != nil {
		return nil, err
	}

	providerProtocol := strings.ToLower(strings.TrimSpace(cfg.LLM.Protocol))

	slashRegistry := slash.NewRegistry(workDir)
	if err := slashRegistry.Load(); err != nil {
		log.Printf("[slash] registry load failed: %v", err)
	}

	ar := &AgentRunner{
		workDir:               workDir,
		model:                 cfg.LLM.Model,
		providerProtocol:      providerProtocol,
		llmConfig:             cfg.LLM,
		enableThinking:        cfg.EnableThinking,
		collaborationMode:     collaboration.ModeDefault,
		maxTurns:              cfg.MaxTurns,
		onModelChange:         cfg.OnModelChange,
		permissionCoordinator: cfg.Permission,
		store:                 store,
		autoMemory:            autoMem,
		manager:               manager,
		llmProvider:           llmProvider,
		currentSession:        sess,
		checkpointer:          cp,
		slashRegistry:         slashRegistry,
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

func (r *AgentRunner) planRunReminders(planRun *planLifecycle, activeRegistry tools.Registry) []string {
	reminders := planRun.runtimeReminders()
	if !registryExposesTool(activeRegistry, "skill") {
		return reminders
	}
	return append(reminders, r.drainPendingActivations()...)
}

func registryExposesTool(registry tools.Registry, name string) bool {
	if registry == nil {
		return false
	}
	for _, definition := range registry.GetAvailableTools() {
		if definition.Name == name {
			return true
		}
	}
	return false
}

// currentSubagentManager returns a freshly-built subagent.Manager bound to
// the runner's current LLM provider. Built per-call so a /model switch is
// immediately reflected in fork-mode skills without rebuilding the
// executor or fork runner.
func (r *AgentRunner) currentSubagentManager() *subagent.Manager {
	r.mu.Lock()
	p := r.llmProvider
	wd := r.workDir
	permissions := r.permissionCoordinator
	r.mu.Unlock()
	return subagent.NewManager(p, wd).WithPermission(permissions)
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
	return r.runInternal(ctx, userPrompt, "", nil, nil, reporter)
}

// RunInCollaborationMode executes one prompt in the mode captured when the
// user submitted it without changing the mode selected for later submissions.
func (r *AgentRunner) RunInCollaborationMode(ctx context.Context, userPrompt string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, "", nil, collaborationModeOverride(mode), reporter)
}

// RunWithDisplay executes one prompt while storing a separate human-facing
// prompt for transcript and history views.
func (r *AgentRunner) RunWithDisplay(ctx context.Context, userPrompt string, displayPrompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, displayPrompt, nil, nil, reporter)
}

// RunWithDisplayInCollaborationMode is RunWithDisplay with an immutable mode
// captured at user submission time.
func (r *AgentRunner) RunWithDisplayInCollaborationMode(ctx context.Context, userPrompt string, displayPrompt string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, displayPrompt, nil, collaborationModeOverride(mode), reporter)
}

// RunRestricted executes one prompt with the tool registry filtered down
// to allowedTools. Calls from prompt commands that declare an
// `allowed-tools` frontmatter use this path so the per-turn restriction
// is enforced at the registry level (NFR-002), not just advisory.
//
// allowedTools must be non-empty; pass nil/empty to Run instead.
func (r *AgentRunner) RunRestricted(ctx context.Context, userPrompt string, allowedTools []string, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, "", allowedTools, nil, reporter)
}

// RunRestrictedInCollaborationMode is RunRestricted with an immutable mode
// captured at user submission time.
func (r *AgentRunner) RunRestrictedInCollaborationMode(ctx context.Context, userPrompt string, allowedTools []string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, "", allowedTools, collaborationModeOverride(mode), reporter)
}

// RunRestrictedWithDisplay is the restricted variant of RunWithDisplay.
func (r *AgentRunner) RunRestrictedWithDisplay(ctx context.Context, userPrompt string, displayPrompt string, allowedTools []string, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, displayPrompt, allowedTools, nil, reporter)
}

// RunRestrictedWithDisplayInCollaborationMode combines display text, a tool
// allow-list, and the immutable mode captured at user submission time.
func (r *AgentRunner) RunRestrictedWithDisplayInCollaborationMode(ctx context.Context, userPrompt string, displayPrompt string, allowedTools []string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, displayPrompt, allowedTools, collaborationModeOverride(mode), reporter)
}

func collaborationModeOverride(mode collaboration.Mode) *collaboration.Mode {
	normalized := collaboration.Normalize(mode)
	return &normalized
}

func (r *AgentRunner) runInternal(ctx context.Context, userPrompt string, displayPrompt string, allowedTools []string, modeOverride *collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	r.runMu.Lock()
	defer r.runMu.Unlock()

	r.mu.Lock()
	sess := r.currentSession
	store := r.store
	autoMem := r.autoMemory
	enableThinking := r.enableThinking
	collaborationMode := collaboration.Normalize(r.collaborationMode)
	llmProvider := r.llmProvider
	cp := r.checkpointer
	providerProtocol := r.providerProtocol
	model := r.model
	maxTurns := r.maxTurns
	r.mu.Unlock()
	if modeOverride != nil {
		collaborationMode = collaboration.Normalize(*modeOverride)
	}
	if collaborationMode == collaboration.ModeFormalPlan && len(allowedTools) > 0 {
		if err := validateFormalPlanAllowedTools(allowedTools); err != nil {
			return nil, err
		}
	}

	nextSeq, err := session.NewMessageLog(sess).NextSeq()
	if err != nil {
		return nil, fmt.Errorf("读取下一条消息序号失败: %w", err)
	}
	if err := memory.NewStateHistory(store).SnapshotBeforeMessage(nextSeq); err != nil {
		return nil, fmt.Errorf("创建 session state 快照失败: %w", err)
	}

	r.mu.Lock()
	registry := r.slashRegistry
	interactiveAsk := r.userAsker != nil
	r.mu.Unlock()

	composer := prompt.NewComposer(r.workDir).
		WithCollaborationMode(collaborationMode).
		WithInteractiveAsk(interactiveAsk).
		WithMemory(sess.MemoryPath())
	if autoMem != nil {
		composer = composer.WithAutoMemory(autoMem)
	}
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

	var hooks *automemory.PerRunHooks
	var tracker *automemory.Tracker
	if autoMem != nil {
		hooks = automemory.NewPerRunHooks(llmProvider, autoMem, r.workDir)
		tracker = hooks.NewTracker()
	}

	toolRegistry := r.buildRegistry(sess, llmProvider, cp, getCurrentMessageID)
	var planRun *planLifecycle
	if collaborationMode == collaboration.ModeFormalPlan {
		planRun = r.buildPlanLifecycle(sess, store, toolRegistry)
		toolRegistry = planRun
	}
	if len(allowedTools) > 0 {
		toolRegistry = slash.NewFilteredRegistry(toolRegistry, allowedTools)
		log.Printf("[slash] restricting next run to allowed tools: %v", allowedTools)
	}

	// Compose the post-tool-call callbacks: conditional skill activation plus
	// the success-gated memory-write tracker (P2-2: a failed write must not set
	// the mutual-exclusion flag).
	skillHook := r.conditionalActivationHook()
	var memoryHook func(schema.ToolCall, schema.ToolResult)
	if hooks != nil {
		memoryHook = hooks.RecordCallback(tracker)
	}
	var onToolCalled func(schema.ToolCall, schema.ToolResult)
	switch {
	case skillHook != nil && memoryHook != nil:
		onToolCalled = func(call schema.ToolCall, result schema.ToolResult) {
			skillHook(call, result)
			memoryHook(call, result)
		}
	case skillHook != nil:
		onToolCalled = skillHook
	case memoryHook != nil:
		onToolCalled = memoryHook
	}

	nextTurnReminders := r.drainPendingActivations
	var completionGate func() string
	if planRun != nil {
		nextTurnReminders = func() []string {
			return r.planRunReminders(planRun, toolRegistry)
		}
		completionGate = planRun.completionReminder
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
			DisplayPrompt:     displayPrompt,
			OnUserMessageID:   setCurrentMessageID,
			OnToolCalled:      onToolCalled,
			NextTurnReminders: nextTurnReminders,
			CompletionGate:    completionGate,
			OnContextEstimate: func(usedTokens, contextWindow int) {
				atomic.StoreInt64(&r.contextUsedTokens, int64(usedTokens))
				atomic.StoreInt64(&r.contextWindowTokens, int64(contextWindow))
			},
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

	result, runErr := eng.RunWithReporter(ctx, sess, userPrompt, reporter)

	// Fire the post-run memory extraction hook (PLD-8), bounded to this run's
	// messages by run ID so a delayed extraction cannot pick up a later run. It
	// is fire-and-forget and runs out-of-band; it never affects the run result.
	// The launch itself is panic-guarded so a misbehaving hook can never disturb
	// the returned result.
	memoryExtractionAllowed := planRun == nil || planRun.memoryExtractionAllowed()
	if hooks != nil && result != nil && memoryExtractionAllowed {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[automemory] extraction launch panic recovered: %v", rec)
				}
			}()
			if r.extractionFire != nil {
				r.extractionFire(sess, result.RunID, tracker)
			} else {
				// Tracked launch so the one-shot CLI can drain extraction before
				// the process exits (P2-A); the interactive TUI simply never waits.
				hooks.FireTracked(&r.extractWG, sess, result.RunID, tracker)
			}
		}()
	}

	return result, runErr
}

// AutoMemoryIndex returns the merged two-tier persistent memory index
// (descriptions only) for sidebar display, or "" when no automemory store is
// wired.
func (r *AgentRunner) AutoMemoryIndex() string {
	r.mu.Lock()
	store := r.autoMemory
	r.mu.Unlock()
	if store == nil {
		return ""
	}
	return store.MergedIndexString()
}

// WaitForExtraction blocks until every in-flight post-run memory extraction
// goroutine has finished. The one-shot CLI calls it before exiting so the
// asynchronous extraction is not killed mid-call; the interactive TUI does not
// call it (extraction stays fire-and-forget across runs).
func (r *AgentRunner) WaitForExtraction() {
	r.extractWG.Wait()
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
	r.collaborationMode = collaboration.ModeDefault
	if r.permissionCoordinator != nil {
		r.permissionCoordinator.State().ClearGrants()
	}
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
// resolved LLM connection. If a run is active, the switch waits until that run
// ends.
func (r *AgentRunner) SetModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	r.runMu.Lock()
	defer r.runMu.Unlock()

	r.mu.Lock()
	nextConfig := r.llmConfig.WithModel(model)
	r.mu.Unlock()

	llmProvider, err := provider.NewProvider(nextConfig)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.model = model
	r.llmConfig = nextConfig
	r.providerProtocol = strings.ToLower(strings.TrimSpace(nextConfig.Protocol))
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
	used := atomic.LoadInt64(&r.contextUsedTokens)
	window := atomic.LoadInt64(&r.contextWindowTokens)
	if used > 0 && window > 0 {
		return formatContextUsage(int(used), int(window))
	}

	r.mu.Lock()
	sess := r.currentSession
	model := r.model
	r.mu.Unlock()
	if sess == nil {
		return "unknown"
	}

	contextWindow := compaction.NewModelRegistry().Lookup(model)
	records, err := session.NewMessageLog(sess).LoadRecords()
	if err != nil {
		log.Printf("[Runner] 读取 Session 上下文使用量失败: %v", err)
		return "unknown"
	}
	state, _ := session.LoadCompactState(sess)
	estimator := compaction.NewHybridEstimator(compaction.ImprovedRoughEstimator{})
	messages := projectedMessages(state, records)
	return formatContextUsage(estimator.Estimate(messages), contextWindow)
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
			text := strings.TrimSpace(record.HumanContent())
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

// CollaborationMode returns the mode selected for the next submitted task.
func (r *AgentRunner) CollaborationMode() collaboration.Mode {
	r.mu.Lock()
	defer r.mu.Unlock()
	return collaboration.Normalize(r.collaborationMode)
}

// SetCollaborationMode changes the mode used by the next submitted task.
func (r *AgentRunner) SetCollaborationMode(mode collaboration.Mode) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collaborationMode = collaboration.Normalize(mode)
}

// CompactNow performs a user-initiated compaction of the current session's
// message history. All messages are summarized and the CompactState is updated
// so that the next engine run sees only the summary. When customInstructions
// is non-empty it is appended to the summarization prompt to guide focus.
func (r *AgentRunner) CompactNow(ctx context.Context, customInstructions string) (*compaction.CompactResult, error) {
	r.runMu.Lock()
	defer r.runMu.Unlock()

	r.mu.Lock()
	sess := r.currentSession
	llmProvider := r.llmProvider
	model := r.model
	r.mu.Unlock()

	if sess == nil {
		return nil, fmt.Errorf("no active session")
	}

	records, err := session.NewMessageLog(sess).LoadRecords()
	if err != nil {
		return nil, fmt.Errorf("failed to load message history: %w", err)
	}

	state, err := session.LoadCompactState(sess)
	if err != nil {
		return nil, err
	}

	projected := projectedMessages(state, records)
	if len(projected) < 2 {
		return nil, fmt.Errorf("not enough messages to compact (%d messages)", len(projected))
	}

	compCfg := compaction.DefaultCompactionConfig()
	compCfg.Model = model
	compCfg.SessionDir = sess.RootDir
	compCfg.TranscriptPath = sess.TranscriptPath()
	compactor, err := compaction.NewCompactor(llmProvider, compCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create compactor: %w", err)
	}

	preTokens := compactor.Estimate(projected)

	summary, err := compactor.SummarizeWithInstructions(ctx, projected, customInstructions)
	if err != nil {
		return nil, err
	}

	var maxSeq int64 = -1
	for _, rec := range records {
		if rec.Seq > maxSeq {
			maxSeq = rec.Seq
		}
	}

	newState := &session.CompactState{
		Summary:         summary,
		CoveredUntilSeq: maxSeq,
	}
	if err := session.SaveCompactState(sess, newState); err != nil {
		return nil, err
	}

	postProjected := projectedMessages(newState, records)
	postTokens := compactor.Estimate(postProjected)

	return &compaction.CompactResult{
		PreTokens:          preTokens,
		PostTokens:         postTokens,
		MessagesSummarized: len(projected),
	}, nil
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

	r.mu.Lock()
	permissions := r.permissionCoordinator
	r.mu.Unlock()

	subManager := subagent.NewManager(llmProvider, r.workDir).WithPermission(permissions)
	registry.Register(subagent.NewTool(subManager, sess.ID))

	r.mu.Lock()
	slashReg := r.slashRegistry
	slashExec := r.slashExecutor
	userAsker := r.userAsker
	r.mu.Unlock()
	if slashReg != nil && slashExec != nil {
		registry.Register(skilltool.NewSkillTool(slashReg, slashExec, func() string { return sess.ID }))
	}
	// The ask_user_question tool is only registered when an interactive asker is
	// available (set by the TUI). Non-interactive runners leave it nil so the
	// model is never offered a tool it cannot get answered — the isEnabled()
	// analog from the reference.
	if userAsker != nil {
		registry.Register(tools.NewAskUserQuestionTool(userAsker))
	}
	return permission.DecorateRegistry(registry, permissions)
}

func (r *AgentRunner) buildPlanLifecycle(sess *session.Session, store *memory.Store, defaultRegistry tools.Registry) *planLifecycle {
	r.mu.Lock()
	userAsker := r.userAsker
	planReviewer := r.planReviewer
	r.mu.Unlock()

	checklistRegistry := tools.NewRegistry()
	checklistRegistry.Register(tools.NewReadFileTool(r.workDir))
	checklistRegistry.Register(tools.NewBashTool(r.workDir))
	checklistRegistry.Register(tools.NewAskUserQuestionTool(userAsker))
	checklistRegistry.Register(tools.NewReadTodoTool(sess.RootDir))
	checklistRegistry.Register(tools.NewUpdateTodoTool(sess.RootDir))
	if r.permissionCoordinator != nil {
		checklistRegistry = permission.DecorateRegistry(checklistRegistry, r.permissionCoordinator)
	}

	lifecycle := newPlanLifecycle(nil, checklistRegistry, defaultRegistry, func() {
		r.SetCollaborationMode(collaboration.ModeDefault)
	})
	formalRegistry := tools.NewRegistry()
	formalRegistry.Register(tools.NewReadFileTool(r.workDir))
	formalRegistry.Register(tools.NewBashTool(r.workDir))
	formalRegistry.Register(tools.NewAskUserQuestionTool(userAsker))
	formalRegistry.Register(tools.NewSubmitPlanTool(store, planReviewer, lifecycle.approve))
	if r.permissionCoordinator != nil {
		formalRegistry = permission.DecorateRegistry(formalRegistry, r.permissionCoordinator)
	}
	lifecycle.setFormalRegistry(formalRegistry)
	return lifecycle
}

// PermissionSnapshot exposes the TUI permission state.
func (r *AgentRunner) PermissionSnapshot() permission.Snapshot {
	r.mu.Lock()
	coordinator := r.permissionCoordinator
	r.mu.Unlock()
	if coordinator == nil {
		return permission.NewState(permission.ModeAsk, false).Snapshot()
	}
	return coordinator.State().Snapshot()
}

// SetPermissionMode updates the process-local selected and effective mode.
func (r *AgentRunner) SetPermissionMode(mode permission.Mode, remembered bool) {
	r.mu.Lock()
	coordinator := r.permissionCoordinator
	r.mu.Unlock()
	if coordinator != nil {
		coordinator.State().SetSelected(mode, remembered)
	}
}

// ActivateFullAccess enables Full Access for the current TUI process.
func (r *AgentRunner) ActivateFullAccess(remember bool) {
	r.mu.Lock()
	coordinator := r.permissionCoordinator
	r.mu.Unlock()
	if coordinator != nil {
		coordinator.State().ActivateFullAccess(remember)
	}
}

// ClearPermissionGrants clears process-local session grants.
func (r *AgentRunner) ClearPermissionGrants() int {
	r.mu.Lock()
	coordinator := r.permissionCoordinator
	r.mu.Unlock()
	if coordinator == nil {
		return 0
	}
	return coordinator.State().ClearGrants()
}

// SetUserAsker installs the interactive asker used by the ask_user_question
// tool. The TUI calls this before running prompts; leaving it unset (nil) keeps
// the tool out of the registry for non-interactive runs.
func (r *AgentRunner) SetUserAsker(asker tools.UserAsker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userAsker = asker
}

// SetPlanReviewer installs the interactive reviewer used by submit_plan.
func (r *AgentRunner) SetPlanReviewer(reviewer tools.PlanReviewer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.planReviewer = reviewer
}

func checkpointDisabledFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FOXHARNESS_DISABLE_FILE_CHECKPOINTING"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// projectedMessages reconstructs the message list the engine would see after
// applying the persisted CompactState. Messages covered by the compaction are
// replaced with the stored summary, matching the projection in
// engine.projectedContext.
func projectedMessages(state *session.CompactState, records []session.MessageRecord) []schema.Message {
	coveredUntil := int64(-1)
	hasSummary := false
	if state != nil && state.Summary != "" {
		coveredUntil = state.CoveredUntilSeq
		hasSummary = true
	}

	var active []session.MessageRecord
	for _, rec := range records {
		if rec.Seq > coveredUntil {
			active = append(active, rec)
		}
	}

	messages := make([]schema.Message, 0, len(active)+1)
	if hasSummary {
		messages = append(messages, schema.Message{
			Role:    schema.RoleUser,
			Content: state.Summary,
		})
	}
	for _, rec := range active {
		messages = append(messages, rec.Message)
	}
	return messages
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
