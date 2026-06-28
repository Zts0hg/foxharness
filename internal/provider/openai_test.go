package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/openai/openai-go/v3/option"
)

func TestOpenAIProviderReturnsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprint(w, `{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 0,
			"model": "test-model",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "ok"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 1000, "completion_tokens": 500, "total_tokens": 1500}
		}`)
		if err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	provider := newTestOpenAIProvider(server.URL, RetryConfig{MaxAttempts: 1})
	resp, err := provider.Generate(context.Background(), []schema.Message{
		{Role: schema.RoleUser, Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp == nil || resp.Message == nil {
		t.Fatalf("Generate() returned nil response or message")
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
}

func TestOpenAIProviderRetriesTransientHTTPFailure(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt == 1 {
			http.Error(w, `{"error":{"message":"temporary outage"}}`, http.StatusBadGateway)
			return
		}
		writeChatCompletion(t, w, "recovered")
	}))
	defer server.Close()

	provider := newTestOpenAIProvider(server.URL, RetryConfig{
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

func TestOpenAIProviderRetriesPerAttemptTimeout(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt == 1 {
			time.Sleep(30 * time.Millisecond)
			return
		}
		writeChatCompletion(t, w, "after timeout")
	}))
	defer server.Close()

	provider := newTestOpenAIProviderWithOptions(server.URL, RetryConfig{
		MaxAttempts:  2,
		InitialDelay: time.Nanosecond,
		MaxDelay:     time.Nanosecond,
	}, option.WithRequestTimeout(5*time.Millisecond))

	resp, err := provider.Generate(context.Background(), []schema.Message{
		{Role: schema.RoleUser, Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Message.Content != "after timeout" {
		t.Fatalf("content = %q, want after timeout", resp.Message.Content)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestOpenAIProviderDoesNotRetryNonTransientHTTPFailure(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		http.Error(w, `{"error":{"message":"bad request"}}`, http.StatusBadRequest)
	}))
	defer server.Close()

	provider := newTestOpenAIProvider(server.URL, RetryConfig{
		MaxAttempts:  3,
		InitialDelay: time.Nanosecond,
		MaxDelay:     time.Nanosecond,
	})

	_, err := provider.Generate(context.Background(), []schema.Message{
		{Role: schema.RoleUser, Content: "hello"},
	}, nil)
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestOpenAIProviderStopsRetryingWhenParentContextIsCanceled(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		http.Error(w, `{"error":{"message":"temporary outage"}}`, http.StatusBadGateway)
	}))
	defer server.Close()

	provider := newTestOpenAIProvider(server.URL, RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := provider.Generate(ctx, []schema.Message{
		{Role: schema.RoleUser, Content: "hello"},
	}, nil)
	if err == nil {
		t.Fatal("Generate() error = nil, want context cancellation")
	}
	if attempts != 0 {
		t.Fatalf("attempts = %d, want 0", attempts)
	}
}

func TestOpenAIProviderNormalizesEmptyToolCallArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChatCompletionToolCall(t, w, "call-1", "read_file", "")
	}))
	defer server.Close()

	provider := newTestOpenAIProvider(server.URL, RetryConfig{MaxAttempts: 1})
	resp, err := provider.Generate(context.Background(), []schema.Message{
		{Role: schema.RoleUser, Content: "read README"},
	}, []schema.ToolDefinition{{
		Name: "read_file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
		},
	}})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.Message.ToolCalls))
	}
	if got := string(resp.Message.ToolCalls[0].Arguments); got != "{}" {
		t.Fatalf("tool call arguments = %q, want {}", got)
	}
}

