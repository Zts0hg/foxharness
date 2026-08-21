package architecturetest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

func TestApplicationContractFilesHaveOnlyApprovedDependencies(t *testing.T) {
	root := moduleRoot(t)
	approved := map[string]map[string]bool{
		"contracts.go":         {"context": true},
		"interactive_state.go": {"context": true, "time": true},
		"interactions.go":      {"context": true, "errors": true},
		"notifications.go":     {"context": true, "reflect": true, modulePath + "/internal/runtime": true},
	}

	for name, allowed := range approved {
		path := filepath.Join(root, "internal", "app", name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", name, err)
			}
			if !allowed[path] {
				t.Errorf("internal/app/%s imports forbidden contract dependency %q", name, path)
			}
		}
		if importsPackage(file, modulePath+"/internal/engine") {
			t.Errorf("internal/app/%s exposes engine through the application contract", name)
		}
	}
}

func importsPackage(file *ast.File, path string) bool {
	for _, imported := range file.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err == nil && value == path {
			return true
		}
	}
	return false
}
