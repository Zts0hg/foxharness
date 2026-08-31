package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

type readOnlyBashRunnerStub struct {
	requests []readOnlyBashRequest
	result   BashCommandResult
}

func (s *readOnlyBashRunnerStub) Run(_ context.Context, request readOnlyBashRequest) BashCommandResult {
	s.requests = append(s.requests, request)
	return s.result
}

func TestReadOnlyBashRetainsCompatibleDefinition(t *testing.T) {
	regular := NewBashTool(t.TempDir()).Definition()
	readOnly := NewReadOnlyBashTool(t.TempDir()).Definition()
	if readOnly.Name != regular.Name || !reflect.DeepEqual(readOnly.InputSchema, regular.InputSchema) {
		t.Fatalf("read-only Bash name/schema = %#v, want compatible %#v", readOnly, regular)
	}
	if !strings.Contains(strings.ToLower(readOnly.Description), "read-only") || strings.Contains(strings.ToLower(readOnly.Description), "arbitrary") {
		t.Fatalf("read-only Bash description = %q, want explicit non-arbitrary ceiling", readOnly.Description)
	}
}

func TestReadOnlyBashRejectsUnclassifiedOrMutatingShellForms(t *testing.T) {
	runner := &readOnlyBashRunnerStub{result: BashCommandResult{Output: "runner-reached"}}
	tool := newReadOnlyBashToolWithRunner(t.TempDir(), runner)
	commands := []string{
		"",
		"# comment only",
		"pwd &",
		"{ pwd; }",
		"if pwd; then pwd; fi",
		"NAME=value pwd",
		"cat $(pwd)/file",
		"cat file > copy",
		"bash -c 'pwd'",
		"find . -exec pwd \\;",
		"rg --pre=cat pattern .",
		"git commit",
		"curl https://example.com",
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			before := len(runner.requests)
			result, err := tool.ExecuteResult(context.Background(), bashCommandArgs(t, command))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Failed {
				t.Fatalf("read-only Bash accepted %q: %#v", command, result)
			}
			if len(runner.requests) != before {
				t.Fatalf("read-only Bash sent rejected command %q to sandbox runner", command)
			}
		})
	}
}

func TestReadOnlyBashAllowsConservativeSafeFormsThroughSandboxRunner(t *testing.T) {
	runner := &readOnlyBashRunnerStub{result: BashCommandResult{Output: "safe"}}
	tool := newReadOnlyBashToolWithRunner(t.TempDir(), runner)
	commands := []string{
		"pwd",
		"ls",
		"cat fixture.txt",
		"head -n 1 fixture.txt",
		"grep needle fixture.txt",
		"rg needle .",
		"find . -maxdepth 1",
		"git status --short",
		"pwd && ls",
		"cat fixture.txt | wc -l",
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			result, err := tool.ExecuteResult(context.Background(), bashCommandArgs(t, command))
			if err != nil {
				t.Fatal(err)
			}
			if result.Failed || result.Output != "safe" {
				t.Fatalf("read-only Bash rejected safe command %q: %#v", command, result)
			}
		})
	}
	if len(runner.requests) != len(commands) {
		t.Fatalf("sandbox runner requests = %d, want %d", len(runner.requests), len(commands))
	}
}

func TestReadOnlyBashExecutesProvenCommandOnlyThroughSandboxRunner(t *testing.T) {
	workDir := t.TempDir()
	runner := &readOnlyBashRunnerStub{result: BashCommandResult{Output: "sandboxed"}}
	tool := newReadOnlyBashToolWithRunner(workDir, runner)

	result, err := tool.ExecuteResult(context.Background(), bashCommandArgs(t, "pwd"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed || result.Output != "sandboxed" {
		t.Fatalf("sandboxed read-only result = %#v", result)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("sandbox runner calls = %d, want one", len(runner.requests))
	}
	request := runner.requests[0]
	if request.WorkDir != workDir || request.Command != "pwd" || request.Timeout != defaultBashTimeout {
		t.Fatalf("sandbox request = %+v", request)
	}
	if !reflect.DeepEqual(request.ReadableRoots, []string{workDir}) {
		t.Fatalf("sandbox readable roots = %v, want [%s]", request.ReadableRoots, workDir)
	}
}

func TestReadOnlyBashNeverFallsBackWhenSandboxIsUnavailable(t *testing.T) {
	runner := &readOnlyBashRunnerStub{result: BashCommandResult{Err: ErrReadOnlyBashSandboxUnavailable}}
	tool := newReadOnlyBashToolWithRunner(t.TempDir(), runner)

	result, err := tool.ExecuteResult(context.Background(), bashCommandArgs(t, "pwd"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Failed || !strings.Contains(result.Output, "read-only sandbox unavailable") {
		t.Fatalf("unavailable sandbox result = %#v/%v", result, runner.result.Err)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("sandbox runner calls = %d, want exactly one and no fallback", len(runner.requests))
	}
}

func TestReadOnlyBashEnvironmentIsFixedAndIsolated(t *testing.T) {
	t.Setenv("FOX_SECRET", "must-not-leak")
	t.Setenv("HTTPS_PROXY", "must-not-leak")

	environment := readOnlyBashEnvironment()
	want := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/nonexistent",
		"TMPDIR=/nonexistent",
		"LANG=C",
		"LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	}
	if !reflect.DeepEqual(environment, want) {
		t.Fatalf("read-only Bash environment = %v, want %v", environment, want)
	}
}

func TestReadOnlyBashTimeoutDoesNotSuggestBackgroundExecution(t *testing.T) {
	runner := &readOnlyBashRunnerStub{result: BashCommandResult{TimedOut: true, Err: context.DeadlineExceeded}}
	tool := newReadOnlyBashToolWithRunner(t.TempDir(), runner)

	result, err := tool.ExecuteResult(context.Background(), bashCommandArgs(t, "pwd"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Failed || !strings.Contains(result.Output, "命令执行超时") {
		t.Fatalf("read-only timeout result = %#v", result)
	}
	if strings.Contains(result.Output, "转入后台") {
		t.Fatalf("read-only timeout suggests forbidden background execution: %q", result.Output)
	}
}