func TestNewProviderOpenAIUsesConfiguredConnectionAndAPIKeyAuth(t *testing.T) {
	requests := make(chan openAIRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("request path = %q, want /chat/completions", r.URL.Path)
		}
		var body openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		body.Authorization = r.Header.Get("Authorization")
		body.XAPIKey = r.Header.Get("X-Api-Key")
		requests <- body
		writeChatCompletion(t, w, "ok")
	}))
	defer server.Close()

	got, err := NewProvider(llmconfig.ResolvedConfig{
		Protocol: llmconfig.ProtocolOpenAI,
		BaseURL:  server.URL,
		Model:    "configured-model",
		Auth:     llmconfig.AuthAPIKey,
		APIKey:   "configured-key",
	})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	_, err = got.Generate(context.Background(), []schema.Message{{Role: schema.RoleUser, Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	req := <-requests
	if req.Model != "configured-model" {
		t.Fatalf("model = %q, want configured-model", req.Model)
	}
	if req.Authorization != "Bearer configured-key" {
		t.Fatalf("Authorization = %q, want Bearer configured-key", req.Authorization)
	}
	if req.XAPIKey != "" {
		t.Fatalf("X-Api-Key = %q, want empty", req.XAPIKey)
	}
}

func TestNewProviderOpenAIAuthNoneSendsNoCredentialHeaders(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "env-anthropic-key")

	requests := make(chan openAIRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		body.Authorization = r.Header.Get("Authorization")
		body.XAPIKey = r.Header.Get("X-Api-Key")
		requests <- body
		writeChatCompletion(t, w, "ok")
	}))
	defer server.Close()

	got, err := NewProvider(llmconfig.ResolvedConfig{
		Protocol: llmconfig.ProtocolOpenAI,
		BaseURL:  server.URL,
		Model:    "local-model",
		Auth:     llmconfig.AuthNone,
	})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	_, err = got.Generate(context.Background(), []schema.Message{{Role: schema.RoleUser, Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	req := <-requests
	if req.Authorization != "" {
		t.Fatalf("Authorization = %q, want empty for auth none", req.Authorization)
	}
	if req.XAPIKey != "" {
		t.Fatalf("X-Api-Key = %q, want empty for auth none", req.XAPIKey)
	}
}

func TestNewProviderOpenAIAuthNoneIgnoresOpenAIEnvironmentDefaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-openai-key")
	t.Setenv("OPENAI_ORG_ID", "env-org")
	t.Setenv("OPENAI_CUSTOM_HEADERS", "X-Leaked-Secret: env-secret")

	requests := make(chan openAIRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		body.Authorization = r.Header.Get("Authorization")
		body.XAPIKey = r.Header.Get("X-Api-Key")
		body.Organization = r.Header.Get("OpenAI-Organization")
		body.LeakedSecret = r.Header.Get("X-Leaked-Secret")
		requests <- body
		writeChatCompletion(t, w, "ok")
	}))
	defer server.Close()

	got, err := NewProvider(llmconfig.ResolvedConfig{
		Protocol: llmconfig.ProtocolOpenAI,
		BaseURL:  server.URL,
		Model:    "local-model",
		Auth:     llmconfig.AuthNone,
	})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	_, err = got.Generate(context.Background(), []schema.Message{{Role: schema.RoleUser, Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	req := <-requests
	if req.Authorization != "" || req.XAPIKey != "" || req.Organization != "" || req.LeakedSecret != "" {
		t.Fatalf("environment defaults leaked into request headers: %+v", req)
	}
}

type openAIRequest struct {
	Model         string `json:"model"`
	Authorization string
	XAPIKey       string
	Organization  string
	LeakedSecret  string
}

func newTestOpenAIProvider(baseURL string, retry RetryConfig) *OpenAIProvider {
	return newTestOpenAIProviderWithOptions(baseURL, retry)
}

func newTestOpenAIProviderWithOptions(baseURL string, retry RetryConfig, opts ...option.RequestOption) *OpenAIProvider {
	clientOptions := []option.RequestOption{
		option.WithAPIKey("test-key"),
		option.WithBaseURL(baseURL),
		option.WithMaxRetries(0),
	}
	clientOptions = append(clientOptions, opts...)
	return &OpenAIProvider{
		client: newOpenAIClient(clientOptions...),
		model:  "test-model",
		retry:  retry,
	}
}

func writeChatCompletion(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, err := fmt.Fprintf(w, `{
		"id": "chatcmpl-test",
		"object": "chat.completion",
		"created": 0,
		"model": "test-model",
		"choices": [{
			"index": 0,
			"message": {"role": "assistant", "content": %q},
			"finish_reason": "stop"
		}]
	}`, strings.ReplaceAll(content, "\n", " "))
	if err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func writeChatCompletionToolCall(t *testing.T, w http.ResponseWriter, id string, name string, arguments string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, err := fmt.Fprintf(w, `{
		"id": "chatcmpl-test",
		"object": "chat.completion",
		"created": 0,
		"model": "test-model",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "",
				"tool_calls": [{
					"id": %q,
					"type": "function",
					"function": {
						"name": %q,
						"arguments": %q
					}
				}]
			},
			"finish_reason": "tool_calls"
		}]
	}`, id, name, arguments)
	if err != nil {
		t.Fatalf("write response: %v", err)
	}
}
