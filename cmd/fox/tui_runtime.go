package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/compaction"
	legacycontext "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/interactionruntime"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/modelinvoke"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/planruntime"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/registryexec"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/runtimecompaction"
	"github.com/Zts0hg/foxharness/internal/runtimejournal"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/settings"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/slash/skilltool"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/todopolicy"
	"github.com/Zts0hg/foxharness/internal/toolexec"
	"github.com/Zts0hg/foxharness/internal/toolresult"
	"github.com/Zts0hg/foxharness/internal/toolruntime"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/tui"
	"github.com/Zts0hg/foxharness/internal/turnpolicy"
)

type tuiProviderFactory func(llmconfig.ResolvedConfig) (provider.LLMProvider, error)

type tuiSessionResources struct {
	stored       *session.StoredSession
	memory       *memory.Store
	checkpointer checkpoint.Checkpointer
	messageID    *runtimeMessageID
	compactorsMu sync.Mutex
	compactors   map[string]*compaction.Compactor
}

type tuiRunResources struct {
	hooks     *automemory.PerRunHooks
	tracker   *automemory.Tracker
	lifecycle *planruntime.Lifecycle
}

type tuiRuntimeComposition struct {
	mu sync.Mutex

	config          app.CLIConfig
	workDir         string
	store           *session.FileStore
	harness         *foxruntime.RuntimeHarness
	providerFactory tuiProviderFactory
	providers       map[string]provider.LLMProvider
	resources       map[session.ID]*tuiSessionResources
	runs            map[session.RunID]tuiRunResources
	current         *foxruntime.AgentSession
	model           string
	effort          string

	autoMemory  *automemory.Store
	registry    *slash.Registry
	executor    *slash.Executor
	activations runtimeActivations
	journals    tuiJournalSet

	permissions *permission.Coordinator
	controller  *interactionruntime.PermissionController
	asker       tools.UserAsker
	reviewer    tools.PlanReviewer
	application *app.InteractiveRuntimeApplication

	extractionCtx    context.Context
	extractionCancel context.CancelFunc
	extraction       sync.WaitGroup
	onModelChange    func(string) error
}

func newTUIStartup(ctx context.Context, config app.CLIConfig, interactions tui.Interactions, onModelChange func(string) error) (tui.Startup, error) {
	return newTUIStartupWithProviderFactory(ctx, config, interactions, provider.NewProvider, onModelChange)
}

