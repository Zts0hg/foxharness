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

// Planner generates execution plans using an LLM for Plan Mode.
// It analyzes the user's request and creates structured plans (PLAN.md)
// and task lists (TODO.md) before the agent begins execution.
type Planner struct {
	// provider is the LLM provider used for plan generation.
	provider provider.LLMProvider
	// store manages the memory files where plans are written.
	store *Store
}

// planDraft represents the parsed JSON response from plan generation.
type planDraft struct {
	// Plan is the Markdown content for PLAN.md.
	Plan string `json:"plan"`
	// Todo is the Markdown content for TODO.md.
	Todo string `json:"todo"`
}

// NewPlanner creates a new Planner with the given provider and store.
// Returns a Planner ready to generate execution plans.
func NewPlanner(p provider.LLMProvider, store *Store) *Planner {
	return &Planner{provider: p, store: store}
}

// BuildPlan generates an execution plan for the given user prompt.
// The LLM is prompted to create a structured plan with goals, strategies,
// and a task list. The plan is written to PLAN.md and TODO.md.
// Returns an error if plan generation or parsing fails.
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

	draft, err := parsePlanDraft(resp.Content)
	if err != nil {
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

func parsePlanDraft(raw string) (planDraft, error) {
	candidates := planJSONCandidates(raw)
	var lastErr error
	for _, candidate := range candidates {
		var draft planDraft
		if err := json.Unmarshal([]byte(candidate), &draft); err != nil {
			lastErr = err
			continue
		}
		return draft, nil
	}
	if lastErr != nil {
		return planDraft{}, lastErr
	}
	return planDraft{}, fmt.Errorf("响应中没有找到 JSON 对象")
}

func planJSONCandidates(raw string) []string {
	var candidates []string
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		for _, existing := range candidates {
			if existing == candidate {
				return
			}
		}
		candidates = append(candidates, candidate)
	}

	add(raw)
	for _, block := range fencedBlocks(raw) {
		add(block)
	}
	if object, ok := firstJSONObject(raw); ok {
		add(object)
	}

	return candidates
}

func fencedBlocks(raw string) []string {
	lines := strings.Split(raw, "\n")
	var blocks []string
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			continue
		}

		start := i + 1
		for j := start; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "```") {
				blocks = append(blocks, strings.Join(lines[start:j], "\n"))
				i = j
				break
			}
		}
	}
	return blocks
}

func firstJSONObject(raw string) (string, bool) {
	start := strings.Index(raw, "{")
	if start < 0 {
		return "", false
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]

		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1], true
			}
		}
	}

	return "", false
}

func ensureTrailingNewline(text string) string {
	if strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}
