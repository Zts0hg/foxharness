package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/compaction"
	legacycontext "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/modelinvoke"
	"github.com/Zts0hg/foxharness/internal/prompt"
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

func newCLIApplication(ctx context.Context, config app.CLIConfig) (*app.RuntimeApplication, error) {
	if err := validateCLILLM(config); err != nil {
		return nil, err
	}
	modelProvider, err := provider.NewProvider(config.ResolvedLLM)
	if err != nil {
		return nil, err
	}
	return newCLIApplicationWithProvider(ctx, config, modelProvider)
}

func newCLIApplicationWithProvider(ctx context.Context, config app.CLIConfig, modelProvider provider.LLMProvider) (*app.RuntimeApplication, error) {
	if err := validateCLILLM(config); err != nil {
		return nil, err
	}
	workDir, err := filepath.Abs(config.WorkDir)
	if err != nil {
		return nil, err
	}
	store := session.NewFileStore(workDir)
	stored, err := selectCLIStoredSession(store, workDir, config)
	if err != nil {
		return nil, err
	}
	memoryStore := memory.NewSessionStore(workDir, stored.RootDir)
	if err := memoryStore.EnsureFiles(); err != nil {
		return nil, fmt.Errorf("初始化文件记忆失败: %w", err)
	}
	checkpointer := checkpoint.New(checkpoint.Config{SessionDir: stored.RootDir})
	if checkpointDisabled() {
		checkpointer.SetDisabled(true)
	}
	if config.SessionID != "" || config.ContinueSession {
		if err := checkpointer.RestoreStateFromLog(); err != nil {
			return nil, fmt.Errorf("恢复 checkpoint 状态失败: %w", err)
		}
	}

	skillRegistry := slash.NewRegistry(workDir)
	if err := skillRegistry.Load(); err != nil {
		log.Printf("[slash] registry load failed: %v", err)
	}
	activations := &runtimeActivations{}
	skillRegistry.OnActivate(activations.record)
	autoMemory := automemory.NewStore(store.HomeDir(), workDir)
	hooks := automemory.NewPerRunHooks(modelProvider, autoMemory, workDir)
	tracker := hooks.NewTracker()

	compactionConfig := compaction.DefaultCompactionConfig()
	compactionConfig.Model = config.Model
	compactionConfig.SessionDir = stored.RootDir
	compactionConfig.TranscriptPath = stored.TranscriptPath()
	compactor, err := compaction.NewCompactor(modelProvider, compactionConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化 Compactor 失败: %w", err)
	}

	messageID := &runtimeMessageID{}
	journals := &cliJournalSet{}
	dependencies := foxruntime.HarnessDependencies{
		NewArtifactJournal: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.SessionArtifactJournal, error) {
			return journals.get(assembly)
		},
		NewTelemetryJournal: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.TelemetryJournal, error) {
			return journals.get(assembly)
		},
		NewModel: func(_ context.Context, assembly foxruntime.RunAssembly) (engine.ModelInvoker, error) {
			journal, err := journals.get(assembly)
			if err != nil {
				return nil, err
			}
			base := modelinvoke.New(modelProvider, modelinvoke.Config{OnSuccess: compactor.ResetCircuitBreaker})
			return journal.WrapModel(base), nil
		},
		NewTools: func(_ context.Context, assembly foxruntime.RunAssembly) (engine.ToolExecutor, error) {
			journal, err := journals.get(assembly)
			if err != nil {
				return nil, err
			}
			registry := buildCLIToolRegistry(config, workDir, stored, modelProvider, checkpointer, messageID, skillRegistry)
			resultHook := combineResultHooks(conditionalSkillHook(skillRegistry), hooks.RecordCallback(tracker))
			capabilities := registryexec.CapabilitiesWithContext(
				registry, assembly.AllowedTools,
				func(ctx context.Context) context.Context {
					return tools.WithRunContext(ctx, string(assembly.Session.ID), string(assembly.Run.RunID))
				},
				resultHook,
			)
			base := toolruntime.New(capabilities, toolresult.OSFileSystem{}, filepath.Join(assembly.Session.RootDir, "tool-results"))
			return journal.WrapTools(base), nil
		},
		NewPolicy: func(context.Context, foxruntime.RunAssembly) (engine.TurnPolicy, error) {
			return turnpolicy.New(turnpolicy.Config{Bind: func(context.Context, engine.RunInput) (turnpolicy.Bindings, error) {
				return turnpolicy.Bindings{
					NextTurn: func(context.Context, int) ([]string, error) { return activations.drain(), nil },
					TODOGate: func(context.Context) (string, error) {
						return todopolicy.CompletionReminder(stored.RootDir, true), nil
					},
				}, nil
			}}), nil
		},
		NewContext: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.ContextCollector, foxruntime.ContextCompactor, error) {
			composer := legacycontext.NewComposer(workDir).
				WithCollaborationMode(collaboration.ModeDefault).
				WithInteractiveAsk(false).
				WithMemory(memoryStore.WorkingMemoryPath()).
				WithAutoMemory(autoMemory).
				WithSkillList(func() string {
					window := compaction.NewModelRegistry().Lookup(assembly.Run.Model)
					return skilltool.FormatSkillsWithinBudget(skillRegistry.ModelInvocable(), window)
				})
			return runtimePromptCollector{composer: composer}, runtimecompaction.New(compactor), nil
		},
	}
	harness, err := foxruntime.NewRuntimeHarness(store, dependencies)
	if err != nil {
		return nil, err
	}
	agentSession, err := harness.OpenSession(ctx, foxruntime.CLIExec, stored.ID)
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = agentSession.Close(context.Background())
		}
	}()

	extractionCtx, cancelExtraction := context.WithCancel(context.Background())
	var extraction sync.WaitGroup
	maxTurns := config.MaxTurns
	thinking := config.EnableThinking
	runSpec := foxruntime.RunSpec{
		SessionID: string(stored.ID), ProviderProtocol: strings.ToLower(strings.TrimSpace(config.ResolvedLLM.Protocol)),
		Model: config.Model, Effort: config.EffortOverride, WorkDir: workDir,
		MaxTurns: &maxTurns, Thinking: &thinking,
	}
	application, err := app.NewRuntimeApplication(app.RuntimeApplicationConfig{
		Session: agentSession,
		Info:    app.SessionInfo{ID: string(stored.ID), Directory: stored.RootDir, TranscriptPath: stored.TranscriptPath()},
		RunSpec: runSpec,
		BeforeRun: func(context.Context, app.RunCommand) error {
			nextSequence, err := session.NewMessageLog(stored).NextSeq()
			if err != nil {
				return fmt.Errorf("读取下一条消息序号失败: %w", err)
			}
			if err := memory.NewStateHistory(memoryStore).SnapshotBeforeMessage(nextSequence); err != nil {
				return fmt.Errorf("创建 session state 快照失败: %w", err)
			}
			id := strconv.FormatInt(nextSequence, 10)
			messageID.set(id)
			if err := checkpointer.MakeSnapshot(id); err != nil {
				log.Printf("[Checkpoint] 创建快照失败，将继续执行: %v", err)
			}
			return nil
		},
		AfterRun: func(_ context.Context, result foxruntime.RunResult, runErr error) {
			if result.RunID == "" || (runErr != nil && result.Outcome.FinalMessage == "" && !result.Outcome.Partial) {
				return
			}
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("[automemory] extraction launch panic recovered: %v", recovered)
				}
			}()
			hooks.FireTrackedContext(extractionCtx, &extraction, stored, string(result.RunID), tracker)
		},
		Drain: func(ctx context.Context) error {
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
	return application, nil
}

