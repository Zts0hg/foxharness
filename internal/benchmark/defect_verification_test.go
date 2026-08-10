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

func TestDVBEN005CaseLoadingRejectsInvalidStructuralDomains(t *testing.T) {
	validPrefix := "id: valid\nfixture: missing-fixture\nprompt: run\n"
	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{name: "unknown top-level field", contents: validPrefix + "unexpected: true\nvalidations:\n  - type: command\n    command: true\n", want: "unexpected"},
		{name: "duplicate field", contents: "id: first\nid: second\nfixture: f\nprompt: run\nvalidations:\n  - type: command\n    command: true\n", want: "id"},
		{name: "blank id", contents: "id: '  '\nfixture: f\nprompt: run\nvalidations:\n  - type: command\n    command: true\n", want: "id"},
		{name: "blank fixture", contents: "id: valid\nfixture: '  '\nprompt: run\nvalidations:\n  - type: command\n    command: true\n", want: "fixture"},
		{name: "blank prompt", contents: "id: valid\nfixture: f\nprompt: '  '\nvalidations:\n  - type: command\n    command: true\n", want: "prompt"},
		{name: "no validations", contents: validPrefix + "validations: []\n", want: "validation"},
		{name: "negative max turns", contents: validPrefix + "max_turns: -1\nvalidations:\n  - type: command\n    command: true\n", want: "max_turns"},
		{name: "null max turns", contents: validPrefix + "max_turns: null\nvalidations:\n  - type: command\n    command: true\n", want: "max_turns"},
		{name: "zero timeout", contents: validPrefix + "timeout_seconds: 0\nvalidations:\n  - type: command\n    command: true\n", want: "timeout_seconds"},
		{name: "negative timeout", contents: validPrefix + "timeout_seconds: -1\nvalidations:\n  - type: command\n    command: true\n", want: "timeout_seconds"},
		{name: "null timeout", contents: validPrefix + "timeout_seconds: null\nvalidations:\n  - type: command\n    command: true\n", want: "timeout_seconds"},
		{name: "overflowing timeout duration", contents: validPrefix + "timeout_seconds: 9223372037\nvalidations:\n  - type: command\n    command: true\n", want: "timeout_seconds"},
		{name: "unknown validation", contents: validPrefix + "validations:\n  - type: unknown\n", want: "type"},
		{name: "unknown validation field", contents: validPrefix + "validations:\n  - type: command\n    command: true\n    extra: value\n", want: "extra"},
		{name: "duplicate validation field", contents: validPrefix + "validations:\n  - type: command\n    command: first\n    command: second\n", want: "command"},
		{name: "command missing command", contents: validPrefix + "validations:\n  - type: command\n", want: "command"},
		{name: "command blank command", contents: validPrefix + "validations:\n  - type: command\n    command: '  '\n", want: "command"},
		{name: "command irrelevant path", contents: validPrefix + "validations:\n  - type: command\n    command: true\n    path: ''\n", want: "path"},
		{name: "command irrelevant null path", contents: validPrefix + "validations:\n  - type: command\n    command: true\n    path: null\n", want: "path"},
		{name: "file missing path", contents: validPrefix + "validations:\n  - type: file_contains\n    contains: text\n", want: "path"},
		{name: "file blank path", contents: validPrefix + "validations:\n  - type: file_contains\n    path: '  '\n    contains: text\n", want: "path"},
		{name: "file missing contains", contents: validPrefix + "validations:\n  - type: file_contains\n    path: result.txt\n", want: "contains"},
		{name: "file blank contains", contents: validPrefix + "validations:\n  - type: file_contains\n    path: result.txt\n    contains: '  '\n", want: "contains"},
		{name: "file irrelevant command", contents: validPrefix + "validations:\n  - type: file_contains\n    path: result.txt\n    contains: text\n    command: ''\n", want: "command"},
		{name: "first validation wins", contents: validPrefix + "validations:\n  - type: command\n  - type: unknown\n", want: "validation[1]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			casePath := filepath.Join(t.TempDir(), "case.yaml")
			if err := os.WriteFile(casePath, []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadCase(casePath); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadCase() error = %v, want structural error containing %q", err, test.want)
			}
		})
	}
}