func newTUIStartupWithProviderFactory(
	ctx context.Context,
	config app.CLIConfig,
	interactions tui.Interactions,
	providerFactory tuiProviderFactory,
	onModelChange func(string) error,
) (tui.Startup, error) {
	if err := validateCLILLM(config); err != nil {
		return tui.Startup{}, err
	}
	workDir, err := filepath.Abs(config.WorkDir)
	if err != nil {
		return tui.Startup{}, err
	}
	store := session.NewFileStore(workDir)
	stored, err := selectCLIStoredSession(store, workDir, config)
	if err != nil {
		return tui.Startup{}, err
	}
	homeDir, _ := os.UserHomeDir()
	loadedSettings, _ := settings.Load(homeDir)
	permissionState := permission.NewState(
		permission.NormalizeMode(loadedSettings.TUI.Permissions.Mode),
		loadedSettings.TUI.Permissions.FullAccessWarningRemembered,
	)
	events := interactionruntime.NewPermissionEvents(interactions.InteractionNotices)
	composition := &tuiRuntimeComposition{
		config: config, workDir: workDir, store: store, providerFactory: providerFactory,
		providers: make(map[string]provider.LLMProvider), resources: make(map[session.ID]*tuiSessionResources),
		runs: make(map[session.RunID]tuiRunResources), model: config.Model, effort: config.EffortOverride,
		autoMemory:    automemory.NewStore(store.HomeDir(), workDir),
		controller:    interactionruntime.NewPermissionController(permissionState),
		asker:         interactionruntime.NewQuestionAsker(interactions.Questions),
		reviewer:      interactionruntime.NewPlanReviewer(interactions.PlanReview),
		onModelChange: onModelChange,
	}
	composition.extractionCtx, composition.extractionCancel = context.WithCancel(context.Background())
	modelProvider, err := composition.providerFor(config.Model)
	if err != nil {
		composition.extractionCancel()
		return tui.Startup{}, err
	}
	providerReviewer := permission.NewProviderReviewer(composition.currentProvider)
	providerReviewer.OnRetry = events.OnReviewRetry
	composition.permissions = permission.NewCoordinator(permission.Config{
		State: permissionState, Workspace: workDir, CWD: workDir, Source: permission.SourceMain,
		Approver: interactionruntime.NewPermissionApprover(interactions.Permissions),
		Reviewer: providerReviewer, Sink: events,
	})
	_ = modelProvider

	composition.registry = slash.NewRegistry(workDir)
	if err := composition.registry.Load(); err != nil {
		log.Printf("[slash] registry load failed: %v", err)
	}
	composition.registry.OnActivate(composition.activations.record)
	composition.executor = slash.NewExecutor(
		slash.WithWorkDir(workDir),
		slash.WithForkRunner(&tuiForkRunner{composition: composition}),
	)

	dependencies := foxruntime.HarnessDependencies{
		InitializeSession: composition.initializeSession,
		NewArtifactJournal: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.SessionArtifactJournal, error) {
			return composition.journals.get(assembly)
		},
		NewTelemetryJournal: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.TelemetryJournal, error) {
			return composition.journals.get(assembly)
		},
		NewModel:   composition.newModel,
		NewTools:   composition.newTools,
		NewPolicy:  composition.newPolicy,
		NewContext: composition.newContext,
	}
	composition.harness, err = foxruntime.NewRuntimeHarness(store, dependencies)
	if err != nil {
		composition.extractionCancel()
		return tui.Startup{}, err
	}
	if err := composition.initializeSession(ctx, foxruntime.AgentSessionSnapshot{
		ID: stored.ID, Profile: foxruntime.TUIInteractive, Source: stored.Source,
		WorkDir: stored.WorkDir, RootDir: stored.RootDir,
	}); err != nil {
		composition.extractionCancel()
		return tui.Startup{}, err
	}
	agentSession, err := composition.harness.OpenSession(ctx, foxruntime.TUIInteractive, stored.ID)
	if err != nil {
		composition.extractionCancel()
		return tui.Startup{}, err
	}
	binding, err := composition.bind(agentSession)
	if err != nil {
		_ = agentSession.Close(context.Background())
		composition.extractionCancel()
		return tui.Startup{}, err
	}
	maxTurns := config.MaxTurns
	thinking := config.EnableThinking
	application, err := app.NewInteractiveRuntimeApplication(app.InteractiveRuntimeApplicationConfig{
		Initial: binding,
		NewSession: func(ctx context.Context) (app.InteractiveRuntimeBinding, error) {
			next, err := composition.harness.CreateSession(ctx, foxruntime.TUIInteractive, foxruntime.SessionOptions{WorkDir: workDir})
			if err != nil {
				return app.InteractiveRuntimeBinding{}, err
			}
			return composition.bind(next)
		},
		RunSpec: foxruntime.RunSpec{
			ProviderProtocol: strings.ToLower(strings.TrimSpace(config.ResolvedLLM.Protocol)), WorkDir: workDir,
			MaxTurns: &maxTurns, Thinking: &thinking,
		},
		Model: config.Model, Effort: config.EffortOverride, CollaborationMode: string(collaboration.ModeDefault),
		NormalizeCollaboration: func(value string) string { return string(collaboration.Normalize(collaboration.Mode(value))) },
		OnModelChange:          composition.changeModel,
		OnEffortChange: func(value string) {
			composition.mu.Lock()
			composition.effort = value
			composition.mu.Unlock()
		},
		Permissions: composition.controller,
	})
	if err != nil {
		_ = agentSession.Close(context.Background())
		composition.extractionCancel()
		return tui.Startup{}, err
	}
	composition.mu.Lock()
	composition.application = application
	composition.mu.Unlock()
	return tui.Startup{
		Application: application, Registry: composition.registry, Executor: composition.executor,
		SessionLogDir: stored.RootDir,
		Close: func(ctx context.Context) error {
			composition.controller.ClearPermissionGrants(ctx)
			composition.extractionCancel()
			waitErr := waitForExtraction(ctx, &composition.extraction)
			composition.mu.Lock()
			current := composition.current
			composition.current = nil
			composition.mu.Unlock()
			if current == nil {
				return waitErr
			}
			return errors.Join(waitErr, current.RecoverRunFinish(ctx), current.Close(ctx))
		},
	}, nil
}

func (c *tuiRuntimeComposition) initializeSession(_ context.Context, snapshot foxruntime.AgentSessionSnapshot) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resources[snapshot.ID] != nil {
		return nil
	}
	stored, err := c.store.Open(snapshot.ID)
	if err != nil {
		return err
	}
	memoryStore := memory.NewSessionStore(c.workDir, stored.RootDir)
	if err := memoryStore.EnsureFiles(); err != nil {
		return fmt.Errorf("初始化文件记忆失败: %w", err)
	}
	checkpointer := checkpoint.New(checkpoint.Config{SessionDir: stored.RootDir})
	if checkpointDisabled() {
		checkpointer.SetDisabled(true)
	}
	if err := checkpointer.RestoreStateFromLog(); err != nil {
		return fmt.Errorf("恢复 checkpoint 状态失败: %w", err)
	}
	c.resources[snapshot.ID] = &tuiSessionResources{
		stored: stored, memory: memoryStore, checkpointer: checkpointer,
		messageID: &runtimeMessageID{}, compactors: make(map[string]*compaction.Compactor),
	}
	return nil
}

