package architecturetest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var legacyEngineNames = map[string]bool{
	"Config": true, "DefaultConfig": true, "DetailedReporter": true,
	"LegacyEngine": true, "MessageDeltaReporter": true, "NewLegacyEngine": true,
	"Reporter": true, "RunResult": true, "TurnLimitError": true,
}

func TestM25EngineContainsNoLegacyImplementationOrContracts(t *testing.T) {
	root := moduleRoot(t)
	engineDir := filepath.Join(root, "internal", "engine")
	paths, err := filepath.Glob(filepath.Join(engineDir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	forbiddenFiles := map[string]bool{
		"config.go": true, "context.go": true, "errors.go": true,
		"loop.go": true, "reporter.go": true, "todo_gate.go": true,
		"current_contract_adapter_test.go": true,
	}

	for _, path := range paths {
		base := filepath.Base(path)
		if forbiddenFiles[base] || strings.HasPrefix(base, "current_contract_") {
			t.Errorf("legacy engine file remains: internal/engine/%s", base)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if legacyEngineNames[value.Name.Name] {
					t.Errorf("legacy engine function remains in %s: %s", base, value.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok && legacyEngineNames[typeSpec.Name.Name] {
						t.Errorf("legacy engine type remains in %s: %s", base, typeSpec.Name.Name)
					}
				}
			}
		}
	}
}

func TestM25RepositoryHasNoLegacyEngineConsumers(t *testing.T) {
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		aliases := make(map[string]bool)
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil || importPath != modulePath+"/internal/engine" {
				continue
			}
			alias := "engine"
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			if alias == "." {
				t.Errorf("legacy-sensitive engine boundary is dot-imported by %s", path)
				continue
			}
			aliases[alias] = true
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || !legacyEngineNames[selector.Sel.Name] {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if ok && aliases[identifier.Name] {
				t.Errorf("legacy engine consumer remains in %s: %s.%s", path, identifier.Name, selector.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