func validateCLILLM(config app.CLIConfig) error {
	if config.ResolvedLLM.Protocol == "" || config.ResolvedLLM.BaseURL == "" || config.ResolvedLLM.Model == "" {
		return errors.New("missing LLM configuration: protocol, base_url, and model are required")
	}
	return nil
}

func selectCLIStoredSession(store *session.FileStore, workDir string, config app.CLIConfig) (*session.StoredSession, error) {
	if config.NewSession && (config.SessionID != "" || config.ContinueSession) {
		return nil, errors.New("-new 不能和 -session 或 -continue 同时使用")
	}
	if config.SessionID != "" && config.ContinueSession {
		return nil, errors.New("-session 不能和 -continue 同时使用")
	}
	if config.SessionID != "" {
		stored, err := store.Open(session.ID(config.SessionID))
		if errors.Is(err, session.ErrNotFound) {
			return nil, fmt.Errorf("Session %s 不存在", config.SessionID)
		}
		return stored, err
	}
	if config.ContinueSession {
		stored, err := store.Latest(session.LookupOptions{Source: session.SOURCECLI})
		if errors.Is(err, session.ErrNotFound) {
			return nil, errors.New("没有可继续的 CLI Session")
		}
		return stored, err
	}
	stored, err := store.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		return nil, fmt.Errorf("创建 Session 失败: %w", err)
	}
	return stored, nil
}

