package llmresolve

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

func TestFromUserSettingsLoadsConfiguredProvider(t *testing.T) {
	home := t.TempDir()
	settingsDir := filepath.Join(home, ".foxharness")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{
  "llm": {
    "default_provider": "local",
    "providers": {
      "local": {
        "protocol": "openai",
        "base_url": "http://127.0.0.1:1234/v1",
        "model": "local-model",
        "auth": "none"
      }
    }
  }
}`)
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := FromUserSettings(home, llmconfig.CLIOverrides{}, nil)
	if err != nil {
		t.Fatalf("FromUserSettings() error = %v", err)
	}
	if cfg.ProviderID != "local" || cfg.Protocol != "openai" || cfg.Model != "local-model" || cfg.Auth != "none" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestFromUserSettingsWrapsResolveFailures(t *testing.T) {
	_, err := FromUserSettings(t.TempDir(), llmconfig.CLIOverrides{}, nil)
	if err == nil {
		t.Fatal("FromUserSettings() error = nil, want missing provider error")
	}
	if !errors.Is(err, llmconfig.ErrNoProviderConfigured) {
		t.Fatalf("error = %v, want ErrNoProviderConfigured", err)
	}
	if !strings.Contains(err.Error(), "resolve LLM configuration") {
		t.Fatalf("error = %q, want wrapping context", err.Error())
	}
}
