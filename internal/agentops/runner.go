// Package agentops provides an automated incident-analysis agent that receives
// tasks from team IM (e.g. Feishu), searches local service logs, and runs an
// LLM-powered engine loop to diagnose root causes and propose fixes.  It
// integrates plan generation, context compaction, sub-agent delegation, and a
// danger-action approval middleware so that high-risk operations require human
// confirmation before execution.
package agentops

import (
	"context"
	"fmt"
	"log"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// Messenger abstracts the ability to send a plain-text message to a chat
// identified by chatID.  Implementations are typically backed by an IM
// platform such as Feishu.
type Messenger interface {
	// SendText delivers text to the specified chat.  A non-nil error
	// indicates a delivery failure.
	SendText(ctx context.Context, chatID, text string) error
}

// Runner orchestrates a single AgentOps incident-analysis task.  It creates a
// session, generates an execution plan, wires up tools (log search, file I/O,
// bash, sub-agent) with a danger-action approval middleware, and drives the
// engine loop to completion.
type Runner struct {
	provider      provider.LLMProvider
	workDir       string
	logDir        string
	messenger     Messenger
	sessions      *session.Manager
	approvalStore *approval.Store
}

// NewRunner constructs a Runner with the given LLM provider, working and log
// directories, messenger for user notifications, and approval store for
// danger-action gating.
func NewRunner(
	p provider.LLMProvider,
	workDir, logDir string,
	messenger Messenger,
	approvalStore *approval.Store,
) *Runner {
	return &Runner{
		provider:      p,
		workDir:       workDir,
		logDir:        logDir,
		messenger:     messenger,
		sessions:      session.NewManager(workDir),
		approvalStore: approvalStore,
	}
}

// Run executes the task to completion.  On failure it logs the error and
// attempts to notify the originating chat.
func (r *Runner) Run(ctx context.Context, task Task) {
	if err := r.run(ctx, task); err != nil {
		log.Printf("[AgentOps] task=%s failed: %v", task.TaskID, err)
		_ = r.messenger.SendText(ctx, task.ChatID, fmt.Sprintf("AgentOps 任务失败： %v", err))
	}
}

func (r *Runner) run(ctx context.Context, task Task) error {
	sess, err := r.sessions.Create(session.CreateOptions{
		Source:  session.SOURCEFeishu,
		WorkDir: r.workDir,
		UserID:  task.SenderID,
		ChatID:  task.ChatID,
	})
	if err != nil {
		return err
	}

	_ = r.messenger.SendText(
		ctx,
		task.ChatID,
		fmt.Sprintf("已创建 AgentOps Session: %s\n开始分析。", sess.ID),
	)

	store := memory.NewStore(r.workDir)
	if err := store.EnsureFiles(); err != nil {
		return err
	}

	taskPrompt := BuildPrompt(task)
	enableThinking := false
	planner := memory.NewPlanner(r.provider, store)
	if err := planner.BuildPlan(ctx, taskPrompt); err != nil {
		log.Printf("[AgentOps][PlanMode] 生成计划失败, 将会退到旧版每轮 Thinking: %v", err)
		enableThinking = true
	} else {
		log.Printf("[AgentOps][PlanMode] 计划已生成，本次任务关闭每轮 Thinking")
	}

	registry := r.buildRegistry(task, sess)
	composer := prompt.NewComposer(r.workDir).WithMemory(sess.MemoryPath())

	eng := engine.NewAgentEngine(
		r.provider,
		registry,
		r.workDir,
		composer,
		engine.Config{
			EnableThinking: enableThinking,
			MaxTurns:       24,
		},
	)
	eng.WithCompactor(compaction.NewCompactor(
		r.provider,
		compaction.RoughEstimator{},
		compaction.DefaultConfig(),
	))

	result, err := eng.Run(ctx, sess, taskPrompt)
	if err != nil {
		return err
	}

	final := "任务执行完成。"
	if result != nil && result.FinalMessage != "" {
		final = result.FinalMessage
	}

	final += fmt.Sprintf(
		"\n\nSession: %s\nTrace: %s\nMetrics: %s",
		sess.ID,
		sess.TracePath(),
		sess.MetricsPath(),
	)

	return r.messenger.SendText(ctx, task.ChatID, final)

}

func (r *Runner) buildRegistry(task Task, sess *session.Session) tools.Registry {
	registry := tools.NewRegistry()

	registry.Register(NewLogSearchTool(r.logDir))
	registry.Register(tools.NewReadFileTool(r.workDir))
	registry.Register(tools.NewWriteFileTool(r.workDir))
	registry.Register(tools.NewBashTool(r.workDir))
	registry.Register(tools.NewBashTool(r.workDir))

	approver := approval.NewFeishuApprover(task.ChatID, r.messenger, r.approvalStore)
	registry.Use(middleware.NewDangerMiddleware(approver))

	subManager := subagent.NewManager(r.provider, r.workDir)
	registry.Register(subagent.NewTool(subManager, sess.ID))

	return registry
}
