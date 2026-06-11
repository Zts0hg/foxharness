package autodev

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeExec scripts ExecRunner outcomes keyed by the joined command line.
type fakeExec struct {
	outputs map[string]string
	errs    map[string]error
	calls   []string
}

func (f *fakeExec) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, key)
	return f.outputs[key], f.errs[key]
}

func allGates() GateConfig { return GateConfig{Build: true, Test: true, Gofmt: true} }

func TestGateCheckAllGreen(t *testing.T) {
	exec := &fakeExec{}
	gate := NewGateRunner(exec, NewTerminalReporter(&bytes.Buffer{}))

	result, err := gate.Check(context.Background(), "/wt", allGates())
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !result.Passed {
		t.Errorf("Passed = false, want true when every command succeeds")
	}
	wantCalls := []string{"go build ./...", "go test ./...", "gofmt -l ."}
	if len(exec.calls) != len(wantCalls) {
		t.Fatalf("calls = %v, want %v", exec.calls, wantCalls)
	}
	for i, want := range wantCalls {
		if exec.calls[i] != want {
			t.Errorf("calls[%d] = %q, want %q", i, exec.calls[i], want)
		}
	}
}

func TestGateCheckTestRedBlocks(t *testing.T) {
	exec := &fakeExec{
		outputs: map[string]string{"go test ./...": "--- FAIL: TestX"},
		errs:    map[string]error{"go test ./...": errors.New("exit status 1")},
	}
	gate := NewGateRunner(exec, NewTerminalReporter(&bytes.Buffer{}))

	result, err := gate.Check(context.Background(), "/wt", allGates())
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Passed {
		t.Error("Passed = true with red tests, want false (TC-010)")
	}
	var testStep *GateStep
	for i := range result.Steps {
		if result.Steps[i].Name == "test" {
			testStep = &result.Steps[i]
		}
	}
	if testStep == nil {
		t.Fatal("no test step in result")
	}
	if testStep.Passed {
		t.Error("test step Passed = true, want false")
	}
	if !strings.Contains(testStep.Output, "FAIL") {
		t.Errorf("test step Output = %q, want failure output captured", testStep.Output)
	}
}

func TestGateCheckGofmtDirtyBlocks(t *testing.T) {
	exec := &fakeExec{outputs: map[string]string{"gofmt -l .": "dirty.go\n"}}
	gate := NewGateRunner(exec, NewTerminalReporter(&bytes.Buffer{}))

	result, err := gate.Check(context.Background(), "/wt", allGates())
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Passed {
		t.Error("Passed = true with unformatted files, want false")
	}
}

func TestGateCheckTestCannotBeDisabled(t *testing.T) {
	exec := &fakeExec{}
	var buf bytes.Buffer
	gate := NewGateRunner(exec, NewTerminalReporter(&buf))

	result, err := gate.Check(context.Background(), "/wt", GateConfig{Build: true, Test: false, Gofmt: true})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	ran := false
	for _, call := range exec.calls {
		if call == "go test ./..." {
			ran = true
		}
	}
	if !ran {
		t.Error("go test did not run with Test=false, want forced run (gate floor, REQ-018)")
	}
	if !strings.Contains(strings.ToLower(buf.String()), "warning") {
		t.Errorf("output = %q, want a prominent warning about the forced test gate", buf.String())
	}
	if !result.Passed {
		t.Error("Passed = false, want true (commands all succeed)")
	}
}

func TestGateCheckDisabledOptionalGatesSkipWithWarning(t *testing.T) {
	exec := &fakeExec{}
	var buf bytes.Buffer
	gate := NewGateRunner(exec, NewTerminalReporter(&buf))

	result, err := gate.Check(context.Background(), "/wt", GateConfig{Build: false, Test: true, Gofmt: false})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	for _, call := range exec.calls {
		if strings.HasPrefix(call, "go build") || strings.HasPrefix(call, "gofmt") {
			t.Errorf("disabled gate still ran: %q", call)
		}
	}
	skipped := 0
	for _, s := range result.Steps {
		if s.Skipped {
			skipped++
		}
	}
	if skipped != 2 {
		t.Errorf("skipped steps = %d, want 2 (build, gofmt)", skipped)
	}
	if !strings.Contains(strings.ToLower(buf.String()), "warning") {
		t.Error("want a warning when optional gates are disabled")
	}
	if !result.Passed {
		t.Error("Passed = false, want true (enabled gates green)")
	}
}

func TestGateRunnerImplementsGateChecker(t *testing.T) {
	var _ GateChecker = NewGateRunner(&fakeExec{}, nil)
}

func TestGateCheckBuildFailureIncludesOutput(t *testing.T) {
	exec := &fakeExec{
		outputs: map[string]string{"go build ./...": "pkg/x: undefined: Foo"},
		errs:    map[string]error{"go build ./...": fmt.Errorf("exit status 2")},
	}
	gate := NewGateRunner(exec, nil)

	result, err := gate.Check(context.Background(), "/wt", allGates())
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Passed {
		t.Error("Passed = true with build failure, want false")
	}
	if !strings.Contains(result.Steps[0].Output, "undefined: Foo") {
		t.Errorf("build output = %q, want compiler diagnostics", result.Steps[0].Output)
	}
}
