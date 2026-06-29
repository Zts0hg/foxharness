package configcmd

import "github.com/Zts0hg/foxharness/internal/llmconfig"

// Preset is a built-in provider template used to pre-fill the add wizard. It is
// ordinary data: the wizard copies its fields into a llmconfig.Profile, and no
// vendor-specific code path reads it at runtime.
type Preset struct {
	ID        string
	Protocol  string
	BaseURL   string
	Model     string
	Auth      string
	APIKeyEnv string
}

// Catalog is the curated v1 set of common OpenAI- or Claude-compatible
// providers. Base URLs and default models are template values (spec assumption
// A-1); they may be corrected without changing product intent.
var Catalog = []Preset{
	{ID: "openai", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "OPENAI_API_KEY"},
	{ID: "anthropic", Protocol: llmconfig.ProtocolClaude, BaseURL: "https://api.anthropic.com", Model: "claude-sonnet-4-6", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "ANTHROPIC_API_KEY"},
	{ID: "xai", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://api.x.ai/v1", Model: "grok-4", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "XAI_API_KEY"},
	{ID: "mistral", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://api.mistral.ai/v1", Model: "mistral-small-latest", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "MISTRAL_API_KEY"},
	{ID: "groq", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.3-70b-versatile", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "GROQ_API_KEY"},
	{ID: "openrouter", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://openrouter.ai/api/v1", Model: "anthropic/claude-3.5-sonnet", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "OPENROUTER_API_KEY"},
	{ID: "zhipu", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://open.bigmodel.cn/api/paas/v4", Model: "glm-4.5-air", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "ZHIPU_API_KEY"},
	{ID: "deepseek", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://api.deepseek.com", Model: "deepseek-chat", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "DEEPSEEK_API_KEY"},
	{ID: "moonshot", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://api.moonshot.cn/v1", Model: "moonshot-v1-8k", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "MOONSHOT_API_KEY"},
	{ID: "qwen", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-plus", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "DASHSCOPE_API_KEY"},
	{ID: "minimax", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://api.minimax.chat/v1", Model: "abab6.5s-chat", Auth: llmconfig.AuthAPIKey, APIKeyEnv: "MINIMAX_API_KEY"},
	{ID: "ollama", Protocol: llmconfig.ProtocolOpenAI, BaseURL: "http://localhost:11434/v1", Model: "llama3.2", Auth: llmconfig.AuthNone},
}

// PresetByID returns the preset with the given id and reports whether it exists.
func PresetByID(id string) (Preset, bool) {
	for _, p := range Catalog {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}
