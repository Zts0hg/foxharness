package llmconfig

import (
	"strings"
	"testing"
)

func TestResolveSelectsConfiguredDefaultProvider(t *testing.T) {
	env := mapEnv{"OPENAI_KEY": "secret"}
	got, err := Resolve(Settings{
		DefaultProvider: "primary",
		Providers: map[string]Profile{
			"primary": {
				Protocol:  ProtocolOpenAI,
				BaseURL:   "https://example.test/v1",
				Model:     "test-model",
				APIKeyEnv: "OPENAI_KEY",
			},
		},
	}, EnvOverrides{}, CLIOverrides{}, env.Lookup)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.ProviderID != "primary" {
		t.Fatalf("ProviderID = %q, want primary", got.ProviderID)
	}
	if got.SettingsProviderID != "primary" {
		t.Fatalf("SettingsProviderID = %q, want primary", got.SettingsProviderID)
	}
	if got.Protocol != ProtocolOpenAI {
		t.Fatalf("Protocol = %q, want openai", got.Protocol)
	}
	if got.BaseURL != "https://example.test/v1" {
		t.Fatalf("BaseURL = %q, want configured value", got.BaseURL)
	}
	if got.Model != "test-model" {
		t.Fatalf("Model = %q, want test-model", got.Model)
	}
	if got.Auth != AuthAPIKey {
		t.Fatalf("Auth = %q, want api-key default", got.Auth)
	}
	if got.APIKey != "secret" {
		t.Fatal("APIKey was not resolved from APIKeyEnv")
	}
	if got.APIKeySource != "env:OPENAI_KEY" {
		t.Fatalf("APIKeySource = %q, want env:OPENAI_KEY", got.APIKeySource)
	}
}

func TestResolveProfileSelectionPriority(t *testing.T) {
	env := mapEnv{"PRIMARY_KEY": "primary-key", "ENV_KEY": "env-key", "CLI_KEY": "cli-key"}
	settings := Settings{
		DefaultProvider: "primary",
		Providers: map[string]Profile{
			"primary": {Protocol: ProtocolOpenAI, BaseURL: "https://primary.test/v1", Model: "primary-model", APIKeyEnv: "PRIMARY_KEY"},
			"env":     {Protocol: ProtocolClaude, BaseURL: "https://env.test", Model: "env-model", APIKeyEnv: "ENV_KEY"},
			"cli":     {Protocol: ProtocolOpenAI, BaseURL: "https://cli.test/v1", Model: "cli-model", APIKeyEnv: "CLI_KEY"},
		},
	}

	got, err := Resolve(settings, EnvOverrides{ProviderID: "env"}, CLIOverrides{}, env.Lookup)
	if err != nil {
		t.Fatalf("Resolve() with env provider error = %v", err)
	}
	if got.ProviderID != "env" || got.Model != "env-model" {
		t.Fatalf("env provider not selected: got id=%q model=%q", got.ProviderID, got.Model)
	}
	if got.SettingsProviderID != "env" {
		t.Fatalf("SettingsProviderID = %q, want env", got.SettingsProviderID)
	}

	got, err = Resolve(settings, EnvOverrides{ProviderID: "env"}, CLIOverrides{ProviderID: "cli"}, env.Lookup)
	if err != nil {
		t.Fatalf("Resolve() with cli provider error = %v", err)
	}
	if got.ProviderID != "cli" || got.Model != "cli-model" {
		t.Fatalf("cli provider not selected: got id=%q model=%q", got.ProviderID, got.Model)
	}
	if got.SettingsProviderID != "cli" {
		t.Fatalf("SettingsProviderID = %q, want cli", got.SettingsProviderID)
	}
}

func TestResolveFieldOverridePriority(t *testing.T) {
	env := mapEnv{"SETTINGS_KEY": "settings-key", "ENV_KEY": "env-key", "CLI_KEY": "cli-key"}
	got, err := Resolve(Settings{
		DefaultProvider: "primary",
		Providers: map[string]Profile{
			"primary": {
				Protocol:  ProtocolOpenAI,
				BaseURL:   "https://settings.test/v1",
				Model:     "settings-model",
				APIKeyEnv: "SETTINGS_KEY",
			},
		},
	}, EnvOverrides{
		Protocol:  ProtocolClaude,
		BaseURL:   "https://env.test",
		Model:     "env-model",
		APIKeyEnv: "ENV_KEY",
	}, CLIOverrides{
		BaseURL:   "https://cli.test",
		Model:     "cli-model",
		APIKeyEnv: "CLI_KEY",
	}, env.Lookup)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Protocol != ProtocolClaude {
		t.Fatalf("Protocol = %q, want env override claude", got.Protocol)
	}
	if got.BaseURL != "https://cli.test" {
		t.Fatalf("BaseURL = %q, want CLI override", got.BaseURL)
	}
	if got.Model != "cli-model" {
		t.Fatalf("Model = %q, want CLI override", got.Model)
	}
	if got.APIKey != "cli-key" {
		t.Fatalf("APIKey = %q, want CLI env key value", got.APIKey)
	}
}

