// Package subagent provides isolated sub-task execution within the foxharness
// agent framework. A Manager spins up a dedicated engine and session for each
// delegated task, optionally restricting the subagent to read-only tools, and
// returns a high-density report to the parent agent.
package subagent

import (
	"context"
	"fmt"

	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
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
}

// Result holds the subagent's session identifier and the final report text
// produced by the subagent's engine run.
type Result struct {
	SessionID string
	Report    string
}

// Manager creates and runs isolated subagent sessions using a shared LLM
// provider and workspace root.
type Manager struct {
	provider provider.LLMProvider
	workDir  string
}

// NewManager creates a Manager that delegates LLM calls to p and roots
// subagent sessions under workDir.
func NewManager(p provider.LLMProvider, workDir string) *Manager {
	return &Manager{provider: p, workDir: workDir}
}

func (m *Manager) buildRegistry(readOnly bool) tools.Registry {
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(m.workDir))
	registry.Register(tools.NewBashTool(m.workDir))

	if !readOnly {
		registry.Register(tools.NewWriteFileTool(m.workDir))
		registry.Register(tools.NewEditFileTool(m.workDir))
	}

	return registry
}

// Run executes the subagent task described by req. It creates a new session,
// builds a scoped tool registry (read-only when requested), and runs the
// engine for up to 8 turns. The returned Result contains the session ID and
// the agent's final message as a report.
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

	registry := m.buildRegistry(req.ReadOnly)
	composer := prompt.NewComposer(m.workDir).WithMemory(sess.MemoryPath())
	eng := engine.NewAgentEngine(
		m.provider,
		registry,
		m.workDir,
		composer,
		engine.Config{
			EnableThinking: false,
			MaxTurns:       8,
		},
	)

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
