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

var legacyApplicationNames = map[string]bool{
	"AgentRunner": true, "AgentRunnerConfig": true, "CLIConfig": true,
	"ChildRunnerFactory": true, "ChildRunnerConfig": true, "ChildParentProfile": true,
	"RunCLI": true, "RunTUI": true, "RunAutodev": true, "NewAgentRunner": true,
	"LegacyInteractiveApplication": true, "LegacyTUIBindings": true, "NewLegacyInteractiveApplication": true,
}

func TestM24ApplicationContainsOnlyTypedBoundary(t *testing.T) {
	root := moduleRoot(t)
	appDir := filepath.Join(root, "internal", "app")
	paths, err := filepath.Glob(filepath.Join(appDir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	forbiddenFiles := map[string]bool{
		"runner.go": true, "cli.go": true, "tui.go": true, "autodev.go": true, "plan_lifecycle.go": true,
		"legacy_runner_test.go": true, "legacy_cli_test.go": true, "legacy_tui_test.go": true,
		"legacy_autodev_test.go": true, "legacy_plan_lifecycle_test.go": true,
	}
	allowedProjectImports := map[string]bool{modulePath + "/internal/runtime": true}

	for _, path := range paths {
		base := filepath.Base(path)
		if forbiddenFiles[base] {
			t.Errorf("legacy application file remains: internal/app/%s", base)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if legacyApplicationNames[value.Name.Name] {
					t.Errorf("legacy application function remains in %s: %s", base, value.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok && legacyApplicationNames[typeSpec.Name.Name] {
						t.Errorf("legacy application type remains in %s: %s", base, typeSpec.Name.Name)
					}
				}
			}
		}
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		for _, imported := range file.Imports {
			pathValue, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(pathValue, modulePath+"/internal/") && !allowedProjectImports[pathValue] {
				t.Errorf("application production import is outside typed boundary: %s imports %s", base, pathValue)
			}
		}
	}
}

func TestM24RepositoryHasNoLegacyApplicationFacadeConsumers(t *testing.T) {
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
			if err != nil || importPath != modulePath+"/internal/app" {
				continue
			}
			alias := "app"
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			if alias == "." {
				t.Errorf("legacy-sensitive application boundary is dot-imported by %s", path)
				continue
			}
			aliases[alias] = true
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || !legacyApplicationNames[selector.Sel.Name] {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if ok && aliases[identifier.Name] {
				t.Errorf("legacy application facade consumer remains in %s: %s.%s", path, identifier.Name, selector.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
