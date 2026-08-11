// Package subagent provides isolated sub-task execution within the foxharness
// agent framework. A Manager spins up a dedicated engine and session for each
// delegated task, optionally restricting the subagent to read-only tools, and
// returns a high-density report to the parent agent.
package subagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// AgentID is the normalized identity of a child agent definition.
type AgentID string

const (
	// AgentGeneralPurpose is the built-in ChildRun agent used by delegation
	// and by fork skills that omit an explicit agent.
	AgentGeneralPurpose AgentID = "general-purpose"
)

type childAgent struct {
	id           AgentID
	persona      string
	allowedTools []string
}

func resolveAgent(raw AgentID) (childAgent, error) {
	id := AgentID(strings.TrimSpace(string(raw)))
	if id == "" {
		id = AgentGeneralPurpose
	}
	if id != AgentGeneralPurpose {
		return childAgent{}, fmt.Errorf("subagent: unknown agent %q", id)
	}
	return childAgent{
		id:      id,
		persona: "通用编码执行代理，严格服从当前 ChildRun 的任务和能力边界。",
	}, nil
}

func narrowAgentTools(caller []string, agent childAgent) []string {
	if len(agent.allowedTools) == 0 {
		return cloneToolNames(caller)
	}
	if len(caller) == 0 {
		if caller != nil {
			return []string{}
		}
		return cloneToolNames(agent.allowedTools)
	}
	agentCeiling := make(map[string]struct{}, len(agent.allowedTools))
	for _, name := range agent.allowedTools {
		agentCeiling[name] = struct{}{}
	}
	effective := make([]string, 0, len(caller))
	for _, name := range caller {
		if _, allowed := agentCeiling[name]; allowed {
			effective = append(effective, name)
		}
	}
	return effective
}

func cloneToolNames(names []string) []string {
	if names == nil {
		return nil
	}
	return append(make([]string, 0, len(names)), names...)
}

// Request describes a subagent task, including the parent session reference,
// the task description, and whether the subagent should operate in read-only
// mode.
type Request struct {
	ParentSessionID string
	Task            string
	ReadOnly        bool
	Agent           AgentID

	// AllowedTools, when non-nil, restricts the sub-agent's tool registry to
	// exactly the named tools; an explicit empty slice permits no tools. The filter is applied on
	// top of the base registry (after ReadOnly trims write/edit), so
	// callers that pass an allow-list overlapping with read-only get
	// the intersection. Used by slash fork-mode skills with
	// `allowed-tools` to honor the constraint inside the sub-agent.
	AllowedTools []string
}

// Result is the single typed terminal outcome for one ChildRun invocation. It
// carries correlation and lineage throughout admission, startup, and execution;
// Report is final on success and the latest committed assistant text otherwise.
type Result struct {
	InvocationID    string
	SessionID       string
	RunID           string
	ParentSessionID string
	Agent           AgentID
	Status          OutcomeStatus
	Report          string
}

// DefaultMaxTurns is the default maximum number of turns a subagent engine may
// execute before its turn budget is considered exhausted. It is sized for
// real-world coding subtasks and aligns with the subagent turn budget used by
// Claude Code. Production callers receive it automatically via NewManager;
// internal and test callers may override it with Manager.WithMaxTurns.
const DefaultMaxTurns = 200

// Manager creates and runs isolated subagent sessions using a shared LLM
// provider and workspace root.
type Manager struct {
	provider          provider.LLMProvider
	executionSnapshot childExecutionSnapshot
	workDir           string
	// homeDir roots the cross-session persistent memory store so subagents read
	// the same merged index as top-level runs. It defaults to the user home.
	homeDir string
	// maxTurns is the turn budget handed to the subagent engine. It defaults to
	// DefaultMaxTurns (applied by NewManager) and may be overridden via
	// WithMaxTurns, primarily for deterministic tests of the exhaustion path.
	maxTurns          int
	permissions       *permission.Coordinator
	parentEvidence    permission.EvidenceProvider
	compactorFactory  func(*session.Session) (*compaction.Compactor, error)
	createSession     func(session.CreateOptions) (*session.Session, error)
	supervisorFactory func() childRunSupervisor
}