func (c *tuiRuntimeComposition) resource(id session.ID) (*tuiSessionResources, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	resource := c.resources[id]
	if resource == nil {
		return nil, fmt.Errorf("TUI runtime session resources unavailable for %s", id)
	}
	return resource, nil
}

func (c *tuiRuntimeComposition) providerFor(model string) (provider.LLMProvider, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if current := c.providers[model]; current != nil {
		return current, nil
	}
	resolved := c.config.ResolvedLLM.WithModel(model)
	created, err := c.providerFactory(resolved)
	if err != nil {
		return nil, err
	}
	c.providers[model] = created
	return created, nil
}

func (c *tuiRuntimeComposition) currentProvider() provider.LLMProvider {
	c.mu.Lock()
	model := c.model
	current := c.providers[model]
	c.mu.Unlock()
	return current
}

func (c *tuiRuntimeComposition) changeModel(model string) error {
	if _, err := c.providerFor(model); err != nil {
		return err
	}
	c.mu.Lock()
	c.model = model
	c.mu.Unlock()
	if c.onModelChange != nil {
		if err := c.onModelChange(model); err != nil {
			log.Printf("[Runner] onModelChange callback failed: %v", err)
		}
	}
	return nil
}

func (c *tuiRuntimeComposition) compactor(resource *tuiSessionResources, model string, modelProvider provider.LLMProvider) (*compaction.Compactor, error) {
	resource.compactorsMu.Lock()
	defer resource.compactorsMu.Unlock()
	if current := resource.compactors[model]; current != nil {
		return current, nil
	}
	config := compaction.DefaultCompactionConfig()
	config.Model = model
	config.SessionDir = resource.stored.RootDir
	config.TranscriptPath = resource.stored.TranscriptPath()
	created, err := compaction.NewCompactor(modelProvider, config)
	if err != nil {
		return nil, err
	}
	resource.compactors[model] = created
	return created, nil
}

func (c *tuiRuntimeComposition) newModel(_ context.Context, assembly foxruntime.RunAssembly) (engine.ModelInvoker, error) {
	modelProvider, err := c.providerFor(assembly.Run.Model)
	if err != nil {
		return nil, err
	}
	resource, err := c.resource(assembly.Session.ID)
	if err != nil {
		return nil, err
	}
	compactor, err := c.compactor(resource, assembly.Run.Model, modelProvider)
	if err != nil {
		return nil, err
	}
	journal, err := c.journals.get(assembly)
	if err != nil {
		return nil, err
	}
	return journal.WrapModel(modelinvoke.New(modelProvider, modelinvoke.Config{OnSuccess: compactor.ResetCircuitBreaker})), nil
}

