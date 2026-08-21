package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
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
	EffortOverride  string
	EnableThinking  bool
	MaxTurns        int
	SessionID       string
	ContinueSession bool
	NewSession      bool
	OnModelChange   func(model string) error
	Permission      *permission.Coordinator
	RuntimeProfile  ChildParentProfile
	NewChildRunner  ChildRunnerFactory
	// ExtractionContext optionally binds asynchronous post-run extraction to
	// an external lifecycle owner. Ordinary profiles leave it nil.
	ExtractionContext context.Context
}

// AgentRunner owns one long-lived session and can execute many user prompts
// as separate runs inside that session.
type AgentRunner struct {
	mu    sync.Mutex
	runMu sync.Mutex

	workDir          string
	model            string
	providerProtocol string
	runtimeProfile   ChildParentProfile
	newChildRunner   ChildRunnerFactory
	llmConfig        llmconfig.ResolvedConfig

	enableThinking    bool
	effortOverride    string
	collaborationMode collaboration.Mode
	maxTurns          int

	onModelChange func(model string) error

	store          *memory.Store
	autoMemory     *automemory.Store
	manager        *session.FileStore
	llmProvider    provider.LLMProvider
	currentSession *session.StoredSession

	// extractionFire overrides the default post-run memory extraction launcher.
	// It is nil in production (which uses automemory.PerRunHooks.Fire); tests set
	// it to observe the hook synchronously.
	extractionFire func(sess *session.StoredSession, runID string, tracker *automemory.Tracker)
	checkpointer   checkpoint.Checkpointer
	slashRegistry  *slash.Registry
	slashExecutor  *slash.Executor

	userAsker              tools.UserAsker
	planReviewer           tools.PlanReviewer
	permissionCoordinator  *permission.Coordinator
	permissionInstructions []string

	pendingMu          sync.Mutex
	pendingActivations []string

	contextUsedTokens   int64
	contextWindowTokens int64

	// extractWG tracks in-flight post-run memory extraction goroutines so
	// short-lived or item-scoped owners can join them before cleanup.
	extractWG     sync.WaitGroup
	extractMu     sync.Mutex
	extractCtx    context.Context
	extractCancel context.CancelFunc
}

/* ChildRunnerConfig contains the legacy-parent snapshot supplied to composition. */
type ChildRunnerConfig struct {
	Provider         provider.LLMProvider
	WorkDir          string
	ParentProfile    ChildParentProfile
	ProviderProtocol string
	Model            string
	Effort           string
	Permission       *permission.Coordinator
	ParentEvidence   permission.EvidenceProvider
}

/* ChildRunnerFactory maps legacy parent state to the consumer-owned subagent port. */
type ChildRunnerFactory func(ChildRunnerConfig) subagent.Runner

/* ChildParentProfile identifies the legacy parent behavior bundle at composition time. */
type ChildParentProfile string

const (
	childParentCLI     ChildParentProfile = "CLIExec"
	childParentTUI     ChildParentProfile = "TUIInteractive"
	childParentAutodev ChildParentProfile = "AutodevPipeline"
)