type childRunSupervisor interface {
	tools.BashCommandRunner
	Cleanup(context.Context) error
}

type childProviderMetadata interface {
	ProviderProtocol() string
	ModelName() string
}

type childExecutionSnapshot struct {
	providerProtocol string
	model            string
	contextWindow    int
}

func snapshotChildExecution(p provider.LLMProvider) childExecutionSnapshot {
	snapshot := childExecutionSnapshot{contextWindow: compaction.DefaultContextWindow}
	if metadata, ok := p.(childProviderMetadata); ok {
		snapshot.providerProtocol = metadata.ProviderProtocol()
		snapshot.model = metadata.ModelName()
	}
	snapshot.contextWindow = compaction.NewModelRegistry().Lookup(snapshot.model)
	return snapshot
}

// NewManager creates a Manager that delegates LLM calls to p and roots
// subagent sessions under workDir. The persistent memory store uses the user
// home directory, matching top-level runs.
func NewManager(p provider.LLMProvider, workDir string) *Manager {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		homeDir = "."
	}
	return &Manager{
		provider:          p,
		executionSnapshot: snapshotChildExecution(p),
		workDir:           workDir,
		homeDir:           homeDir,
		maxTurns:          DefaultMaxTurns,
	}
}

// WithMaxTurns overrides the subagent turn budget and returns the receiver for
// chaining. It is intended for internal and test injection; production callers
// use the DefaultMaxTurns default applied by NewManager.
func (m *Manager) WithMaxTurns(n int) *Manager {
	m.maxTurns = n
	return m
}

// WithPermission makes delegated tool calls use the parent TUI coordinator.
func (m *Manager) WithPermission(coordinator *permission.Coordinator) *Manager {
	m.permissions = coordinator.WithSource(permission.SourceSubagent)
	return m
}

// WithParentEvidence supplies live parent-session context for child reviews.
func (m *Manager) WithParentEvidence(provider permission.EvidenceProvider) *Manager {
	m.parentEvidence = provider
	return m
}

// PermissionEnforced reports whether child registries inherit a coordinator.
func (m *Manager) PermissionEnforced() bool {
	return m != nil && m.permissions != nil
}

// buildComposer assembles the subagent system-prompt composer, injecting the
// cross-session persistent memory index (read-only) so delegated tasks share the
// project/user memory that top-level runs see. Subagents do not write or
// extract memory; that remains the main agent's responsibility.
func (m *Manager) buildComposer(sess *session.Session, snapshots ...*childToolSnapshot) *prompt.Composer {
	store := automemory.NewStore(m.homeDir, m.workDir)
	composer := prompt.NewComposer(m.workDir).WithReadOnlyMemory(sess.MemoryPath()).WithReadOnlyAutoMemory(store)
	if len(snapshots) > 0 && snapshots[0] != nil {
		composer = composer.WithToolCapabilities(snapshots[0].capabilityNames())
	}
	return composer
}

func (m *Manager) buildRegistry(readOnly bool, allowedTools []string, childSessions ...*session.Session) *childToolSnapshot {
	return m.buildRegistryWithSupervisor(readOnly, allowedTools, nil, childSessions...)
}

func (m *Manager) buildRegistryWithSupervisor(readOnly bool, allowedTools []string, supervisor tools.BashCommandRunner, childSessions ...*session.Session) *childToolSnapshot {
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(m.workDir))
	if readOnly {
		registry.Register(tools.NewReadOnlyBashTool(m.workDir))
	} else if supervisor != nil {
		registry.Register(tools.NewSupervisedBashTool(m.workDir, supervisor))
	} else {
		registry.Register(tools.NewBashTool(m.workDir))
	}

	if !readOnly {
		registry.Register(tools.NewWriteFileTool(m.workDir))
		registry.Register(tools.NewEditFileTool(m.workDir))
	}

	var evidenceProvider permission.EvidenceProvider
	if len(childSessions) > 0 && childSessions[0] != nil {
		childSession := childSessions[0]
		evidenceProvider = func(request permission.Request) permission.Evidence {
			parent := permission.BuildEvidence(nil, nil, request)
			if m.parentEvidence != nil {
				parent = m.parentEvidence(request)
			}
			messages, _ := session.NewMessageLog(childSession).LoadMessages()
			return permission.BuildChildEvidence(parent, messages, request)
		}
	}
	if allowedTools != nil {
		registry = tools.NewFilteredRegistry(registry, allowedTools)
	}
	decorated := permission.DecorateRegistry(registry, m.permissions, evidenceProvider)
	return newChildToolSnapshot(decorated)
}

