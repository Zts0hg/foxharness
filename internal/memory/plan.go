package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type Planner struct {
	provider provider.LLMProvider
	store    *Store
}

type planDraft struct {
	Plan string `json:"plan"`
	Todo string `json:"todo"`
}

func NewPlanner(p provider.LLMProvider, store *Store) *Planner {
	return &Planner{provider: p, store: store}
}

func (p *Planner) BuildPlan(ctx context.Context, userPrompt string) error {
	prompt := fmt.Sprintf(`
请为下面的 Agent 任务生成一份可执行计划。

要求：
- 只输出一个合法 JSON 对象，不要输出 Markdown 代码块，不要输出解释性文字。
- JSON 必须包含 plan 和 todo 两个字符串字段。
- plan 字段的值是将要写入 PLAN.md 的完整 Markdown 内容，包含宏观目标、策略和验证方式。
- todo 字段的值是将要写入 TODO.md 的完整 Markdown 内容，包含可勾选的短任务列表。
- JSON 字符串内的换行必须使用 \n 转义。
- 不要调用工具，不要假设尚未读取的文件内容。

输出格式示例：
{"plan":"# PLAN\n\n## Goal\n\n...\n","todo":"# TODO\n\n- [ ] ...\n"}

用户任务：
%s
`, userPrompt)
	resp, err := p.provider.Generate(ctx, []schema.Message{
		{Role: schema.RoleUser, Content: prompt},
	}, nil)
	if err != nil {
		return fmt.Errorf("生成 Plan 失败: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("生成 Plan 失败: provider 返回空响应")
	}

	var draft planDraft
	if err := json.Unmarshal([]byte(resp.Content), &draft); err != nil {
		return fmt.Errorf("解析 Plan JSON 失败: %w\nRaw Response Content:\n%s", err, resp.Content)
	}

	plan := strings.TrimSpace(draft.Plan)
	todo := strings.TrimSpace(draft.Todo)
	if strings.TrimSpace(plan) == "" || strings.TrimSpace(todo) == "" {
		return fmt.Errorf("Plan JSON 缺少有效的 plan 或 todo 字段\nRaw Response Content:\n%s", resp.Content)
	}

	if err := os.WriteFile(p.store.PlanPath(), []byte(ensureTrailingNewline(plan)), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(p.store.TodoPath(), []byte(ensureTrailingNewline(todo)), 0644); err != nil {
		return err
	}

	return nil
}

func ensureTrailingNewline(text string) string {
	if strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}