func agentRunnerConfigFromCLI(cfg CLIConfig) AgentRunnerConfig {
	return AgentRunnerConfig{
		WorkDir:         cfg.WorkDir,
		Model:           cfg.Model,
		LLM:             cfg.ResolvedLLM,
		EffortOverride:  cfg.EffortOverride,
		EnableThinking:  cfg.EnableThinking,
		MaxTurns:        cfg.MaxTurns,
		SessionID:       cfg.SessionID,
		ContinueSession: cfg.ContinueSession,
		NewSession:      cfg.NewSession,
		NewChildRunner:  cfg.NewChildRunner,
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

	manager := session.NewFileStore(workDir)
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
	if cfg.RuntimeProfile == "" {
		cfg.RuntimeProfile = childParentCLI
	}

	slashRegistry := slash.NewRegistry(workDir)
	if err := slashRegistry.Load(); err != nil {
		log.Printf("[slash] registry load failed: %v", err)
	}

	extractParent := cfg.ExtractionContext
	if extractParent == nil {
		extractParent = context.Background()
	}
	extractCtx, extractCancel := context.WithCancel(extractParent)
	ar := &AgentRunner{
		workDir:                workDir,
		model:                  cfg.LLM.Model,
		providerProtocol:       providerProtocol,
		runtimeProfile:         cfg.RuntimeProfile,
		newChildRunner:         cfg.NewChildRunner,
		llmConfig:              cfg.LLM,
		enableThinking:         cfg.EnableThinking,
		effortOverride:         cfg.EffortOverride,
		collaborationMode:      collaboration.ModeDefault,
		maxTurns:               cfg.MaxTurns,
		onModelChange:          cfg.OnModelChange,
		permissionCoordinator:  cfg.Permission,
		permissionInstructions: snapshotPermissionInstructions(workDir),
		store:                  store,
		autoMemory:             autoMem,
		manager:                manager,
		llmProvider:            llmProvider,
		currentSession:         sess,
		checkpointer:           cp,
		slashRegistry:          slashRegistry,
		extractCtx:             extractCtx,
		extractCancel:          extractCancel,
	}
	ar.slashExecutor = slash.NewExecutor(
		slash.WithWorkDir(workDir),
		slash.WithForkRunner(&subagentForkRunner{
			getRunner:  ar.currentSubagentRunner,
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
	return skilltool.FormatActivationReminder(cmd)
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

// currentSubagentRunner returns a freshly-built target ChildRun adapter bound to
// the runner's current LLM provider. Built per-call so a /model switch is
// immediately reflected in fork-mode skills without rebuilding the
// executor or fork runner.
func (r *AgentRunner) currentSubagentRunner() subagent.Runner {
	r.mu.Lock()
	p := r.llmProvider
	wd := r.workDir
	permissions := r.permissionCoordinator
	sess := r.currentSession
	profile := r.runtimeProfile
	protocol := r.providerProtocol
	model := r.model
	effort := r.effortOverride
	newChildRunner := r.newChildRunner
	r.mu.Unlock()
	if newChildRunner == nil {
		return nil
	}
	return newChildRunner(ChildRunnerConfig{
		Provider: p, WorkDir: wd, ParentProfile: profile, ProviderProtocol: protocol,
		Model: model, Effort: effort, Permission: permissions,
		ParentEvidence: r.permissionEvidenceProvider(sess, ""),
	})
}

func (r *AgentRunner) permissionEvidenceProvider(sess *session.StoredSession, currentPrompt string, trustCurrentPrompt ...bool) permission.EvidenceProvider {
	trustPrompt := true
	if len(trustCurrentPrompt) > 0 {
		trustPrompt = trustCurrentPrompt[0]
	}
	instructions := append([]string(nil), r.permissionInstructions...)
	return func(request permission.Request) permission.Evidence {
		var messages []schema.Message
		if sess != nil {
			records, err := session.NewMessageLog(sess).LoadRecords()
			if err == nil {
				messages = make([]schema.Message, 0, len(records))
				for _, record := range records {
					message := record.Message
					generatedDisplay := strings.TrimSpace(record.DisplayContent) != "" && record.DisplayContent != message.Content
					if generatedDisplay {
						messages = append(messages, schema.Message{Role: schema.RoleUser, Content: record.DisplayContent})
					}
					if message.Role == schema.RoleUser && message.ToolCallID == "" && (record.IsMeta || record.IsCompactSummary || record.IsVisibleInTranscriptOnly || generatedDisplay) {
						message.Role = schema.RoleSystem
					}
					messages = append(messages, message)
				}
			}
		}
		if trustPrompt && strings.TrimSpace(currentPrompt) != "" && !containsDirectUserMessage(messages, currentPrompt) {
			messages = append(messages, schema.Message{Role: schema.RoleUser, Content: currentPrompt})
		}
		return permission.BuildEvidence(messages, instructions, request)
	}
}

func snapshotPermissionInstructions(workDir string) []string {
	content, err := os.ReadFile(filepath.Join(workDir, "AGENTS.md"))
	if err != nil || len(content) == 0 {
		return nil
	}
	return []string{string(content)}
}

func containsDirectUserMessage(messages []schema.Message, content string) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role == schema.RoleUser && message.ToolCallID == "" && message.Content == content {
			return true
		}
	}
	return false
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
	return string(r.currentSession.ID)
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
	return r.runInternal(ctx, userPrompt, "", nil, nil, "", reporter)
}

// RunInCollaborationMode executes one prompt in the mode captured when the
// user submitted it without changing the mode selected for later submissions.
func (r *AgentRunner) RunInCollaborationMode(ctx context.Context, userPrompt string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, "", nil, collaborationModeOverride(mode), "", reporter)
}

// RunWithDisplay executes one prompt while storing a separate human-facing
// prompt for transcript and history views.
func (r *AgentRunner) RunWithDisplay(ctx context.Context, userPrompt string, displayPrompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, displayPrompt, nil, nil, "", reporter)
}

// RunWithDisplayInCollaborationMode is RunWithDisplay with an immutable mode
// captured at user submission time.
func (r *AgentRunner) RunWithDisplayInCollaborationMode(ctx context.Context, userPrompt string, displayPrompt string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, displayPrompt, nil, collaborationModeOverride(mode), "", reporter)
}

// RunWithDisplayAndEffortInCollaborationMode executes one prompt with a
// prompt-command effort override for this run only.
func (r *AgentRunner) RunWithDisplayAndEffortInCollaborationMode(ctx context.Context, userPrompt string, displayPrompt string, effort string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, displayPrompt, nil, collaborationModeOverride(mode), effort, reporter)
}

// RunRestricted executes one prompt with the tool registry filtered down
// to allowedTools. Calls from prompt commands that declare an
// `allowed-tools` frontmatter use this path so the per-turn restriction
// is enforced at the registry level (NFR-002), not just advisory.
//
// allowedTools must be non-empty; pass nil/empty to Run instead.
func (r *AgentRunner) RunRestricted(ctx context.Context, userPrompt string, allowedTools []string, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, "", allowedTools, nil, "", reporter)
}

// RunRestrictedInCollaborationMode is RunRestricted with an immutable mode
// captured at user submission time.
func (r *AgentRunner) RunRestrictedInCollaborationMode(ctx context.Context, userPrompt string, allowedTools []string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, "", allowedTools, collaborationModeOverride(mode), "", reporter)
}

