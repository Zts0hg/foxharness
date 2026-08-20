/* Package childruntime composes the concrete target runtime for synchronous ChildRun execution. */
package childruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/compaction"
	legacycontext "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/modelinvoke"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/prompt"
	"github.com/Zts0hg/foxharness/internal/provider"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/runtimecompaction"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/toolexec"
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
	config.ParentTools = append([]string(nil), config.ParentTools...)
	if config.ParentProfile == "" {
		config.ParentProfile = CLIExec
	}
	if config.HomeDir == "" {
		config.HomeDir, _ = os.UserHomeDir()
		if config.HomeDir == "" {
			config.HomeDir = "."
		}
	}
	if metadata, ok := config.Provider.(providerMetadata); ok {
		if config.ProviderProtocol == "" {
			config.ProviderProtocol = metadata.ProviderProtocol()
		}
		if config.Model == "" {
			config.Model = metadata.ModelName()
		}
	}
	return &Runner{config: config}
}

/* PermissionEnforced reports whether nested tool calls inherit a coordinator. */
func (r *Runner) PermissionEnforced() bool {
	return r != nil && r.config.Permission != nil
}

/* Run resolves model-facing input and delegates all lifecycle work to runtime.ChildRunner. */
func (r *Runner) Run(ctx context.Context, request subagent.Request) (*subagent.Result, error) {
	if r == nil {
		return nil, errors.New("child runtime runner is required")
	}
	request.AllowedTools = append([]string(nil), request.AllowedTools...)
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
				capabilities(registry, assembly.AllowedTools), toolresult.OSFileSystem{},
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
			composer := legacycontext.NewComposer(r.config.WorkDir).
				WithReadOnlyMemory(workingMemory).
				WithReadOnlyAutoMemory(automemory.NewStore(r.config.HomeDir, r.config.WorkDir)).
				WithToolCapabilities(assembly.AllowedTools)
			return promptCollector{composer: composer}, runtimecompaction.New(compactor), nil
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

func (r *Runner) parentTools() ([]string, error) {
	if r.config.ParentTools != nil {
		return append([]string(nil), r.config.ParentTools...), nil
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

type promptCollector struct {
	composer engine.PromptComposer
}

func (c promptCollector) Collect(_ context.Context, request foxruntime.ContextCollectionRequest) ([]prompt.Fragment, error) {
	text, err := c.composer.Compose(request.Prompt)
	if err != nil {
		return nil, fmt.Errorf("compose child prompt: %w", err)
	}
	return []prompt.Fragment{prompt.Text(text)}, nil
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
		return nil, errors.New("child runtime permission scope is required")
	}
	return permission.DecorateRegistry(registry, scope.coordinator, scope.evidenceProvider(store, assembly)), nil
}

func capabilities(registry tools.Registry, allowed []string) []toolexec.Capability {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	var result []toolexec.Capability
	for _, definition := range registry.GetAvailableTools() {
		if _, ok := allowedSet[definition.Name]; !ok {
			continue
		}
		definition := definition
		result = append(result, toolexec.Capability{
			Definition: definition, ParallelSafe: registry.IsParallelSafe(definition.Name),
			Execute: func(ctx context.Context, call schema.ToolCall) engine.ToolExecutionResult {
				executed := registry.Execute(ctx, call)
				return engine.ToolExecutionResult{
					CallID: executed.ToolCallID, FullContent: executed.Output,
					ModelContent: executed.Output, ObserverContent: executed.Output, IsError: executed.IsError,
				}
			},
		})
	}
	return result
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
	request.AllowedTools = append([]string(nil), request.AllowedTools...)
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

var _ subagent.Runner = (*Runner)(nil)
