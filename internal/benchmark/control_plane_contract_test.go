package benchmark

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEVBEN002AcceptedCaseShapeAndParseFailures(t *testing.T) {
	root := t.TempDir()
	fixture := filepath.Join(root, "fixture")
	if err := os.Mkdir(fixture, 0o700); err != nil {
		t.Fatal(err)
	}
	casePath := filepath.Join(root, "case.yaml")
	contents := "id: '  accepted  '\nname: Optional Name\nfixture: fixture\nprompt: |\n  first line\n  second line\nmax_turns: 5\ntimeout_seconds: 7\nvalidations:\n  - type: command\n    command: 'printf ok'\n  - type: file_contains\n    path: ' result.txt '\n    contains: ' done '\n"
	if err := os.WriteFile(casePath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCase(casePath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "accepted" || loaded.Name != "Optional Name" || loaded.Fixture != fixture || loaded.MaxTurns != 5 || loaded.TimeoutSeconds != 7 {
		t.Fatalf("loaded case metadata = %#v", loaded)
	}
	if loaded.Prompt != "first line\nsecond line\n" {
		t.Fatalf("multiline prompt = %q", loaded.Prompt)
	}
	if len(loaded.Validations) != 2 || loaded.Validations[0].Command != "printf ok" || loaded.Validations[1].Path != "result.txt" || loaded.Validations[1].Contains != " done " {
		t.Fatalf("loaded validations = %#v", loaded.Validations)
	}

	missing := filepath.Join(root, "missing.yaml")
	if _, err := LoadCase(missing); err == nil || !strings.Contains(err.Error(), "读取 benchmark case 失败") {
		t.Fatalf("missing case error = %v", err)
	}
	malformed := filepath.Join(root, "malformed.yaml")
	if err := os.WriteFile(malformed, []byte("id: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCase(malformed); err == nil || !strings.Contains(err.Error(), "解析 benchmark case 失败") {
		t.Fatalf("malformed case error = %v", err)
	}
	multiple := filepath.Join(root, "multiple.yaml")
	if err := os.WriteFile(multiple, []byte(contents+"---\nid: second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCase(multiple); err == nil || !strings.Contains(err.Error(), "只允许一个 YAML document") {
		t.Fatalf("multiple document error = %v", err)
	}
}

func TestEVBEN003FixtureMaterializationNormalizesContentAndModes(t *testing.T) {
	source := t.TempDir()
	nested := filepath.Join(source, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(nested, "regular.txt")
	empty := filepath.Join(source, "empty.txt")
	if err := os.WriteFile(regular, []byte("fixture content\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	destination := t.TempDir()
	if err := copyDir(source, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "nested", "regular.txt"))
	if err != nil || string(data) != "fixture content\n" {
		t.Fatalf("copied content/error = %q/%v", data, err)
	}
	emptyData, err := os.ReadFile(filepath.Join(destination, "empty.txt"))
	if err != nil || len(emptyData) != 0 {
		t.Fatalf("copied empty file/error = %q/%v", emptyData, err)
	}
	assertBenchmarkMode(t, filepath.Join(destination, "nested"), 0o755)
	assertBenchmarkMode(t, filepath.Join(destination, "nested", "regular.txt"), 0o644)
	assertBenchmarkMode(t, filepath.Join(destination, "empty.txt"), 0o644)
	assertBenchmarkMode(t, nested, 0o700)
	assertBenchmarkMode(t, regular, 0o700)

	collision := t.TempDir()
	if err := os.Mkdir(filepath.Join(collision, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collision, "nested", "regular.txt"), []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(source, collision); err == nil {
		t.Fatal("copyDir() overwrote an existing destination file")
	}
	if err := copyDir(filepath.Join(source, "missing"), t.TempDir()); err == nil {
		t.Fatal("copyDir() accepted a missing source fixture")
	}
}

func TestEVBEN004WorkspaceCreationFailurePreventsFactory(t *testing.T) {
	factoryCalled := false
	runner := NewRunner(func(context.Context, string, *Case) (*Harness, error) {
		factoryCalled = true
		return nil, nil
	})
	runner.createWorkspace = func() (string, error) {
		return "", errors.New("workspace unavailable")
	}
	result, err := runner.RunRepeat(context.Background(), &Case{ID: "workspace", Fixture: t.TempDir(), Prompt: "run"}, 1)
	if err == nil || !strings.Contains(err.Error(), "workspace unavailable") || factoryCalled {
		t.Fatalf("workspace creation = result:%#v error:%v factoryCalled:%t", result, err, factoryCalled)
	}
	if result.Status != ResultStatusInfrastructureFailed || result.Workspace != "" || result.RuntimeStatus != RuntimeStatusNotStarted {
		t.Fatalf("workspace failure result = %#v", result)
	}
}

func TestEVBEN004HarnessFactoryOrderAndStructuralFailures(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "input.txt"), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	caseInput := &Case{ID: "factory", Fixture: fixture, Prompt: "run", MaxTurns: 1, TimeoutSeconds: 60, Validations: []Validation{{Type: "command", Command: "true"}}}
	baseFactory := benchmarkHarnessFactory(t, NewRuntimeSpec("test", "model", 1, nil), benchmarkComposer{})

	tests := []struct {
		name   string
		mutate func(*Harness) *Harness
	}{
		{name: "nil harness", mutate: func(*Harness) *Harness { return nil }},
		{name: "missing engine", mutate: func(h *Harness) *Harness { h.Engine = nil; return h }},
		{name: "missing session", mutate: func(h *Harness) *Harness { h.Session = nil; return h }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factoryCalled := false
			runner := NewRunner(func(ctx context.Context, workDir string, c *Case) (*Harness, error) {
				factoryCalled = true
				if data, err := os.ReadFile(filepath.Join(workDir, "input.txt")); err != nil || string(data) != "ready" {
					t.Fatalf("factory ran before fixture materialization: %q/%v", data, err)
				}
				harness, err := baseFactory(ctx, workDir, c)
				if err != nil {
					return nil, err
				}
				return test.mutate(harness), nil
			})
			result, err := runner.RunRepeat(context.Background(), caseInput, 1)
			if !factoryCalled || err == nil || result.Status != ResultStatusInfrastructureFailed || result.RuntimeStatus != RuntimeStatusNotStarted {
				t.Fatalf("factory structural failure = called:%t result:%#v error:%v", factoryCalled, result, err)
			}
			if result.FixtureID == "" || result.CaseDefinitionID == "" {
				t.Fatalf("factory did not receive post-fixture identities: %#v", result)
			}
			if _, statErr := os.Stat(result.Workspace); !os.IsNotExist(statErr) {
				t.Fatalf("failed factory workspace stat = %v", statErr)
			}
		})
	}

	runner := NewRunner(func(context.Context, string, *Case) (*Harness, error) {
		return nil, errors.New("factory failure")
	})
	result, err := runner.RunRepeat(context.Background(), caseInput, 1)
	if err == nil || !strings.Contains(err.Error(), "创建 Harness 失败") || result.Status != ResultStatusInfrastructureFailed {
		t.Fatalf("factory error = result:%#v error:%v", result, err)
	}
}

func TestEVBEN005CommandValidationUsesWorkspaceAndSeparateStreams(t *testing.T) {
	workDir := t.TempDir()
	result := validateCommand(context.Background(), workDir, "printf stdout; printf stderr >&2; pwd")
	if !result.Passed || result.Status != ValidationStatusPassed || result.Stderr != "stderr" {
		t.Fatalf("command result = %#v", result)
	}
	if result.Stdout != "stdout"+workDir+"\n" {
		t.Fatalf("command stdout = %q", result.Stdout)
	}
	if result.Deadline == nil {
		t.Fatal("command deadline is nil")
	}
	remaining := time.Until(*result.Deadline)
	if remaining < 119*time.Second || remaining > 2*time.Minute {
		t.Fatalf("command deadline remaining = %s", remaining)
	}
	spawnFailure := validateCommand(context.Background(), filepath.Join(workDir, "missing"), "true")
	if spawnFailure.Passed || spawnFailure.Status != ValidationStatusFailed || !strings.Contains(spawnFailure.Message, "命令启动失败") {
		t.Fatalf("command spawn failure = %#v", spawnFailure)
	}
}

func TestEVBEN006FileContainsAndUnknownValidationMatrix(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "unicode.txt"), []byte("状态: 完成\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "empty.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workDir, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		path     string
		contains string
		passed   bool
		message  string
	}{
		{name: "unicode match", path: "unicode.txt", contains: "完成", passed: true},
		{name: "mismatch", path: "unicode.txt", contains: "失败", message: "不包括目标文本"},
		{name: "empty content", path: "empty.txt", contains: "value", message: "不包括目标文本"},
		{name: "missing", path: "missing.txt", contains: "value"},
		{name: "read error", path: "directory", contains: "value", message: "not a regular file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := validateFileContains(workDir, test.path, test.contains)
			if result.Passed != test.passed || result.Status == "" || (!test.passed && result.Message == "") || (test.message != "" && !strings.Contains(result.Message, test.message)) {
				t.Fatalf("file_contains result = %#v", result)
			}
		})
	}
	results := ValidateAll(context.Background(), workDir, []Validation{{Type: "unknown"}, {Type: "file_contains", Path: "unicode.txt", Contains: "完成"}})
	if len(results) != 2 || results[0].Passed || results[0].Message != "未知验证类型" || !results[1].Passed {
		t.Fatalf("unknown validation ordering = %#v", results)
	}
}

func TestEVBEN009HumanSummaryIsExactAndOrdered(t *testing.T) {
	results := []*Result{
		{CaseID: "pass", Success: true, DurationMS: 12, SessionID: "session-1", Workspace: "/work/one"},
		{CaseID: "fail", DurationMS: 34, SessionID: "session-2", Workspace: "/work/two"},
	}
	got := captureBenchmarkStdout(t, func() { PrintSummary(results) })
	want := "Benchmark Summary: 1/2 passed\n" +
		"- [PASS] pass duration=12ms session=session-1 workspace=/work/one\n" +
		"- [FAIL] fail duration=34ms session=session-2 workspace=/work/two\n"
	if got != want {
		t.Fatalf("summary mismatch\ngot:  %q\nwant: %q", got, want)
	}
	if got := captureBenchmarkStdout(t, func() { PrintSummary(nil) }); got != "Benchmark Summary: 0/0 passed\n" {
		t.Fatalf("empty summary = %q", got)
	}
}

func TestEVBEN010JSONOverwritePermissionsAndWriteErrors(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "report.json")
	if err := os.WriteFile(report, []byte("obsolete trailing data"), 0o600); err != nil {
		t.Fatal(err)
	}
	results := []*Result{{SchemaVersion: ResultSchemaVersion, CaseID: "first"}, {SchemaVersion: ResultSchemaVersion, CaseID: "second"}}
	if err := WriteJSON(report, results); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "obsolete") || strings.Index(string(data), `"case_id": "first"`) > strings.Index(string(data), `"case_id": "second"`) || !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("overwritten report = %s", data)
	}
	assertBenchmarkMode(t, report, 0o600)

	newReport := filepath.Join(root, "new.json")
	if err := WriteJSON(newReport, nil); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(newReport); err != nil || string(data) != "null\n" {
		t.Fatalf("empty report/error = %q/%v", data, err)
	}
	info, err := os.Stat(newReport)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && (info.Mode().Perm()&0o200 == 0 || info.Mode().Perm()&0o111 != 0) {
		t.Fatalf("new report mode = %o", info.Mode().Perm())
	}
	if err := WriteJSON(root, results); err == nil {
		t.Fatal("WriteJSON() wrote to a directory")
	}
	if err := WriteJSON(filepath.Join(root, "missing", "report.json"), results); err == nil {
		t.Fatal("WriteJSON() created a missing parent")
	}
}

func assertBenchmarkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode(%s) = %o, want %o", path, got, want)
	}
}

func captureBenchmarkStdout(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	type readResult struct {
		data []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		data, readErr := io.ReadAll(reader)
		done <- readResult{data: data, err: readErr}
	}()
	func() {
		defer func() {
			os.Stdout = oldStdout
			_ = writer.Close()
		}()
		fn()
	}()
	result := <-done
	_ = reader.Close()
	if result.err != nil {
		t.Fatal(result.err)
	}
	return string(result.data)
}
