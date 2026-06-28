package provider

import (
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

const (
	ProviderProtocolOpenAI = "openai"
	ProviderProtocolClaude = "claude"
)

// NewProvider returns an LLM provider for the resolved protocol and connection
// fields.
func NewProvider(config llmconfig.ResolvedConfig) (LLMProvider, error) {
	switch normalizeProviderProtocol(config.Protocol) {
	case ProviderProtocolOpenAI:
		return NewOpenAIProvider(config)
	case ProviderProtocolClaude:
		return NewClaudeProvider(config)
	default:
		return nil, fmt.Errorf("unsupported provider protocol %q; expected %q or %q", config.Protocol, ProviderProtocolOpenAI, ProviderProtocolClaude)
	}
}

func normalizeProviderProtocol(protocol string) string {
	return strings.TrimSpace(strings.ToLower(protocol))
}
