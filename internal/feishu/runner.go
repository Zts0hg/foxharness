package feishu

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
)

type Runner struct {
	engine    *engine.AgentEngine
	Messenger *Messenger
}

func NewRunner(engine *engine.AgentEngine, messenger *Messenger) *Runner {
	return &Runner{engine: engine, Messenger: messenger}
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

	prompt := fmt.Sprintf(
		"以下任务来自飞书用户 %s，消息 ID 为 %s。\n\n%s",
		task.SenderID,
		task.MessageID,
		task.Text,
	)

	if err := r.engine.Run(runCtx, prompt); err != nil {
		log.Printf("[Feishu Runner] task=%s failed: %v", task.TaskID, err)
		_ = r.Messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("任务 %s 执行失败：%v", task.TaskID, err))
	}

	_ = r.Messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("任务 %s 执行完成。", task.TaskID))
}
