package compaction

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

type fakeProvider struct {
	seen []schema.Message
}

func (p *fakeProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	p.seen = messages
	return &schema.Message{
		Role:    schema.RoleAssistant,
		Content: "压缩摘要",
	}, nil
}

type alwaysCompactEstimator struct{}

func (alwaysCompactEstimator) Estimate(messages []schema.Message) int {
	return 100
}

func TestMaybeCompactKeepsOriginalUserAndToolProtocolSuffix(t *testing.T) {
	provider := &fakeProvider{}
	c := NewCompactor(provider, alwaysCompactEstimator{}, Config{
		MaxTokens:        10,
		SoftRatio:        0.5,
		RecentKeep:       1,
		SummaryMaxTokens: 128,
	})

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "system rules"},
		{Role: schema.RoleUser, Content: "请生成 README"},
		{Role: schema.RoleAssistant, Content: "先读取项目结构"},
		{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{
				{
					ID:        "call_1",
					Name:      "bash",
					Arguments: json.RawMessage(`{"command":"ls"}`),
				},
			},
		},
		{Role: schema.RoleUser, Content: "go.mod\ncmd/fox/main.go", ToolCallID: "call_1"},
	}

	compacted, err := c.MaybeCompact(context.Background(), messages)
	if err != nil {
		t.Fatalf("MaybeCompact returned error: %v", err)
	}

	if got, want := len(compacted), 4; got != want {
		t.Fatalf("len(compacted) = %d, want %d", got, want)
	}
	if compacted[0].Role != schema.RoleSystem || !strings.Contains(compacted[0].Content, "压缩摘要") {
		t.Fatalf("first message should be compacted system summary, got role=%s content=%q", compacted[0].Role, compacted[0].Content)
	}
	if compacted[1].Role != schema.RoleUser || compacted[1].Content != "请生成 README" || compacted[1].ToolCallID != "" {
		t.Fatalf("second message should keep original user task, got %#v", compacted[1])
	}
	if got := compacted[2].ToolCalls; len(got) != 1 || got[0].ID != "call_1" {
		t.Fatalf("third message should keep assistant tool call, got %#v", compacted[2])
	}
	if compacted[3].ToolCallID != "call_1" {
		t.Fatalf("fourth message should keep matching tool result, got %#v", compacted[3])
	}

	if len(provider.seen) != 1 || !strings.Contains(provider.seen[0].Content, "先读取项目结构") {
		t.Fatalf("summary prompt should include compacted old message content, got %#v", provider.seen)
	}
}