// RunRestrictedWithDisplay is the restricted variant of RunWithDisplay.
func (r *AgentRunner) RunRestrictedWithDisplay(ctx context.Context, userPrompt string, displayPrompt string, allowedTools []string, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, displayPrompt, allowedTools, nil, "", reporter)
}

// RunRestrictedWithDisplayInCollaborationMode combines display text, a tool
// allow-list, and the immutable mode captured at user submission time.
func (r *AgentRunner) RunRestrictedWithDisplayInCollaborationMode(ctx context.Context, userPrompt string, displayPrompt string, allowedTools []string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, displayPrompt, allowedTools, collaborationModeOverride(mode), "", reporter)
}

// RunRestrictedWithDisplayAndEffortInCollaborationMode combines display text,
// tool restrictions, immutable collaboration mode, and a prompt-command effort
// override for this run only.
func (r *AgentRunner) RunRestrictedWithDisplayAndEffortInCollaborationMode(ctx context.Context, userPrompt string, displayPrompt string, allowedTools []string, effort string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInternal(ctx, userPrompt, displayPrompt, allowedTools, collaborationModeOverride(mode), effort, reporter)
}

func collaborationModeOverride(mode collaboration.Mode) *collaboration.Mode {
	normalized := collaboration.Normalize(mode)
	return &normalized
}

