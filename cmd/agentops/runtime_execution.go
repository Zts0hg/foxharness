package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/agentops"
	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/childruntime"
	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/feishu"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/modelinvoke"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/registryexec"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/runtimecompaction"
	"github.com/Zts0hg/foxharness/internal/runtimejournal"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/todopolicy"
	"github.com/Zts0hg/foxharness/internal/toolresult"
	"github.com/Zts0hg/foxharness/internal/toolruntime"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/turnpolicy"
)

type agentOpsTaskExecutionFactory struct {
	provider  provider.LLMProvider
	workDir   string
	logDir    string
	messenger feishu.TextMessenger
	store     *session.FileStore
	approvals *approval.Store
}

func newAgentOpsTaskExecutionFactory(
	modelProvider provider.LLMProvider,
	workDir string,
	logDir string,
	messenger feishu.TextMessenger,
	store *session.FileStore,
	approvals *approval.Store,
) agentops.TaskExecutionFactory {
	return &agentOpsTaskExecutionFactory{
		provider: modelProvider, workDir: workDir, logDir: logDir,
		messenger: messenger, store: store, approvals: approvals,
	}
}

func (f *agentOpsTaskExecutionFactory) PrepareTask(_ context.Context, request agentops.TaskExecutionRequest) (agentops.PreparedTaskExecution, error) {
	taskProvider := snapshotAgentOpsProvider(f.provider)
	stored, err := f.store.Create(session.CreateOptions{
		Source: session.SOURCEFeishu, WorkDir: f.workDir,
		UserID: request.Task.SenderID, ChatID: request.Task.ChatID,
	})
	if err != nil {
		return agentops.PreparedTaskExecution{}, err
	}
	if err := memory.NewSessionStore(f.workDir, stored.RootDir).EnsureWorkingMemory(); err != nil {
		return agentops.PreparedTaskExecution{}, fmt.Errorf("初始化 Working Memory 失败: %w", err)
	}
	return agentops.PreparedTaskExecution{
		Session: app.SessionInfo{
			ID: string(stored.ID), Directory: stored.RootDir, TranscriptPath: stored.TranscriptPath(),
		},
		TracePath: stored.TracePath(), MetricsPath: stored.MetricsPath(),
		Start: func(ctx context.Context) (agentops.TaskApplication, error) {
			return f.newApplication(ctx, request, stored, taskProvider)
		},
	}, nil
}

