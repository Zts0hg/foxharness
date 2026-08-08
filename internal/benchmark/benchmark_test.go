package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCaseDefaultsAndValidatesRequiredFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case.yml")
	data := []byte(`
id: edit-file
name: Edit file
fixture: fixtures/basic
prompt: update the file
validations:
  - type: file_contains
    path: README.md
    contains: done
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := LoadCase(path)
	if err != nil {
		t.Fatalf("LoadCase() error = %v", err)
	}
	if c.MaxTurns != 12 {
		t.Fatalf("MaxTurns = %d, want default 12", c.MaxTurns)
	}
	if c.ID != "edit-file" || c.Validations[0].Type != "file_contains" {
		t.Fatalf("case = %+v", c)
	}

	invalid := filepath.Join(t.TempDir(), "invalid.yml")
	if err := os.WriteFile(invalid, []byte("id: missing-validation\nfixture: f\nprompt: p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCase(invalid); err == nil {
		t.Fatal("LoadCase() error = nil, want validation error")
	}
}

func TestValidateAllCoversSuccessAndFailurePaths(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "result.txt"), []byte("status: done\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results := ValidateAll(context.Background(), workDir, []Validation{
		{Type: "command", Command: "test -f result.txt"},
		{Type: "file_contains", Path: "result.txt", Contains: "done"},
		{Type: "file_contains", Path: "result.txt", Contains: "missing"},
		{Type: "unknown"},
	})
	if len(results) != 4 {
		t.Fatalf("results len = %d, want 4", len(results))
	}
	if !results[0].Passed || !results[1].Passed {
		t.Fatalf("success validations failed: %+v", results)
	}
	if results[2].Passed || !strings.Contains(results[2].Message, "不包括目标文本") {
		t.Fatalf("file_contains failure = %+v", results[2])
	}
	if results[3].Passed || results[3].Message != "未知验证类型" {
		t.Fatalf("unknown validation = %+v", results[3])
	}
	if allPassed(results) {
		t.Fatal("allPassed() = true, want false when any validation fails")
	}
}

func TestWriteJSONWritesIndentedResultsWithTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	results := []*Result{{CaseID: "case-1", Success: true, Workspace: "/tmp/work", SessionID: "sess"}}

	if err := WriteJSON(path, results); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("report missing trailing newline: %q", data)
	}
	var decoded []Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, data)
	}
	if len(decoded) != 1 || decoded[0].CaseID != "case-1" || !decoded[0].Success {
		t.Fatalf("decoded = %+v", decoded)
	}
}