func (c *tuiRuntimeComposition) newTools(_ context.Context, assembly foxruntime.RunAssembly) (engine.ToolExecutor, error) {
	resource, err := c.resource(assembly.Session.ID)
	if err != nil {
		return nil, err
	}
	modelProvider, err := c.providerFor(assembly.Run.Model)
	if err != nil {
		return nil, err
	}
	hooks := automemory.NewPerRunHooks(modelProvider, c.autoMemory, c.workDir)
	tracker := hooks.NewTracker()
	evidence := c.permissionEvidence(resource.stored, assembly.Spec.Prompt)
	registry := c.buildToolRegistry(assembly, resource, modelProvider)
	registry = permission.DecorateRegistry(registry, c.permissions, evidence)
	var lifecycle *planruntime.Lifecycle
	capabilityNames := assembly.AllowedTools
	if collaboration.Normalize(collaboration.Mode(assembly.Spec.CollaborationMode)) == collaboration.ModeFormalPlan {
		for _, required := range []string{"read_file", "bash", "ask_user_question", "update_todo"} {
			if !containsToolName(capabilityNames, required) {
				return nil, fmt.Errorf("restricted Formal Plan run is missing required lifecycle tool: %s", required)
			}
		}
		checklistRegistry := tools.NewRegistry()
		checklistRegistry.Register(tools.NewReadFileTool(c.workDir))
		checklistRegistry.Register(tools.NewBashTool(c.workDir))
		checklistRegistry.Register(tools.NewAskUserQuestionTool(c.asker))
		checklistRegistry.Register(tools.NewReadTodoTool(resource.stored.RootDir))
		checklistRegistry.Register(tools.NewUpdateTodoTool(resource.stored.RootDir))
		checklist := permission.DecorateRegistry(checklistRegistry, c.permissions, evidence)
		lifecycle = planruntime.New(nil, checklist, registry, c.resetCollaboration)
		formalRegistry := tools.NewRegistry()
		formalRegistry.Register(tools.NewReadFileTool(c.workDir))
		formalRegistry.Register(tools.NewBashTool(c.workDir))
		formalRegistry.Register(tools.NewAskUserQuestionTool(c.asker))
		formalRegistry.Register(tools.NewSubmitPlanTool(resource.memory, c.reviewer, lifecycle.Approve))
		lifecycle.SetFormalRegistry(permission.DecorateRegistry(formalRegistry, c.permissions, evidence))
		registry = lifecycle
		capabilityNames = append(append([]string(nil), capabilityNames...), "submit_plan")
	}
	c.mu.Lock()
	c.runs[assembly.Run.RunID] = tuiRunResources{hooks: hooks, tracker: tracker, lifecycle: lifecycle}
	c.mu.Unlock()
	resultHook := combineResultHooks(conditionalSkillHook(c.registry), hooks.RecordCallback(tracker))
	contextHook := func(ctx context.Context) context.Context {
		return tools.WithRunContext(ctx, string(assembly.Session.ID), string(assembly.Run.RunID))
	}
	capabilitySource := func() []toolexec.Capability {
		return registryexec.CapabilitiesWithContext(registry, capabilityNames, contextHook, resultHook)
	}
	beginTurn := func(context.Context) error { return nil }
	if lifecycle != nil {
		beginTurn = func(context.Context) error { lifecycle.BeginTurn(); return nil }
	}
	journal, err := c.journals.get(assembly)
	if err != nil {
		return nil, err
	}
	base := toolruntime.NewDynamic(capabilitySource, beginTurn, toolresult.OSFileSystem{}, filepath.Join(assembly.Session.RootDir, "tool-results"))
	return journal.WrapTools(base), nil
}

func (c *tuiRuntimeComposition) resetCollaboration() {
	c.mu.Lock()
	application := c.application
	c.mu.Unlock()
	if application != nil {
		application.UpdateCollaborationMode(context.Background(), app.CollaborationCommand{Mode: string(collaboration.ModeDefault)})
	}
}

func (c *tuiRuntimeComposition) buildToolRegistry(assembly foxruntime.RunAssembly, resource *tuiSessionResources, modelProvider provider.LLMProvider) tools.Registry {
	registry := tools.NewRegistry()
	registry.Use(middleware.NewCheckpointMiddleware(resource.checkpointer, resource.messageID.get, c.workDir))
	registry.Register(tools.NewReadFileTool(c.workDir))
	registry.Register(tools.NewWriteFileTool(c.workDir))
	registry.Register(tools.NewBashTool(c.workDir))
	registry.Register(tools.NewEditFileTool(c.workDir))
	registry.Register(tools.NewReadTodoTool(resource.stored.RootDir))
	registry.Register(tools.NewUpdateTodoTool(resource.stored.RootDir))
	registry.Register(tools.NewAskUserQuestionTool(c.asker))

	var child subagent.Runner
	if c.config.NewChildRunner != nil {
		child = c.config.NewChildRunner(app.ChildRunnerConfig{
			Provider: modelProvider, WorkDir: c.workDir, ParentProfile: app.ChildParentProfile(foxruntime.TUIInteractive),
			ProviderProtocol: assembly.Spec.ProviderProtocol, Model: assembly.Run.Model, Effort: assembly.Run.Effort,
			Permission: c.permissions, ParentEvidence: c.permissionEvidence(resource.stored, assembly.Spec.Prompt),
		})
	}
	registry.Register(subagent.NewTool(child, string(assembly.Session.ID)))
	fork := &runtimeForkRunner{runner: child, parentSessionID: string(assembly.Session.ID)}
	executor := slash.NewExecutor(slash.WithWorkDir(c.workDir), slash.WithForkRunner(fork))
	registry.Register(skilltool.NewSkillTool(c.registry, executor, func() string { return string(assembly.Session.ID) }))
	return registry
}

