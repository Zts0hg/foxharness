package compaction

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type fakeProvider struct {
	seen []schema.Message
}

func (p *fakeProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.seen = messages
	return &provider.GenerateResponse{
		Message: &schema.Message{
			Role:    schema.RoleAssistant,
			Content: "压缩摘要",
		},
	}, nil
}

type stubProvider struct {
	calls    int
	response *provider.GenerateResponse
	err      error
}

func (p *stubProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.response != nil {
		return p.response, nil
	}
	return &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "stub summary"},
	}, nil
}

type alwaysCompactEstimator struct{}

func (alwaysCompactEstimator) Estimate(messages []schema.Message) int {
	return 100
}

// newTestCompactor wires up a Compactor that always triggers compaction so
// tests can drive small message sets through MaybeCompact without needing
// hundreds of synthetic messages to cross the production threshold.
func newTestCompactor(t *testing.T, p provider.LLMProvider, opts ...func(*CompactionConfig)) *Compactor {
	t.Helper()
	cfg := DefaultCompactionConfig()
	cfg.Model = "test-model"
	cfg.RecentKeep = 1
	cfg.ContextWindow = 10000
	cfg.Estimator = alwaysCompactEstimator{}
	cfg.AutoCompactThreshold = 1
	for _, opt := range opts {
		opt(&cfg)
	}
	c, err := NewCompactor(p, cfg)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}
	return c
}

func TestMaybeCompactKeepsOriginalUserAndToolProtocolSuffix(t *testing.T) {
	p := &fakeProvider{}
	c := newTestCompactor(t, p)

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

	if compacted[0].Role != schema.RoleSystem {
		t.Fatalf("first message should be system message, got role=%s", compacted[0].Role)
	}

	var summaryIdx = -1
	for i, m := range compacted {
		if m.Role == schema.RoleUser && strings.Contains(m.Content, "压缩摘要") {
			summaryIdx = i
			break
		}
	}
	if summaryIdx < 0 {
		t.Fatalf("expected summary user message in compacted history, got: %#v", compacted)
	}

	last := compacted[len(compacted)-1]
	if last.ToolCallID != "call_1" {
		t.Fatalf("last message should be matching tool result, got %#v", last)
	}

	if len(p.seen) == 0 {
		t.Fatalf("provider should have been called with summary prompt")
	}
}

func TestCompactor_RecursiveGuard(t *testing.T) {
	p := &stubProvider{}
	c := newTestCompactor(t, p)
	c.compacting = true

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "u1"},
		{Role: schema.RoleAssistant, Content: "a1"},
		{Role: schema.RoleUser, Content: "u2"},
		{Role: schema.RoleAssistant, Content: "a2"},
	}
	got, err := c.MaybeCompact(context.Background(), messages)
	if err != nil {
		t.Fatalf("MaybeCompact returned error: %v", err)
	}
	if p.calls != 0 {
		t.Fatalf("provider.calls = %d, want 0 (recursive guard should short-circuit)", p.calls)
	}
	if len(got) != len(messages) {
		t.Fatalf("len(got) = %d, want %d (original messages preserved)", len(got), len(messages))
	}
}

func TestCompactor_DisableViaEnvAll(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "1")
	p := &stubProvider{}
	c := newTestCompactor(t, p)

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "u1"},
		{Role: schema.RoleAssistant, Content: "a1"},
		{Role: schema.RoleUser, Content: "u2"},
		{Role: schema.RoleAssistant, Content: "a2"},
	}
	got, err := c.MaybeCompact(context.Background(), messages)
	if err != nil {
		t.Fatalf("MaybeCompact returned error: %v", err)
	}
	if p.calls != 0 {
		t.Fatalf("provider.calls = %d, want 0 when FOXHARNESS_DISABLE_COMPACT is set", p.calls)
	}
	if len(got) != len(messages) {
		t.Fatalf("len(got) = %d, want %d when compaction is disabled", len(got), len(messages))
	}
}

