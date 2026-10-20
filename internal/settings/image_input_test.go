package settings

import (
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"testing"
)

func TestImageInputSurvivesSettingsSaveAndReload(t *testing.T) {
	home := t.TempDir()
	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetProvider(s, "text-only", llmconfig.Profile{Protocol: "openai", Model: "m", ImageInput: "disabled"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(home, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LLM.Providers["text-only"].ImageInput != "disabled" {
		t.Fatal("lost image policy")
	}
}