func (f *agentOpsTaskExecutionFactory) newApplication(
	ctx context.Context,
	request agentops.TaskExecutionRequest,
	stored *session.StoredSession,
	taskProvider agentOpsTaskProvider,
) (*app.RuntimeApplication, error) {
	workingMemory := memory.NewSessionStore(f.workDir, stored.RootDir)
	if err := workingMemory.EnsureFiles(); err != nil {
		return nil, err
	}
	autoMemory := automemory.NewStore(f.store.HomeDir(), f.workDir)
	hooks := automemory.NewPerRunHooks(taskProvider, autoMemory, f.workDir)
	tracker := hooks.NewTracker()
	evidence := agentOpsPermissionEvidence(f.store, stored.ID, request.Prompt)
	permissionPort := feishu.NewPermissionPort(request.Task.ChatID, f.messenger, f.approvals)
	coordinator := permission.NewCoordinator(permission.Config{
		State: permission.NewState(permission.ModeAsk, false), Workspace: f.workDir, CWD: f.workDir,
		Source: permission.SourceMain,
		Approver: agentOpsApplicationPermissionApprover{
			port: permissionPort, taskID: request.Task.TaskID,
		},
		Evidence: evidence,
	})
	compactionConfig := compaction.DefaultCompactionConfig()
	compactionConfig.Model = taskProvider.model
	compactionConfig.SessionDir = stored.RootDir
	compactionConfig.TranscriptPath = stored.TranscriptPath()
	compactor, err := compaction.NewCompactor(taskProvider, compactionConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化 Compactor 失败: %w", err)
	}
	assets := &agentOpsRunAssets{}
	dependencies := foxruntime.HarnessDependencies{
		NewArtifactJournal: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.SessionArtifactJournal, error) {
			return assets.journalFor(assembly)
		},
		NewTelemetryJournal: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.TelemetryJournal, error) {
			return assets.journalFor(assembly)
		},
		NewModel: func(_ context.Context, assembly foxruntime.RunAssembly) (engine.ModelInvoker, error) {
			journal, err := assets.journalFor(assembly)
			if err != nil {
				return nil, err
			}
			return journal.WrapModel(modelinvoke.New(taskProvider, modelinvoke.Config{OnSuccess: compactor.ResetCircuitBreaker})), nil
		},
		NewTools: func(_ context.Context, assembly foxruntime.RunAssembly) (engine.ToolExecutor, error) {
			journal, err := assets.journalFor(assembly)
			if err != nil {
				return nil, err
			}
			registry := f.buildTools(stored, taskProvider, coordinator, evidence)
			capabilities := registryexec.CapabilitiesWithContext(
				registry, assembly.AllowedTools,
				func(ctx context.Context) context.Context {
					return tools.WithRunContext(ctx, string(assembly.Session.ID), string(assembly.Run.RunID))
				},
				hooks.RecordCallback(tracker),
			)
			base := toolruntime.New(capabilities, toolresult.OSFileSystem{}, filepath.Join(assembly.Session.RootDir, "tool-results"))
			return journal.WrapTools(base), nil
		},
		NewPolicy: func(context.Context, foxruntime.RunAssembly) (engine.TurnPolicy, error) {
			return newAgentOpsTurnPolicy(stored.RootDir), nil
		},
		NewContext: func(_ context.Context, _ foxruntime.RunAssembly) (foxruntime.ContextCollector, foxruntime.ContextCompactor, error) {
			collector := foxruntime.NewPromptCollector(f.workDir).WithMemory(workingMemory.WorkingMemoryPath()).WithAutoMemory(autoMemory, automemory.MainMemoryGuidance)
			return collector, runtimecompaction.New(compactor), nil
		},
	}
	harness, err := foxruntime.NewRuntimeHarness(f.store, dependencies)
	if err != nil {
		return nil, err
	}
	agentSession, err := harness.OpenSession(ctx, foxruntime.AgentOpsTask, stored.ID)
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = agentSession.Close(context.Background())
		}
	}()
	application, err := app.NewRuntimeApplication(app.RuntimeApplicationConfig{
		Session: agentSession,
		Info: app.SessionInfo{
			ID: string(stored.ID), Directory: stored.RootDir, TranscriptPath: stored.TranscriptPath(),
		},
		RunSpec: foxruntime.RunSpec{
			ProviderProtocol: taskProvider.protocol, Model: taskProvider.model,
			WorkDir: f.workDir, LogDir: f.logDir, Task: request.Task.Text,
		},
		AfterRun: func(_ context.Context, result foxruntime.RunResult, _ error) {
			if !result.ReturnedResult() {
				return
			}
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("[AgentOps Runtime] memory extraction launch panic recovered: %v", recovered)
				}
			}()
			hooks.Fire(stored, string(result.RunID), tracker)
		},
		Drain: func(ctx context.Context) error {
			return errors.Join(agentSession.RecoverRunFinish(ctx), agentSession.Close(ctx))
		},
	})
	if err != nil {
		return nil, err
	}
	failed = false
	return application, nil
}

/* newAgentOpsTurnPolicy builds the run policy for one AgentOps task session
 * root, binding the TODO completion gate to that session's checklist. */
func newAgentOpsTurnPolicy(sessionRoot string) engine.TurnPolicy {
	return turnpolicy.New(turnpolicy.Config{Bind: func(context.Context, engine.RunInput) (turnpolicy.Bindings, error) {
		return turnpolicy.Bindings{
			TODOGate: func(context.Context) (string, error) {
				return todopolicy.CompletionReminder(sessionRoot, true), nil
			},
		}, nil
	}})
}

func (f *agentOpsTaskExecutionFactory) buildTools(
	stored *session.StoredSession,
	taskProvider agentOpsTaskProvider,
	coordinator *permission.Coordinator,
	evidence permission.EvidenceProvider,
) tools.Registry {
	registry := tools.NewRegistry()
	registry.Register(agentops.NewLogSearchTool(f.logDir))
	registry.Register(tools.NewReadFileTool(f.workDir))
	registry.Register(tools.NewWriteFileTool(f.workDir))
	registry.Register(tools.NewBashTool(f.workDir))
	registry.Register(tools.NewEditFileTool(f.workDir))
	registry.Register(tools.NewReadTodoTool(stored.RootDir))
	registry.Register(tools.NewUpdateTodoTool(stored.RootDir))
	child := childruntime.New(childruntime.Config{
		Provider: taskProvider, WorkDir: f.workDir, HomeDir: f.store.HomeDir(),
		ParentProfile:    childruntime.AgentOpsTask,
		ProviderProtocol: taskProvider.protocol, Model: taskProvider.model,
		Permission: coordinator, ParentEvidence: evidence,
	})
	registry.Register(subagent.NewTool(child, string(stored.ID)))
	return permission.DecorateRegistry(registry, coordinator, evidence)
}

type agentOpsTaskProvider struct {
	provider.LLMProvider
	protocol string
	model    string
}

