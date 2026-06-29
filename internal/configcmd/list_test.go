package configcmd

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/settings"
)

func seedSettings(t *testing.T, home string, providers map[string]llmconfig.Profile, def string) {
	t.Helper()
	s := &settings.Settings{}
	for id, p := range providers {
		if err := settings.SetProvider(s, id, p); err != nil {
			t.Fatal(err)
		}
	}
	if def != "" {
		if err := settings.SetDefaultProvider(s, def); err != nil {
			t.Fatal(err)
		}
	}
	if err := settings.Save(home, s); err != nil {
		t.Fatal(err)
	}
}

func TestListProfilesMarksDefault(t *testing.T) {
	w, out := newTestWizard(t, &fakePrompter{})
	seedSettings(t, w.HomeDir, map[string]llmconfig.Profile{
		"alpha": {Protocol: llmconfig.ProtocolOpenAI, Model: "a-model"},
		"zeta":  {Protocol: llmconfig.ProtocolClaude, Model: "z-model"},
	}, "zeta")

	if err := w.ListProfiles(); err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "* zeta") {
		t.Errorf("output = %q, want default marker on zeta", got)
	}
	if !strings.Contains(got, "  alpha") {
		t.Errorf("output = %q, want non-default alpha entry", got)
	}
}

func TestListProfilesEmpty(t *testing.T) {
	w, out := newTestWizard(t, &fakePrompter{})
	if err := w.ListProfiles(); err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if !strings.Contains(out.String(), "No provider profiles") {
		t.Errorf("output = %q, want empty-state guidance", out.String())
	}
}

func TestSetDefaultByID(t *testing.T) {
	w, _ := newTestWizard(t, &fakePrompter{})
	seedSettings(t, w.HomeDir, map[string]llmconfig.Profile{"a": {Model: "a"}, "b": {Model: "b"}}, "a")

	if err := w.SetDefault("b"); err != nil {
		t.Fatalf("SetDefault() error = %v", err)
	}
	s := loadPersisted(t, w.HomeDir)
	if s.LLM.DefaultProvider != "b" {
		t.Errorf("default_provider = %q, want b", s.LLM.DefaultProvider)
	}
}

func TestSetDefaultByChoose(t *testing.T) {
	fp := &fakePrompter{choices: []int{1}} // second id in sorted order
	w, _ := newTestWizard(t, fp)
	seedSettings(t, w.HomeDir, map[string]llmconfig.Profile{"a": {Model: "a"}, "b": {Model: "b"}}, "a")

	if err := w.SetDefault(""); err != nil {
		t.Fatalf("SetDefault() error = %v", err)
	}
	s := loadPersisted(t, w.HomeDir)
	if s.LLM.DefaultProvider != "b" {
		t.Errorf("default_provider = %q, want b (chosen)", s.LLM.DefaultProvider)
	}
}

func TestSetDefaultRejectsUnknown(t *testing.T) {
	w, _ := newTestWizard(t, &fakePrompter{})
	seedSettings(t, w.HomeDir, map[string]llmconfig.Profile{"a": {Model: "a"}}, "a")
	if err := w.SetDefault("missing"); err == nil {
		t.Fatal("SetDefault(unknown) error = nil, want error")
	}
}
