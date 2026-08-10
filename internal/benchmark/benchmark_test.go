package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type benchmarkFinalProvider struct{}

func (p benchmarkFinalProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

type benchmarkComposer struct{}

func (c benchmarkComposer) Compose(userPrompt string) (string, error) {
	return "system", nil
}

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
	results := []*Result{{
		CaseID:    "case-1",
		Success:   true,
		Workspace: "/tmp/work",
		SessionID: "sess",
		RuntimeFidelity: RuntimeFidelity{
			SharedInvariants:       []string{"tool surface"},
			IntentionalDifferences: []string{"no interactive approval"},
			Warning:                "benchmark runtime differs from product runtime",
		},
	}}

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
	if decoded[0].RuntimeFidelity.Warning == "" || len(decoded[0].RuntimeFidelity.IntentionalDifferences) != 1 {
		t.Fatalf("runtime fidelity metadata missing from decoded result: %+v", decoded[0].RuntimeFidelity)
	}
}

func TestRunCaseIncludesRuntimeFidelityMetadata(t *testing.T) {
	fixture := t.TempDir()
	runner := NewRunner(func(ctx context.Context, workDir string, c *Case) (*Harness, error) {
		manager := session.NewManagerWithHome(workDir, t.TempDir())
		sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
		if err != nil {
			return nil, err
		}
		eng := engine.NewAgentEngine(benchmarkFinalProvider{}, tools.NewRegistry(), workDir, benchmarkComposer{}, engine.Config{MaxTurns: 1})
		return &Harness{
			Engine:  eng,
			Session: sess,
			RuntimeFidelity: RuntimeFidelity{
				SharedInvariants:       []string{"canonical tools"},
				IntentionalDifferences: []string{"no interactive approval"},
				Warning:                "benchmark runtime differs from product runtime",
			},
		}, nil
	})

	result, err := runner.RunCase(context.Background(), &Case{
		ID:      "case-1",
		Fixture: fixture,
		Prompt:  "finish",
	})
	if err != nil {
		t.Fatalf("RunCase() error = %v", err)
	}
	if result.RuntimeFidelity.Warning == "" {
		t.Fatalf("RuntimeFidelity warning missing: %+v", result.RuntimeFidelity)
	}
	if len(result.RuntimeFidelity.SharedInvariants) != 1 || result.RuntimeFidelity.SharedInvariants[0] != "canonical tools" {
		t.Fatalf("SharedInvariants = %+v", result.RuntimeFidelity.SharedInvariants)
	}
	if _, err := os.Stat(result.Workspace); err != nil {
		t.Fatalf("successful workspace was not retained: %v", err)
	}
}
