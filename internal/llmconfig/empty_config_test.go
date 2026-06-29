package llmconfig

import (
	"errors"
	"testing"
)

// TestResolveEmptyInputReturnsNoProviderConfigured verifies that completely
// empty resolution inputs produce the onboarding sentinel rather than the bare
// field error, so the CLI can guide the user to `fox config`.
func TestResolveEmptyInputReturnsNoProviderConfigured(t *testing.T) {
	_, err := Resolve(Settings{}, EnvOverrides{}, CLIOverrides{}, mapEnv{}.Lookup)
	if !errors.Is(err, ErrNoProviderConfigured) {
		t.Fatalf("Resolve() error = %v, want ErrNoProviderConfigured", err)
	}
}

// TestResolveNonEmptyInputNeverReportsNoProviderConfigured verifies the guard
// intercepts only the entirely-empty case: any non-empty input (a default
// provider, any env field, any CLI field, or a present profile) must fall
// through to the existing field-specific errors instead.
func TestResolveNonEmptyInputNeverReportsNoProviderConfigured(t *testing.T) {
	cases := []struct {
		name     string
		settings Settings
		env      EnvOverrides
		cli      CLIOverrides
	}{
		{name: "default_provider_set", settings: Settings{DefaultProvider: "missing", Providers: map[string]Profile{}}, env: EnvOverrides{}, cli: CLIOverrides{}},
		{name: "env_protocol_only", settings: Settings{}, env: EnvOverrides{Protocol: ProtocolOpenAI}, cli: CLIOverrides{}},
		{name: "cli_provider_only", settings: Settings{}, env: EnvOverrides{}, cli: CLIOverrides{ProviderID: "x"}},
		{name: "settings_provider_present", settings: Settings{Providers: map[string]Profile{"p": {Protocol: ProtocolOpenAI}}}, env: EnvOverrides{}, cli: CLIOverrides{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(tc.settings, tc.env, tc.cli, mapEnv{}.Lookup)
			if err == nil {
				return
			}
			if errors.Is(err, ErrNoProviderConfigured) {
				t.Fatalf("Resolve() returned ErrNoProviderConfigured for non-empty input %q; want a field-specific error", tc.name)
			}
		})
	}
}
