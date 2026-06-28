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