func TestDVBEN005RelativeFixtureResolvesFromCaseDirectory(t *testing.T) {
	root := t.TempDir()
	caseDir := filepath.Join(root, "cases")
	fixture := filepath.Join(root, "fixtures", "sample")
	if err := os.MkdirAll(caseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fixture, 0o700); err != nil {
		t.Fatal(err)
	}
	casePath := filepath.Join(caseDir, "case.yaml")
	contents := "id: relative\nfixture: ../fixtures/sample\nprompt: run\nvalidations:\n  - type: command\n    command: true\n"
	if err := os.WriteFile(casePath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCase(casePath)
	if err != nil {
		t.Fatalf("LoadCase() error = %v", err)
	}
	if loaded.Fixture != fixture {
		t.Fatalf("Fixture = %q, want %q", loaded.Fixture, fixture)
	}
}

func TestDVBEN006CommandOutputIsIndependentlyBounded(t *testing.T) {
	if maxValidationOutputBytes != 1<<20 {
		t.Fatalf("maxValidationOutputBytes = %d, want 1 MiB", maxValidationOutputBytes)
	}
	tests := []struct {
		name           string
		command        string
		wantStdoutOver bool
		wantStderrOver bool
	}{
		{name: "stdout", command: "yes stdout", wantStdoutOver: true},
		{name: "stderr", command: "yes stderr >&2", wantStderrOver: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := defaultCommandValidationConfig()
			config.timeout = time.Second
			config.terminateGrace = 20 * time.Millisecond
			result := executeCommandValidation(context.Background(), t.TempDir(), test.command, config)
			if result.Status != ValidationStatusFailed || result.Passed {
				t.Fatalf("overflow result = %#v, want failed", result)
			}
			if result.StdoutOverflow != test.wantStdoutOver || result.StderrOverflow != test.wantStderrOver {
				t.Fatalf("overflow flags = stdout:%t stderr:%t", result.StdoutOverflow, result.StderrOverflow)
			}
			if test.wantStdoutOver && len(result.Stdout) != maxValidationOutputBytes {
				t.Fatalf("retained stdout length = %d, want %d", len(result.Stdout), maxValidationOutputBytes)
			}
			if test.wantStderrOver && len(result.Stderr) != maxValidationOutputBytes {
				t.Fatalf("retained stderr length = %d, want %d", len(result.Stderr), maxValidationOutputBytes)
			}
			if len(result.Stdout) > maxValidationOutputBytes || len(result.Stderr) > maxValidationOutputBytes {
				t.Fatalf("retained output lengths = stdout:%d stderr:%d", len(result.Stdout), len(result.Stderr))
			}
			if !strings.Contains(result.Message, test.name) {
				t.Fatalf("overflow message = %q, want stream evidence", result.Message)
			}
		})
	}
}

func TestDVBEN006CommandFailurePreservesSeparateOutput(t *testing.T) {
	result := executeCommandValidation(context.Background(), t.TempDir(), "printf stdout; printf stderr >&2; exit 7", defaultCommandValidationConfig())
	if result.Status != ValidationStatusFailed || result.Stdout != "stdout" || result.Stderr != "stderr" {
		t.Fatalf("command failure = %#v", result)
	}
	if result.StdoutOverflow || result.StderrOverflow {
		t.Fatalf("moderate output marked overflow: %#v", result)
	}
}

func TestDVBEN006CancellationKillsIgnoringDescendantsAndReaps(t *testing.T) {
	workDir := t.TempDir()
	config := defaultCommandValidationConfig()
	config.timeout = time.Minute
	config.terminateGrace = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan ValidationResult, 1)
	go func() {
		resultCh <- executeCommandValidation(ctx, workDir, "touch started; (trap '' TERM; sleep 0.3; touch leaked) & while :; do sleep 1; done", config)
	}()
	waitForBenchmarkPath(t, filepath.Join(workDir, "started"), time.Second)
	cancel()
	select {
	case result := <-resultCh:
		if result.Status != ValidationStatusCancelled {
			t.Fatalf("cancelled result = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("command process tree was not terminated and reaped")
	}
	time.Sleep(350 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(workDir, "leaked")); !os.IsNotExist(err) {
		t.Fatalf("descendant survived cancellation: %v", err)
	}
}

func TestDVBEN006ValidatorTimeoutIsDistinctAndOrdered(t *testing.T) {
	config := defaultCommandValidationConfig()
	config.timeout = 20 * time.Millisecond
	config.terminateGrace = 20 * time.Millisecond
	result := executeCommandValidation(context.Background(), t.TempDir(), "sleep 1", config)
	if result.Status != ValidationStatusTimedOut {
		t.Fatalf("timeout result = %#v", result)
	}
	parentCtx, cancelParent := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelParent()
	config.timeout = time.Minute
	result = executeCommandValidation(parentCtx, t.TempDir(), "sleep 1", config)
	if result.Status != ValidationStatusTimedOut {
		t.Fatalf("parent deadline result = %#v", result)
	}

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "result.txt"), []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
	results := ValidateAll(context.Background(), workDir, []Validation{
		{Type: "command", Command: "yes overflow"},
		{Type: "file_contains", Path: "result.txt", Contains: "done"},
	})
	if len(results) != 2 || results[0].Status != ValidationStatusFailed || !results[0].StdoutOverflow || !results[1].Passed {
		t.Fatalf("ordered overflow results = %#v", results)
	}
}

func TestDVBEN006ActiveCancellationSynthesizesRemainingResults(t *testing.T) {
	workDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	resultsCh := make(chan []ValidationResult, 1)
	go func() {
		resultsCh <- ValidateAll(ctx, workDir, []Validation{
			{Type: "command", Command: "touch started; sleep 1"},
			{Type: "command", Command: "touch should-not-run"},
		})
	}()
	waitForBenchmarkPath(t, filepath.Join(workDir, "started"), time.Second)
	cancel()
	results := <-resultsCh
	if len(results) != 2 || results[0].Status != ValidationStatusCancelled || results[1].Status != ValidationStatusCancelled {
		t.Fatalf("cancelled ordered results = %#v", results)
	}
	if _, err := os.Stat(filepath.Join(workDir, "should-not-run")); !os.IsNotExist(err) {
		t.Fatalf("post-cancellation validation executed: %v", err)
	}
}

func waitForBenchmarkPath(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("path %q was not created before %s", path, timeout)
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