func TestResolveCompleteInlineConfigWithoutProviderProfile(t *testing.T) {
	got, err := Resolve(Settings{}, EnvOverrides{
		Protocol: ProtocolOpenAI,
		BaseURL:  "http://127.0.0.1:11434/v1",
		Model:    "local-model",
		Auth:     AuthNone,
	}, CLIOverrides{}, mapEnv{}.Lookup)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.ProviderID != "" {
		t.Fatalf("ProviderID = %q, want empty inline provider", got.ProviderID)
	}
	if got.Auth != AuthNone {
		t.Fatalf("Auth = %q, want none", got.Auth)
	}
}

func TestResolveUnknownProviderRequiresCompleteInlineConfig(t *testing.T) {
	_, err := Resolve(Settings{
		DefaultProvider: "missing",
		Providers:       map[string]Profile{},
	}, EnvOverrides{}, CLIOverrides{}, mapEnv{}.Lookup)
	if err == nil {
		t.Fatal("Resolve() error = nil, want unknown provider error")
	}
	if !strings.Contains(err.Error(), `provider profile "missing" not found`) {
		t.Fatalf("error = %q, want missing provider id", err.Error())
	}
}

func TestResolveUnknownProviderWithCompleteInlineConfigHasNoSettingsProvider(t *testing.T) {
	got, err := Resolve(Settings{
		DefaultProvider: "primary",
		Providers:       map[string]Profile{},
	}, EnvOverrides{}, CLIOverrides{
		ProviderID: "typo",
		Protocol:   ProtocolOpenAI,
		BaseURL:    "http://127.0.0.1:11434/v1",
		Model:      "local-model",
		Auth:       AuthNone,
	}, mapEnv{}.Lookup)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.ProviderID != "typo" {
		t.Fatalf("ProviderID = %q, want typo", got.ProviderID)
	}
	if got.SettingsProviderID != "" {
		t.Fatalf("SettingsProviderID = %q, want empty for inline config", got.SettingsProviderID)
	}
}

