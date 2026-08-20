package architecturetest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFoxAutodevEntryUsesRuntimeBackedComposition(t *testing.T) {
	root := moduleRoot(t)
	mainSource, err := os.ReadFile(filepath.Join(root, "cmd", "fox", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(mainSource)
	if !strings.Contains(source, "runAutodev(") {
		t.Fatal("cmd/fox autodev entry does not route through target runtime composition")
	}
	for _, forbidden := range []string{"app.RunAutodev(", "app.NewAgentRunner(", "engine.NewLegacyEngine("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("cmd/fox main retains obsolete Autodev production call %q", forbidden)
		}
	}
}

func TestAutodevProductionDoesNotDependOnLegacyExecutionContracts(t *testing.T) {
	root := moduleRoot(t)
	for _, edge := range productionImportEdges(t, root) {
		if edge.From != "internal/autodev" {
			continue
		}
		if edge.To == "internal/engine" || edge.To == "internal/tools" || edge.To == "internal/app" {
			t.Errorf("Autodev production dependency remains: %s -> %s", edge.From, edge.To)
		}
	}
}

func TestNoProductionCallerUsesLegacyAppAutodevFacade(t *testing.T) {
	root := moduleRoot(t)
	callers := productionSelectorReferences(t, root, modulePath+"/internal/app", "RunAutodev")
	if len(callers) != 0 {
		t.Fatalf("legacy app.RunAutodev production callers = %v, want none", callers)
	}
}