func TestCompactor_DisableViaEnvAuto(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "")
	t.Setenv("FOXHARNESS_DISABLE_AUTO_COMPACT", "true")
	p := &stubProvider{}
	c := newTestCompactor(t, p)

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "u1"},
		{Role: schema.RoleAssistant, Content: "a1"},
		{Role: schema.RoleUser, Content: "u2"},
		{Role: schema.RoleAssistant, Content: "a2"},
	}
	if _, err := c.MaybeCompact(context.Background(), messages); err != nil {
		t.Fatalf("MaybeCompact returned error: %v", err)
	}
	if p.calls != 0 {
		t.Fatalf("provider.calls = %d, want 0 when auto compaction is disabled", p.calls)
	}
}

func TestCompactor_DisableViaConfig(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "")
	t.Setenv("FOXHARNESS_DISABLE_AUTO_COMPACT", "")
	p := &stubProvider{}
	c := newTestCompactor(t, p, func(c *CompactionConfig) {
		c.Enabled = false
	})

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "u1"},
		{Role: schema.RoleAssistant, Content: "a1"},
		{Role: schema.RoleUser, Content: "u2"},
		{Role: schema.RoleAssistant, Content: "a2"},
	}
	if _, err := c.MaybeCompact(context.Background(), messages); err != nil {
		t.Fatalf("MaybeCompact returned error: %v", err)
	}
	if p.calls != 0 {
		t.Fatalf("provider.calls = %d, want 0 when compaction.enabled=false", p.calls)
	}
}

func TestCompactor_EnvVarOverridesConfig(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "1")
	p := &stubProvider{}
	c := newTestCompactor(t, p, func(c *CompactionConfig) {
		c.Enabled = true
	})

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "u1"},
		{Role: schema.RoleAssistant, Content: "a1"},
		{Role: schema.RoleUser, Content: "u2"},
		{Role: schema.RoleAssistant, Content: "a2"},
	}
	if _, err := c.MaybeCompact(context.Background(), messages); err != nil {
		t.Fatalf("MaybeCompact returned error: %v", err)
	}
	if p.calls != 0 {
		t.Fatalf("provider.calls = %d, want 0 (env var must override config Enabled=true)", p.calls)
	}
}

func TestNewCompactor_WithRegistry(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "")
	t.Setenv("FOXHARNESS_DISABLE_AUTO_COMPACT", "")
	p := &stubProvider{}
	cfg := DefaultCompactionConfig()
	cfg.Model = "claude-4-sonnet"
	c, err := NewCompactor(p, cfg)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}
	if got := c.thresholds.ContextWindow; got != 200000 {
		t.Fatalf("ContextWindow = %d, want 200000 from registry lookup", got)
	}

	override := DefaultCompactionConfig()
	override.Model = "claude-4-sonnet"
	override.Overrides = map[string]int{"claude-4-sonnet": 300000}
	c2, err := NewCompactor(p, override)
	if err != nil {
		t.Fatalf("NewCompactor with overrides: %v", err)
	}
	if got := c2.thresholds.ContextWindow; got != 300000 {
		t.Fatalf("ContextWindow with override = %d, want 300000", got)
	}
}

func TestSummaryMessage_Continuation(t *testing.T) {
	wrapper := BuildSummaryMessage("Section 1: ...", "/sess/transcript.jsonl")
	if wrapper.Role != schema.RoleUser {
		t.Fatalf("BuildSummaryMessage role = %q, want user", wrapper.Role)
	}
	if !strings.Contains(wrapper.Content, "Continue the conversation from where it left off") {
		t.Fatalf("BuildSummaryMessage missing continuation instruction: %q", wrapper.Content)
	}
	if !strings.Contains(wrapper.Content, "Section 1") {
		t.Fatalf("BuildSummaryMessage missing summary body")
	}
	if !strings.Contains(wrapper.Content, "/sess/transcript.jsonl") {
		t.Fatalf("BuildSummaryMessage missing transcript path: %q", wrapper.Content)
	}
}

