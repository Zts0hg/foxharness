package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

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

	msg, err := provider.Generate(context.Background(), []schema.Message{
		{Role: schema.RoleUser, Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if msg.Content != "recovered" {
		t.Fatalf("content = %q, want recovered", msg.Content)
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

	msg, err := provider.Generate(context.Background(), []schema.Message{
		{Role: schema.RoleUser, Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if msg.Content != "after timeout" {
		t.Fatalf("content = %q, want after timeout", msg.Content)
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
	msg, err := provider.Generate(context.Background(), []schema.Message{
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
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(msg.ToolCalls))
	}
	if got := string(msg.ToolCalls[0].Arguments); got != "{}" {
		t.Fatalf("tool call arguments = %q, want {}", got)
	}
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
		client: openai.NewClient(clientOptions...),
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
