package feishu

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// Runner consumes Task values from a channel and executes each one in a
// dedicated goroutine using the full agent engine stack: session creation,
// tool registration (file I/O, bash, sub-agent), danger-action approval
// middleware, context compaction, and a 5-minute per-task timeout.
type Runner struct {
	provider       provider.LLMProvider
	workDir        string
	messenger      *Messenger
	sessionManager *session.Manager
	approvalStore  *approval.Store
}

// NewRunner constructs a Runner with the given LLM provider, working
// directory, Feishu messenger for user notifications, session manager, and
// approval store.
func NewRunner(
	provider provider.LLMProvider,
	workDir string,
	messenger *Messenger,
	sessionManager *session.Manager,
	approvalStore *approval.Store,
) *Runner {
	return &Runner{
		provider:       provider,
		workDir:        workDir,
		messenger:      messenger,
		sessionManager: sessionManager,
		approvalStore:  approvalStore,
	}
}

// Start begins consuming tasks from the tasks channel.  Each task is
// dispatched to a separate goroutine.  Start blocks until the context is
// cancelled or the tasks channel is closed.
func (r *Runner) Start(ctx context.Context, tasks <-chan Task) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-tasks:
			if !ok {
				return
			}
			go r.runOne(ctx, task)
		}
	}
}

func (r *Runner) runOne(ctx context.Context, task Task) {
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("已收到任务 %s，开始执行。", task.TaskID))

	sess, err := r.sessionManager.Create(session.CreateOptions{
		Source:  session.SOURCEFeishu,
		WorkDir: r.workDir,
		UserID:  task.SenderID,
		ChatID:  task.ChatID,
	})
	if err != nil {
		_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("创建 Session 失败: %v", err))
		return
	}

	_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("任务已进入 Session: %s", sess.ID))

	approver := approval.NewFeishuApprover(task.ChatID, r.messenger, r.approvalStore)
	subManager := subagent.NewManager(r.provider, r.workDir)

	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(r.workDir))
	registry.Register(tools.NewWriteFileTool(r.workDir))
	registry.Register(tools.NewEditFileTool(r.workDir))
	registry.Register(tools.NewBashTool(r.workDir))
	registry.Register(subagent.NewTool(subManager, sess.ID))
	registry.Use(middleware.NewDangerMiddleware(approver))

	composer := prompt.NewComposer(r.workDir).WithMemory(sess.MemoryPath())
	eng := engine.NewAgentEngine(
		r.provider,
		registry,
		r.workDir,
		composer,
		engine.Config{
			EnableThinking: false,
			MaxTurns:       20,
		},
	)
	eng.WithCompactor(
		compaction.NewCompactor(
			r.provider,
			compaction.RoughEstimator{},
			compaction.DefaultConfig(),
		),
	)
	taskPrompt := fmt.Sprintf(
		"以下任务来自飞书用户 %s，消息 ID 为 %s。\n\n%s",
		task.SenderID,
		task.MessageID,
		task.Text,
	)

	reporter := NewReporter(r.messenger, task.ChatID, task.TaskID)
	result, err := eng.RunWithReporter(runCtx, sess, taskPrompt, reporter)
	if err != nil {
		log.Printf("[Feishu Runner] task=%s session=%s  failed: %v", task.TaskID, sess.ID, err)
		_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("Session %s 执行失败：%v", sess.ID, err))
		return
	}

	if result == nil || result.FinalMessage == "" {
		_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("任务 %s 执行完成，Session: %s", task.TaskID, sess.ID))
		return
	}

	_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("任务 %s 已完成，Session: %s", task.TaskID, sess.ID))
}
