package configcmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/settings"
)

func newTestWizard(t *testing.T, fp *fakePrompter) (*Wizard, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	return &Wizard{
		HomeDir:  t.TempDir(),
		Env:      func(string) string { return "set" },
		Prompter: fp,
		Out:      out,
	}, out
}

func newWizardEnvUnset(t *testing.T, fp *fakePrompter) (*Wizard, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	return &Wizard{
		HomeDir:  t.TempDir(),
		Env:      func(string) string { return "" },
		Prompter: fp,
		Out:      out,
	}, out
}

func loadPersisted(t *testing.T, home string) *settings.Settings {
	t.Helper()
	s, err := settings.Load(home)
	if err != nil {
		t.Fatalf("Load persisted: %v", err)
	}
	return s
}

func TestAddProfilePresetPreFillsAndPersists(t *testing.T) {
	fp := &fakePrompter{
		choices: []int{0},                     // openai preset
		lines:   []string{"", "", "", "", ""}, // protocol, base, model, name, api_key_env (all defaults)
		yesnos:  []bool{true},                 // set as default
	}
	w, _ := newTestWizard(t, fp)

	if err := w.AddProfile(); err != nil {
		t.Fatalf("AddProfile() error = %v", err)
	}

	s := loadPersisted(t, w.HomeDir)
	profile := s.LLM.Providers["openai"]
	if profile.Protocol != llmconfig.ProtocolOpenAI {
		t.Errorf("protocol = %q, want openai", profile.Protocol)
	}
	if profile.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("base_url = %q, want preset default", profile.BaseURL)
	}
	if profile.Model != "gpt-4o-mini" || profile.APIKeyEnv != "OPENAI_API_KEY" {
		t.Errorf("model/api_key_env = %q/%q, want preset defaults", profile.Model, profile.APIKeyEnv)
	}
	if profile.Auth != llmconfig.AuthAPIKey {
		t.Errorf("auth = %q, want api-key", profile.Auth)
	}
	if s.LLM.DefaultProvider != "openai" {
		t.Errorf("default_provider = %q, want openai", s.LLM.DefaultProvider)
	}
}

func TestAddProfileCustomCollectsEachField(t *testing.T) {
	customIdx := len(Catalog) // the "Custom" option follows the presets
	fp := &fakePrompter{
		choices: []int{customIdx, 0}, // custom source, api-key auth
		lines:   []string{"openai", "https://x.test/v1", "m", "myprov", "MY_KEY"},
		yesnos:  []bool{false}, // do not set as default
	}
	w, _ := newTestWizard(t, fp)

	if err := w.AddProfile(); err != nil {
		t.Fatalf("AddProfile() error = %v", err)
	}

	s := loadPersisted(t, w.HomeDir)
	profile := s.LLM.Providers["myprov"]
	if profile.Protocol != "openai" || profile.BaseURL != "https://x.test/v1" ||
		profile.Model != "m" || profile.APIKeyEnv != "MY_KEY" {
		t.Errorf("profile = %+v, want entered custom fields", profile)
	}
	if s.LLM.DefaultProvider != "" {
		t.Errorf("default_provider = %q, want empty (declined)", s.LLM.DefaultProvider)
	}
}

func TestAddProfileRejectsUnsupportedProtocolThenRetries(t *testing.T) {
	customIdx := len(Catalog)
	fp := &fakePrompter{
		choices: []int{customIdx, 0},
		lines:   []string{"custom", "openai", "https://x.test/v1", "m", "myprov", "MY_KEY"},
		yesnos:  []bool{false},
	}
	w, out := newTestWizard(t, fp)

	if err := w.AddProfile(); err != nil {
		t.Fatalf("AddProfile() error = %v", err)
	}
	if !strings.Contains(out.String(), "unsupported protocol") {
		t.Errorf("output = %q, want unsupported protocol warning", out.String())
	}
	s := loadPersisted(t, w.HomeDir)
	if got := s.LLM.Providers["myprov"].Protocol; got != "openai" {
		t.Errorf("protocol = %q, want openai after retry", got)
	}
}