func (c *tuiRuntimeComposition) newPolicy(_ context.Context, assembly foxruntime.RunAssembly) (engine.TurnPolicy, error) {
	c.mu.Lock()
	run := c.runs[assembly.Run.RunID]
	c.mu.Unlock()
	return turnpolicy.New(turnpolicy.Config{Bind: func(context.Context, engine.RunInput) (turnpolicy.Bindings, error) {
		nextTurn := func(context.Context, int) ([]string, error) { return c.activations.drain(), nil }
		var completionGate func(context.Context) (string, error)
		if run.lifecycle != nil {
			nextTurn = func(context.Context, int) ([]string, error) {
				return append(run.lifecycle.RuntimeReminders(), c.activations.drain()...), nil
			}
			completionGate = func(context.Context) (string, error) { return run.lifecycle.CompletionReminder(), nil }
		}
		return turnpolicy.Bindings{
			NextTurn:       nextTurn,
			CompletionGate: completionGate,
			TODOGate: func(context.Context) (string, error) {
				resource, err := c.resource(assembly.Session.ID)
				if err != nil {
					return "", err
				}
				return todopolicy.CompletionReminder(resource.stored.RootDir, true), nil
			},
		}, nil
	}}), nil
}

func (c *tuiRuntimeComposition) newContext(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.ContextCollector, foxruntime.ContextCompactor, error) {
	resource, err := c.resource(assembly.Session.ID)
	if err != nil {
		return nil, nil, err
	}
	modelProvider, err := c.providerFor(assembly.Run.Model)
	if err != nil {
		return nil, nil, err
	}
	compactor, err := c.compactor(resource, assembly.Run.Model, modelProvider)
	if err != nil {
		return nil, nil, err
	}
	composer := legacycontext.NewComposer(c.workDir).
		WithCollaborationMode(collaboration.Mode(assembly.Run.CollaborationMode)).
		WithInteractiveAsk(true).
		WithMemory(resource.memory.WorkingMemoryPath()).
		WithAutoMemory(c.autoMemory).
		WithSkillList(func() string {
			window := compaction.NewModelRegistry().Lookup(assembly.Run.Model)
			return skilltool.FormatSkillsWithinBudget(c.registry.ModelInvocable(), window)
		})
	return runtimePromptCollector{composer: composer}, runtimecompaction.New(compactor), nil
}

func (c *tuiRuntimeComposition) bind(agentSession *foxruntime.AgentSession) (app.InteractiveRuntimeBinding, error) {
	resource, err := c.resource(agentSession.Snapshot().ID)
	if err != nil {
		return app.InteractiveRuntimeBinding{}, err
	}
	c.mu.Lock()
	c.current = agentSession
	c.mu.Unlock()
	return app.InteractiveRuntimeBinding{
		Session: agentSession,
		State:   func() app.InteractiveSessionState { return c.sessionState(resource) },
		Conversation: func(context.Context) ([]app.ConversationRecord, error) {
			records, err := session.NewMessageLog(resource.stored).LoadRecords()
			return mapTUIConversation(records), err
		},
		ProjectInputHistory: func(_ context.Context, limit int) ([]string, error) {
			return c.projectInputHistory(resource.stored.ID, limit)
		},
		RewindTargets: func(context.Context) ([]app.RewindTarget, error) { return tuiRewindTargets(resource) },
		Compact: func(ctx context.Context, command app.CompactCommand) (app.CompactOutcome, error) {
			return c.compact(ctx, agentSession, resource, command)
		},
		Rewind: func(ctx context.Context, command app.RewindCommand) app.RewindOutcome {
			return c.rewind(ctx, agentSession, resource, command)
		},
		RestoreLatestInput: func(ctx context.Context) (app.RestoreInputOutcome, error) {
			return c.restoreLatestInput(ctx, agentSession, resource)
		},
		BeforeRun: func(_ context.Context, _ app.RunCommand) error {
			nextSequence, err := session.NewMessageLog(resource.stored).NextSeq()
			if err != nil {
				return fmt.Errorf("读取下一条消息序号失败: %w", err)
			}
			if err := memory.NewStateHistory(resource.memory).SnapshotBeforeMessage(nextSequence); err != nil {
				return fmt.Errorf("创建 session state 快照失败: %w", err)
			}
			id := strconv.FormatInt(nextSequence, 10)
			resource.messageID.set(id)
			if err := resource.checkpointer.MakeSnapshot(id); err != nil {
				log.Printf("[Checkpoint] 创建快照失败，将继续执行: %v", err)
			}
			return nil
		},
		AfterRun: func(_ context.Context, result foxruntime.RunResult, runErr error) {
			c.afterRun(resource, result, runErr)
		},
		Close: func(ctx context.Context) error {
			return errors.Join(agentSession.RecoverRunFinish(ctx), agentSession.Close(ctx))
		},
	}, nil
}

func (c *tuiRuntimeComposition) sessionState(resource *tuiSessionResources) app.InteractiveSessionState {
	return app.InteractiveSessionState{
		Session: app.SessionInfo{
			ID: string(resource.stored.ID), Directory: resource.stored.RootDir,
			TranscriptPath: resource.stored.TranscriptPath(),
		},
		WorkDir: c.workDir, ContextUsage: c.contextUsage(resource),
		AutoMemoryIndex: c.autoMemory.MergedIndexString(), RewindAvailable: resource.checkpointer != nil,
		RunCapabilities: app.RunCapabilities{ToolRestrictions: true, EffortOverrides: true},
	}
}

