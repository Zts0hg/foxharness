package benchmark

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDVBEN001CaseDeadlineCoversFactoryAndStopsUnstartedValidations(t *testing.T) {
	casePath := filepath.Join(t.TempDir(), "case.yaml")
	caseYAML := "id: timeout-default\nfixture: fixture\nprompt: run\nvalidations:\n  - type: command\n    command: 'true'\n"
	if err := os.WriteFile(casePath, []byte(caseYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCase(casePath)
	if err != nil {
		t.Fatalf("LoadCase() error = %v", err)
	}
	if loaded.TimeoutSeconds != 600 {
		t.Fatalf("LoadCase() timeout = %d, want 600", loaded.TimeoutSeconds)
	}

	fixture := t.TempDir()
	deadlineObserved := make(chan bool, 1)
	runner := NewRunner(func(ctx context.Context, _ string, _ *Case) (*Harness, error) {
		_, ok := ctx.Deadline()
		deadlineObserved <- ok
		<-ctx.Done()
		return nil, ctx.Err()
	})
	runner.caseTimeout = func(*Case) time.Duration { return 10 * time.Millisecond }
	timeoutResult, err := runner.RunCase(context.Background(), &Case{ID: "timeout", Fixture: fixture, Prompt: "run", TimeoutSeconds: 600})
	if err != nil || timeoutResult.Status != ResultStatusTimedOut {
		t.Fatalf("RunCase() result/error = %#v/%v, want whole-case timeout", timeoutResult, err)
	}
	if !<-deadlineObserved {
		t.Fatal("HarnessFactory did not receive the case deadline")
	}

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "result.txt"), []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := ValidateAll(ctx, workDir, []Validation{
		{Type: "command", Command: "touch should-not-run"},
		{Type: "file_contains", Path: "result.txt", Contains: "done"},
	})
	if len(results) != 2 || results[0].Status != ValidationStatusCancelled || results[1].Status != ValidationStatusCancelled {
		t.Fatalf("cancelled validation results = %#v, want ordered synthetic cancellation", results)
	}
	if _, err := os.Stat(filepath.Join(workDir, "should-not-run")); !os.IsNotExist(err) {
		t.Fatalf("cancelled command executed, stat error = %v", err)
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer timeoutCancel()
	<-timeoutCtx.Done()
	timedOut := ValidateAll(timeoutCtx, workDir, []Validation{{Type: "command", Command: "true"}})
	if len(timedOut) != 1 || timedOut[0].Status != ValidationStatusTimedOut {
		t.Fatalf("timed-out validation results = %#v, want timed_out", timedOut)
	}
}

func TestDVBEN002AcceptedRepeatAlwaysHasTypedStatusAndSeparateEvidence(t *testing.T) {
	fixture := t.TempDir()
	infrastructure := NewRunner(func(context.Context, string, *Case) (*Harness, error) {
		return nil, errors.New("factory unavailable")
	})
	result, err := infrastructure.RunCase(context.Background(), &Case{ID: "infra", Fixture: fixture, Prompt: "run"})
	if err == nil || result == nil || result.Status != ResultStatusInfrastructureFailed || result.InfrastructureError == "" {
		t.Fatalf("infrastructure result/error = %#v/%v", result, err)
	}
	if result.RuntimeError != "" || result.EvaluationError != "" || result.Success {
		t.Fatalf("infrastructure evidence conflated = %#v", result)
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := NewRunner(func(ctx context.Context, _ string, _ *Case) (*Harness, error) {
		return nil, ctx.Err()
	})
	result, err = cancelled.RunCase(cancelledCtx, &Case{ID: "cancel", Fixture: fixture, Prompt: "run"})
	if err != nil || result == nil || result.Status != ResultStatusCancelled || result.InfrastructureError != "" {
		t.Fatalf("cancelled result/error = %#v/%v", result, err)
	}
}

func TestDVBEN003RuntimeFidelityDerivesFromResolvedSpecification(t *testing.T) {
	spec := NewRuntimeSpec("openai", "model-a", 17, []string{"read_file", "read_todo"})
	fidelity := spec.Fidelity()
	if fidelity.Spec.ProviderProtocol != "openai" || fidelity.Spec.Model != "model-a" || fidelity.Spec.MaxTurns != 17 {
		t.Fatalf("fidelity spec = %#v, want resolved provider/model/turn budget", fidelity.Spec)
	}
	if !reflect.DeepEqual(fidelity.Spec.ToolSurface, []string{"read_file", "read_todo"}) {
		t.Fatalf("tool surface = %#v", fidelity.Spec.ToolSurface)
	}
	if len(fidelity.SharedInvariants) == 0 || len(fidelity.IntentionalDifferences) == 0 {
		t.Fatalf("derived human fidelity = %#v", fidelity)
	}
}

func TestDVBEN004FixtureAndValidationStayWithinOwnedRoots(t *testing.T) {
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
	if err := copyDir(fixture, destination); err == nil {
		t.Fatal("copyDir() accepted a source symlink")
	}
	linkTarget, err := os.Readlink(filepath.Join(fixture, "linked.txt"))
	if err != nil || linkTarget != outside {
		t.Fatalf("source fixture changed after rejected copy: target/error = %q/%v", linkTarget, err)
	}

	workspace := t.TempDir()
	relativeEscape, err := filepath.Rel(workspace, outside)
	if err != nil {
		t.Fatal(err)
	}
	if result := validateFileContains(workspace, relativeEscape, "external"); result.Passed {
		t.Fatalf("traversal validation = %#v, want rejection", result)
	}
	if result := validateFileContains(workspace, outside, "external"); result.Passed {
		t.Fatalf("absolute validation = %#v, want rejection", result)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "result.txt")); err != nil {
		t.Fatal(err)
	}
	if result := validateFileContains(workspace, "result.txt", "external"); result.Passed {
		t.Fatalf("symlink validation = %#v, want rejection", result)
	}
	if err := os.Mkdir(filepath.Join(workspace, "directory.txt"), 0o700); err != nil {
		t.Fatal(err)
	}
	if result := validateFileContains(workspace, "directory.txt", "anything"); result.Passed {
		t.Fatalf("directory validation = %#v, want regular-file rejection", result)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "nested", "valid.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := validateFileContains(workspace, filepath.Join("nested", "valid.txt"), "inside"); !result.Passed {
		t.Fatalf("valid rooted file = %#v", result)
	}

	directoryFixture := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(directoryFixture, "linked-directory")); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(directoryFixture, t.TempDir()); err == nil {
		t.Fatal("copyDir() accepted a source directory symlink")
	}

	rootTarget := t.TempDir()
	rootLink := filepath.Join(t.TempDir(), "fixture-link")
	if err := os.Symlink(rootTarget, rootLink); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(rootLink, t.TempDir()); err == nil {
		t.Fatal("copyDir() accepted a symlink fixture root")
	}

	unsupportedFixture := t.TempDir()
	listener, err := net.Listen("unix", filepath.Join(unsupportedFixture, "socket"))
	if err != nil {
		t.Skipf("unix socket fixture unavailable: %v", err)
	}
	defer listener.Close()
	if err := copyDir(unsupportedFixture, t.TempDir()); err == nil {
		t.Fatal("copyDir() accepted an unsupported source file type")
	}
}

func TestDVBEN004PartialFixtureCopyFailureRemovesWorkspace(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "a-regular.txt"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a-regular.txt", filepath.Join(fixture, "z-symlink.txt")); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(func(context.Context, string, *Case) (*Harness, error) {
		t.Fatal("factory called after fixture copy failure")
		return nil, nil
	})
	result, err := runner.RunCase(context.Background(), &Case{ID: "partial-copy", Fixture: fixture, Prompt: "run"})
	if err == nil || result == nil || result.Status != ResultStatusInfrastructureFailed {
		t.Fatalf("RunCase() result/error = %#v/%v, want infrastructure failure", result, err)
	}
	if _, statErr := os.Stat(result.Workspace); !os.IsNotExist(statErr) {
		t.Fatalf("partial workspace stat = %v, want removed", statErr)
	}
}

