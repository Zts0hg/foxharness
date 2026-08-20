package architecturetest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCLIAdapterDependsOnlyOnApplicationBoundary(t *testing.T) {
	root := moduleRoot(t)
	var imports []string
	for _, edge := range productionImportEdges(t, root) {
		if edge.From == "internal/cli" {
			imports = append(imports, modulePath+"/"+edge.To)
		}
	}
	want := []string{"github.com/Zts0hg/foxharness/internal/app"}
	if !reflect.DeepEqual(imports, want) {
		t.Fatalf("internal/cli production imports = %#v, want %#v", imports, want)
	}
}

func TestFoxPrintEntryUsesOnlyTargetCLIAdapter(t *testing.T) {
	root := moduleRoot(t)
	mainSource, err := os.ReadFile(filepath.Join(root, "cmd", "fox", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(mainSource)
	if !strings.Contains(source, "cli.Run(") || !strings.Contains(source, "newCLIApplication(") {
		t.Fatal("cmd/fox print entry does not route through the target CLI adapter and composition")
	}
	for _, forbidden := range []string{"app.RunCLI(", "app.NewAgentRunner(", "engine.NewLegacyEngine("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("cmd/fox main retains obsolete CLI production call %q", forbidden)
		}
	}
}
