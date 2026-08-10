package benchmark

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestDVBEN001NoWholeCaseDeadlineAndCancellationDoesNotStopAllValidations(t *testing.T) {
	source := readBenchmarkSource(t, "runner.go")
	if strings.Contains(source, "context.WithTimeout") {
		t.Fatal("RunCase now owns a whole-case timeout; update DV-BEN-001 classification")
	}
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "result.txt"), []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := ValidateAll(ctx, workDir, []Validation{
		{Type: "command", Command: "true"},
		{Type: "file_contains", Path: "result.txt", Contains: "done"},
	})
	if results[0].Passed || !results[1].Passed {
		t.Fatalf("cancelled validation results = %#v, want command cancelled but later file validation still run", results)
	}
}

func TestDVBEN003RuntimeFidelityHasNoResolvedSpecificationIdentity(t *testing.T) {
	typeOf := reflect.TypeOf(RuntimeFidelity{})
	for _, field := range []string{"Profile", "ProviderProtocol", "Model", "TurnBudget", "ToolSurface", "MemoryPolicy", "CompactionPolicy"} {
		if _, ok := typeOf.FieldByName(field); ok {
			t.Fatalf("RuntimeFidelity now records resolved field %s; update DV-BEN-003 classification", field)
		}
	}
}

func TestDVBEN004FixtureAndValidationCanResolveOutsideOwnedRoots(t *testing.T) {
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(fixture, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := copyDir(fixture, destination); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}
	copied, err := os.ReadFile(filepath.Join(destination, "linked.txt"))
	if err != nil || string(copied) != "external" {
		t.Fatalf("copied symlink target = %q, %v; want outside content", copied, err)
	}

	workspace := t.TempDir()
	relativeEscape, err := filepath.Rel(workspace, outside)
	if err != nil {
		t.Fatal(err)
	}
	if result := validateFileContains(workspace, relativeEscape, "external"); !result.Passed {
		t.Fatalf("traversal validation = %#v, want current outside-root success", result)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "result.txt")); err != nil {
		t.Fatal(err)
	}
	if result := validateFileContains(workspace, "result.txt", "external"); !result.Passed {
		t.Fatalf("symlink validation = %#v, want current outside-root success", result)
	}
	if !strings.Contains(readBenchmarkSource(t, "runner.go"), "os.MkdirTemp") || strings.Contains(readBenchmarkSource(t, "runner.go"), "os.RemoveAll(workspace)") {
		t.Log("RunCase retains temporary workspaces on setup failure")
	}
}

func TestDVBEN005InvalidCaseDomainsRemainAccepted(t *testing.T) {
	casePath := filepath.Join(t.TempDir(), "case.yaml")
	contents := "id: invalid\nfixture: fixture\nprompt: run\nmax_turns: -1\nvalidations:\n  - type: command\n    command: ''\n  - type: file_contains\n    path: ''\n    contains: ''\n"
	if err := os.WriteFile(casePath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCase(casePath)
	if err != nil {
		t.Fatalf("LoadCase() error = %v, want current acceptance", err)
	}
	if loaded.MaxTurns != -1 || loaded.Validations[0].Command != "" || loaded.Validations[1].Path != "" || loaded.Validations[1].Contains != "" {
		t.Fatalf("loaded invalid domains = %#v", loaded)
	}
}

func TestDVBEN006CommandValidationIsUnboundedAndImmediateProcessOnly(t *testing.T) {
	source := readBenchmarkSource(t, "validate.go")
	for _, current := range []string{"cmd.CombinedOutput()", "exec.CommandContext"} {
		if !strings.Contains(source, current) {
			t.Fatalf("validation no longer contains %q; update DV-BEN-006 classification", current)
		}
	}
	for _, missing := range []string{"Setpgid", "StdoutPipe", "StderrPipe", "io.LimitReader"} {
		if strings.Contains(source, missing) {
			t.Fatalf("validation now contains %q; update DV-BEN-006 classification", missing)
		}
	}
}

func TestDVBEN007ResultOmitsStableExecutionAndDefinitionIdentity(t *testing.T) {
	typeOf := reflect.TypeOf(Result{})
	for _, field := range []string{"RepeatIndex", "RunID", "CaseDefinitionID", "FixtureID", "RuntimeStatus", "ProviderProtocol", "Model"} {
		if _, ok := typeOf.FieldByName(field); ok {
			t.Fatalf("Result now records %s; update DV-BEN-007 classification", field)
		}
	}
}

func readBenchmarkSource(t *testing.T, name string) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(current), name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	return string(data)
}