func (r *AgentRunner) runInternal(ctx context.Context, userPrompt string, displayPrompt string, allowedTools []string, modeOverride *collaboration.Mode, runEffortOverride string, reporter engine.Reporter) (*engine.RunResult, error) {
	r.runMu.Lock()
	defer r.runMu.Unlock()

	r.mu.Lock()
	sess := r.currentSession
	store := r.store
	autoMem := r.autoMemory
	enableThinking := r.enableThinking
	effortOverride := r.effortOverride
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
	if strings.TrimSpace(runEffortOverride) != "" {
		effortOverride = strings.TrimSpace(runEffortOverride)
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
		WithMemory(store.WorkingMemoryPath())
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

	evidenceProvider := r.permissionEvidenceProvider(sess, userPrompt, displayPrompt == "" || displayPrompt == userPrompt)
	toolRegistry := r.buildRegistry(sess, llmProvider, cp, getCurrentMessageID, evidenceProvider)
	var planRun *planLifecycle
	if collaborationMode == collaboration.ModeFormalPlan {
		planRun = r.buildPlanLifecycle(sess, store, toolRegistry, evidenceProvider)
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

	eng := engine.NewLegacyEngine(
		llmProvider,
		toolRegistry,
		r.workDir,
		composer,
		engine.Config{
			EnableThinking:    enableThinking,
			EffortOverride:    effortOverride,
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
				// Tracked launch lets one-shot and item-scoped callers drain before
				// process or worktree cleanup; long-lived profiles need not wait.
				hooks.FireTrackedContext(r.extractionContext(), &r.extractWG, sess, result.RunID, tracker)
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
	_ = r.DrainExtraction(context.Background())
}

// DrainExtraction waits for every post-run extraction launched before the
// call. It serializes with Run so no later run can race the join boundary.
func (r *AgentRunner) DrainExtraction(ctx context.Context) error {
	r.runMu.Lock()
	defer r.runMu.Unlock()
	return r.waitForExtraction(ctx)
}

// CloseExtraction cancels the runner-owned extraction lifecycle and joins all
// launched work before returning. Calls are idempotent.
func (r *AgentRunner) CloseExtraction(ctx context.Context) error {
	r.runMu.Lock()
	defer r.runMu.Unlock()
	r.extractMu.Lock()
	if r.extractCancel != nil {
		r.extractCancel()
	}
	r.extractMu.Unlock()
	return r.waitForExtraction(ctx)
}

func (r *AgentRunner) extractionContext() context.Context {
	r.extractMu.Lock()
	defer r.extractMu.Unlock()
	if r.extractCtx == nil {
		r.extractCtx, r.extractCancel = context.WithCancel(context.Background())
	}
	return r.extractCtx
}

func (r *AgentRunner) waitForExtraction(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		r.extractWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
	return string(sess.ID), nil
}

func (r *AgentRunner) SessionID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.currentSession.ID)
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
				sessionID: string(sess.ID),
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
	state, err := session.LoadCompactState(sess)
	if err != nil {
		return err
	}
	if state.Summary != "" && seq <= state.CoveredUntilSeq {
		if err := session.SaveCompactState(sess, &session.CompactState{CoveredUntilSeq: -1}); err != nil {
			return err
		}
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

// SetEffortOverride updates the session-level effort override used for later
// user-run model calls. An empty value clears the override.
func (r *AgentRunner) SetEffortOverride(value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.effortOverride = strings.TrimSpace(value)
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

// subagentForkRunner implements slash.ForkRunner by delegating to a Runner
// built on demand. Both the runner and parent session ID are read through
// getters so /new and /model changes take effect immediately. The selected
// agent is resolved before child-session creation, so unsupported fork
// metadata cannot silently fall back.
type subagentForkRunner struct {
	getRunner  func() subagent.Runner
	getSession func() string
}

func (s *subagentForkRunner) PermissionEnforced() bool {
	runner := s.getRunner()
	return runner != nil && runner.PermissionEnforced()
}

func (s *subagentForkRunner) Run(ctx context.Context, task string, agentType string, allowedTools []string) (string, error) {
	runner := s.getRunner()
	if runner == nil {
		return "", fmt.Errorf("fork runner: subagent runner unavailable")
	}
	invocation, _ := tools.InvocationContextFrom(ctx)
	res, err := runner.Run(ctx, subagent.Request{
		ParentSessionID: s.getSession(),
		ParentRunID:     invocation.RunID,
		DelegationID:    invocation.ToolCallID,
		Task:            task,
		ReadOnly:        false,
		Agent:           subagent.AgentID(agentType),
		Depth:           1,
		AllowedTools:    allowedTools,
	})
	if err != nil {
		if res == nil {
			return "", err
		}
		return subagent.FormatFailureOutcome(res, err), &subagent.OutcomeError{Outcome: res, Err: err}
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

func (r *AgentRunner) buildRegistry(sess *session.StoredSession, llmProvider provider.LLMProvider, cp checkpoint.Checkpointer, getMessageID func() string, evidenceProviders ...permission.EvidenceProvider) tools.Registry {
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

	var evidenceProvider permission.EvidenceProvider
	if len(evidenceProviders) > 0 {
		evidenceProvider = evidenceProviders[0]
	}
	r.mu.Lock()
	profile := r.runtimeProfile
	protocol := r.providerProtocol
	model := r.model
	effort := r.effortOverride
	newChildRunner := r.newChildRunner
	r.mu.Unlock()
	var childRunner subagent.Runner
	if newChildRunner != nil {
		childRunner = newChildRunner(ChildRunnerConfig{
			Provider: llmProvider, WorkDir: r.workDir, ParentProfile: profile,
			ProviderProtocol: protocol, Model: model, Effort: effort,
			Permission: permissions, ParentEvidence: evidenceProvider,
		})
	}
	registry.Register(subagent.NewTool(childRunner, string(sess.ID)))

	r.mu.Lock()
	slashReg := r.slashRegistry
	slashExec := r.slashExecutor
	userAsker := r.userAsker
	r.mu.Unlock()
	if slashReg != nil && slashExec != nil {
		registry.Register(skilltool.NewSkillTool(slashReg, slashExec, func() string { return string(sess.ID) }))
	}
	// The ask_user_question tool is only registered when an interactive asker is
	// available (set by the TUI). Non-interactive runners leave it nil so the
	// model is never offered a tool it cannot get answered — the isEnabled()
	// analog from the reference.
	if userAsker != nil {
		registry.Register(tools.NewAskUserQuestionTool(userAsker))
	}
	return permission.DecorateRegistry(registry, permissions, evidenceProviders...)
}

func (r *AgentRunner) buildPlanLifecycle(sess *session.StoredSession, store *memory.Store, defaultRegistry tools.Registry, evidenceProviders ...permission.EvidenceProvider) *planLifecycle {
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
		checklistRegistry = permission.DecorateRegistry(checklistRegistry, r.permissionCoordinator, evidenceProviders...)
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
		formalRegistry = permission.DecorateRegistry(formalRegistry, r.permissionCoordinator, evidenceProviders...)
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

/* LegacyInteractiveApplication maps the temporary AgentRunner facade to application-owned TUI capabilities. */
type LegacyInteractiveApplication struct {
	runner *AgentRunner
}

/* NewLegacyInteractiveApplication creates the M21 compatibility adapter for the unmigrated TUI entry. */
func NewLegacyInteractiveApplication(runner *AgentRunner) *LegacyInteractiveApplication {
	return &LegacyInteractiveApplication{runner: runner}
}

/* Run executes one interactive command through the legacy runner and maps observations into application notifications. */
func (a *LegacyInteractiveApplication) Run(ctx context.Context, command RunCommand, sink NotificationSink) (*RunOutcome, error) {
	mode := collaboration.Normalize(collaboration.Mode(command.CollaborationMode))
	reporter := newLegacyNotificationReporter(sink)
	result, err := a.runner.runInternal(
		ctx,
		command.Prompt,
		command.DisplayPrompt,
		command.AllowedTools,
		collaborationModeOverride(mode),
		command.Effort,
		reporter,
	)
	if result == nil {
		return nil, err
	}
	return mapLegacyRunResult(result), err
}

/* State returns a presentation-safe snapshot of the current legacy session. */
func (a *LegacyInteractiveApplication) State() InteractiveSessionState {
	a.runner.mu.Lock()
	sess := a.runner.currentSession
	state := InteractiveSessionState{
		WorkDir: a.runner.workDir, Model: a.runner.model, Effort: a.runner.effortOverride,
		CollaborationMode: string(collaboration.Normalize(a.runner.collaborationMode)),
		RewindAvailable:   a.runner.checkpointer != nil,
		RunCapabilities:   RunCapabilities{ToolRestrictions: true, EffortOverrides: true},
	}
	if sess != nil {
		state.Session = SessionInfo{
			ID: string(sess.ID), Directory: sess.RootDir, TranscriptPath: sess.TranscriptPath(),
		}
	}
	a.runner.mu.Unlock()
	state.ContextUsage = a.runner.ContextUsage()
	state.AutoMemoryIndex = a.runner.AutoMemoryIndex()
	return state
}

/* Conversation loads the current persisted conversation as application-owned values. */
func (a *LegacyInteractiveApplication) Conversation(context.Context) ([]ConversationRecord, error) {
	records, err := a.runner.MessageHistory()
	if err != nil {
		return nil, err
	}
	return mapLegacyConversation(records), nil
}

/* ProjectInputHistory loads recent project prompts using the legacy compatibility ordering. */
func (a *LegacyInteractiveApplication) ProjectInputHistory(_ context.Context, limit int) ([]string, error) {
	return a.runner.ProjectInputHistory(limit)
}

/* RewindTargets loads user-authored targets and their checkpoint diff summaries. */
func (a *LegacyInteractiveApplication) RewindTargets(context.Context) ([]RewindTarget, error) {
	records, err := a.runner.MessageHistory()
	if err != nil {
		return nil, err
	}
	messages := checkpoint.SelectableMessages(records)
	a.runner.mu.Lock()
	cp := a.runner.checkpointer
	a.runner.mu.Unlock()
	targets := make([]RewindTarget, 0, len(messages))
	for _, message := range messages {
		target := RewindTarget{
			Sequence: message.Seq, Content: message.Content, Timestamp: message.Timestamp,
			IsCurrent: message.IsCurrent,
		}
		if cp != nil {
			stats, statsErr := cp.GetDiffStats(messageID(message.Seq))
			if statsErr != nil {
				target.DiffError = statsErr.Error()
			} else if stats != nil {
				target.Diff = RewindDiff{
					FilesChanged: stats.FilesChanged, Insertions: stats.Insertions, Deletions: stats.Deletions,
					ChangedFiles: append([]string(nil), stats.ChangedFiles...),
				}
			}
		}
		targets = append(targets, target)
	}
	return targets, nil
}

/* NewSession switches to a fresh CLI-source session and returns its state snapshot. */
func (a *LegacyInteractiveApplication) NewSession(ctx context.Context, _ NewSessionCommand) (InteractiveSessionState, error) {
	if _, err := a.runner.NewSession(ctx); err != nil {
		return InteractiveSessionState{}, err
	}
	return a.State(), nil
}

/* UpdateModel changes the model used by future runs and returns the resulting state. */
func (a *LegacyInteractiveApplication) UpdateModel(_ context.Context, command ModelCommand) (InteractiveSessionState, error) {
	if err := a.runner.SetModel(command.Model); err != nil {
		return InteractiveSessionState{}, err
	}
	return a.State(), nil
}

/* UpdateEffort changes the effort used by future runs and returns the resulting state. */
func (a *LegacyInteractiveApplication) UpdateEffort(_ context.Context, command EffortCommand) InteractiveSessionState {
	a.runner.SetEffortOverride(command.Effort)
	return a.State()
}

/* UpdateCollaborationMode changes the selected mode for future runs and returns the resulting state. */
func (a *LegacyInteractiveApplication) UpdateCollaborationMode(_ context.Context, command CollaborationCommand) InteractiveSessionState {
	a.runner.SetCollaborationMode(collaboration.Mode(command.Mode))
	return a.State()
}

/* Compact runs legacy manual compaction and maps its stable statistics. */
func (a *LegacyInteractiveApplication) Compact(ctx context.Context, command CompactCommand) (CompactOutcome, error) {
	result, err := a.runner.CompactNow(ctx, command.Instructions)
	if result == nil {
		return CompactOutcome{}, err
	}
	return CompactOutcome{
		PreTokens: result.PreTokens, PostTokens: result.PostTokens,
		MessagesSummarized: result.MessagesSummarized,
	}, err
}

/* Rewind preserves the legacy code, conversation, and session-state restore ordering. */
func (a *LegacyInteractiveApplication) Rewind(_ context.Context, command RewindCommand) RewindOutcome {
	outcome := RewindOutcome{}
	records, err := a.runner.MessageHistory()
	if err != nil {
		outcome.Error = err.Error()
		return outcome
	}
	content := legacyMessageContentBySequence(records, command.Sequence)

	if command.Action == RewindBoth || command.Action == RewindCode {
		a.runner.mu.Lock()
		cp := a.runner.checkpointer
		a.runner.mu.Unlock()
		if cp != nil {
			outcome.CodeAttempted = true
			files, rewindErr := cp.Rewind(messageID(command.Sequence))
			if rewindErr != nil {
				outcome.CodeError = rewindErr.Error()
				if command.Action == RewindCode {
					return outcome
				}
			} else {
				outcome.CodeFiles = append([]string(nil), files...)
			}
		}
	}

	if command.Action != RewindBoth && command.Action != RewindConversation {
		return outcome
	}
	outcome.ConversationAttempted = true
	if err := a.runner.TruncateMessageHistory(command.Sequence); err != nil {
		outcome.ConversationError = err.Error()
		return outcome
	}
	records, err = a.runner.MessageHistory()
	if err != nil {
		outcome.ConversationError = err.Error()
		return outcome
	}
	outcome.Conversation = mapLegacyConversation(records)
	outcome.RestoredInput = content
	outcome.SessionStateAttempted = true
	restored, err := a.runner.RestoreSessionStateBeforeMessage(command.Sequence)
	if err != nil {
		outcome.SessionStateError = err.Error()
		return outcome
	}
	outcome.SessionStateRestored = restored
	return outcome
}

/* RestoreLatestInput restores the latest cancellable user input when only synthetic records follow it. */
func (a *LegacyInteractiveApplication) RestoreLatestInput(context.Context) (RestoreInputOutcome, error) {
	records, err := a.runner.MessageHistory()
	if err != nil {
		return RestoreInputOutcome{}, nil
	}
	index := -1
	var target checkpoint.SelectableMessage
	for candidate := len(records) - 1; candidate >= 0; candidate-- {
		messages := checkpoint.SelectableMessages(records[candidate : candidate+1])
		if len(messages) == 0 {
			continue
		}
		index = candidate
		target = messages[0]
		break
	}
	if index < 0 || !checkpoint.MessagesAfterAreOnlySynthetic(records, index) {
		return RestoreInputOutcome{}, nil
	}
	outcome := RestoreInputOutcome{Attempted: true}
	if err := a.runner.TruncateMessageHistory(target.Seq); err != nil {
		return outcome, err
	}
	records, err = a.runner.MessageHistory()
	if err != nil {
		return outcome, err
	}
	return RestoreInputOutcome{
		Attempted: true, Restored: true, Conversation: mapLegacyConversation(records), Input: target.Content,
	}, nil
}

/* PermissionState returns the application-owned interactive permission snapshot. */
func (a *LegacyInteractiveApplication) PermissionState() PermissionState {
	return mapLegacyPermissionState(a.runner.PermissionSnapshot())
}

/* UpdatePermissionMode changes the selected permission mode and returns its resulting state. */
func (a *LegacyInteractiveApplication) UpdatePermissionMode(_ context.Context, command PermissionModeCommand) PermissionState {
	a.runner.SetPermissionMode(permission.Mode(command.Mode), command.FullAccessWarningRemembered)
	return a.PermissionState()
}

/* ActivateFullAccess confirms Full Access and returns its resulting state. */
func (a *LegacyInteractiveApplication) ActivateFullAccess(_ context.Context, command FullAccessCommand) PermissionState {
	a.runner.ActivateFullAccess(command.Remember)
	return a.PermissionState()
}

/* ClearPermissionGrants removes session grants and returns the resulting state. */
func (a *LegacyInteractiveApplication) ClearPermissionGrants(context.Context) PermissionGrantClearOutcome {
	cleared := a.runner.ClearPermissionGrants()
	return PermissionGrantClearOutcome{Cleared: cleared, State: a.PermissionState()}
}

func mapLegacyPermissionState(snapshot permission.Snapshot) PermissionState {
	return PermissionState{
		SelectedMode: PermissionMode(snapshot.SelectedMode), EffectiveMode: PermissionMode(snapshot.EffectiveMode),
		FullAccessRemembered:   snapshot.FullAccessRemembered,
		FullAccessNeedsWarning: snapshot.FullAccessNeedsWarning,
		SessionGrantCount:      snapshot.SessionGrantCount,
	}
}

func mapLegacyConversation(records []session.MessageRecord) []ConversationRecord {
	result := make([]ConversationRecord, len(records))
	for index, record := range records {
		calls := make([]ConversationToolCall, len(record.Message.ToolCalls))
		for callIndex, call := range record.Message.ToolCalls {
			calls[callIndex] = ConversationToolCall{ID: call.ID, Name: call.Name, Arguments: string(call.Arguments)}
		}
		result[index] = ConversationRecord{
			Sequence: record.Seq, Time: record.Time, Role: string(record.Message.Role),
			Content: record.Message.Content, DisplayContent: record.DisplayContent,
			ToolCallID: record.Message.ToolCallID, ToolCalls: calls,
			IsMeta: record.IsMeta, IsCompactSummary: record.IsCompactSummary,
			IsVisibleInTranscriptOnly: record.IsVisibleInTranscriptOnly,
		}
	}
	return result
}

func mapLegacyRunResult(result *engine.RunResult) *RunOutcome {
	var warnings []Warning
	if result.TelemetryWarnings != nil {
		warnings = make([]Warning, len(result.TelemetryWarnings))
		for index, warning := range result.TelemetryWarnings {
			warnings[index] = Warning{Sink: warning.Sink, Operation: warning.Operation, Error: warning.Error}
		}
	}
	return &RunOutcome{
		SessionID: result.SessionID, RunID: result.RunID, FinalMessage: result.FinalMessage,
		MetricsPath: result.MetricsPath, TracePath: result.TracePath, Warnings: warnings,
	}
}

func legacyMessageContentBySequence(records []session.MessageRecord, sequence int64) string {
	for _, record := range records {
		if record.Seq == sequence {
			return strings.TrimSpace(record.HumanContent())
		}
	}
	return ""
}

func messageID(sequence int64) string {
	return strconv.FormatInt(sequence, 10)
}

type legacyNotificationReporter struct {
	sink      NotificationSink
	mu        sync.Mutex
	sequence  int
	sessionID string
	runID     string
}

func newLegacyNotificationReporter(sink NotificationSink) engine.Reporter {
	if isNilNotificationSink(sink) {
		return nil
	}
	return &legacyNotificationReporter{sink: sink}
}

func (r *legacyNotificationReporter) OnRunStart(ctx context.Context, sessionID string, runID string) {
	r.mu.Lock()
	r.sessionID, r.runID = sessionID, runID
	r.mu.Unlock()
	r.notify(ctx, Notification{Kind: NotificationRunStarted, SessionID: sessionID, RunID: runID})
}

func (r *legacyNotificationReporter) OnThinking(ctx context.Context, turn int) {
	r.notify(ctx, Notification{Kind: NotificationThinking, Turn: turn})
}

func (r *legacyNotificationReporter) OnCompaction(ctx context.Context, scope string) {
	r.notify(ctx, Notification{Kind: NotificationContextCompacted, Phase: scope})
}

func (r *legacyNotificationReporter) OnToolCall(ctx context.Context, name string, arguments string) {
	r.notify(ctx, Notification{Kind: NotificationToolCall, Name: name, Content: arguments})
}

func (r *legacyNotificationReporter) OnToolResult(ctx context.Context, name string, result string, isError bool) {
	r.notify(ctx, Notification{Kind: NotificationToolResult, Name: name, Content: result, IsError: isError})
}

func (r *legacyNotificationReporter) OnMessage(ctx context.Context, content string) {
	r.notify(ctx, Notification{Kind: NotificationMessage, Content: content})
}

func (r *legacyNotificationReporter) OnMessageDelta(ctx context.Context, content string) {
	r.notify(ctx, Notification{Kind: NotificationMessageDelta, Content: content})
}

func (r *legacyNotificationReporter) OnRunComplete(ctx context.Context, result engine.RunResult) {
	r.notify(ctx, Notification{Kind: NotificationRunCompleted, RunID: result.RunID})
}

func (r *legacyNotificationReporter) OnRunError(ctx context.Context, sessionID string, runID string, err error) {
	content := ""
	if err != nil {
		content = err.Error()
	}
	r.notify(ctx, Notification{Kind: NotificationRunError, SessionID: sessionID, RunID: runID, Content: content, IsError: true})
}

func (r *legacyNotificationReporter) notify(ctx context.Context, notification Notification) {
	r.mu.Lock()
	r.sequence++
	notification.Sequence = r.sequence
	if notification.SessionID == "" {
		notification.SessionID = r.sessionID
	}
	if notification.RunID == "" {
		notification.RunID = r.runID
	}
	r.mu.Unlock()
	r.sink.Notify(ctx, notification)
}

var _ InteractiveApplication = (*LegacyInteractiveApplication)(nil)
var _ engine.MessageDeltaReporter = (*legacyNotificationReporter)(nil)

type legacyQuestionAsker struct {
	port QuestionPort
}

func newLegacyQuestionAsker(port QuestionPort) tools.UserAsker {
	if isNilLegacyInteraction(port) {
		return nil
	}
	return &legacyQuestionAsker{port: port}
}

func (a *legacyQuestionAsker) Ask(ctx context.Context, questions []tools.Question) ([]tools.Answer, error) {
	correlation := legacyInteractionCorrelation(ctx, "question", "")
	request := QuestionRequest{Correlation: correlation, Questions: make([]Question, len(questions))}
	for index, question := range questions {
		options := make([]QuestionOption, len(question.Options))
		for optionIndex, option := range question.Options {
			options[optionIndex] = QuestionOption{
				Label: option.Label, Description: option.Description, Preview: option.Preview,
			}
		}
		request.Questions[index] = Question{
			ID: fmt.Sprintf("%s:question:%d", correlation.ID, index+1), Header: question.Header,
			Prompt: question.Prompt, Options: options, MultiSelect: question.MultiSelect,
		}
	}
	response, err := a.port.AskQuestions(ctx, request)
	if err != nil {
		if errors.Is(err, ErrQuestionCancelled) {
			return nil, tools.ErrUserCancelled
		}
		return nil, err
	}
	if err := validateInteractionResponse(correlation.ID, response.CorrelationID); err != nil {
		return nil, err
	}
	answers := make([]tools.Answer, len(response.Answers))
	for index, answer := range response.Answers {
		answers[index] = tools.Answer{
			QuestionText: answer.QuestionText, Value: answer.Value, Preview: answer.Preview, Notes: answer.Notes,
		}
	}
	return answers, nil
}

type legacyPlanReviewer struct {
	port PlanReviewPort
}

func newLegacyPlanReviewer(port PlanReviewPort) tools.PlanReviewer {
	if isNilLegacyInteraction(port) {
		return nil
	}
	return &legacyPlanReviewer{port: port}
}

func (r *legacyPlanReviewer) ReviewPlan(ctx context.Context, planMarkdown string) (tools.PlanReview, error) {
	correlation := legacyInteractionCorrelation(ctx, "plan", "")
	response, err := r.port.ReviewPlan(ctx, PlanReviewRequest{Correlation: correlation, PlanMarkdown: planMarkdown})
	if err != nil {
		if errors.Is(err, ErrPlanReviewCancelled) {
			return tools.PlanReview{}, tools.ErrPlanReviewCancelled
		}
		return tools.PlanReview{}, err
	}
	if err := validateInteractionResponse(correlation.ID, response.CorrelationID); err != nil {
		return tools.PlanReview{}, err
	}
	return tools.PlanReview{Decision: tools.PlanReviewDecision(response.Decision), Feedback: response.Feedback}, nil
}

type legacyPermissionApprover struct {
	port PermissionPort
}

func newLegacyPermissionApprover(port PermissionPort) permission.UserApprover {
	if isNilLegacyInteraction(port) {
		return nil
	}
	return &legacyPermissionApprover{port: port}
}

func (a *legacyPermissionApprover) Approve(ctx context.Context, approval permission.ApprovalRequest) (permission.UserDecision, error) {
	request := approval.Request
	correlation := legacyInteractionCorrelation(ctx, "permission", request.ToolCall.ID)
	var effects []string
	if request.Capabilities.Effects != nil {
		effects = make([]string, len(request.Capabilities.Effects))
		for index, effect := range request.Capabilities.Effects {
			effects[index] = string(effect)
		}
	}
	policyReason := ""
	if string(request.Capabilities.Behavior) == "human_only" {
		policyReason = request.Capabilities.Reason
	}
	reviewerReason := ""
	if approval.Review != nil {
		reviewerReason = approval.Review.Rationale
	}
	response, err := a.port.RequestPermission(ctx, PermissionRequest{
		Correlation: correlation, ToolName: request.ToolName, Arguments: request.Arguments,
		Action: request.Action, Risk: string(request.Risk), Source: string(request.Source),
		CWD: request.CWD, Workspace: request.Workspace, Effects: effects,
		PolicyReason: policyReason, ReviewerReason: reviewerReason, ReviewerFailure: approval.ReviewerFailure,
	})
	if err != nil {
		return permission.UserDecision{}, err
	}
	if err := validateInteractionResponse(correlation.ID, response.CorrelationID); err != nil {
		return permission.UserDecision{}, err
	}
	return permission.UserDecision{
		Kind: permission.UserDecisionKind(response.Decision), Feedback: response.Feedback,
	}, nil
}

var legacyInteractionSequence uint64

func legacyInteractionCorrelation(ctx context.Context, kind string, fallbackToolCallID string) InteractionCorrelation {
	invocation, _ := tools.InvocationContextFrom(ctx)
	toolCallID := invocation.ToolCallID
	if toolCallID == "" {
		toolCallID = fallbackToolCallID
	}
	id := strings.Join([]string{kind, invocation.SessionID, invocation.RunID, toolCallID}, ":")
	if invocation.SessionID == "" && invocation.RunID == "" && toolCallID == "" {
		id = fmt.Sprintf("%s:%d", kind, atomic.AddUint64(&legacyInteractionSequence, 1))
	}
	return InteractionCorrelation{
		ID: id, SessionID: invocation.SessionID, RunID: invocation.RunID, ToolCallID: toolCallID,
	}
}

func validateInteractionResponse(requestID string, responseID string) error {
	if responseID != requestID {
		return fmt.Errorf("interaction response correlation %q does not match request %q", responseID, requestID)
	}
	return nil
}

type legacyPermissionEventSink struct {
	sink InteractionNoticeSink
}

func newLegacyPermissionEventSink(sink InteractionNoticeSink) *legacyPermissionEventSink {
	if isNilLegacyInteraction(sink) {
		sink = nil
	}
	return &legacyPermissionEventSink{sink: sink}
}

func isNilLegacyInteraction(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (s *legacyPermissionEventSink) OnReviewStart(request permission.Request) {
	s.notify(request, InteractionPermissionReviewStarted, 0)
}

func (s *legacyPermissionEventSink) OnReviewRetry(request permission.Request, attempt int) {
	s.notify(request, InteractionPermissionReviewRetry, attempt)
}

func (s *legacyPermissionEventSink) OnAutoApproved(request permission.Request, _ permission.ReviewResult) {
	s.notify(request, InteractionPermissionAutoApproved, 0)
}

func (s *legacyPermissionEventSink) OnEscalated(request permission.Request, _ permission.ReviewResult) {
	s.notify(request, InteractionPermissionEscalated, 0)
}

func (s *legacyPermissionEventSink) OnPermissionStateChanged() {
	if s == nil || s.sink == nil {
		return
	}
	s.sink.NotifyInteraction(context.Background(), InteractionNotice{Kind: InteractionPermissionStateChanged})
}

func (s *legacyPermissionEventSink) notify(request permission.Request, kind InteractionNoticeKind, attempt int) {
	if s == nil || s.sink == nil {
		return
	}
	s.sink.NotifyInteraction(context.Background(), InteractionNotice{
		Kind: kind, Correlation: legacyInteractionCorrelation(context.Background(), "permission", request.ToolCall.ID),
		ToolName: request.ToolName, Action: request.Action, Attempt: attempt,
	})
}

var _ permission.EventSink = (*legacyPermissionEventSink)(nil)
var _ permission.StateChangeSink = (*legacyPermissionEventSink)(nil)
