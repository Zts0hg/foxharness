package architecturetest

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

func TestTUIRuntimeStateUsesApplicationContracts(t *testing.T) {
	root := moduleRoot(t)
	wantImports := map[string][]string{
		"internal/tui/model.go":          {"internal/app"},
		"internal/tui/reporter.go":       {"internal/app"},
		"internal/tui/runtime_info.go":   {"internal/app"},
		"internal/tui/snapshot.go":       {"internal/app"},
		"internal/tui/snapshot_html.go":  {"internal/app"},
		"internal/tui/selector/model.go": {"internal/app"},
		"internal/tui/selector/view.go":  {"internal/app"},
	}
	for name, required := range wantImports {
		imports := fileImports(t, filepath.Join(root, name))
		for _, dependency := range required {
			if !imports[dependency] {
				t.Errorf("%s must import %s", name, dependency)
			}
		}
		for _, forbidden := range []string{
			"internal/checkpoint", "internal/compaction", "internal/engine", "internal/session",
		} {
			if imports[forbidden] {
				t.Errorf("%s retains concrete runtime-state dependency %s", name, forbidden)
			}
		}
	}
}

func fileImports(t *testing.T, path string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	imports := make(map[string]bool, len(file.Imports))
	for _, imported := range file.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", path, err)
		}
		imports[value] = true
		imports[trimModulePath(value)] = true
	}
	return imports
}

func trimModulePath(path string) string {
	if len(path) > len(modulePath)+1 && path[:len(modulePath)+1] == modulePath+"/" {
		return path[len(modulePath)+1:]
	}
	return path
}