func (c *tuiRuntimeComposition) afterRun(resource *tuiSessionResources, result foxruntime.RunResult, runErr error) {
	if result.RunID == "" {
		return
	}
	c.mu.Lock()
	run := c.runs[result.RunID]
	delete(c.runs, result.RunID)
	c.mu.Unlock()
	c.journals.remove(result.RunID)
	if runErr != nil && result.Outcome.FinalMessage == "" && !result.Outcome.Partial {
		return
	}
	if run.hooks == nil || (run.lifecycle != nil && !run.lifecycle.MemoryExtractionAllowed()) {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[automemory] extraction launch panic recovered: %v", recovered)
		}
	}()
	run.hooks.FireTrackedContext(c.extractionCtx, &c.extraction, resource.stored, string(result.RunID), run.tracker)
}

func containsToolName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func (c *tuiRuntimeComposition) permissionEvidence(stored *session.StoredSession, prompt string) permission.EvidenceProvider {
	instructions := snapshotTUIInstructions(c.workDir)
	return func(request permission.Request) permission.Evidence {
		records, _ := session.NewMessageLog(stored).LoadRecords()
		messages := make([]schema.Message, 0, len(records)+1)
		for _, record := range records {
			message := record.Message
			if message.Role == schema.RoleUser && message.ToolCallID == "" && (record.IsMeta || record.IsCompactSummary || record.IsVisibleInTranscriptOnly) {
				message.Role = schema.RoleSystem
			}
			messages = append(messages, message)
		}
		if strings.TrimSpace(prompt) != "" {
			messages = append(messages, schema.Message{Role: schema.RoleUser, Content: prompt})
		}
		return permission.BuildEvidence(messages, instructions, request)
	}
}

func snapshotTUIInstructions(workDir string) []string {
	content, err := os.ReadFile(filepath.Join(workDir, "AGENTS.md"))
	if err != nil || len(content) == 0 {
		return nil
	}
	return []string{string(content)}
}

func mapTUIConversation(records []session.MessageRecord) []app.ConversationRecord {
	result := make([]app.ConversationRecord, len(records))
	for index, record := range records {
		calls := make([]app.ConversationToolCall, len(record.Message.ToolCalls))
		for callIndex, call := range record.Message.ToolCalls {
			calls[callIndex] = app.ConversationToolCall{ID: call.ID, Name: call.Name, Arguments: string(call.Arguments)}
		}
		result[index] = app.ConversationRecord{
			Sequence: record.Seq, Time: record.Time, Role: string(record.Message.Role),
			Content: record.Message.Content, DisplayContent: record.DisplayContent,
			ToolCallID: record.Message.ToolCallID, ToolCalls: calls,
			IsMeta: record.IsMeta, IsCompactSummary: record.IsCompactSummary,
			IsVisibleInTranscriptOnly: record.IsVisibleInTranscriptOnly,
		}
	}
	return result
}

