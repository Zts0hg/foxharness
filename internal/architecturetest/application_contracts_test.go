package architecturetest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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

func TestApplicationLegacyFacadeFileSetIsClosedUntilM24(t *testing.T) {
	root := moduleRoot(t)
	directory := filepath.Join(root, "internal", "app")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}

	var legacy []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", name, err)
			}
			if strings.HasPrefix(path, modulePath+"/internal/") && path != modulePath+"/internal/runtime" {
				legacy = append(legacy, fmt.Sprintf("%s:%s", name, strings.TrimPrefix(path, modulePath+"/")))
			}
		}
	}
	sort.Strings(legacy)
	want := []string{
		"autodev.go:internal/autodev", "autodev.go:internal/engine", "autodev.go:internal/llmconfig",
		"autodev.go:internal/permission", "autodev.go:internal/provider", "autodev.go:internal/schema",
		"autodev.go:internal/slash", "autodev.go:internal/tools", "cli.go:internal/engine",
		"cli.go:internal/llmconfig", "cli.go:internal/session",
		"plan_lifecycle.go:internal/middleware",
		"plan_lifecycle.go:internal/schema", "plan_lifecycle.go:internal/tools", "runner.go:internal/automemory",
		"runner.go:internal/checkpoint", "runner.go:internal/collaboration", "runner.go:internal/compaction",
		"runner.go:internal/context", "runner.go:internal/engine", "runner.go:internal/llmconfig",
		"runner.go:internal/memory", "runner.go:internal/middleware", "runner.go:internal/permission",
		"runner.go:internal/provider", "runner.go:internal/schema", "runner.go:internal/session",
		"runner.go:internal/slash", "runner.go:internal/slash/skilltool", "runner.go:internal/subagent",
		"runner.go:internal/tools", "tui.go:internal/permission",
		"tui.go:internal/provider", "tui.go:internal/settings",
	}
	if fmt.Sprint(legacy) != fmt.Sprint(want) {
		t.Fatalf("legacy application facade imports = %v, want exact closed M24 ceiling %v", legacy, want)
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