func TestCompact_SummaryWithNoTools(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "")
	t.Setenv("FOXHARNESS_DISABLE_AUTO_COMPACT", "")

	p := &capturingProvider{
		response: &provider.GenerateResponse{
			Message: &schema.Message{
				Role:    schema.RoleAssistant,
				Content: "<summary>summarized body</summary>",
				ToolCalls: []schema.ToolCall{{
					ID:        "should-be-ignored",
					Name:      "rogue_tool",
					Arguments: json.RawMessage(`{}`),
				}},
			},
		},
	}
	c := newTestCompactor(t, p)

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "Anchor user request"},
		{Role: schema.RoleAssistant, Content: "earlier work 1"},
		{Role: schema.RoleAssistant, Content: "earlier work 2"},
		{Role: schema.RoleUser, Content: "recent thread"},
	}
	out, err := c.MaybeCompact(context.Background(), messages)
	if err != nil {
		t.Fatalf("MaybeCompact: %v", err)
	}
	if len(p.lastTools) != 0 {
		t.Fatalf("compaction must pass an empty tool list; got %#v", p.lastTools)
	}
	if p.calls == 0 {
		t.Fatalf("expected compaction provider to be called")
	}
	if !strings.Contains(p.lastMessages[0].Content, NoToolsPreamble) {
		t.Fatalf("prompt missing NO_TOOLS preamble")
	}
	if !strings.Contains(p.lastMessages[0].Content, NoToolsTrailer) {
		t.Fatalf("prompt missing NO_TOOLS trailer")
	}

	var summary *schema.Message
	for i := range out {
		if out[i].Role == schema.RoleUser && strings.Contains(out[i].Content, "summarized body") {
			summary = &out[i]
			break
		}
	}
	if summary == nil {
		t.Fatalf("expected summary message in compacted output: %#v", out)
	}
}

func TestMaybeCompact_BoundaryUsesInjectedClock(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "")
	t.Setenv("FOXHARNESS_DISABLE_AUTO_COMPACT", "")
	fixed := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	p := &stubProvider{
		response: &provider.GenerateResponse{
			Message: &schema.Message{Role: schema.RoleAssistant, Content: "<summary>ok</summary>"},
		},
	}
	c := newTestCompactor(t, p, func(c *CompactionConfig) {
		c.Clock = func() time.Time { return fixed }
	})

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "anchor"},
		{Role: schema.RoleAssistant, Content: "earlier 1"},
		{Role: schema.RoleAssistant, Content: "earlier 2"},
		{Role: schema.RoleUser, Content: "recent"},
	}
	out, err := c.MaybeCompact(context.Background(), messages)
	if err != nil {
		t.Fatalf("MaybeCompact: %v", err)
	}
	var boundaryContent string
	for _, m := range out {
		if m.Role == schema.RoleSystem && strings.HasPrefix(m.Content, BoundaryMarkerPrefix) {
			boundaryContent = m.Content
			break
		}
	}
	if boundaryContent == "" {
		t.Fatalf("no boundary marker found in output: %#v", out)
	}
	want := fixed.Format(time.RFC3339)
	if !strings.Contains(boundaryContent, want) {
		t.Fatalf("boundary timestamp = %q, want it to contain %q", boundaryContent, want)
	}
}

