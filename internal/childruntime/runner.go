/* Package childruntime composes the concrete target runtime for synchronous ChildRun execution. */
package childruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/modelinvoke"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/registryexec"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/runtimecompaction"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/toolresult"
	"github.com/Zts0hg/foxharness/internal/toolruntime"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/turnpolicy"
)

/* Config freezes concrete dependencies and the parent profile for child invocations. */
type Config struct {
	Provider          provider.LLMProvider
	WorkDir           string
	HomeDir           string
	ParentProfile     ParentProfile
	ProviderProtocol  string
	Model             string
	Effort            string
	ParentTools       []string
	Permission        *permission.Coordinator
	ParentEvidence    permission.EvidenceProvider
	MaxTurns          int
	SupervisorFactory func() *tools.BashProcessSupervisor
}

/* ParentProfile identifies the already-running root profile without exposing runtime types outward. */
type ParentProfile string

const (
	TUIInteractive  ParentProfile = "TUIInteractive"
	CLIExec         ParentProfile = "CLIExec"
	FeishuRemote    ParentProfile = "FeishuRemote"
	AgentOpsTask    ParentProfile = "AgentOpsTask"
	AutodevPipeline ParentProfile = "AutodevPipeline"
)

/* Runner adapts subagent's consumer-owned protocol to runtime.ChildRunner. */
type Runner struct {
	config Config
}

/* New freezes caller-owned configuration for future synchronous invocations. */
func New(config Config) *Runner {
	config.ParentTools = cloneToolNames(config.ParentTools)
	if config.ParentProfile == "" {
		config.ParentProfile = CLIExec
	}
	if config.HomeDir == "" {
		config.HomeDir, _ = os.UserHomeDir()
		if config.HomeDir == "" {
			config.HomeDir = "."
		}
	}
	if !isNilProvider(config.Provider) {
		if metadata, ok := config.Provider.(providerMetadata); ok {
			if config.ProviderProtocol == "" {
				config.ProviderProtocol = metadata.ProviderProtocol()
			}
			if config.Model == "" {
				config.Model = metadata.ModelName()
			}
		}
	}
	return &Runner{config: config}
}

/* PermissionEnforced reports whether nested tool calls inherit a coordinator. */
func (r *Runner) PermissionEnforced() bool {
	return r != nil && r.config.Permission != nil
}

/* DelegationAllowed distinguishes profiles that intentionally run without a human coordinator from missing required coordination. */
func (r *Runner) DelegationAllowed() bool {
	if r == nil {
		return false
	}
	if r.config.Permission != nil {
		return true
	}
	switch r.config.ParentProfile {
	case CLIExec, AutodevPipeline:
		return true
	default:
		return false
	}
}

