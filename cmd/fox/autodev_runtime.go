package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/compaction"
	legacycontext "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/modelinvoke"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/registryexec"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/runtimecompaction"
	"github.com/Zts0hg/foxharness/internal/runtimejournal"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/slash/skilltool"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/todopolicy"
	"github.com/Zts0hg/foxharness/internal/toolresult"
	"github.com/Zts0hg/foxharness/internal/toolruntime"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/turnpolicy"
)

type autodevProviderFactory func(llmconfig.ResolvedConfig) (provider.LLMProvider, error)

type autodevRuntimeCoreFactory struct {
	llmConfig      llmconfig.ResolvedConfig
	maxTurns       int
	newChildRunner app.ChildRunnerFactory
	newProvider    autodevProviderFactory
}

var _ autodev.CoreRunnerFactory = (*autodevRuntimeCoreFactory)(nil)

func (f *autodevRuntimeCoreFactory) New(ctx context.Context, workDir, model string) (autodev.CoreRunner, error) {
	if model == "" {
		model = f.llmConfig.Model
	}
	providerFactory := f.newProvider
	if providerFactory == nil {
		providerFactory = provider.NewProvider
	}
	providerState, err := newAutodevProviderState(f.llmConfig.WithModel(model), providerFactory)
	if err != nil {
		return nil, err
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return nil, err
	}

	store := session.NewFileStore(workDir)
	stored, err := store.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		return nil, fmt.Errorf("创建 Session 失败: %w", err)
	}
	memoryStore := memory.NewSessionStore(workDir, stored.RootDir)
	if err := memoryStore.EnsureFiles(); err != nil {
		return nil, fmt.Errorf("初始化文件记忆失败: %w", err)
	}
	checkpointer := checkpoint.New(checkpoint.Config{SessionDir: stored.RootDir})
	if checkpointDisabled() {
		checkpointer.SetDisabled(true)
	}
	permissions := permission.NewCoordinator(permission.Config{
		State: permission.NewState(permission.ModeFullAccess, true), Workspace: workDir, CWD: workDir,
	})
	questionBridge := &autodevQuestionBridge{}
	skillRegistry := slash.NewRegistry(workDir)
	if err := skillRegistry.Load(); err != nil {
		log.Printf("[slash] registry load failed: %v", err)
	}
	activations := &runtimeActivations{}
	skillRegistry.OnActivate(activations.record)
	autoMemory := automemory.NewStore(store.HomeDir(), workDir)
	messageID := &runtimeMessageID{}
	extractionCtx, cancelExtraction := context.WithCancel(ctx)
	var extraction sync.WaitGroup

	assets := &autodevRunAssets{
		provider: providerState, workDir: workDir,
		autoMemory: autoMemory,
		byRun:      make(map[session.RunID]*autodevRunAsset),
	}
	childFor := func() subagent.Runner {
		if f.newChildRunner == nil {
			return nil
		}
		return f.newChildRunner(app.ChildRunnerConfig{
			Provider: providerState, WorkDir: workDir,
			ParentProfile:    app.ChildParentProfile(foxruntime.AutodevPipeline),
			ProviderProtocol: strings.ToLower(strings.TrimSpace(providerState.configSnapshot().Protocol)),
			Model:            providerState.model(), Permission: permissions,
		})
	}
	fork := &autodevForkRunner{newRunner: childFor, parentSessionID: string(stored.ID)}
	slashExecutor := slash.NewExecutor(slash.WithWorkDir(workDir), slash.WithForkRunner(fork))

	dependencies := foxruntime.HarnessDependencies{
		NewArtifactJournal: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.SessionArtifactJournal, error) {
			asset, err := assets.get(assembly)
			if err != nil {
				return nil, err
			}
			return asset.journal, nil
		},
		NewTelemetryJournal: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.TelemetryJournal, error) {
			asset, err := assets.get(assembly)
			if err != nil {
				return nil, err
			}
			return asset.journal, nil
		},
		NewModel: func(_ context.Context, assembly foxruntime.RunAssembly) (engine.ModelInvoker, error) {
			asset, err := assets.get(assembly)
			if err != nil {
				return nil, err
			}
			return asset.journal.WrapModel(modelinvoke.New(providerState, modelinvoke.Config{OnSuccess: asset.compactor.ResetCircuitBreaker})), nil
		},
		NewTools: func(_ context.Context, assembly foxruntime.RunAssembly) (engine.ToolExecutor, error) {
			asset, err := assets.get(assembly)
			if err != nil {
				return nil, err
			}
			registry := tools.NewRegistry()
			registry.Use(middleware.NewCheckpointMiddleware(checkpointer, messageID.get, workDir))
			registry.Register(tools.NewReadFileTool(workDir))
			registry.Register(tools.NewWriteFileTool(workDir))
			registry.Register(tools.NewBashTool(workDir))
			registry.Register(tools.NewEditFileTool(workDir))
			registry.Register(tools.NewReadTodoTool(stored.RootDir))
			registry.Register(tools.NewUpdateTodoTool(stored.RootDir))
			registry.Register(subagent.NewTool(childFor(), string(stored.ID)))
			registry.Register(skilltool.NewSkillTool(skillRegistry, slashExecutor, func() string { return string(stored.ID) }))
			registry.Register(tools.NewAskUserQuestionTool(questionBridge))
			registry = permission.DecorateRegistry(registry, permissions)
			hook := combineResultHooks(conditionalSkillHook(skillRegistry), asset.hooks.RecordCallback(asset.tracker))
			capabilities := registryexec.CapabilitiesWithContext(registry, assembly.AllowedTools, func(ctx context.Context) context.Context {
				return tools.WithRunContext(ctx, string(assembly.Session.ID), string(assembly.Run.RunID))
			}, hook)
			base := toolruntime.New(capabilities, toolresult.OSFileSystem{}, filepath.Join(assembly.Session.RootDir, "tool-results"))
			return asset.journal.WrapTools(base), nil
		},
		NewPolicy: func(context.Context, foxruntime.RunAssembly) (engine.TurnPolicy, error) {
			return turnpolicy.New(turnpolicy.Config{Bind: func(context.Context, engine.RunInput) (turnpolicy.Bindings, error) {
				return turnpolicy.Bindings{
					NextTurn: func(context.Context, int) ([]string, error) { return activations.drain(), nil },
					TODOGate: func(context.Context) (string, error) { return todopolicy.CompletionReminder(stored.RootDir, true), nil },
				}, nil
			}}), nil
		},
		NewContext: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.ContextCollector, foxruntime.ContextCompactor, error) {
			asset, err := assets.get(assembly)
			if err != nil {
				return nil, nil, err
			}
			composer := legacycontext.NewComposer(workDir).
				WithCollaborationMode(collaboration.ModeDefault).
				WithInteractiveAsk(true).
				WithMemory(memoryStore.WorkingMemoryPath()).
				WithAutoMemory(autoMemory).
				WithSkillList(func() string {
					window := compaction.NewModelRegistry().Lookup(assembly.Spec.Model)
					return skilltool.FormatSkillsWithinBudget(skillRegistry.ModelInvocable(), window)
				})
			return runtimePromptCollector{composer: composer}, runtimecompaction.New(asset.compactor), nil
		},
	}
	harness, err := foxruntime.NewRuntimeHarness(store, dependencies)
	if err != nil {
		cancelExtraction()
		return nil, err
	}
	agentSession, err := harness.OpenSession(ctx, foxruntime.AutodevPipeline, stored.ID)
	if err != nil {
		cancelExtraction()
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			cancelExtraction()
			_ = agentSession.Close(context.Background())
		}
	}()

	maxTurns := f.maxTurns
	runner, err := autodev.NewRuntimeCoreRunner(autodev.RuntimeCoreRunnerConfig{
		Session: agentSession, WorkDir: workDir,
		BaseSpec: foxruntime.RunSpec{
			ProviderProtocol: strings.ToLower(strings.TrimSpace(f.llmConfig.Protocol)),
			Model:            model, WorkDir: workDir, MaxTurns: &maxTurns,
		},
		SetQuestionAsker: questionBridge.set,
		SetModel:         providerState.setModel,
		StagePrompt: func(ctx context.Context, command, args string) (string, error) {
			commandValue, ok := skillRegistry.Lookup(command)
			if !ok {
				return "", fmt.Errorf("slash command %q not found", command)
			}
			result, err := slashExecutor.Execute(ctx, commandValue, args, string(stored.ID))
			if err != nil {
				return "", fmt.Errorf("materialize %q: %w", command, err)
			}
			return result.Content, nil
		},
		BeforeRun: func(context.Context, autodev.CoreAttempt) error {
			nextSequence, err := session.NewMessageLog(stored).NextSeq()
			if err != nil {
				return fmt.Errorf("读取下一条消息序号失败: %w", err)
			}
			if err := memory.NewStateHistory(memoryStore).SnapshotBeforeMessage(nextSequence); err != nil {
				return fmt.Errorf("创建 session state 快照失败: %w", err)
			}
			id := fmt.Sprintf("%d", nextSequence)
			messageID.set(id)
			if err := checkpointer.MakeSnapshot(id); err != nil {
				log.Printf("[Checkpoint] 创建快照失败，将继续执行: %v", err)
			}
			return nil
		},
		AfterRun: func(_ context.Context, result foxruntime.RunResult, _ error) {
			if result.RunID == "" {
				return
			}
			asset := assets.lookup(result.RunID)
			if asset == nil {
				return
			}
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("[automemory] extraction launch panic recovered: %v", recovered)
				}
			}()
			asset.hooks.FireTrackedContext(extractionCtx, &extraction, stored, string(result.RunID), asset.tracker)
		},
		Drain: func(ctx context.Context) error { return waitForExtraction(ctx, &extraction) },
		Close: func(ctx context.Context) error {
			waitErr := waitForExtraction(ctx, &extraction)
			cancelExtraction()
			recoveryErr := agentSession.RecoverRunFinish(ctx)
			closeErr := agentSession.Close(ctx)
			return errors.Join(waitErr, recoveryErr, closeErr)
		},
	})
	if err != nil {
		return nil, err
	}
	failed = false
	return runner, nil
}

