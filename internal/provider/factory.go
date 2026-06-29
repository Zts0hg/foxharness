package provider

import (
	"fmt"
	"strings"
	"time"

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

// NewProbeProvider builds a provider configured for a single connectivity-check
// request: one attempt (no retries) with a short per-request timeout. It is
// used by the `fox config` wizard to verify a provider works before saving, so
// a misconfigured or unauthorized endpoint fails fast instead of retrying with
// backoff like a production request.
func NewProbeProvider(config llmconfig.ResolvedConfig) (LLMProvider, error) {
	probeRetry := RetryConfig{MaxAttempts: 1, RequestTimeout: 15 * time.Second}
	switch normalizeProviderProtocol(config.Protocol) {
	case ProviderProtocolOpenAI:
		return newOpenAIProviderWithRetry(config, probeRetry)
	case ProviderProtocolClaude:
		return newClaudeProviderWithRetry(config, probeRetry)
	default:
		return nil, fmt.Errorf("unsupported provider protocol %q; expected %q or %q", config.Protocol, ProviderProtocolOpenAI, ProviderProtocolClaude)
	}
}

func normalizeProviderProtocol(protocol string) string {
	return strings.TrimSpace(strings.ToLower(protocol))
}