func TestResolveValidatesRequiredFields(t *testing.T) {
	cases := []struct {
		name    string
		profile Profile
		want    string
	}{
		{name: "protocol", profile: Profile{BaseURL: "https://example.test", Model: "m", Auth: AuthNone}, want: "protocol"},
		{name: "base_url", profile: Profile{Protocol: ProtocolOpenAI, Model: "m", Auth: AuthNone}, want: "base_url"},
		{name: "model", profile: Profile{Protocol: ProtocolOpenAI, BaseURL: "https://example.test", Auth: AuthNone}, want: "model"},
		{name: "unsupported_protocol", profile: Profile{Protocol: "custom", BaseURL: "https://example.test", Model: "m", Auth: AuthNone}, want: "unsupported protocol"},
		{name: "unsupported_auth", profile: Profile{Protocol: ProtocolOpenAI, BaseURL: "https://example.test", Model: "m", Auth: "bearer"}, want: "unsupported auth"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(Settings{DefaultProvider: "p", Providers: map[string]Profile{"p": tc.profile}}, EnvOverrides{}, CLIOverrides{}, mapEnv{}.Lookup)
			if err == nil {
				t.Fatal("Resolve() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestResolveAPIKeyAuthRequiresResolvableSource(t *testing.T) {
	settings := Settings{
		DefaultProvider: "primary",
		Providers: map[string]Profile{
			"primary": {Protocol: ProtocolOpenAI, BaseURL: "https://example.test/v1", Model: "m"},
		},
	}
	_, err := Resolve(settings, EnvOverrides{}, CLIOverrides{}, mapEnv{}.Lookup)
	if err == nil {
		t.Fatal("Resolve() error = nil, want missing API key source error")
	}
	if !strings.Contains(err.Error(), "api_key_env") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %q, want source name without secret", err.Error())
	}

	settings.Providers["primary"] = Profile{Protocol: ProtocolOpenAI, BaseURL: "https://example.test/v1", Model: "m", APIKeyEnv: "OPENAI_KEY"}
	_, err = Resolve(settings, EnvOverrides{}, CLIOverrides{}, mapEnv{}.Lookup)
	if err == nil {
		t.Fatal("Resolve() error = nil, want unresolved env var error")
	}
	if !strings.Contains(err.Error(), "OPENAI_KEY") {
		t.Fatalf("error = %q, want env var name", err.Error())
	}
}

func TestResolveEnvAPIKeyEnvOverrideReplacesSettingsDirectAPIKey(t *testing.T) {
	got, err := Resolve(Settings{
		DefaultProvider: "primary",
		Providers: map[string]Profile{
			"primary": {
				Protocol: ProtocolOpenAI,
				BaseURL:  "https://example.test/v1",
				Model:    "m",
				APIKey:   "settings-secret",
			},
		},
	}, EnvOverrides{APIKeyEnv: "ENV_KEY"}, CLIOverrides{}, mapEnv{"ENV_KEY": "env-secret"}.Lookup)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.APIKey != "env-secret" {
		t.Fatalf("APIKey = %q, want env-secret", got.APIKey)
	}
	if got.APIKeySource != "env:ENV_KEY" {
		t.Fatalf("APIKeySource = %q, want env:ENV_KEY", got.APIKeySource)
	}
}

func TestResolveCLIAPIKeyEnvOverrideReplacesEnvDirectAPIKey(t *testing.T) {
	got, err := Resolve(Settings{
		DefaultProvider: "primary",
		Providers: map[string]Profile{
			"primary": {
				Protocol:  ProtocolOpenAI,
				BaseURL:   "https://example.test/v1",
				Model:     "m",
				APIKeyEnv: "SETTINGS_KEY",
			},
		},
	}, EnvOverrides{APIKey: "env-secret"}, CLIOverrides{APIKeyEnv: "CLI_KEY"}, mapEnv{
		"SETTINGS_KEY": "settings-secret",
		"CLI_KEY":      "cli-secret",
	}.Lookup)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.APIKey != "cli-secret" {
		t.Fatalf("APIKey = %q, want cli-secret", got.APIKey)
	}
	if got.APIKeySource != "env:CLI_KEY" {
		t.Fatalf("APIKeySource = %q, want env:CLI_KEY", got.APIKeySource)
	}
}

func TestResolveAuthNoneDoesNotRequireOrResolveAPIKey(t *testing.T) {
	got, err := Resolve(Settings{
		DefaultProvider: "local",
		Providers: map[string]Profile{
			"local": {Protocol: ProtocolOpenAI, BaseURL: "http://127.0.0.1:11434/v1", Model: "local-model", Auth: AuthNone, APIKeyEnv: "OPENAI_API_KEY"},
		},
	}, EnvOverrides{}, CLIOverrides{}, mapEnv{"OPENAI_API_KEY": "secret"}.Lookup)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.APIKey != "" || got.APIKeySource != "" {
		t.Fatalf("API key resolved for auth none: key=%q source=%q", got.APIKey, got.APIKeySource)
	}
}

func TestEnvOverridesFromLookup(t *testing.T) {
	got := EnvOverridesFromLookup(mapEnv{
		"FOXHARNESS_LLM_PROVIDER":    "openrouter",
		"FOXHARNESS_LLM_PROTOCOL":    "openai",
		"FOXHARNESS_LLM_BASE_URL":    "https://openrouter.ai/api/v1",
		"FOXHARNESS_LLM_MODEL":       "model",
		"FOXHARNESS_LLM_AUTH":        "api-key",
		"FOXHARNESS_LLM_API_KEY_ENV": "OPENROUTER_API_KEY",
		"FOXHARNESS_LLM_API_KEY":     "secret",
	}.Lookup)
	want := EnvOverrides{
		ProviderID: "openrouter",
		Protocol:   "openai",
		BaseURL:    "https://openrouter.ai/api/v1",
		Model:      "model",
		Auth:       "api-key",
		APIKeyEnv:  "OPENROUTER_API_KEY",
		APIKey:     "secret",
	}
	if got != want {
		t.Fatalf("EnvOverridesFromLookup() = %+v, want %+v", got, want)
	}
}

func TestResolvedConfigWithModelPreservesConnectionFields(t *testing.T) {
	cfg := ResolvedConfig{
		ProviderID:         "primary",
		SettingsProviderID: "primary",
		Protocol:           ProtocolClaude,
		BaseURL:            "https://example.test",
		Model:              "old",
		Auth:               AuthAPIKey,
		APIKey:             "secret",
		APIKeyEnv:          "KEY",
		APIKeySource:       "env:KEY",
	}
	got := cfg.WithModel("new")
	if got.Model != "new" {
		t.Fatalf("Model = %q, want new", got.Model)
	}
	if got.ProviderID != cfg.ProviderID || got.SettingsProviderID != cfg.SettingsProviderID || got.Protocol != cfg.Protocol || got.BaseURL != cfg.BaseURL || got.APIKey != cfg.APIKey {
		t.Fatalf("WithModel changed connection fields: got %+v want based on %+v", got, cfg)
	}
}

type mapEnv map[string]string

func (m mapEnv) Lookup(name string) string {
	return m[name]
}
