package feishu

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Zts0hg/foxharness/internal/engine"
)

// Reporter streams engine lifecycle events back to the Feishu chat that
// originated a task.
type Reporter struct {
	messenger *Messenger
	chatID    string
	taskID    string
}

// NewReporter creates a Feishu-backed engine reporter for one task.
func NewReporter(messenger *Messenger, chatID, taskID string) *Reporter {
	return &Reporter{
		messenger: messenger,
		chatID:    chatID,
		taskID:    taskID,
	}
}

func (r *Reporter) OnThinking(ctx context.Context, turn int) {
	r.send(ctx, fmt.Sprintf("任务 %s：第 %d 轮正在规划。", r.taskID, turn))
}

func (r *Reporter) OnToolCall(ctx context.Context, toolName string, args string) {
	msg := fmt.Sprintf(
		"任务 %s：准备执行工具 %s。\n参数：%s",
		r.taskID,
		toolName,
		truncateFeishuText(args, 500),
	)
	r.send(ctx, msg)
}

func (r *Reporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	if isError {
		r.send(ctx, fmt.Sprintf(
			"任务 %s：工具 %s 执行失败。\n%s",
			r.taskID,
			toolName,
			truncateFeishuText(result, 800),
		))
		return
	}

	result = strings.TrimSpace(result)
	if result == "" {
		r.send(ctx, fmt.Sprintf("任务 %s：工具 %s 执行成功。", r.taskID, toolName))
		return
	}

	r.send(ctx, fmt.Sprintf(
		"任务 %s：工具 %s 执行成功。\n输出摘要：%s",
		r.taskID,
		toolName,
		truncateFeishuText(result, 400),
	))
}

func (r *Reporter) OnMessage(ctx context.Context, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	r.send(ctx, truncateFeishuText(content, 1800))
}

func (r *Reporter) send(ctx context.Context, text string) {
	if r == nil || r.messenger == nil || strings.TrimSpace(text) == "" {
		return
	}
	if err := r.messenger.SendText(ctx, r.chatID, text); err != nil {
		log.Printf("[Feishu Reporter] send task=%s chat=%s failed: %v", r.taskID, r.chatID, err)
	}
}

func truncateFeishuText(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return fmt.Sprintf("%s\n... (已截断，原始内容约 %d 字节)", string(runes[:limit]), len(s))
}

var _ engine.Reporter = (*Reporter)(nil)
