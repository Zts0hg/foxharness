/*
Package architecturetest enforces the production package dependency contract.
*/
package architecturetest

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/Zts0hg/foxharness"

type importEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type allowlistFile struct {
	Violations []allowlistEntry `json:"violations"`
}

type allowlistEntry struct {
	From     string `json:"from"`
	To       string `json:"to"`
	RemoveBy string `json:"remove_by"`
}

func TestProductionImportsMatchArchitectureAllowlist(t *testing.T) {
	root := moduleRoot(t)
	edges := productionImportEdges(t, root)
	actual := violationSet(edges)
	expected := readAllowlist(t, filepath.Join(root, "internal", "architecturetest", "allowlist.json"))
	baselineCeiling := readAllowlist(t, filepath.Join(root, "internal", "architecturetest", "baseline_allowlist.json"))

	if diff := setDifference(expected, baselineCeiling); len(diff) > 0 {
		t.Fatalf("architecture allowlist additions or broadening are forbidden:\n%s", strings.Join(diff, "\n"))
	}

	if diff := setDifference(actual, expected); len(diff) > 0 {
		t.Fatalf("new architecture violations are not allowed:\n%s", strings.Join(diff, "\n"))
	}
	if diff := setDifference(expected, actual); len(diff) > 0 {
		t.Fatalf("stale architecture allowlist entries must be removed:\n%s", strings.Join(diff, "\n"))
	}
}

func TestViolationRulesDetectConfirmedForbiddenEdges(t *testing.T) {
	tests := []struct {
		name string
		edge importEdge
	}{
		{name: "engine concrete provider", edge: importEdge{From: "internal/engine", To: "internal/provider"}},
		{name: "turn policy persistence", edge: importEdge{From: "internal/turnpolicy", To: "internal/session"}},
		{name: "runtime presentation", edge: importEdge{From: "internal/runtime", To: "internal/tui"}},
		{name: "runtime bypasses engine schema contract", edge: importEdge{From: "internal/runtime", To: "internal/schema"}},
		{name: "session owns runtime lifecycle", edge: importEdge{From: "internal/session", To: "internal/runtime"}},
		{name: "prompt discovers automemory", edge: importEdge{From: "internal/prompt", To: "internal/automemory"}},
		{name: "application concrete engine", edge: importEdge{From: "internal/app", To: "internal/engine"}},
		{name: "tui concrete session", edge: importEdge{From: "internal/tui", To: "internal/session"}},
		{name: "subagent concrete runtime", edge: importEdge{From: "internal/subagent", To: "internal/runtime"}},
		{name: "benchmark independent engine", edge: importEdge{From: "internal/benchmark", To: "internal/engine"}},
		{name: "aggregate infrastructure package", edge: importEdge{From: "internal/runtime", To: "internal/infrastructure"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if reason := violationReason(tt.edge); reason == "" {
				t.Fatalf("expected %s -> %s to violate the target dependency contract", tt.edge.From, tt.edge.To)
			}
		})
	}
}

func TestViolationRulesAllowConfirmedEdges(t *testing.T) {
	tests := []importEdge{
		{From: "internal/engine", To: "internal/schema"},
		{From: "internal/turnpolicy", To: "internal/engine"},
		{From: "internal/turnpolicy", To: "internal/recovery"},
		{From: "internal/turnpolicy", To: "internal/reminder"},
		{From: "internal/turnpolicy", To: "internal/schema"},
		{From: "internal/runtime", To: "internal/engine"},
		{From: "internal/runtime", To: "internal/session"},
		{From: "internal/runtime", To: "internal/prompt"},
		{From: "internal/app", To: "internal/runtime"},
		{From: "internal/tui", To: "internal/app"},
		{From: "internal/benchmark", To: "internal/runtime"},
	}

	for _, edge := range tests {
		if reason := violationReason(edge); reason != "" {
			t.Errorf("expected %s -> %s to be allowed, got %s", edge.From, edge.To, reason)
		}
	}
}

func TestDeprecatedSessionCompatibilityUsageMatchesCeiling(t *testing.T) {
	root := moduleRoot(t)
	got := deprecatedSessionCompatibilityUsage(t, root)
	want := []string{
		"internal/engine/loop.go:Finish=1",
		"internal/engine/loop.go:StartRun=1",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("deprecated session compatibility usage = %v, want exact decreasing ceiling %v", got, want)
	}
}

func TestMemoryStoreIsOnlyWorkingMemoryOwner(t *testing.T) {
	root := moduleRoot(t)
	if got := forbiddenWorkingMemoryOwnership(t, root); len(got) > 0 {
		t.Fatalf("working-memory ownership escaped internal/memory:\n%s", strings.Join(got, "\n"))
	}
	got := workingMemoryPathCompatibilityUsage(t, root)
	want := []string{
		"internal/feishu/runner.go:MemoryPath=1",
		"internal/subagent/manager.go:MemoryPath=1",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("persistence working-memory path usage = %v, want exact decreasing ceiling %v", got, want)
	}
}

