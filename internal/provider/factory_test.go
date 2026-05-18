package provider

import (
	"strings"
	"testing"
)

func TestNewZhipuProviderMissingAPIKeyReturnsHelpfulError(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "")

	_, err := NewZhipuProvider(ProviderProtocolOpenAI, "test-model")
	if err == nil {
		t.Fatal("NewZhipuProvider returned nil error, want missing key error")
	}
	if !strings.Contains(err.Error(), "ZHIPU_API_KEY is not set") {
		t.Fatalf("error = %q, want missing key message", err.Error())
	}
	if !strings.Contains(err.Error(), `export ZHIPU_API_KEY="your-api-key"`) {
		t.Fatalf("error = %q, want export hint", err.Error())
	}
}

func TestNewZhipuOpenAIProviderMissingAPIKeyDoesNotPanic(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "")

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("NewZhipuOpenAIProvider panicked: %v", recovered)
		}
	}()
	if _, err := NewZhipuOpenAIProvider("test-model"); err == nil {
		t.Fatal("NewZhipuOpenAIProvider returned nil error, want missing key error")
	}
}

func TestNewZhipuClaudeProviderMissingAPIKeyDoesNotPanic(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "")

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("NewZhipuClaudeProvider panicked: %v", recovered)
		}
	}()
	if _, err := NewZhipuClaudeProvider("test-model"); err == nil {
		t.Fatal("NewZhipuClaudeProvider returned nil error, want missing key error")
	}
}
