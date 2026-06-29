package configcmd

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/settings"
)

// ProviderFactory builds an LLM provider from a resolved configuration. It is
// injected so the connectivity probe is testable without real network calls.
type ProviderFactory func(llmconfig.ResolvedConfig) (provider.LLMProvider, error)

// Wizard drives the interactive `fox config` flows. Dependencies are injected so
// the flow is unit-testable with a scripted Prompter and a temporary home
// directory.
type Wizard struct {
	HomeDir     string
	Env         llmconfig.EnvLookup
	Prompter    Prompter
	Out         io.Writer
	NewProvider ProviderFactory // when nil, the connectivity probe is not offered
}

// AddProfile runs the guided add flow: choose a preset or custom entry, collect
// editable connection fields, collect the API key source, confirm overwrite when
// the profile id already exists, optionally set it as default, and persist.
func (w *Wizard) AddProfile() error {
	sourceOptions := make([]string, 0, len(Catalog)+1)
	for _, p := range Catalog {
		sourceOptions = append(sourceOptions, p.ID)
	}
	sourceOptions = append(sourceOptions, "Custom (enter all fields)")
	idx, err := w.Prompter.Choose("Choose a provider", sourceOptions)
	if err != nil {
		return err
	}

	custom := idx >= len(Catalog)
	var preset Preset
	if !custom {
		preset = Catalog[idx]
	}

	id, profile, err := w.collectConnectionFields(custom, preset)
	if err != nil {
		return err
	}
	profile, err = w.collectKey(profile, preset)
	if err != nil {
		return err
	}

	if err := w.offerProbe(id, profile); err != nil {
		return err
	}

	loaded, err := settings.Load(w.HomeDir)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if _, exists := loaded.LLM.Providers[id]; exists {
		ok, err := w.Prompter.YesNo(fmt.Sprintf("Profile %q already exists. Overwrite?", id), false)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("profile add aborted")
		}
	}

	if err := settings.SetProvider(loaded, id, profile); err != nil {
		return err
	}

	setDefault, err := w.Prompter.YesNo(fmt.Sprintf("Set %q as the default provider?", id), true)
	if err != nil {
		return err
	}
	if setDefault {
		if err := settings.SetDefaultProvider(loaded, id); err != nil {
			return err
		}
	}

	if err := settings.Save(w.HomeDir, loaded); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	fmt.Fprintf(w.Out, "Saved provider %q.\n", id)
	if setDefault {
		fmt.Fprintf(w.Out, "Default provider is now %q.\n", id)
	}
	return nil
}

// collectConnectionFields gathers protocol, base URL, model, auth mode, and
// profile id, pre-filled from the preset when one was selected.
func (w *Wizard) collectConnectionFields(custom bool, preset Preset) (string, llmconfig.Profile, error) {
	var profile llmconfig.Profile

	defProtocol := preset.Protocol
	protocol, err := w.validatedProtocol(defProtocol)
	if err != nil {
		return "", profile, err
	}
	profile.Protocol = protocol

	baseURL, err := w.Prompter.Line("API base URL", preset.BaseURL)
	if err != nil {
		return "", profile, err
	}
	profile.BaseURL = baseURL

	model, err := w.Prompter.Line("Default model", preset.Model)
	if err != nil {
		return "", profile, err
	}
	profile.Model = model

	auth := preset.Auth
	if auth == "" {
		auth = llmconfig.AuthAPIKey
	}
	if custom {
		idx, err := w.Prompter.Choose("Auth mode", []string{
			"api-key (requires an API key)",
			"none (local or no-key endpoint)",
		})
		if err != nil {
			return "", profile, err
		}
		if idx == 0 {
			auth = llmconfig.AuthAPIKey
		} else {
			auth = llmconfig.AuthNone
		}
	}
	profile.Auth = auth

	id, err := w.Prompter.Line("Profile name", preset.ID)
	if err != nil {
		return "", profile, err
	}
	return id, profile, nil
}

// validatedProtocol prompts for a protocol until a supported value is given.
func (w *Wizard) validatedProtocol(def string) (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		value, err := w.Prompter.Line("Protocol (openai or claude)", def)
		if err != nil {
			return "", err
		}
		if value == llmconfig.ProtocolOpenAI || value == llmconfig.ProtocolClaude {
			return value, nil
		}
		fmt.Fprintf(w.Out, "unsupported protocol %q; expected %q or %q\n", value, llmconfig.ProtocolOpenAI, llmconfig.ProtocolClaude)
	}
	return "", fmt.Errorf("invalid protocol after retries")
}