func TestMaybeCompact_MessageFormat(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "")
	t.Setenv("FOXHARNESS_DISABLE_AUTO_COMPACT", "")
	p := &stubProvider{
		response: &provider.GenerateResponse{
			Message: &schema.Message{
				Role:    schema.RoleAssistant,
				Content: "<summary>final summary</summary>",
			},
		},
	}
	c := newTestCompactor(t, p)

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "first request"},
		{Role: schema.RoleAssistant, Content: "earlier 1"},
		{Role: schema.RoleAssistant, Content: "earlier 2"},
		{Role: schema.RoleUser, Content: "recent"},
	}
	out, err := c.MaybeCompact(context.Background(), messages)
	if err != nil {
		t.Fatalf("MaybeCompact: %v", err)
	}
	if out[0].Role != schema.RoleSystem {
		t.Fatalf("first message role = %q, want system", out[0].Role)
	}

	var hasBoundary, hasSummary bool
	for _, m := range out {
		if m.Role == schema.RoleSystem && strings.Contains(m.Content, BoundaryMarkerPrefix) {
			hasBoundary = true
		}
		if m.Role == schema.RoleUser && strings.Contains(m.Content, "final summary") {
			hasSummary = true
		}
	}
	if !hasBoundary {
		t.Fatalf("compacted output missing boundary marker: %#v", out)
	}
	if !hasSummary {
		t.Fatalf("compacted output missing summary message")
	}

	last := out[len(out)-1]
	if last.Content != "recent" {
		t.Fatalf("last message should be the recent user message, got %#v", last)
	}
}

type capturingProvider struct {
	response     *provider.GenerateResponse
	calls        int
	lastMessages []schema.Message
	lastTools    []schema.ToolDefinition
}

func (p *capturingProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	p.lastMessages = append([]schema.Message(nil), messages...)
	p.lastTools = append([]schema.ToolDefinition(nil), availableTools...)
	if p.response != nil {
		return p.response, nil
	}
	return &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "default"},
	}, nil
}

func TestMaybeCompact_ClearsStaleUsageOnRecentMessages(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "")
	t.Setenv("FOXHARNESS_DISABLE_AUTO_COMPACT", "")
	p := &stubProvider{
		response: &provider.GenerateResponse{
			Message: &schema.Message{Role: schema.RoleAssistant, Content: "<summary>ok</summary>"},
		},
	}
	c := newTestCompactor(t, p, func(cfg *CompactionConfig) {
		cfg.RecentKeep = 2
	})

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "anchor"},
		{Role: schema.RoleAssistant, Content: "old work", Usage: &schema.Usage{InputTokens: 80000, OutputTokens: 2000}},
		{Role: schema.RoleUser, Content: "more work"},
		{Role: schema.RoleAssistant, Content: "recent reply", Usage: &schema.Usage{InputTokens: 90000, OutputTokens: 3000}},
		{Role: schema.RoleUser, Content: "latest"},
	}
	out, err := c.MaybeCompact(context.Background(), messages)
	if err != nil {
		t.Fatalf("MaybeCompact: %v", err)
	}

	for i, msg := range out {
		if msg.Role == schema.RoleAssistant && msg.Usage != nil {
			t.Fatalf("compacted message[%d] (role=%s) still has stale Usage data; "+
				"HybridEstimator would overcount on the next turn", i, msg.Role)
		}
	}

	// After clearing Usage, HybridEstimator should fall back to rough
	// estimation, producing a much lower count than the stale 93000.
	est := NewHybridEstimator(ImprovedRoughEstimator{})
	postTokens := est.Estimate(out)
	staleCount := 90000 + 3000
	if postTokens >= staleCount {
		t.Fatalf("post-compaction estimate %d >= stale usage %d; Usage was not cleared", postTokens, staleCount)
	}
}

func TestMaybeCompact_SummaryFailureReturnsOriginal(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "")
	t.Setenv("FOXHARNESS_DISABLE_AUTO_COMPACT", "")
	p := &stubProvider{err: errors.New("upstream failure")}
	c := newTestCompactor(t, p)

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "u1"},
		{Role: schema.RoleAssistant, Content: "a1"},
		{Role: schema.RoleUser, Content: "u2"},
		{Role: schema.RoleAssistant, Content: "a2"},
	}
	got, err := c.MaybeCompact(context.Background(), messages)
	if err == nil {
		t.Fatalf("expected error from summary failure")
	}
	if len(got) != len(messages) {
		t.Fatalf("len(got) = %d, want %d (original messages on failure)", len(got), len(messages))
	}
}