/* Run resolves model-facing input and delegates all lifecycle work to runtime.ChildRunner. */
func (r *Runner) Run(ctx context.Context, request subagent.Request) (*subagent.Result, error) {
	if r == nil {
		return nil, errors.New("child runtime runner is required")
	}
	if !r.DelegationAllowed() {
		return nil, errors.New("child runtime permission policy is unavailable")
	}
	request.AllowedTools = cloneToolNames(request.AllowedTools)
	if request.Depth == 0 {
		request.Depth = 1
	}
	invocationID := subagent.NewInvocationID()
	delegationID := strings.TrimSpace(request.DelegationID)
	if delegationID == "" {
		delegationID = invocationID
	}
	request.DelegationID = delegationID
	agent, err := subagent.ResolveAgent(request.Agent)
	if err != nil {
		return rejectedResult(request, invocationID), err
	}
	parentTools, err := r.parentTools()
	if err != nil {
		return nil, err
	}
	store := session.NewFileStore(r.config.WorkDir)
	supervisor := tools.NewBashProcessSupervisor()
	if r.config.SupervisorFactory != nil {
		supervisor = r.config.SupervisorFactory()
	}
	if supervisor == nil {
		return nil, errors.New("child runtime supervisor is required")
	}
	var compactorMu sync.Mutex
	var compactor *compaction.Compactor
	dependencies := foxruntime.HarnessDependencies{
		InitializeSession: func(_ context.Context, snapshot foxruntime.AgentSessionSnapshot) error {
			return memory.NewSessionStore(r.config.WorkDir, snapshot.RootDir).EnsureFiles()
		},
		NewModel: func(_ context.Context, assembly foxruntime.RunAssembly) (engine.ModelInvoker, error) {
			compactorMu.Lock()
			defer compactorMu.Unlock()
			if compactor == nil {
				var err error
				compactor, err = newCompactor(r.config.Provider, r.config.Model, assembly.Session.RootDir)
				if err != nil {
					return nil, err
				}
			}
			return modelinvoke.New(r.config.Provider, modelinvoke.Config{OnSuccess: compactor.ResetCircuitBreaker}), nil
		},
		NewTools: func(_ context.Context, assembly foxruntime.RunAssembly) (engine.ToolExecutor, error) {
			registry, err := r.buildRegistry(store, assembly, supervisor)
			if err != nil {
				return nil, err
			}
			return toolruntime.New(
				registryexec.Capabilities(registry, assembly.AllowedTools, nil), toolresult.OSFileSystem{},
				filepath.Join(assembly.Session.RootDir, "tool-results"),
			), nil
		},
		NewPolicy: func(context.Context, foxruntime.RunAssembly) (engine.TurnPolicy, error) {
			return turnpolicy.New(turnpolicy.Config{}), nil
		},
		NewContext: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.ContextCollector, foxruntime.ContextCompactor, error) {
			compactorMu.Lock()
			defer compactorMu.Unlock()
			if compactor == nil {
				return nil, nil, errors.New("child runtime compactor was not initialized")
			}
			workingMemory := memory.NewSessionStore(r.config.WorkDir, assembly.Session.RootDir).WorkingMemoryPath()
			collector := foxruntime.NewPromptCollector(r.config.WorkDir).
				WithReadOnlyMemory(workingMemory).
				WithAutoMemory(automemory.NewStore(r.config.HomeDir, r.config.WorkDir), automemory.ReadOnlyMemoryGuidance).
				WithToolCapabilities(assembly.AllowedTools)
			return collector, runtimecompaction.New(compactor), nil
		},
	}
	harness, err := foxruntime.NewRuntimeHarness(store, dependencies)
	if err != nil {
		return nil, err
	}
	permissionScope := newParentPermissionScope(r.config.Permission, r.config.ParentEvidence)
	child, err := harness.NewChildRunnerFromFrozenParent(foxruntime.FrozenParentRun{
		Profile: foxruntime.ProfileName(r.config.ParentProfile), SessionID: session.ID(request.ParentSessionID),
		RunID: session.RunID(request.ParentRunID), WorkDir: r.config.WorkDir,
		ProviderProtocol: r.config.ProviderProtocol, Model: r.config.Model, Effort: r.config.Effort,
		AllowedTools: parentTools, Permission: permissionScope, Context: ctx,
	})
	if err != nil {
		return nil, err
	}
	var maxTurns *int
	if r.config.MaxTurns > 0 && r.config.MaxTurns < subagent.DefaultMaxTurns {
		value := r.config.MaxTurns
		maxTurns = &value
	}
	result, runErr := child.Run(ctx, foxruntime.ChildRunRequest{
		InvocationID: invocationID, DelegationID: delegationID, Agent: string(agent.ID),
		AgentInstructions: agent.Instructions, AgentAllowedTools: agent.AllowedTools,
		Task: request.Task, ReadOnly: request.ReadOnly, AllowedTools: request.AllowedTools,
		Depth: request.Depth, MaxTurns: maxTurns, Cleanup: supervisor,
	})
	return adaptResult(result), runErr
}

type providerMetadata interface {
	ProviderProtocol() string
	ModelName() string
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

func (r *Runner) parentTools() ([]string, error) {
	if r.config.ParentTools != nil {
		return cloneToolNames(r.config.ParentTools), nil
	}
	profile, err := foxruntime.ResolveProfile(foxruntime.ProfileName(r.config.ParentProfile))
	if err != nil {
		return nil, err
	}
	if profile.Snapshot().ToolCeiling == "" {
		return []string{}, nil
	}
	return strings.Split(profile.Snapshot().ToolCeiling, ","), nil
}

func newCompactor(modelProvider provider.LLMProvider, model, sessionDir string) (*compaction.Compactor, error) {
	config := compaction.DefaultCompactionConfig()
	config.Model = model
	config.ContextWindow = compaction.NewModelRegistry().Lookup(model)
	config.SessionDir = sessionDir
	config.TranscriptPath = filepath.Join(sessionDir, "transcript.jsonl")
	return compaction.NewCompactor(modelProvider, config)
}

func (r *Runner) buildRegistry(store *session.FileStore, assembly foxruntime.RunAssembly, supervisor *tools.BashProcessSupervisor) (tools.Registry, error) {
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(r.config.WorkDir))
	if assembly.Spec.ReadOnly {
		registry.Register(tools.NewReadOnlyBashTool(r.config.WorkDir))
	} else {
		registry.Register(tools.NewSupervisedBashTool(r.config.WorkDir, supervisor))
		registry.Register(tools.NewWriteFileTool(r.config.WorkDir))
		registry.Register(tools.NewEditFileTool(r.config.WorkDir))
	}
	registry = tools.NewFilteredRegistry(registry, assembly.AllowedTools)
	scope, ok := assembly.Permission.(*permissionScope)
	if !ok || scope == nil || scope.coordinator == nil {
		if r.DelegationAllowed() && !r.PermissionEnforced() {
			return registry, nil
		}
		return nil, errors.New("child runtime permission scope is required")
	}
	return permission.DecorateRegistry(registry, scope.coordinator, scope.evidenceProvider(store, assembly)), nil
}