func TestDVBEN004FailedWorkspaceCleanupUsesFreshBoundedContext(t *testing.T) {
	fixture := t.TempDir()
	runner := NewRunner(func(_ context.Context, _ string, _ *Case) (*Harness, error) {
		return nil, errors.New("factory failure")
	})
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	cleanupContext := make(chan struct {
		err      error
		deadline bool
	}, 1)
	runner.removeWorkspace = func(ctx context.Context, path string) error {
		_, deadline := ctx.Deadline()
		cleanupContext <- struct {
			err      error
			deadline bool
		}{err: ctx.Err(), deadline: deadline}
		return os.RemoveAll(path)
	}
	result, err := runner.RunCase(parent, &Case{ID: "cleanup", Fixture: fixture, Prompt: "run"})
	if err != nil || result == nil || result.Status != ResultStatusCancelled {
		t.Fatalf("RunCase() result/error = %#v/%v", result, err)
	}
	observed := <-cleanupContext
	if observed.err != nil || !observed.deadline {
		t.Fatalf("cleanup context = %#v, want fresh bounded context", observed)
	}
	if _, err := os.Stat(result.Workspace); !os.IsNotExist(err) {
		t.Fatalf("failed workspace stat = %v, want removed", err)
	}
}

func TestDVBEN004CleanupFailureBecomesInfrastructureEvidence(t *testing.T) {
	runner := NewRunner(func(context.Context, string, *Case) (*Harness, error) {
		return nil, errors.New("factory failure")
	})
	runner.removeWorkspace = func(context.Context, string) error { return errors.New("cleanup failure") }
	result, err := runner.RunCase(context.Background(), &Case{ID: "cleanup-failure", Fixture: t.TempDir(), Prompt: "run"})
	if err == nil || result == nil || result.Status != ResultStatusInfrastructureFailed || !strings.Contains(result.CleanupError, "cleanup failure") {
		t.Fatalf("cleanup failure result/error = %#v/%v", result, err)
	}
}