func TestAddProfileDuplicateOverwriteConfirm(t *testing.T) {
	// Seed an existing provider.
	seedHome := ""
	_ = seedHome

	t.Run("decline_aborts_without_change", func(t *testing.T) {
		fp := &fakePrompter{
			choices: []int{0},
			lines:   []string{"", "", "", "", ""},
			yesnos:  []bool{false}, // decline overwrite
		}
		w, _ := newTestWizard(t, fp)
		// Pre-seed an openai profile with a distinct model.
		seed := &settings.Settings{}
		settings.SetProvider(seed, "openai", llmconfig.Profile{Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://seed.test/v1", Model: "seed-model", APIKeyEnv: "SEED_KEY"})
		if err := settings.Save(w.HomeDir, seed); err != nil {
			t.Fatal(err)
		}

		err := w.AddProfile()
		if err == nil {
			t.Fatal("AddProfile() error = nil, want abort error on declined overwrite")
		}

		s := loadPersisted(t, w.HomeDir)
		if got := s.LLM.Providers["openai"].Model; got != "seed-model" {
			t.Errorf("model = %q, want unchanged seed-model", got)
		}
	})

	t.Run("accept_overwrites", func(t *testing.T) {
		fp := &fakePrompter{
			choices: []int{0},
			lines:   []string{"", "", "", "", ""},
			yesnos:  []bool{true, false}, // accept overwrite, decline set-default
		}
		w, _ := newTestWizard(t, fp)
		seed := &settings.Settings{}
		settings.SetProvider(seed, "openai", llmconfig.Profile{Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://seed.test/v1", Model: "seed-model", APIKeyEnv: "SEED_KEY"})
		settings.Save(w.HomeDir, seed)

		if err := w.AddProfile(); err != nil {
			t.Fatalf("AddProfile() error = %v", err)
		}
		s := loadPersisted(t, w.HomeDir)
		if got := s.LLM.Providers["openai"].Model; got != "gpt-4o-mini" {
			t.Errorf("model = %q, want overwritten preset default", got)
		}
	})
}

func TestCollectKeyPreflightWarnsWhenEnvUnset(t *testing.T) {
	fp := &fakePrompter{
		choices: []int{0},                     // openai preset
		lines:   []string{"", "", "", "", ""}, // proto, base, model, name, api_key_env (defaults)
		yesnos:  []bool{false, false},         // decline inline, decline set-default
	}
	w, out := newWizardEnvUnset(t, fp)

	if err := w.AddProfile(); err != nil {
		t.Fatalf("AddProfile() error = %v", err)
	}
	if !strings.Contains(out.String(), "not set") {
		t.Errorf("output = %q, want a preflight warning that the env var is not set", out.String())
	}
	s := loadPersisted(t, w.HomeDir)
	profile := s.LLM.Providers["openai"]
	if profile.APIKeyEnv != "OPENAI_API_KEY" {
		t.Errorf("APIKeyEnv = %q, want kept env reference", profile.APIKeyEnv)
	}
	if profile.APIKey != "" {
		t.Errorf("APIKey = %q, want empty (inline declined)", profile.APIKey)
	}
}

func TestCollectKeyInlineOptInStoresPlaintext(t *testing.T) {
	fp := &fakePrompter{
		choices: []int{0},
		lines:   []string{"", "", "", "", ""},
		yesnos:  []bool{true, false}, // accept inline, decline set-default
		secrets: []string{"sk-test"},
	}
	w, out := newWizardEnvUnset(t, fp)

	if err := w.AddProfile(); err != nil {
		t.Fatalf("AddProfile() error = %v", err)
	}
	if !strings.Contains(out.String(), "PLAINTEXT") {
		t.Errorf("output = %q, want a plaintext warning before inline storage", out.String())
	}
	s := loadPersisted(t, w.HomeDir)
	profile := s.LLM.Providers["openai"]
	if profile.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want inline sk-test", profile.APIKey)
	}
	if profile.APIKeyEnv != "" {
		t.Errorf("APIKeyEnv = %q, want cleared after inline entry", profile.APIKeyEnv)
	}
}

func TestCollectKeyAuthNoneSkipsKey(t *testing.T) {
	fp := &fakePrompter{
		choices: []int{11},                // ollama preset (auth none)
		lines:   []string{"", "", "", ""}, // proto, base, model, name (no api_key_env step)
		yesnos:  []bool{false},            // decline set-default
	}
	w, _ := newWizardEnvUnset(t, fp)

	if err := w.AddProfile(); err != nil {
		t.Fatalf("AddProfile() error = %v", err)
	}
	s := loadPersisted(t, w.HomeDir)
	profile := s.LLM.Providers["ollama"]
	if profile.Auth != llmconfig.AuthNone {
		t.Errorf("Auth = %q, want none", profile.Auth)
	}
	if profile.APIKeyEnv != "" || profile.APIKey != "" {
		t.Errorf("key fields = %q/%q, want both empty for auth none", profile.APIKeyEnv, profile.APIKey)
	}
}

type fakeProvider struct {
	err    error
	called bool
}

func (f *fakeProvider) Generate(ctx context.Context, msgs []schema.Message, tools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	f.called = true
	if f.err != nil {
		return nil, f.err
	}
	return &provider.GenerateResponse{}, nil
}

func TestProbeReportsSuccess(t *testing.T) {
	fp := &fakeProvider{}
	wizard := &Wizard{
		HomeDir:     t.TempDir(),
		Env:         func(string) string { return "set" },
		Prompter:    &fakePrompter{choices: []int{0}, lines: []string{"", "", "", "", ""}, yesnos: []bool{true, false}},
		Out:         &bytes.Buffer{},
		NewProvider: func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return fp, nil },
	}
	if err := wizard.AddProfile(); err != nil {
		t.Fatalf("AddProfile() error = %v", err)
	}
	if !fp.called {
		t.Fatal("probe provider was not invoked")
	}
	if !strings.Contains(wizard.Out.(*bytes.Buffer).String(), "Connection OK") {
		t.Errorf("output = %q, want Connection OK", wizard.Out.(*bytes.Buffer).String())
	}
}

func TestProbeReportsFailure(t *testing.T) {
	fp := &fakeProvider{err: errors.New("401 unauthorized")}
	wizard := &Wizard{
		HomeDir:     t.TempDir(),
		Env:         func(string) string { return "set" },
		Prompter:    &fakePrompter{choices: []int{0}, lines: []string{"", "", "", "", ""}, yesnos: []bool{true, false}},
		Out:         &bytes.Buffer{},
		NewProvider: func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return fp, nil },
	}
	if err := wizard.AddProfile(); err != nil {
		t.Fatalf("AddProfile() error = %v", err)
	}
	out := wizard.Out.(*bytes.Buffer).String()
	if !strings.Contains(out, "Probe failed") || !strings.Contains(out, "401 unauthorized") {
		t.Errorf("output = %q, want Probe failed with the reason", out)
	}
}

func TestProbeSkippable(t *testing.T) {
	fp := &fakeProvider{}
	wizard := &Wizard{
		HomeDir:     t.TempDir(),
		Env:         func(string) string { return "set" },
		Prompter:    &fakePrompter{choices: []int{0}, lines: []string{"", "", "", "", ""}, yesnos: []bool{false, false}},
		Out:         &bytes.Buffer{},
		NewProvider: func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return fp, nil },
	}
	if err := wizard.AddProfile(); err != nil {
		t.Fatalf("AddProfile() error = %v", err)
	}
	if fp.called {
		t.Fatal("probe provider was invoked despite skipping")
	}
	out := wizard.Out.(*bytes.Buffer).String()
	if strings.Contains(out, "Connection OK") || strings.Contains(out, "Probe failed") {
		t.Errorf("output = %q, want no probe result when skipped", out)
	}
}
