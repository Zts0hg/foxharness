// Package subagent provides isolated sub-task execution within the foxharness
// agent framework. A Manager spins up a dedicated engine and session for each
// delegated task, optionally restricting the subagent to read-only tools, and
// returns a high-density report to the parent agent.
package subagent

import (
	"context"
	"fmt"
	"os"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// Request describes a subagent task, including the parent session reference,
// the task description, and whether the subagent should operate in read-only
// mode.
type Request struct {
	ParentSessionID string
	Task            string
	ReadOnly        bool

	// AllowedTools, when non-empty, restricts the sub-agent's tool
	// registry to exactly the named tools. The filter is applied on
	// top of the base registry (after ReadOnly trims write/edit), so
	// callers that pass an allow-list overlapping with read-only get
	// the intersection. Used by slash fork-mode skills with
	// `allowed-tools` to honor the constraint inside the sub-agent.
	AllowedTools []string
}

// Result holds the subagent's session identifier and the final report text
// produced by the subagent's engine run.
type Result struct {
	SessionID string
	Report    string
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
	maxTurns         int
	permissions      *permission.Coordinator
	parentEvidence   permission.EvidenceProvider
	compactorFactory func(*session.Session) (*compaction.Compactor, error)
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
func (m *Manager) buildComposer(sess *session.Session) *prompt.Composer {
	store := automemory.NewStore(m.homeDir, m.workDir)
	return prompt.NewComposer(m.workDir).WithReadOnlyMemory(sess.MemoryPath()).WithReadOnlyAutoMemory(store)
}

func (m *Manager) buildRegistry(readOnly bool, allowedTools []string, childSessions ...*session.Session) tools.Registry {
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(m.workDir))
	registry.Register(tools.NewBashTool(m.workDir))

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
	decorated := permission.DecorateRegistry(registry, m.permissions, evidenceProvider)
	if len(allowedTools) > 0 {
		return tools.NewFilteredRegistry(decorated, allowedTools)
	}
	return decorated
}

// Run executes the subagent task described by req. It creates a new session,
// builds a scoped tool registry (read-only when requested), and runs the
// engine for up to the configured turn budget (DefaultMaxTurns by default).
// The returned Result contains the session ID and the agent's final message
// as a report.
func (m *Manager) Run(ctx context.Context, req Request) (*Result, error) {
	manager := session.NewManager(m.workDir)
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCESubagent,
		WorkDir: m.workDir,
		UserID:  "subagent-of-" + req.ParentSessionID,
	})

	if err != nil {
		return nil, err
	}

	registry := m.buildRegistry(req.ReadOnly, req.AllowedTools, sess)
	composer := m.buildComposer(sess)
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
		return nil, err
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

子任务：
%s
`, req.ParentSessionID, req.Task)

	result, err := eng.Run(ctx, sess, subPrompt)
	if err != nil {
		return nil, err
	}

	report := ""
	if result != nil {
		report = result.FinalMessage
	}

	return &Result{
		SessionID: sess.ID,
		Report:    report,
	}, nil
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
