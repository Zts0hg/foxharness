package llmconfig

import (
	"fmt"
	"strings"
)

const (
	// ProtocolOpenAI selects the OpenAI-compatible adapter.
	ProtocolOpenAI = "openai"
	// ProtocolClaude selects the Claude/Anthropic Messages-compatible adapter.
	ProtocolClaude = "claude"

	// AuthAPIKey requires a resolvable API key source.
	AuthAPIKey = "api-key"
	// AuthNone disables API key resolution for no-key compatible endpoints.
	AuthNone = "none"
)

// Profile contains one named LLM provider configuration from settings.
type Profile struct {
	Protocol  string `json:"protocol,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	Model     string `json:"model,omitempty"`
	Auth      string `json:"auth,omitempty"`
	APIKeyEnv string `json:"api_key_env,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
}

// Settings contains the LLM section of ~/.foxharness/settings.json.
type Settings struct {
	DefaultProvider string             `json:"default_provider,omitempty"`
	Providers       map[string]Profile `json:"providers,omitempty"`
}

// CLIOverrides contains LLM fields supplied by command-line flags.
type CLIOverrides struct {
	ProviderID string
	Protocol   string
	BaseURL    string
	Model      string
	Auth       string
	APIKeyEnv  string
	APIKey     string
}

// EnvOverrides contains LLM fields supplied by foxharness-scoped environment
// variables.
type EnvOverrides struct {
	ProviderID string
	Protocol   string
	BaseURL    string
	Model      string
	Auth       string
	APIKeyEnv  string
	APIKey     string
}

// ResolvedConfig is the complete, validated provider configuration used to
// construct an LLM provider.
type ResolvedConfig struct {
	ProviderID   string
	Protocol     string
	BaseURL      string
	Model        string
	Auth         string
	APIKeyEnv    string
	APIKey       string
	APIKeySource string
}

// EnvLookup returns the value of an environment variable name.
type EnvLookup func(name string) string

// EnvOverridesFromLookup reads all supported FOXHARNESS_LLM_* overrides using
// lookup.
func EnvOverridesFromLookup(lookup EnvLookup) EnvOverrides {
	if lookup == nil {
		lookup = func(string) string { return "" }
	}
	return EnvOverrides{
		ProviderID: strings.TrimSpace(lookup("FOXHARNESS_LLM_PROVIDER")),
		Protocol:   strings.TrimSpace(lookup("FOXHARNESS_LLM_PROTOCOL")),
		BaseURL:    strings.TrimSpace(lookup("FOXHARNESS_LLM_BASE_URL")),
		Model:      strings.TrimSpace(lookup("FOXHARNESS_LLM_MODEL")),
		Auth:       strings.TrimSpace(lookup("FOXHARNESS_LLM_AUTH")),
		APIKeyEnv:  strings.TrimSpace(lookup("FOXHARNESS_LLM_API_KEY_ENV")),
		APIKey:     strings.TrimSpace(lookup("FOXHARNESS_LLM_API_KEY")),
	}
}

// Resolve applies settings, environment overrides, and CLI overrides in the
// configured priority order and returns a complete provider config.
func Resolve(settings Settings, env EnvOverrides, cli CLIOverrides, lookup EnvLookup) (ResolvedConfig, error) {
	if lookup == nil {
		lookup = func(string) string { return "" }
	}

	providerID := strings.TrimSpace(settings.DefaultProvider)
	if strings.TrimSpace(env.ProviderID) != "" {
		providerID = strings.TrimSpace(env.ProviderID)
	}
	if strings.TrimSpace(cli.ProviderID) != "" {
		providerID = strings.TrimSpace(cli.ProviderID)
	}

	profile, found := selectedProfile(settings, providerID)
	applyEnvOverrides(&profile, env)
	applyCLIOverrides(&profile, cli)

	resolved := ResolvedConfig{
		ProviderID: providerID,
		Protocol:   normalize(strings.ToLower(profile.Protocol)),
		BaseURL:    normalize(profile.BaseURL),
		Model:      normalize(profile.Model),
		Auth:       normalize(strings.ToLower(profile.Auth)),
		APIKeyEnv:  normalize(profile.APIKeyEnv),
		APIKey:     normalize(profile.APIKey),
	}
	if resolved.Auth == "" {
		resolved.Auth = AuthAPIKey
	}

	if !found && providerID != "" && !hasCompleteInlineConfig(resolved) {
		return ResolvedConfig{}, fmt.Errorf("provider profile %q not found", providerID)
	}
	if err := validateRequired(resolved); err != nil {
		return ResolvedConfig{}, err
	}
	if err := resolveAPIKey(&resolved, lookup); err != nil {
		return ResolvedConfig{}, err
	}
	return resolved, nil
}