type agentOpsProviderMetadataSource interface {
	ProviderProtocol() string
	ModelName() string
}

func snapshotAgentOpsProvider(modelProvider provider.LLMProvider) agentOpsTaskProvider {
	result := agentOpsTaskProvider{LLMProvider: modelProvider}
	if isNilProvider(modelProvider) {
		return result
	}
	metadata, ok := modelProvider.(agentOpsProviderMetadataSource)
	if !ok {
		return result
	}
	result.protocol = metadata.ProviderProtocol()
	result.model = metadata.ModelName()
	return result
}

func isNilProvider(value provider.LLMProvider) bool {
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

func (p agentOpsTaskProvider) ProviderProtocol() string {
	return p.protocol
}

func (p agentOpsTaskProvider) ModelName() string {
	return p.model
}

type agentOpsApplicationPermissionApprover struct {
	port   app.PermissionPort
	taskID string
}

func (a agentOpsApplicationPermissionApprover) Approve(ctx context.Context, request permission.ApprovalRequest) (permission.UserDecision, error) {
	invocation, _ := tools.InvocationContextFrom(ctx)
	correlationID := invocation.ToolCallID
	if correlationID == "" {
		correlationID = request.Request.ToolCall.ID
	}
	if correlationID == "" {
		correlationID = a.taskID
	}
	effects := make([]string, len(request.Request.Capabilities.Effects))
	for index, effect := range request.Request.Capabilities.Effects {
		effects[index] = string(effect)
	}
	mapped := app.PermissionRequest{
		Correlation: app.InteractionCorrelation{
			ID: correlationID, SessionID: invocation.SessionID,
			RunID: invocation.RunID, ToolCallID: request.Request.ToolCall.ID,
		},
		ToolName: request.Request.ToolName, Arguments: request.Request.Arguments,
		Action: request.Request.Action, Risk: string(request.Request.Risk), Source: string(request.Request.Source),
		CWD: request.Request.CWD, Workspace: request.Request.Workspace, Effects: effects,
		ReviewerFailure: request.ReviewerFailure,
	}
	if request.Review != nil {
		mapped.ReviewerReason = request.Review.Rationale
	}
	response, err := a.port.RequestPermission(ctx, mapped)
	if err != nil {
		return permission.UserDecision{}, err
	}
	if response.CorrelationID != correlationID {
		return permission.UserDecision{}, fmt.Errorf("AgentOps permission response correlation %q does not match %q", response.CorrelationID, correlationID)
	}
	switch response.Decision {
	case app.PermissionAllowOnce:
		return permission.UserDecision{Kind: permission.UserAllowOnce}, nil
	case app.PermissionAllowSession:
		return permission.UserDecision{Kind: permission.UserAllowSession}, nil
	case app.PermissionDenyWithFeedback:
		return permission.UserDecision{Kind: permission.UserDenyFeedback, Feedback: response.Feedback}, nil
	default:
		return permission.UserDecision{Kind: permission.UserDeny}, nil
	}
}

func agentOpsPermissionEvidence(store *session.FileStore, sessionID session.ID, currentPrompt string) permission.EvidenceProvider {
	return func(request permission.Request) permission.Evidence {
		var messages []schema.Message
		stored, err := store.Open(sessionID)
		if err == nil {
			records, loadErr := session.NewMessageLog(stored).LoadRecords()
			if loadErr == nil {
				messages = make([]schema.Message, 0, len(records)+1)
				for _, record := range records {
					messages = append(messages, record.Message)
				}
			}
		}
		if strings.TrimSpace(currentPrompt) != "" && !containsAgentOpsUserMessage(messages, currentPrompt) {
			messages = append(messages, schema.Message{Role: schema.RoleUser, Content: currentPrompt})
		}
		return permission.BuildEvidence(messages, nil, request)
	}
}

func containsAgentOpsUserMessage(messages []schema.Message, content string) bool {
	for _, message := range messages {
		if message.Role == schema.RoleUser && message.ToolCallID == "" && message.Content == content {
			return true
		}
	}
	return false
}

type agentOpsRunAssets struct {
	mu      sync.Mutex
	journal *runtimejournal.Journal
	runID   session.RunID
}

func (a *agentOpsRunAssets) journalFor(assembly foxruntime.RunAssembly) (*runtimejournal.Journal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.journal != nil {
		if a.runID != assembly.Run.RunID {
			return nil, errors.New("AgentOps runtime assets cannot span multiple runs")
		}
		return a.journal, nil
	}
	journal, err := runtimejournal.New(assembly)
	if err != nil {
		return nil, err
	}
	a.journal = journal
	a.runID = assembly.Run.RunID
	return journal, nil
}