func tuiRewindTargets(resource *tuiSessionResources) ([]app.RewindTarget, error) {
	records, err := session.NewMessageLog(resource.stored).LoadRecords()
	if err != nil {
		return nil, err
	}
	messages := checkpoint.SelectableMessages(records)
	targets := make([]app.RewindTarget, 0, len(messages))
	for _, message := range messages {
		target := app.RewindTarget{Sequence: message.Seq, Content: message.Content, Timestamp: message.Timestamp, IsCurrent: message.IsCurrent}
		if resource.checkpointer != nil {
			stats, statsErr := resource.checkpointer.GetDiffStats(strconv.FormatInt(message.Seq, 10))
			if statsErr != nil {
				target.DiffError = statsErr.Error()
			} else if stats != nil {
				target.Diff = app.RewindDiff{
					FilesChanged: stats.FilesChanged, Insertions: stats.Insertions, Deletions: stats.Deletions,
					ChangedFiles: append([]string(nil), stats.ChangedFiles...),
				}
			}
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func (c *tuiRuntimeComposition) compact(ctx context.Context, agentSession *foxruntime.AgentSession, resource *tuiSessionResources, command app.CompactCommand) (app.CompactOutcome, error) {
	c.mu.Lock()
	model := c.model
	c.mu.Unlock()
	modelProvider, err := c.providerFor(model)
	if err != nil {
		return app.CompactOutcome{}, err
	}
	mechanism, err := c.compactor(resource, model, modelProvider)
	if err != nil {
		return app.CompactOutcome{}, err
	}
	records, err := session.NewMessageLog(resource.stored).LoadRecords()
	if err != nil {
		return app.CompactOutcome{}, err
	}
	state, err := session.LoadCompactState(resource.stored)
	if err != nil {
		return app.CompactOutcome{}, err
	}
	projected := tuiProjectedMessages(state, records)
	if len(projected) < 2 {
		return app.CompactOutcome{}, fmt.Errorf("not enough messages to compact (%d messages)", len(projected))
	}
	preTokens := mechanism.Estimate(projected)
	proposal, err := agentSession.CompactContext(ctx, runtimecompaction.New(mechanism), command.Instructions)
	if err != nil {
		return app.CompactOutcome{}, err
	}
	postState := proposal.CompactState
	if postState == nil {
		postState, _ = session.LoadCompactState(resource.stored)
	}
	postTokens := mechanism.Estimate(tuiProjectedMessages(postState, records))
	return app.CompactOutcome{PreTokens: preTokens, PostTokens: postTokens, MessagesSummarized: len(projected)}, nil
}

func (c *tuiRuntimeComposition) rewind(ctx context.Context, agentSession *foxruntime.AgentSession, resource *tuiSessionResources, command app.RewindCommand) app.RewindOutcome {
	outcome := app.RewindOutcome{}
	records, err := session.NewMessageLog(resource.stored).LoadRecords()
	if err != nil {
		outcome.Error = err.Error()
		return outcome
	}
	content := conversationContent(records, command.Sequence)
	if command.Action == app.RewindBoth || command.Action == app.RewindCode {
		if resource.checkpointer != nil {
			outcome.CodeAttempted = true
			files, rewindErr := resource.checkpointer.Rewind(strconv.FormatInt(command.Sequence, 10))
			if rewindErr != nil {
				outcome.CodeError = rewindErr.Error()
				if command.Action == app.RewindCode {
					return outcome
				}
			} else {
				outcome.CodeFiles = append([]string(nil), files...)
			}
		}
	}
	if command.Action != app.RewindBoth && command.Action != app.RewindConversation {
		return outcome
	}
	outcome.ConversationAttempted = true
	if err := agentSession.RewindContext(ctx, command.Sequence); err != nil {
		outcome.ConversationError = err.Error()
		return outcome
	}
	records, err = session.NewMessageLog(resource.stored).LoadRecords()
	if err != nil {
		outcome.ConversationError = err.Error()
		return outcome
	}
	outcome.Conversation = mapTUIConversation(records)
	outcome.RestoredInput = content
	outcome.SessionStateAttempted = true
	err = memory.NewStateHistory(resource.memory).RestoreBeforeMessage(command.Sequence)
	if errors.Is(err, memory.ErrStateSnapshotNotFound) {
		return outcome
	}
	if err != nil {
		outcome.SessionStateError = err.Error()
		return outcome
	}
	outcome.SessionStateRestored = true
	return outcome
}

func (c *tuiRuntimeComposition) restoreLatestInput(ctx context.Context, agentSession *foxruntime.AgentSession, resource *tuiSessionResources) (app.RestoreInputOutcome, error) {
	records, err := session.NewMessageLog(resource.stored).LoadRecords()
	if err != nil {
		return app.RestoreInputOutcome{}, nil
	}
	index := -1
	var target checkpoint.SelectableMessage
	for candidate := len(records) - 1; candidate >= 0; candidate-- {
		messages := checkpoint.SelectableMessages(records[candidate : candidate+1])
		if len(messages) > 0 {
			index = candidate
			target = messages[0]
			break
		}
	}
	if index < 0 || !checkpoint.MessagesAfterAreOnlySynthetic(records, index) {
		return app.RestoreInputOutcome{}, nil
	}
	outcome := app.RestoreInputOutcome{Attempted: true}
	if err := agentSession.RewindContext(ctx, target.Seq); err != nil {
		return outcome, err
	}
	records, err = session.NewMessageLog(resource.stored).LoadRecords()
	if err != nil {
		return outcome, err
	}
	return app.RestoreInputOutcome{Attempted: true, Restored: true, Conversation: mapTUIConversation(records), Input: target.Content}, nil
}

func conversationContent(records []session.MessageRecord, sequence int64) string {
	for _, record := range records {
		if record.Seq == sequence {
			return record.HumanContent()
		}
	}
	return ""
}

func tuiProjectedMessages(state *session.CompactState, records []session.MessageRecord) []schema.Message {
	coveredUntil := int64(-1)
	var messages []schema.Message
	if state != nil && state.Summary != "" {
		coveredUntil = state.CoveredUntilSeq
		messages = append(messages, schema.Message{Role: schema.RoleUser, Content: state.Summary})
	}
	for _, record := range records {
		if record.Seq > coveredUntil {
			messages = append(messages, record.Message)
		}
	}
	return messages
}

func (c *tuiRuntimeComposition) contextUsage(resource *tuiSessionResources) string {
	records, err := session.NewMessageLog(resource.stored).LoadRecords()
	if err != nil {
		return "unknown"
	}
	state, _ := session.LoadCompactState(resource.stored)
	c.mu.Lock()
	model := c.model
	c.mu.Unlock()
	estimator := compaction.NewHybridEstimator(compaction.ImprovedRoughEstimator{})
	return formatTUIContextUsage(estimator.Estimate(tuiProjectedMessages(state, records)), compaction.NewModelRegistry().Lookup(model))
}

func formatTUIContextUsage(used int, maximum int) string {
	if maximum <= 0 {
		return "unknown"
	}
	if used <= 0 {
		return "0%"
	}
	if used*100 < maximum {
		return "<1%"
	}
	return fmt.Sprintf("%d%%", (used*100+maximum-1)/maximum)
}

func (c *tuiRuntimeComposition) projectInputHistory(current session.ID, limit int) ([]string, error) {
	sessions, err := c.store.List(session.LookupOptions{Source: session.SOURCECLI})
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
	for _, stored := range sessions {
		records, err := session.NewMessageLog(stored).LoadRecords()
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			message := record.Message
			text := strings.TrimSpace(record.HumanContent())
			if message.Role != schema.RoleUser || message.ToolCallID != "" || text == "" || strings.HasPrefix(text, "## Compacted Context Summary") {
				continue
			}
			prompts = append(prompts, promptRecord{text: text, when: record.Time, seq: record.Seq, sessionID: string(stored.ID), current: stored.ID == current})
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
		if len(history) == 0 || history[len(history)-1] != prompt.text {
			history = append(history, prompt.text)
		}
	}
	return history, nil
}

type tuiJournalSet struct {
	mu       sync.Mutex
	journals map[session.RunID]*runtimejournal.Journal
}

func (s *tuiJournalSet) get(assembly foxruntime.RunAssembly) (*runtimejournal.Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journals == nil {
		s.journals = make(map[session.RunID]*runtimejournal.Journal)
	}
	if journal := s.journals[assembly.Run.RunID]; journal != nil {
		return journal, nil
	}
	journal, err := runtimejournal.New(assembly)
	if err != nil {
		return nil, err
	}
	s.journals[assembly.Run.RunID] = journal
	return journal, nil
}

func (s *tuiJournalSet) remove(runID session.RunID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.journals, runID)
}

type tuiForkRunner struct{ composition *tuiRuntimeComposition }

func (r *tuiForkRunner) PermissionEnforced() bool {
	return r.composition != nil && r.composition.permissions != nil
}

func (r *tuiForkRunner) Run(ctx context.Context, task string, agentType string, allowedTools []string) (string, error) {
	if r.composition == nil || r.composition.config.NewChildRunner == nil {
		return "", errors.New("fork runner: subagent runner unavailable")
	}
	r.composition.mu.Lock()
	current := r.composition.current
	model := r.composition.model
	effort := r.composition.effort
	modelProvider := r.composition.providers[model]
	r.composition.mu.Unlock()
	if current == nil || modelProvider == nil {
		return "", errors.New("fork runner: active runtime session unavailable")
	}
	resource, err := r.composition.resource(current.Snapshot().ID)
	if err != nil {
		return "", err
	}
	invocation, _ := tools.InvocationContextFrom(ctx)
	child := r.composition.config.NewChildRunner(app.ChildRunnerConfig{
		Provider: modelProvider, WorkDir: r.composition.workDir,
		ParentProfile:    app.ChildParentProfile(foxruntime.TUIInteractive),
		ProviderProtocol: strings.ToLower(strings.TrimSpace(r.composition.config.ResolvedLLM.Protocol)),
		Model:            model, Effort: effort, Permission: r.composition.permissions,
		ParentEvidence: r.composition.permissionEvidence(resource.stored, ""),
	})
	if child == nil {
		return "", errors.New("fork runner: subagent runner unavailable")
	}
	result, err := child.Run(ctx, subagent.Request{
		ParentSessionID: string(current.Snapshot().ID), ParentRunID: invocation.RunID,
		DelegationID: invocation.ToolCallID, Task: task, Agent: subagent.AgentID(agentType), Depth: 1,
		AllowedTools: allowedTools,
	})
	if err != nil {
		if result == nil {
			return "", err
		}
		return subagent.FormatFailureOutcome(result, err), &subagent.OutcomeError{Outcome: result, Err: err}
	}
	if result == nil {
		return "", nil
	}
	return result.Report, nil
}
