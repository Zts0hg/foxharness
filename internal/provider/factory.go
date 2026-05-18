package provider

import (
	"fmt"
	"os"
	"strings"
)

const (
	ProviderProtocolOpenAI = "openai"
	ProviderProtocolClaude = "claude"
)

// NewZhipuProvider returns a Zhipu-backed provider for the requested wire
// protocol. The model name is shared because Zhipu exposes the same model
// behind both OpenAI-compatible and Claude-compatible APIs.
func NewZhipuProvider(protocol string, model string) (LLMProvider, error) {
	switch normalizeProviderProtocol(protocol) {
	case ProviderProtocolOpenAI:
		return NewZhipuOpenAIProvider(model)
	case ProviderProtocolClaude:
		return NewZhipuClaudeProvider(model)
	default:
		return nil, fmt.Errorf("unsupported provider protocol %q; expected %q or %q", protocol, ProviderProtocolOpenAI, ProviderProtocolClaude)
	}
}

func normalizeProviderProtocol(protocol string) string {
	protocol = strings.TrimSpace(strings.ToLower(protocol))
	if protocol == "" {
		return ProviderProtocolOpenAI
	}
	return protocol
}

func zhipuAPIKeyFromEnv() (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("ZHIPU_API_KEY"))
	if apiKey == "" {
		return "", fmt.Errorf("ZHIPU_API_KEY is not set\nexport ZHIPU_API_KEY=\"your-api-key\"")
	}
	return apiKey, nil
}
