package feishu

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Zts0hg/foxharness/internal/app"
)

/* Reporter maps application notifications to the existing Feishu task messages. */
type Reporter struct {
	messenger               TextMessenger
	chatID                  string
	taskID                  string
	deliveryFailureObserver DeliveryFailureObserver
}

/* WithDeliveryFailureObserver installs the non-blocking delivery failure observer. */
func (r *Reporter) WithDeliveryFailureObserver(observer DeliveryFailureObserver) *Reporter {
	r.deliveryFailureObserver = observer
	return r
}

/* NewReporter creates a Feishu-backed application notification sink for one task. */
func NewReporter(messenger TextMessenger, chatID, taskID string) *Reporter {
	return &Reporter{
		messenger: messenger,
		chatID:    chatID,
		taskID:    taskID,
	}
}

/* Notify maps one UI-neutral application notification without retaining run state. */
func (r *Reporter) Notify(ctx context.Context, notification app.Notification) {
	switch notification.Kind {
	case app.NotificationRunStarted:
		r.send(ctx, fmt.Sprintf("任务 %s：Run %s 已开始，Session: %s。", r.taskID, notification.RunID, notification.SessionID))
	case app.NotificationThinking:
		r.send(ctx, fmt.Sprintf("任务 %s：第 %d 轮正在规划。", r.taskID, notification.Turn))
	case app.NotificationContextCompacted:
		r.send(ctx, fmt.Sprintf("任务 %s：上下文已压缩（%s）。", r.taskID, notification.Name))
	case app.NotificationToolCall:
		r.onToolCall(ctx, notification.Name, notification.Content)
	case app.NotificationToolResult:
		r.onToolResult(ctx, notification.Name, notification.Content, notification.IsError)
	case app.NotificationMessage:
		r.onMessage(ctx, notification.Content)
	}
}

func (r *Reporter) onToolCall(ctx context.Context, toolName string, args string) {
	msg := fmt.Sprintf(
		"任务 %s：准备执行工具 %s。\n参数：%s",
		r.taskID,
		toolName,
		truncateFeishuText(args, 500),
	)
	r.send(ctx, msg)
}

func (r *Reporter) onToolResult(ctx context.Context, toolName string, result string, isError bool) {
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

func (r *Reporter) onMessage(ctx context.Context, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	r.send(ctx, truncateFeishuText(content, 1800))
}

func (r *Reporter) send(ctx context.Context, text string) {
	if r == nil || isNilDependency(r.messenger) || strings.TrimSpace(text) == "" {
		return
	}
	if err := r.messenger.SendText(ctx, r.chatID, text); err != nil {
		failure := DeliveryFailure{TaskID: r.taskID, ChatID: r.chatID, Stage: DeliveryStageLifecycle, Cause: err}
		if r.deliveryFailureObserver == nil {
			log.Printf("[Feishu Reporter] send task=%s chat=%s failed: %v", r.taskID, r.chatID, err)
			return
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[Feishu Reporter] task=%s delivery observer panic recovered: %v", r.taskID, recovered)
			}
		}()
		r.deliveryFailureObserver.ObserveDeliveryFailure(failure)
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
	suffix := fmt.Sprintf("\n... (已截断，原始内容约 %d 字节)", len(s))
	suffixRunes := []rune(suffix)
	if len(suffixRunes) >= limit {
		return string(suffixRunes[:limit])
	}
	return string(runes[:limit-len(suffixRunes)]) + suffix
}

var _ app.NotificationSink = (*Reporter)(nil)
