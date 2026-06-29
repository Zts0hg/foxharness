package configcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

func TestRunRequiresInteractiveTerminal(t *testing.T) {
	var stderr bytes.Buffer
	deps := Deps{HomeDir: t.TempDir(), Stderr: &stderr, Interactive: false}
	err := Run(context.Background(), deps, nil)
	if err == nil {
		t.Fatal("Run() error = nil, want error for non-interactive terminal")
	}
	if !strings.Contains(stderr.String(), "interactive terminal") {
		t.Errorf("stderr = %q, want interactive terminal guidance", stderr.String())
	}
}

func TestRunDispatchesList(t *testing.T) {
	var stdout bytes.Buffer
	deps := Deps{
		HomeDir:     t.TempDir(),
		Stdout:      &stdout,
		Interactive: true,
	}
	seedSettings(t, deps.HomeDir, map[string]llmconfig.Profile{
		"alpha": {Protocol: llmconfig.ProtocolOpenAI, Model: "a"},
	}, "alpha")

	if err := Run(context.Background(), deps, []string{"list"}); err != nil {
		t.Fatalf("Run(list) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "alpha") {
		t.Errorf("stdout = %q, want alpha listed", stdout.String())
	}
}

func TestRunDispatchesDefaultByID(t *testing.T) {
	deps := Deps{HomeDir: t.TempDir(), Interactive: true}
	seedSettings(t, deps.HomeDir, map[string]llmconfig.Profile{
		"a": {Model: "a"}, "b": {Model: "b"},
	}, "a")

	if err := Run(context.Background(), deps, []string{"default", "b"}); err != nil {
		t.Fatalf("Run(default b) error = %v", err)
	}
	s := loadPersisted(t, deps.HomeDir)
	if s.LLM.DefaultProvider != "b" {
		t.Errorf("default_provider = %q, want b", s.LLM.DefaultProvider)
	}
}

func TestRunMenuRoutesToList(t *testing.T) {
	var stdout bytes.Buffer
	deps := Deps{
		HomeDir:     t.TempDir(),
		Stdin:       strings.NewReader("2\n"), // menu index 2 -> "List providers"
		Stdout:      &stdout,
		Interactive: true,
	}
	seedSettings(t, deps.HomeDir, map[string]llmconfig.Profile{
		"alpha": {Protocol: llmconfig.ProtocolOpenAI, Model: "a"},
	}, "alpha")

	if err := Run(context.Background(), deps, nil); err != nil {
		t.Fatalf("Run(menu) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "alpha") {
		t.Errorf("stdout = %q, want alpha listed via menu", stdout.String())
	}
}

func TestRunListWorksNonInteractive(t *testing.T) {
	var stdout bytes.Buffer
	deps := Deps{
		HomeDir:     t.TempDir(),
		Stdout:      &stdout,
		Interactive: false, // list is read-only and must not require a TTY
	}
	seedSettings(t, deps.HomeDir, map[string]llmconfig.Profile{
		"alpha": {Protocol: llmconfig.ProtocolOpenAI, Model: "a"},
	}, "alpha")

	if err := Run(context.Background(), deps, []string{"list"}); err != nil {
		t.Fatalf("Run(list) non-interactive error = %v", err)
	}
	if !strings.Contains(stdout.String(), "alpha") {
		t.Errorf("stdout = %q, want alpha listed without a TTY", stdout.String())
	}
}

func TestRunUnknownAction(t *testing.T) {
	deps := Deps{HomeDir: t.TempDir(), Interactive: true}
	err := Run(context.Background(), deps, []string{"bogus"})
	if err == nil {
		t.Fatal("Run(bogus) error = nil, want unknown action error")
	}
	if !strings.Contains(err.Error(), "unknown config action") {
		t.Errorf("error = %v, want unknown config action", err)
	}
}
