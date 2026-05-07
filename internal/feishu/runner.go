package feishu

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type Runner struct {
	provider  provider.LLMProvider
	registry  tools.Registry
	workDir   string
	Messenger *Messenger
}

func NewRunner(provider provider.LLMProvider, registry tools.Registry, workDir string, messenger *Messenger) *Runner {
	return &Runner{provider: provider, registry: registry, workDir: workDir, Messenger: messenger}
}

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

	_ = r.Messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("已收到任务 %s，开始执行。", task.TaskID))

	taskPrompt := fmt.Sprintf(
		"以下任务来自飞书用户 %s，消息 ID 为 %s。\n\n%s",
		task.SenderID,
		task.MessageID,
		task.Text,
	)

	manager := session.NewManager(r.workDir)
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCEFeishu,
		WorkDir: r.workDir,
		UserID:  task.SenderID,
		ChatID:  task.ChatID,
	})
	if err != nil {
		_ = r.Messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("创建 Session 失败: %v", err))
	}

	_ = r.Messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("任务已进入 Session: %s", sess.ID))

	composer := prompt.NewComposer(r.workDir).WithMemory(sess.MemoryPath())
	eng := engine.NewAgentEngine(r.provider, r.registry, r.workDir, true, composer)
	eng.WithCompactor(
		compaction.NewCompactor(
			r.provider,
			compaction.RoughEstimator{},
			compaction.DefaultConfig(),
		),
	)

	result, err := eng.Run(runCtx, sess, taskPrompt)
	if err != nil {
		log.Printf("[Feishu Runner] task=%s session=%s  failed: %v", task.TaskID, sess.ID, err)
		_ = r.Messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("Session %s 执行失败：%v", sess.ID, err))
		return
	}

	if result != nil && result.FinalMessage != "" {
		_ = r.Messenger.SendText(runCtx, task.ChatID, result.FinalMessage)
		return
	}

	_ = r.Messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("任务 %s 执行完成，Session: %s", task.TaskID, sess.ID))
}