type autodevRunAsset struct {
	journal   *runtimejournal.Journal
	compactor *compaction.Compactor
	hooks     *automemory.PerRunHooks
	tracker   *automemory.Tracker
}

type autodevRunAssets struct {
	mu         sync.Mutex
	provider   provider.LLMProvider
	workDir    string
	autoMemory *automemory.Store
	byRun      map[session.RunID]*autodevRunAsset
}

func (s *autodevRunAssets) get(assembly foxruntime.RunAssembly) (*autodevRunAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if asset := s.byRun[assembly.Run.RunID]; asset != nil {
		return asset, nil
	}
	config := compaction.DefaultCompactionConfig()
	config.Model = assembly.Spec.Model
	config.SessionDir = assembly.Session.RootDir
	config.TranscriptPath = filepath.Join(assembly.Session.RootDir, "transcript.jsonl")
	compactor, err := compaction.NewCompactor(s.provider, config)
	if err != nil {
		return nil, err
	}
	journal, err := runtimejournal.New(assembly)
	if err != nil {
		return nil, err
	}
	hooks := automemory.NewPerRunHooks(s.provider, s.autoMemory, s.workDir)
	tracker := hooks.NewTracker()
	asset := &autodevRunAsset{journal: journal, compactor: compactor, hooks: hooks, tracker: tracker}
	s.byRun[assembly.Run.RunID] = asset
	return asset, nil
}

