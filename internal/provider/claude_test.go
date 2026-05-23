package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestClaudeProviderTranslatesMessagesToolsAndToolUseResponse(t *testing.T) {
	requests := make(chan claudeRequestBody, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("request path = %q, want /v1/messages", r.URL.Path)
		}
		var body claudeRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requests <- body
		writeClaudeToolUseMessage(t, w, "toolu-2", "read_file", `{"path":"README.md"}`)
	}))
	defer server.Close()

	provider := newTestClaudeProvider(server.URL, RetryConfig{MaxAttempts: 1})
	resp, err := provider.Generate(context.Background(), []schema.Message{
		{Role: schema.RoleSystem, Content: "You are a coding agent."},
		{Role: schema.RoleUser, Content: "Inspect README"},
		{Role: schema.RoleAssistant, Content: "I will read it.", ToolCalls: []schema.ToolCall{{
			ID:        "toolu-1",
			Name:      "read_file",
			Arguments: json.RawMessage(`{"path":"README.md"}`),
		}}},
		{Role: schema.RoleUser, ToolCallID: "toolu-1", Content: "# foxharness"},
	}, []schema.ToolDefinition{{
		Name:        "read_file",
		Description: "Read a file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
	}})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	msg := resp.Message
	if msg.Role != schema.RoleAssistant {
		t.Fatalf("response role = %q, want assistant", msg.Role)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("response ToolCalls len = %d, want 1", len(msg.ToolCalls))
	}
	call := msg.ToolCalls[0]
	if call.ID != "toolu-2" || call.Name != "read_file" || string(call.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("response tool call = %#v", call)
	}

	body := <-requests
	if body.Model != "test-model" || body.MaxTokens != 4096 {
		t.Fatalf("request model/max_tokens = %q/%d", body.Model, body.MaxTokens)
	}
	if len(body.System) != 1 || body.System[0].Text != "You are a coding agent." {
		t.Fatalf("request system = %#v", body.System)
	}
	if len(body.Messages) != 3 {
		t.Fatalf("request messages len = %d, want 3: %#v", len(body.Messages), body.Messages)
	}
	if body.Messages[0].Role != "user" || contentType(body.Messages[0], 0) != "text" {
		t.Fatalf("first message = %#v", body.Messages[0])
	}
	if body.Messages[1].Role != "assistant" || contentType(body.Messages[1], 1) != "tool_use" {
		t.Fatalf("assistant tool_use message = %#v", body.Messages[1])
	}
	if body.Messages[2].Role != "user" || contentType(body.Messages[2], 0) != "tool_result" {
		t.Fatalf("tool result message = %#v", body.Messages[2])
	}
	if len(body.Tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(body.Tools))
	}
	tool := body.Tools[0]
	if tool.Name != "read_file" || tool.Description != "Read a file" {
		t.Fatalf("tool metadata = %#v", tool)
	}
	if _, ok := tool.InputSchema["properties"].(map[string]any); !ok {
		t.Fatalf("tool input_schema missing properties: %#v", tool.InputSchema)
	}
	if got := tool.InputSchema["additionalProperties"]; got != false {
		t.Fatalf("tool input_schema additionalProperties = %#v, want false", got)
	}
}

func TestClaudeProviderReturnsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprint(w, `{
			"id": "msg-test",
			"type": "message",
			"role": "assistant",
			"model": "test-model",
			"content": [{"type": "text", "text": "ok"}],
			"stop_reason": "end_turn",
			"stop_sequence": null,
			"usage": {"input_tokens": 1000, "output_tokens": 500, "cache_creation_input_tokens": 200, "cache_read_input_tokens": 50}
		}`)
		if err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	provider := newTestClaudeProvider(server.URL, RetryConfig{MaxAttempts: 1})
	resp, err := provider.Generate(context.Background(), []schema.Message{
		{Role: schema.RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp == nil || resp.Message == nil {
		t.Fatalf("Generate() returned nil response")
	}
	if resp.Message.Content != "ok" {
		t.Fatalf("Message.Content = %q, want ok", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 1000 {
		t.Fatalf("Usage.InputTokens = %d, want 1000", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 500 {
		t.Fatalf("Usage.OutputTokens = %d, want 500", resp.Usage.OutputTokens)
	}
	if resp.Usage.CacheCreationTokens != 200 {
		t.Fatalf("Usage.CacheCreationTokens = %d, want 200", resp.Usage.CacheCreationTokens)
	}
	if resp.Usage.CacheReadTokens != 50 {
		t.Fatalf("Usage.CacheReadTokens = %d, want 50", resp.Usage.CacheReadTokens)
	}
}

func TestClaudeProviderRetriesTransientHTTPFailure(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt == 1 {
			http.Error(w, `{"error":{"type":"api_error","message":"temporary outage"}}`, http.StatusBadGateway)
			return
		}
		writeClaudeTextMessage(t, w, "recovered")
	}))
	defer server.Close()

	provider := newTestClaudeProvider(server.URL, RetryConfig{
		MaxAttempts:  2,
		InitialDelay: time.Nanosecond,
		MaxDelay:     time.Nanosecond,
	})

	resp, err := provider.Generate(context.Background(), []schema.Message{
		{Role: schema.RoleUser, Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Message.Content != "recovered" {
		t.Fatalf("content = %q, want recovered", resp.Message.Content)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestNormalizeProviderProtocol(t *testing.T) {
	if got := normalizeProviderProtocol(""); got != ProviderProtocolOpenAI {
		t.Fatalf("empty protocol = %q, want openai", got)
	}
	if got := normalizeProviderProtocol(" CLAUDE "); got != ProviderProtocolClaude {
		t.Fatalf("normalized protocol = %q, want claude", got)
	}
}

type claudeRequestBody struct {
	Model     string `json:"model"`
	MaxTokens int64  `json:"max_tokens"`
	System    []struct {
		Text string `json:"text"`
	} `json:"system"`
	Messages []struct {
		Role    string           `json:"role"`
		Content []map[string]any `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"input_schema"`
	} `json:"tools"`
}

func newTestClaudeProvider(baseURL string, retry RetryConfig) *ClaudeProvider {
	return &ClaudeProvider{
		client: anthropic.NewClient(
			option.WithAPIKey("test-key"),
			option.WithBaseURL(baseURL),
			option.WithMaxRetries(0),
		),
		model: "test-model",
		retry: retry,
	}
}

func writeClaudeTextMessage(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, err := fmt.Fprintf(w, `{
		"id": "msg-test",
		"type": "message",
		"role": "assistant",
		"model": "test-model",
		"content": [{"type": "text", "text": %q}],
		"stop_reason": "end_turn",
		"stop_sequence": null,
		"usage": {"input_tokens": 1, "output_tokens": 1}
	}`, content)
	if err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func writeClaudeToolUseMessage(t *testing.T, w http.ResponseWriter, id string, name string, input string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, err := fmt.Fprintf(w, `{
		"id": "msg-test",
		"type": "message",
		"role": "assistant",
		"model": "test-model",
		"content": [{"type": "tool_use", "id": %q, "name": %q, "input": %s}],
		"stop_reason": "tool_use",
		"stop_sequence": null,
		"usage": {"input_tokens": 1, "output_tokens": 1}
	}`, id, name, input)
	if err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func contentType(message struct {
	Role    string           `json:"role"`
	Content []map[string]any `json:"content"`
}, index int) string {
	if index < 0 || index >= len(message.Content) {
		return ""
	}
	value, _ := message.Content[index]["type"].(string)
	return value
}
