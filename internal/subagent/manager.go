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

type Request struct {
	ParentSessionID string
	Task            string
	ReadOnly        bool
}

type Result struct {
	SessionID string
	Report    string
}

type Manager struct {
	provider provider.LLMProvider
	workDir  string
}

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
		false,
		composer,
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