// Run executes the subagent task described by req. It creates a new session,
// builds a scoped tool registry (read-only when requested), and runs the
// engine for up to the configured turn budget (DefaultMaxTurns by default).
// The returned Result is non-nil for every admitted, rejected, or failed
// invocation and retains any identities established before termination.
func (m *Manager) Run(ctx context.Context, req Request) (outcome *Result, resultErr error) {
	outcome = newChildOutcome(req)
	agent, err := resolveAgent(req.Agent)
	if err != nil {
		outcome.Status = OutcomeRejected
		return outcome, err
	}
	outcome.Agent = agent.id

	createSession := m.createSession
	if createSession == nil {
		createSession = session.NewManager(m.workDir).Create
	}
	sess, err := createSession(session.CreateOptions{
		Source:          session.SOURCESubagent,
		WorkDir:         m.workDir,
		UserID:          "subagent-of-" + req.ParentSessionID,
		ParentSessionID: req.ParentSessionID,
		Agent:           string(agent.id),
	})

	if err != nil {
		return outcome, err
	}
	outcome.SessionID = sess.ID

	supervisor := childRunSupervisor(tools.NewBashProcessSupervisor())
	if m.supervisorFactory != nil {
		supervisor = m.supervisorFactory()
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	recorder := &outcomeRecorder{}
	defer func() {
		if recovered := recover(); recovered != nil {
			resultErr = fmt.Errorf("subagent execution panic: %v", recovered)
			outcome.RunID = recorder.runID
			outcome.Report = recorder.report
			outcome.Status = OutcomeFailed
		}
		cancelRun()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if cleanupErr := supervisor.Cleanup(cleanupCtx); cleanupErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("subagent process cleanup failed: %w", cleanupErr))
			outcome.Status = OutcomeFailed
		}
	}()

	registry := m.buildRegistryWithSupervisor(req.ReadOnly, narrowAgentTools(req.AllowedTools, agent), supervisor, sess)
	composer := m.buildComposer(sess, registry)
	eng := engine.NewAgentEngine(
		m.provider,
		registry,
		m.workDir,
		composer,
		engine.Config{
			EnableThinking:   false,
			MaxTurns:         m.maxTurns,
			ProviderProtocol: m.executionSnapshot.providerProtocol,
			Model:            m.executionSnapshot.model,
		},
	)
	compactor, err := m.newCompactor(sess)
	if err != nil {
		return outcome, err
	}
	eng.WithCompactor(compactor)

	subPrompt := fmt.Sprintf(`
你是一个 Subagent，负责为主 Agent 完成一个边界清晰的子任务。

约束：
- 只回答子任务，不要扩展目标。
- 优先使用只读探索。
- 如果需要修改文件但未被明确允许，必须拒绝。
- 最终只返回高密度报告，不要输出冗长原始日志。

父 Session: %s
Agent: %s
角色: %s

子任务：
%s
`, req.ParentSessionID, agent.id, agent.persona, req.Task)

	result, err := eng.RunWithReporter(runCtx, sess, subPrompt, recorder)
	outcome.RunID = recorder.runID
	outcome.Report = recorder.report
	if result != nil {
		outcome.RunID = result.RunID
	}
	outcome.Status = classifyOutcome(err, outcome.RunID)
	return outcome, err
}

func (m *Manager) newCompactor(sess *session.Session) (*compaction.Compactor, error) {
	if m.compactorFactory != nil {
		return m.compactorFactory(sess)
	}
	cfg := compaction.DefaultCompactionConfig()
	cfg.Model = m.executionSnapshot.model
	cfg.ContextWindow = m.executionSnapshot.contextWindow
	cfg.SessionDir = sess.RootDir
	cfg.TranscriptPath = sess.TranscriptPath()
	return compaction.NewCompactor(m.provider, cfg)
}
