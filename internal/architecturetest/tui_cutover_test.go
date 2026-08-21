package architecturetest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestM23ProductionTUIUsesSingleAdapterEntryAndTargetApplication(t *testing.T) {
	root := moduleRoot(t)
	mainSource, err := os.ReadFile(filepath.Join(root, "cmd", "fox", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(mainSource)
	for _, forbidden := range []string{"app.RunTUI(", "app.NewAgentRunner(", "app.AgentRunner", "NewLegacyInteractiveApplication"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("production fox TUI retains legacy route %q", forbidden)
		}
	}
	if calls := strings.Count(text, "tui.Run("); calls != 1 {
		t.Errorf("production fox tui.Run calls = %d, want exactly one", calls)
	}

	paths, err := filepath.Glob(filepath.Join(root, "internal", "tui", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	publicRuns := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && function.Name.IsExported() && function.Name.Name == "Run" {
				publicRuns++
			}
		}
	}
	if publicRuns != 1 {
		t.Fatalf("public tui.Run declarations = %d, want exactly one", publicRuns)
	}
}