func TestTargetAgentEngineHasNoProductionCallersBeforeProfileCutover(t *testing.T) {
	root := moduleRoot(t)
	callers := productionSelectorReferences(t, root, modulePath+"/internal/engine", "NewAgentEngine")
	if len(callers) > 0 {
		t.Fatalf("target AgentEngine production callers before profile cutover:\n%s", strings.Join(callers, "\n"))
	}
}

func TestRuntimeProfilesHaveNoProductionCallersBeforeProfileCutover(t *testing.T) {
	root := moduleRoot(t)
	var callers []string
	for _, edge := range productionImportEdges(t, root) {
		if edge.To == "internal/runtime" {
			callers = append(callers, edge.From)
		}
	}
	if len(callers) > 0 {
		t.Fatalf("target runtime production callers before profile cutover:\n%s", strings.Join(callers, "\n"))
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above architecture test")
		}
		dir = parent
	}
}

func productionImportEdges(t *testing.T, root string) []importEdge {
	t.Helper()
	edges := make(map[importEdge]struct{})
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "vendor" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		from := filepath.ToSlash(rel)
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, spec := range file.Imports {
			to, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import in %s: %w", path, err)
			}
			prefix := modulePath + "/"
			if !strings.HasPrefix(to, prefix) {
				continue
			}
			edges[importEdge{From: from, To: strings.TrimPrefix(to, prefix)}] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production imports: %v", err)
	}

	result := make([]importEdge, 0, len(edges))
	for edge := range edges {
		result = append(result, edge)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].From == result[j].From {
			return result[i].To < result[j].To
		}
		return result[i].From < result[j].From
	})
	return result
}