func (s *autodevRunAssets) lookup(runID session.RunID) *autodevRunAsset {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byRun[runID]
}

type autodevProviderState struct {
	mu          sync.RWMutex
	config      llmconfig.ResolvedConfig
	provider    provider.LLMProvider
	newProvider autodevProviderFactory
}

func newAutodevProviderState(config llmconfig.ResolvedConfig, factory autodevProviderFactory) (*autodevProviderState, error) {
	created, err := factory(config)
	if err != nil {
		return nil, err
	}
	return &autodevProviderState{config: config, provider: created, newProvider: factory}, nil
}

func (s *autodevProviderState) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	s.mu.RLock()
	current := s.provider
	s.mu.RUnlock()
	return current.Generate(ctx, messages, definitions)
}

func (s *autodevProviderState) GenerateWithOptions(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	s.mu.RLock()
	current := s.provider
	s.mu.RUnlock()
	if generator, ok := current.(provider.OptionsGenerator); ok {
		return generator.GenerateWithOptions(ctx, messages, definitions, options)
	}
	return current.Generate(ctx, messages, definitions)
}

func (s *autodevProviderState) setModel(model string) error {
	s.mu.RLock()
	config := s.config.WithModel(model)
	s.mu.RUnlock()
	created, err := s.newProvider(config)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.config, s.provider = config, created
	s.mu.Unlock()
	return nil
}

func (s *autodevProviderState) model() string { return s.configSnapshot().Model }

func (s *autodevProviderState) configSnapshot() llmconfig.ResolvedConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

type autodevQuestionBridge struct {
	mu   sync.RWMutex
	next autodev.QuestionAsker
}