// collectKey gathers the API key source. For auth:none profiles it is a no-op.
// Otherwise it collects an api_key_env, runs a preflight check that the variable
// is currently set, and offers inline plaintext entry as a fallback when it is
// not. Inline entry requires explicit confirmation because it stores the secret
// in plaintext in settings.json.
func (w *Wizard) collectKey(profile llmconfig.Profile, preset Preset) (llmconfig.Profile, error) {
	if profile.Auth == llmconfig.AuthNone {
		return profile, nil
	}
	envName, err := w.Prompter.Line("API key environment variable", preset.APIKeyEnv)
	if err != nil {
		return profile, err
	}
	profile.APIKeyEnv = strings.TrimSpace(envName)
	profile.APIKey = ""

	resolved := ""
	if profile.APIKeyEnv != "" {
		resolved = strings.TrimSpace(w.Env(profile.APIKeyEnv))
	}
	if resolved != "" {
		return profile, nil
	}

	if profile.APIKeyEnv != "" {
		fmt.Fprintf(w.Out, "WARNING: environment variable %q is not set in the current shell.\n", profile.APIKeyEnv)
	} else {
		fmt.Fprintln(w.Out, "WARNING: no API key environment variable was provided.")
	}
	fmt.Fprintln(w.Out, "WARNING: an inline key will be stored in PLAINTEXT in ~/.foxharness/settings.json.")
	wantInline, err := w.Prompter.YesNo("Enter the API key inline now?", false)
	if err != nil {
		return profile, err
	}
	if !wantInline {
		fmt.Fprintf(w.Out, "Keeping %q as the key source; set it in your shell before running fox.\n", profile.APIKeyEnv)
		return profile, nil
	}
	secret, err := w.Prompter.Secret("API key")
	if err != nil {
		return profile, err
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return profile, fmt.Errorf("inline API key was empty")
	}
	profile.APIKey = secret
	profile.APIKeyEnv = ""
	fmt.Fprintln(w.Out, "Stored API key inline (plaintext).")
	return profile, nil
}

// offerProbe asks whether to test the connection and runs the probe when a
// provider factory is configured. The probe is advisory: failures are reported
// but never block saving.
func (w *Wizard) offerProbe(id string, profile llmconfig.Profile) error {
	if w.NewProvider == nil {
		return nil
	}
	want, err := w.Prompter.YesNo("Test the connection now?", false)
	if err != nil {
		return err
	}
	if !want {
		return nil
	}
	w.probe(id, profile)
	return nil
}

// probe builds a resolved configuration from the collected profile and sends a
// minimal request, reporting success or the failure reason. It never returns a
// blocking error.
func (w *Wizard) probe(id string, profile llmconfig.Profile) {
	probeSettings := llmconfig.Settings{
		DefaultProvider: id,
		Providers:       map[string]llmconfig.Profile{id: profile},
	}
	resolved, err := llmconfig.Resolve(probeSettings, llmconfig.EnvOverrides{}, llmconfig.CLIOverrides{}, w.Env)
	if err != nil {
		fmt.Fprintf(w.Out, "Probe skipped: configuration unresolved (%v).\n", err)
		return
	}
	llm, err := w.NewProvider(resolved)
	if err != nil {
		fmt.Fprintf(w.Out, "Probe failed: %v\n", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := llm.Generate(ctx, []schema.Message{{Role: schema.RoleUser, Content: "ping"}}, nil); err != nil {
		fmt.Fprintf(w.Out, "Probe failed: %v\n", err)
		return
	}
	fmt.Fprintln(w.Out, "Connection OK.")
}

// ListProfiles prints the saved provider profiles in stable order and marks the
// default. It reports an empty state when none are configured.
func (w *Wizard) ListProfiles() error {
	loaded, err := settings.Load(w.HomeDir)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if len(loaded.LLM.Providers) == 0 {
		fmt.Fprintln(w.Out, "No provider profiles configured. Run `fox config` and choose a provider to add one.")
		return nil
	}
	fmt.Fprintf(w.Out, "Configured providers (%d):\n", len(loaded.LLM.Providers))
	for _, id := range sortedProviderIDs(loaded.LLM.Providers) {
		profile := loaded.LLM.Providers[id]
		marker := "  "
		if id == loaded.LLM.DefaultProvider {
			marker = "* "
		}
		fmt.Fprintf(w.Out, "%s%-14s %s  %s\n", marker, id, profile.Protocol, profile.Model)
	}
	if loaded.LLM.DefaultProvider == "" {
		fmt.Fprintln(w.Out, "No default provider set.")
	}
	return nil
}

// SetDefault sets the default provider. When id is empty, it prompts the user to
// choose from the saved profiles.
func (w *Wizard) SetDefault(id string) error {
	loaded, err := settings.Load(w.HomeDir)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if id == "" {
		ids := sortedProviderIDs(loaded.LLM.Providers)
		if len(ids) == 0 {
			fmt.Fprintln(w.Out, "No provider profiles configured.")
			return nil
		}
		idx, err := w.Prompter.Choose("Default provider", ids)
		if err != nil {
			return err
		}
		id = ids[idx]
	}
	if err := settings.SetDefaultProvider(loaded, id); err != nil {
		return err
	}
	if err := settings.Save(w.HomeDir, loaded); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	fmt.Fprintf(w.Out, "Default provider is now %q.\n", id)
	return nil
}

func sortedProviderIDs(providers map[string]llmconfig.Profile) []string {
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