func productionSelectorReferences(t *testing.T, root, importPath, selectorName string) []string {
	t.Helper()
	var callers []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if oneOf(entry.Name(), ".git", "vendor", "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		aliases := make(map[string]bool)
		dotImported := false
		for _, spec := range file.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import in %s: %w", path, err)
			}
			if value != importPath {
				continue
			}
			alias := filepath.Base(importPath)
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			if alias == "." {
				dotImported = true
			} else if alias != "_" {
				aliases[alias] = true
			}
		}
		if len(aliases) == 0 && !dotImported {
			return nil
		}
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.SelectorExpr:
				identifier, ok := value.X.(*ast.Ident)
				found = found || (ok && aliases[identifier.Name] && value.Sel.Name == selectorName)
			case *ast.Ident:
				found = found || (dotImported && value.Name == selectorName)
			}
			return !found
		})
		if found {
			rel, err := filepath.Rel(root, path)
			if err == nil {
				callers = append(callers, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production selector callers: %v", err)
	}
	sort.Strings(callers)
	return callers
}

func deprecatedSessionCompatibilityUsage(t *testing.T, root string) []string {
	t.Helper()
	counts := make(map[string]int)
	fset := token.NewFileSet()
	deprecatedQualified := map[string]bool{
		"Event": true, "Manager": true, "NewManager": true,
		"NewManagerWithHome": true, "NewTranscript": true, "Run": true,
		"Session": true, "Transcript": true,
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if oneOf(entry.Name(), ".git", "vendor", "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/session/") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		aliases := make(map[string]bool)
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import in %s: %w", path, err)
			}
			if importPath != modulePath+"/internal/session" {
				continue
			}
			alias := "session"
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			if alias == "." || alias == "_" {
				counts[rel+":dot-or-blank-session-import"]++
				continue
			}
			aliases[alias] = true
		}
		if len(aliases) == 0 {
			return nil
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if ident, ok := selector.X.(*ast.Ident); ok && aliases[ident.Name] && deprecatedQualified[selector.Sel.Name] {
				counts[rel+":"+selector.Sel.Name]++
			}
			if selector.Sel.Name == "StartRun" || selector.Sel.Name == "Finish" {
				counts[rel+":"+selector.Sel.Name]++
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan deprecated session compatibility usage: %v", err)
	}

	keys := make([]string, 0, len(counts))
	for key, count := range counts {
		keys = append(keys, fmt.Sprintf("%s=%d", key, count))
	}
	sort.Strings(keys)
	return keys
}

func forbiddenWorkingMemoryOwnership(t *testing.T, root string) []string {
	t.Helper()
	var violations []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if oneOf(entry.Name(), ".git", "vendor", "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.TypeSpec:
				if strings.HasPrefix(rel, "internal/session/") && value.Name.Name == "WorkingMemory" {
					violations = append(violations, rel+": declares WorkingMemory")
				}
			case *ast.FuncDecl:
				if strings.HasPrefix(rel, "internal/session/") && oneOf(value.Name.Name, "NewMemory", "initialWorkingMemory") {
					violations = append(violations, rel+": declares "+value.Name.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan working-memory ownership: %v", err)
	}
	sort.Strings(violations)
	return violations
}

func workingMemoryPathCompatibilityUsage(t *testing.T, root string) []string {
	t.Helper()
	counts := make(map[string]int)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if oneOf(entry.Name(), ".git", "vendor", "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/session/") || strings.HasPrefix(rel, "internal/memory/") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if selector, ok := node.(*ast.SelectorExpr); ok && selector.Sel.Name == "MemoryPath" {
				counts[rel+":MemoryPath"]++
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan persistence working-memory path usage: %v", err)
	}
	keys := make([]string, 0, len(counts))
	for key, count := range counts {
		keys = append(keys, fmt.Sprintf("%s=%d", key, count))
	}
	sort.Strings(keys)
	return keys
}

func violationSet(edges []importEdge) map[string]struct{} {
	result := make(map[string]struct{})
	for _, edge := range edges {
		if reason := violationReason(edge); reason != "" {
			result[edgeKey(edge, reason)] = struct{}{}
		}
	}
	return result
}

func violationReason(edge importEdge) string {
	if hasPackagePrefix(edge.From, "internal/infrastructure") || hasPackagePrefix(edge.To, "internal/infrastructure") {
		return "aggregate infrastructure package is forbidden"
	}

	switch {
	case edge.From == "internal/prompt":
		return "prompt renderer must not depend on project mechanisms"
	case edge.From == "internal/session" && edge.To == "internal/runtime":
		return "session persistence must not depend on runtime lifecycle"
	case edge.From == "internal/engine":
		if edge.To != "internal/schema" {
			return "engine may depend only on schema"
		}
	case edge.From == "internal/turnpolicy":
		if !oneOf(edge.To, "internal/engine", "internal/recovery", "internal/reminder", "internal/schema") {
			return "turn policy may depend only on engine contracts and focused policy mechanisms"
		}
	case edge.From == "internal/runtime":
		if !oneOf(edge.To, "internal/engine", "internal/session", "internal/prompt") {
			return "runtime dependency is outside its confirmed inward contracts"
		}
	case edge.From == "internal/app":
		if edge.To != "internal/runtime" {
			return "app may depend on runtime but not concrete subsystems or adapters"
		}
	case isPresentationPackage(edge.From):
		if isPresentationForbiddenTarget(edge.To) {
			return "presentation adapter directly depends on a runtime subsystem"
		}
	case edge.From == "internal/subagent":
		if isSubagentForbiddenTarget(edge.To) {
			return "subagent invocation adapter directly depends on runtime construction"
		}
	case edge.From == "internal/benchmark":
		if oneOf(edge.To, "internal/engine", "internal/provider", "internal/session", "internal/tools", "internal/app") {
			return "benchmark independently assembles runtime internals"
		}
	case edge.From == "internal/autodev":
		if oneOf(edge.To, "internal/engine", "internal/tools", "internal/app", "internal/tui", "internal/cli") {
			return "autodev core execution bypasses the runtime harness"
		}
	}
	return ""
}

func isPresentationPackage(path string) bool {
	return hasPackagePrefix(path, "internal/tui") || path == "internal/cli" || path == "internal/feishu" || path == "internal/agentops"
}

func isPresentationForbiddenTarget(path string) bool {
	return oneOf(path,
		"internal/engine", "internal/runtime", "internal/session", "internal/checkpoint",
		"internal/compaction", "internal/provider", "internal/tools", "internal/memory",
		"internal/automemory", "internal/metrics", "internal/tracing", "internal/schema",
		"internal/permission", "internal/toolpolicy",
	)
}

func isSubagentForbiddenTarget(path string) bool {
	return oneOf(path,
		"internal/runtime", "internal/engine", "internal/provider", "internal/session",
		"internal/compaction", "internal/context", "internal/prompt", "internal/automemory",
		"internal/tools", "internal/memory", "internal/metrics", "internal/tracing",
	)
}

func hasPackagePrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func readAllowlist(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read architecture allowlist: %v", err)
	}
	var file allowlistFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("decode architecture allowlist: %v", err)
	}
	result := make(map[string]struct{}, len(file.Violations))
	for _, entry := range file.Violations {
		if entry.From == "" || entry.To == "" || entry.RemoveBy == "" {
			t.Fatalf("allowlist entries require from, to, and remove_by: %+v", entry)
		}
		edge := importEdge{From: entry.From, To: entry.To}
		reason := violationReason(edge)
		if reason == "" {
			t.Fatalf("allowlist contains a permitted edge: %s -> %s", entry.From, entry.To)
		}
		key := edgeKey(edge, reason)
		if _, duplicate := result[key]; duplicate {
			t.Fatalf("duplicate architecture allowlist entry: %s", key)
		}
		result[key] = struct{}{}
	}
	return result
}

func edgeKey(edge importEdge, reason string) string {
	return fmt.Sprintf("%s -> %s: %s", edge.From, edge.To, reason)
}

func setDifference(left, right map[string]struct{}) []string {
	var result []string
	for item := range left {
		if _, ok := right[item]; !ok {
			result = append(result, item)
		}
	}
	sort.Strings(result)
	return result
}