func buildCLIToolRegistry(
	config app.CLIConfig,
	workDir string,
	stored *session.StoredSession,
	modelProvider provider.LLMProvider,
	checkpointer checkpoint.Checkpointer,
	messageID *runtimeMessageID,
	skillRegistry *slash.Registry,
) tools.Registry {
	registry := tools.NewRegistry()
	registry.Use(middleware.NewCheckpointMiddleware(checkpointer, messageID.get, workDir))
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))
	registry.Register(tools.NewReadTodoTool(stored.RootDir))
	registry.Register(tools.NewUpdateTodoTool(stored.RootDir))

	var child subagent.Runner
	if config.NewChildRunner != nil {
		child = config.NewChildRunner(app.ChildRunnerConfig{
			Provider: modelProvider, WorkDir: workDir, ParentProfile: app.ChildParentProfile(foxruntime.CLIExec),
			ProviderProtocol: strings.ToLower(strings.TrimSpace(config.ResolvedLLM.Protocol)),
			Model:            config.Model, Effort: config.EffortOverride,
		})
	}
	registry.Register(subagent.NewTool(child, string(stored.ID)))
	fork := &runtimeForkRunner{runner: child, parentSessionID: string(stored.ID)}
	executor := slash.NewExecutor(slash.WithWorkDir(workDir), slash.WithForkRunner(fork))
	registry.Register(skilltool.NewSkillTool(skillRegistry, executor, func() string { return string(stored.ID) }))

	return registry
}

type runtimePromptCollector struct {
	composer engine.PromptComposer
}

func (c runtimePromptCollector) Collect(_ context.Context, request foxruntime.ContextCollectionRequest) ([]prompt.Fragment, error) {
	text, err := c.composer.Compose(request.Prompt)
	if err != nil {
		return nil, fmt.Errorf("组装系统提示词失败: %w", err)
	}
	return []prompt.Fragment{prompt.Text(text)}, nil
}

type cliJournalSet struct {
	mu      sync.Mutex
	journal *runtimejournal.Journal
	runID   session.RunID
}

func (s *cliJournalSet) get(assembly foxruntime.RunAssembly) (*runtimejournal.Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal != nil {
		if s.runID != assembly.Run.RunID {
			return nil, errors.New("CLI runtime journal cannot span multiple runs")
		}
		return s.journal, nil
	}
	journal, err := runtimejournal.New(assembly)
	if err != nil {
		return nil, err
	}
	s.journal = journal
	s.runID = assembly.Run.RunID
	return journal, nil
}

type runtimeMessageID struct {
	mu    sync.Mutex
	value string
}

func (m *runtimeMessageID) set(value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.value = value
}

func (m *runtimeMessageID) get() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.value
}

type runtimeActivations struct {
	mu      sync.Mutex
	pending []string
}

func (a *runtimeActivations) record(command *slash.Command) {
	if command == nil || !command.IsModelInvocable() {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pending = append(a.pending, skilltool.FormatActivationReminder(command))
}

func (a *runtimeActivations) drain() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := append([]string(nil), a.pending...)
	a.pending = nil
	return result
}

func conditionalSkillHook(registry *slash.Registry) registryexec.ResultHook {
	return func(call schema.ToolCall, result schema.ToolResult) {
		if result.IsError {
			return
		}
		switch call.Name {
		case "read_file", "write_file", "edit_file":
		default:
			return
		}
		var arguments struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil && arguments.Path != "" {
			registry.CheckConditional(arguments.Path)
		}
	}
}

func combineResultHooks(hooks ...registryexec.ResultHook) registryexec.ResultHook {
	return func(call schema.ToolCall, result schema.ToolResult) {
		for _, hook := range hooks {
			if hook != nil {
				hook(call, result)
			}
		}
	}
}

type runtimeForkRunner struct {
	runner          subagent.Runner
	parentSessionID string
}

func (r *runtimeForkRunner) PermissionEnforced() bool {
	return r.runner != nil && r.runner.PermissionEnforced()
}

func (r *runtimeForkRunner) Run(ctx context.Context, task string, agentType string, allowedTools []string) (string, error) {
	if r.runner == nil {
		return "", errors.New("fork runner: subagent runner unavailable")
	}
	invocation, _ := tools.InvocationContextFrom(ctx)
	result, err := r.runner.Run(ctx, subagent.Request{
		ParentSessionID: r.parentSessionID, ParentRunID: invocation.RunID,
		DelegationID: invocation.ToolCallID, Task: task, ReadOnly: false,
		Agent: subagent.AgentID(agentType), Depth: 1, AllowedTools: allowedTools,
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

func checkpointDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FOXHARNESS_DISABLE_FILE_CHECKPOINTING"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func waitForExtraction(ctx context.Context, group *sync.WaitGroup) error {
	done := make(chan struct{})
	go func() {
		group.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
