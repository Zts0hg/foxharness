package provider

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

func TestNewProviderConstructsOpenAIProviderFromResolvedConfig(t *testing.T) {
	got, err := NewProvider(llmconfig.ResolvedConfig{
		Protocol: llmconfig.ProtocolOpenAI,
		BaseURL:  "https://example.test/v1",
		Model:    "test-model",
		Auth:     llmconfig.AuthAPIKey,
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	provider, ok := got.(*OpenAIProvider)
	if !ok {
		t.Fatalf("NewProvider() = %T, want *OpenAIProvider", got)
	}
	if provider.ModelName() != "test-model" {
		t.Fatalf("ModelName() = %q, want test-model", provider.ModelName())
	}
}

func TestNewProviderConstructsClaudeProviderFromResolvedConfig(t *testing.T) {
	got, err := NewProvider(llmconfig.ResolvedConfig{
		Protocol: llmconfig.ProtocolClaude,
		BaseURL:  "https://example.test",
		Model:    "claude-model",
		Auth:     llmconfig.AuthAPIKey,
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	provider, ok := got.(*ClaudeProvider)
	if !ok {
		t.Fatalf("NewProvider() = %T, want *ClaudeProvider", got)
	}
	if provider.ModelName() != "claude-model" {
		t.Fatalf("ModelName() = %q, want claude-model", provider.ModelName())
	}
}

func TestNewProbeProviderUsesSingleAttempt(t *testing.T) {
	t.Run("openai", func(t *testing.T) {
		got, err := NewProbeProvider(llmconfig.ResolvedConfig{
			Protocol: llmconfig.ProtocolOpenAI,
			BaseURL:  "https://example.test/v1",
			Model:    "test-model",
			Auth:     llmconfig.AuthAPIKey,
			APIKey:   "test-key",
		})
		if err != nil {
			t.Fatalf("NewProbeProvider() error = %v", err)
		}
		op, ok := got.(*OpenAIProvider)
		if !ok {
			t.Fatalf("NewProbeProvider() = %T, want *OpenAIProvider", got)
		}
		if op.retry.MaxAttempts != 1 {
			t.Errorf("retry.MaxAttempts = %d, want 1 (probe must not retry)", op.retry.MaxAttempts)
		}
		if op.retry.RequestTimeout <= 0 {
			t.Errorf("retry.RequestTimeout = %v, want a bounded probe timeout", op.retry.RequestTimeout)
		}
	})

	t.Run("claude", func(t *testing.T) {
		got, err := NewProbeProvider(llmconfig.ResolvedConfig{
			Protocol: llmconfig.ProtocolClaude,
			BaseURL:  "https://example.test",
			Model:    "claude-model",
			Auth:     llmconfig.AuthAPIKey,
			APIKey:   "test-key",
		})
		if err != nil {
			t.Fatalf("NewProbeProvider() error = %v", err)
		}
		cp, ok := got.(*ClaudeProvider)
		if !ok {
			t.Fatalf("NewProbeProvider() = %T, want *ClaudeProvider", got)
		}
		if cp.retry.MaxAttempts != 1 {
			t.Errorf("retry.MaxAttempts = %d, want 1 (probe must not retry)", cp.retry.MaxAttempts)
		}
	})
}

func TestNewProviderRejectsUnsupportedProtocol(t *testing.T) {
	_, err := NewProvider(llmconfig.ResolvedConfig{
		Protocol: "custom",
		BaseURL:  "https://example.test",
		Model:    "test-model",
		Auth:     llmconfig.AuthNone,
	})
	if err == nil {
		t.Fatal("NewProvider() error = nil, want unsupported protocol")
	}
	if !strings.Contains(err.Error(), "unsupported provider protocol") {
		t.Fatalf("error = %q, want unsupported protocol", err.Error())
	}
}

func TestNewProviderDoesNotReadLegacyZhipuAPIKey(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "legacy-key")

	_, err := NewProvider(llmconfig.ResolvedConfig{
		Protocol: llmconfig.ProtocolOpenAI,
		BaseURL:  "https://example.test/v1",
		Model:    "test-model",
		Auth:     llmconfig.AuthAPIKey,
	})
	if err == nil {
		t.Fatal("NewProvider() error = nil, want missing API key")
	}
	if strings.Contains(err.Error(), "ZHIPU_API_KEY") {
		t.Fatalf("error = %q, want no legacy key guidance", err.Error())
	}
}