func TestDVBEN004CleanupTimeoutBecomesInfrastructureEvidence(t *testing.T) {
	runner := NewRunner(func(context.Context, string, *Case) (*Harness, error) {
		return nil, errors.New("factory failure")
	})
	runner.cleanupTimeout = func() time.Duration { return time.Millisecond }
	runner.removeWorkspace = func(ctx context.Context, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}
	result, err := runner.RunCase(context.Background(), &Case{ID: "cleanup-timeout", Fixture: t.TempDir(), Prompt: "run"})
	if err == nil || result == nil || result.Status != ResultStatusInfrastructureFailed || !strings.Contains(result.CleanupError, context.DeadlineExceeded.Error()) {
		t.Fatalf("cleanup timeout result/error = %#v/%v", result, err)
	}
}

func TestDVBEN004PanicStillCleansWorkspace(t *testing.T) {
	runner := NewRunner(func(context.Context, string, *Case) (*Harness, error) {
		panic("factory panic")
	})
	var workspace string
	runner.removeWorkspace = func(ctx context.Context, path string) error {
		if ctx.Err() != nil {
			t.Fatalf("cleanup received expired context: %v", ctx.Err())
		}
		workspace = path
		return os.RemoveAll(path)
	}
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = runner.RunCase(context.Background(), &Case{ID: "panic", Fixture: t.TempDir(), Prompt: "run"})
	}()
	if recovered == nil {
		t.Fatal("RunCase() did not propagate factory panic")
	}
	if workspace == "" {
		t.Fatal("panic path did not attempt workspace cleanup")
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("panic workspace stat = %v, want removed", err)
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