// WithModel returns cfg with a different model while preserving all connection
// fields.
func (cfg ResolvedConfig) WithModel(model string) ResolvedConfig {
	cfg.Model = strings.TrimSpace(model)
	return cfg
}

func selectedProfile(settings Settings, providerID string) (Profile, bool) {
	if providerID == "" || settings.Providers == nil {
		return Profile{}, providerID == ""
	}
	profile, ok := settings.Providers[providerID]
	return profile, ok
}

func applyEnvOverrides(profile *Profile, env EnvOverrides) {
	if strings.TrimSpace(env.Protocol) != "" {
		profile.Protocol = env.Protocol
	}
	if strings.TrimSpace(env.BaseURL) != "" {
		profile.BaseURL = env.BaseURL
	}
	if strings.TrimSpace(env.Model) != "" {
		profile.Model = env.Model
	}
	if strings.TrimSpace(env.Auth) != "" {
		profile.Auth = env.Auth
	}
	applyAPIKeySourceOverride(profile, env.APIKeyEnv, env.APIKey)
}

func applyCLIOverrides(profile *Profile, cli CLIOverrides) {
	if strings.TrimSpace(cli.Protocol) != "" {
		profile.Protocol = cli.Protocol
	}
	if strings.TrimSpace(cli.BaseURL) != "" {
		profile.BaseURL = cli.BaseURL
	}
	if strings.TrimSpace(cli.Model) != "" {
		profile.Model = cli.Model
	}
	if strings.TrimSpace(cli.Auth) != "" {
		profile.Auth = cli.Auth
	}
	applyAPIKeySourceOverride(profile, cli.APIKeyEnv, cli.APIKey)
}

func applyAPIKeySourceOverride(profile *Profile, apiKeyEnv string, apiKey string) {
	apiKey = strings.TrimSpace(apiKey)
	apiKeyEnv = strings.TrimSpace(apiKeyEnv)
	switch {
	case apiKey != "":
		profile.APIKey = apiKey
		profile.APIKeyEnv = ""
	case apiKeyEnv != "":
		profile.APIKey = ""
		profile.APIKeyEnv = apiKeyEnv
	}
}

func hasCompleteInlineConfig(cfg ResolvedConfig) bool {
	if cfg.Protocol == "" || cfg.BaseURL == "" || cfg.Model == "" {
		return false
	}
	if cfg.Auth == AuthNone {
		return true
	}
	return cfg.APIKey != "" || cfg.APIKeyEnv != ""
}

func validateRequired(cfg ResolvedConfig) error {
	switch {
	case cfg.Protocol == "":
		return fmt.Errorf("missing LLM protocol")
	case cfg.BaseURL == "":
		return fmt.Errorf("missing LLM base_url")
	case cfg.Model == "":
		return fmt.Errorf("missing LLM model")
	}

	switch cfg.Protocol {
	case ProtocolOpenAI, ProtocolClaude:
	default:
		return fmt.Errorf("unsupported protocol %q; expected %q or %q", cfg.Protocol, ProtocolOpenAI, ProtocolClaude)
	}

	switch cfg.Auth {
	case AuthAPIKey, AuthNone:
	default:
		return fmt.Errorf("unsupported auth %q; expected %q or %q", cfg.Auth, AuthAPIKey, AuthNone)
	}
	return nil
}

func resolveAPIKey(cfg *ResolvedConfig, lookup EnvLookup) error {
	if cfg.Auth == AuthNone {
		cfg.APIKey = ""
		cfg.APIKeySource = ""
		return nil
	}
	if cfg.APIKey != "" {
		cfg.APIKeySource = "direct"
		return nil
	}
	if cfg.APIKeyEnv == "" {
		return fmt.Errorf("missing API key source: set api_key_env or api_key for auth %q", AuthAPIKey)
	}
	key := strings.TrimSpace(lookup(cfg.APIKeyEnv))
	if key == "" {
		return fmt.Errorf("API key environment variable %q is not set", cfg.APIKeyEnv)
	}
	cfg.APIKey = key
	cfg.APIKeySource = "env:" + cfg.APIKeyEnv
	return nil
}

func normalize(value string) string {
	return strings.TrimSpace(value)
}
