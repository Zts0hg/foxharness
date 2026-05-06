package compaction

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type TokenEstimator interface {
	Estimate(messages []schema.Message) int
}

type RoughEstimator struct{}

func (RoughEstimator) Estimate(messages []schema.Message) int {
	chars := 0
	for _, msg := range messages {
		chars += utf8.RuneCountInString(msg.Content)
		for _, call := range msg.ToolCalls {
			chars += utf8.RuneCountInString(call.Name)
			chars += utf8.RuneCount(call.Arguments)
		}
	}

	if chars == 0 {
		return 0
	}

	return chars + 1
}

type Config struct {
	MaxTokens        int
	SoftRatio        float64
	RecentKeep       int
	SummaryMaxTokens int
}

func DefaultConfig() Config {
	return Config{
		MaxTokens:        128000,
		SoftRatio:        0.75,
		RecentKeep:       12,
		SummaryMaxTokens: 2048,
	}

}

type Compactor struct {
	provider  provider.LLMProvider
	estimator TokenEstimator
	config    Config
}

func NewCompactor(p provider.LLMProvider, estimator TokenEstimator, config Config) *Compactor {
	return &Compactor{
		provider:  p,
		estimator: estimator,
		config:    config,
	}
}

func (c *Compactor) MaybeCompact(ctx context.Context, messages []schema.Message) ([]schema.Message, error) {
	used := c.estimator.Estimate(messages)
	threshold := int(float64(c.config.MaxTokens) * c.config.SoftRatio)

	if used < threshold {
		return messages, nil
	}

	if len(messages) <= c.config.RecentKeep+2 {
		return messages, nil
	}

	system := messages[0]
	keepStart := 1
	var anchors []schema.Message
	if len(messages) > 1 && messages[1].Role == schema.RoleUser && messages[1].ToolCallID == "" {
		anchors = append(anchors, messages[1])
		keepStart = 2
	}

	split := len(messages) - c.config.RecentKeep
	if split < keepStart {
		return messages, nil
	}
	split = moveSplitToProtocolBoundary(messages, split, keepStart)
	if split <= keepStart {
		return messages, nil
	}

	old := messages[keepStart:split]
	recent := messages[split:]
	summary, err := c.summarize(ctx, old)
	if err != nil {
		return messages, fmt.Errorf("context compaction 失败: %w", err)
	}

	compactedSystem := system
	compactedSystem.Content = strings.TrimSpace(stripExistingCompactionSummary(system.Content)) +
		"\n\n## Compacted Context Summary\n\n" +
		"以下是较早会话历史的压缩摘要。它替代了已被压缩的原始消息，用于帮助你延续任务上下文。\n\n" +
		summary

	compacted := []schema.Message{compactedSystem}
	compacted = append(compacted, anchors...)
	compacted = append(compacted, recent...)
	return compacted, nil
}

func moveSplitToProtocolBoundary(messages []schema.Message, split int, min int) int {
	if split >= len(messages) {
		return split
	}

	for split > min && messages[split].ToolCallID != "" {
		split--
	}

	return split
}

func stripExistingCompactionSummary(content string) string {
	marker := "\n\n## Compacted Context Summary\n\n"
	if idx := strings.Index(content, marker); idx >= 0 {
		return content[:idx]
	}
	return content
}

func (c *Compactor) summarize(ctx context.Context, old []schema.Message) (string, error) {
	text := renderMessagesForSummary(old)
	prompt := fmt.Sprintf(`
请将以下 Agent 会话历史压缩成一份高密度中文摘要。

必须保留：
- 用户的原始目标和约束。
- 已经确认的关键事实。
- 已经修改过的文件和修改意图。
- 失败过的命令、错误原因和修复尝试。
- 当前尚未解决的问题。
- 下一步最合理的行动建议。

不要保留：
- 大段原始文件内容。
- 重复日志。
- 与任务无关的寒暄。

会话历史如下：

%s
`, text)
	resp, err := c.provider.Generate(ctx, []schema.Message{
		{Role: schema.RoleUser, Content: prompt},
	}, nil)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp.Content), nil

}

func renderMessagesForSummary(messages []schema.Message) string {
	var b strings.Builder
	for i, msg := range messages {
		b.WriteString(fmt.Sprintf("\n--- message %d role=%s ---\n", i+1, msg.Role))

		if msg.Content != "" {
			b.WriteString(truncate(msg.Content, 4000))
			b.WriteByte('\n')
		}

		for _, call := range msg.ToolCalls {
			b.WriteString(fmt.Sprintf(
				"[tool_call] id=%s name=%s args=%s\n",
				call.ID,
				call.Name,
				truncate(string(call.Arguments), 1000),
			))
		}

		if msg.ToolCallID != "" {
			b.WriteString(fmt.Sprintf("[tool_result_for] %s\n", msg.ToolCallID))
		}
	}

	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated for compaction]..."
}