type permissionScope struct {
	coordinator *permission.Coordinator
	parent      permission.EvidenceProvider
	request     *foxruntime.ChildPermissionRequest
	leaf        bool
}

func newParentPermissionScope(coordinator *permission.Coordinator, evidence permission.EvidenceProvider) foxruntime.PermissionScope {
	if coordinator == nil {
		return nil
	}
	return &permissionScope{coordinator: coordinator, parent: evidence}
}

func (s *permissionScope) ChildScope(_ context.Context, request foxruntime.ChildPermissionRequest) (foxruntime.PermissionScope, error) {
	if s == nil || s.coordinator == nil {
		return nil, errors.New("child permission coordinator is required")
	}
	if s.leaf {
		return nil, errors.New("child permission scope cannot delegate")
	}
	request.AllowedTools = cloneToolNames(request.AllowedTools)
	return &permissionScope{
		coordinator: s.coordinator.WithSource(permission.SourceSubagent), parent: s.parent,
		request: &request, leaf: true,
	}, nil
}

func (s *permissionScope) evidenceProvider(store *session.FileStore, assembly foxruntime.RunAssembly) permission.EvidenceProvider {
	return func(request permission.Request) permission.Evidence {
		parent := permission.BuildEvidence(nil, nil, request)
		if s.parent != nil {
			parent = s.parent(request)
		}
		stored, _ := store.Open(assembly.Session.ID)
		var messages []schema.Message
		if stored != nil {
			messages, _ = session.NewMessageLog(stored).LoadMessages()
		}
		evidence := permission.BuildChildEvidence(parent, messages, request)
		if s.request != nil {
			evidence.Correlation = permission.EvidenceCorrelation{
				ParentSessionID: string(s.request.ParentSessionID), ParentRunID: string(s.request.ParentRunID),
				ChildSessionID: string(s.request.ChildSessionID), ChildRunID: string(s.request.ChildRunID),
				DelegationID: s.request.DelegationID,
			}
		}
		return evidence
	}
}

func adaptResult(result foxruntime.ChildRunResult) *subagent.Result {
	return &subagent.Result{
		InvocationID: result.InvocationID, SessionID: string(result.SessionID), RunID: string(result.RunID),
		ParentSessionID: string(result.ParentSessionID), ParentRunID: string(result.ParentRunID),
		DelegationID: result.DelegationID, Agent: subagent.AgentID(result.Agent), Depth: result.Depth,
		Status: adaptStatus(result.Status), Report: result.Report,
	}
}

func rejectedResult(request subagent.Request, invocationID string) *subagent.Result {
	agent := request.Agent
	if agent == "" {
		agent = subagent.AgentGeneralPurpose
	}
	return &subagent.Result{
		InvocationID: invocationID, ParentSessionID: request.ParentSessionID,
		ParentRunID: request.ParentRunID, DelegationID: request.DelegationID,
		Agent: agent, Depth: request.Depth, Status: subagent.OutcomeRejected,
	}
}

func adaptStatus(status foxruntime.ChildOutcomeStatus) subagent.OutcomeStatus {
	switch status {
	case foxruntime.ChildSucceeded:
		return subagent.OutcomeSucceeded
	case foxruntime.ChildCancelled:
		return subagent.OutcomeCancelled
	case foxruntime.ChildTurnExhausted:
		return subagent.OutcomeTurnExhausted
	case foxruntime.ChildRejected:
		return subagent.OutcomeRejected
	case foxruntime.ChildStartFailed:
		return subagent.OutcomeStartFailed
	default:
		return subagent.OutcomeFailed
	}
}

func cloneToolNames(tools []string) []string {
	if tools == nil {
		return nil
	}
	return append([]string{}, tools...)
}

var _ subagent.Runner = (*Runner)(nil)