func (b *autodevQuestionBridge) set(next autodev.QuestionAsker) {
	b.mu.Lock()
	b.next = next
	b.mu.Unlock()
}

func (b *autodevQuestionBridge) Ask(ctx context.Context, questions []tools.Question) ([]tools.Answer, error) {
	b.mu.RLock()
	next := b.next
	b.mu.RUnlock()
	if next == nil {
		return nil, errors.New("autodev Engineer question port is unavailable")
	}
	mapped := make([]autodev.Question, len(questions))
	for index, question := range questions {
		options := make([]autodev.Option, len(question.Options))
		for optionIndex, option := range question.Options {
			options[optionIndex] = autodev.Option{Label: option.Label, Description: option.Description, Preview: option.Preview}
		}
		mapped[index] = autodev.Question{Header: question.Header, Prompt: question.Prompt, Options: options, MultiSelect: question.MultiSelect}
	}
	answers, err := next.Ask(ctx, mapped)
	if err != nil {
		return nil, err
	}
	result := make([]tools.Answer, len(answers))
	for index, answer := range answers {
		result[index] = tools.Answer{QuestionText: answer.QuestionText, Value: answer.Value, Preview: answer.Preview, Notes: answer.Notes}
	}
	return result, nil
}

type autodevForkRunner struct {
	newRunner       func() subagent.Runner
	parentSessionID string
}

func (r *autodevForkRunner) PermissionEnforced() bool {
	runner := r.newRunner()
	return runner != nil && runner.PermissionEnforced()
}

func (r *autodevForkRunner) Run(ctx context.Context, task, agentType string, allowedTools []string) (string, error) {
	runner := r.newRunner()
	if runner == nil {
		return "", errors.New("fork runner: subagent runner unavailable")
	}
	invocation, _ := tools.InvocationContextFrom(ctx)
	result, err := runner.Run(ctx, subagent.Request{
		ParentSessionID: r.parentSessionID, ParentRunID: invocation.RunID,
		DelegationID: invocation.ToolCallID, Task: task, Agent: subagent.AgentID(agentType),
		Depth: 1, AllowedTools: allowedTools,
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

func buildRuntimeAutodevDeps(ctx context.Context, config app.CLIConfig, reporter autodev.Reporter) (autodev.Deps, error) {
	repoRoot, err := filepath.Abs(config.WorkDir)
	if err != nil {
		return autodev.Deps{}, err
	}
	autodevConfig, err := autodev.Load(repoRoot)
	if err != nil {
		return autodev.Deps{}, err
	}
	if strings.TrimSpace(config.Prompt) != "" {
		autodevConfig.BacklogFile = strings.TrimSpace(config.Prompt)
	}
	autodevConfig.Model = autodev.ResolveModel(config.Model, autodevConfig)
	persona, err := autodev.ResolveEngineerPersona(autodevConfig, repoRoot)
	if err != nil {
		return autodev.Deps{}, err
	}
	if err := validateCLILLM(config); err != nil {
		return autodev.Deps{}, err
	}
	llmConfig := config.ResolvedLLM.WithModel(autodevConfig.Model)
	engineerProvider, err := provider.NewProvider(llmConfig)
	if err != nil {
		return autodev.Deps{}, err
	}
	return autodev.Deps{
		Config: autodevConfig, RepoRoot: repoRoot,
		CoreFactory: &autodevRuntimeCoreFactory{
			llmConfig: llmConfig, maxTurns: config.MaxTurns, newChildRunner: config.NewChildRunner,
		},
		Engineer: autodev.NewEngineerAgent(engineerProvider, autodevConfig.Model, persona),
		Git:      autodev.NewExecGitRunner(), Exec: autodev.NewExecCommandRunner(), Reporter: reporter,
	}, nil
}

func runAutodev(ctx context.Context, config app.CLIConfig, reporter autodev.Reporter) error {
	deps, err := buildRuntimeAutodevDeps(ctx, config, reporter)
	if err != nil {
		return err
	}
	return autodev.New(deps).Run(ctx)
}

func autodevConfigForTUILaunch(config app.CLIConfig, backlogPath string) app.CLIConfig {
	config.Prompt = backlogPath
	return config
}

var _ provider.LLMProvider = (*autodevProviderState)(nil)
var _ provider.OptionsGenerator = (*autodevProviderState)(nil)
var _ tools.UserAsker = (*autodevQuestionBridge)(nil)
