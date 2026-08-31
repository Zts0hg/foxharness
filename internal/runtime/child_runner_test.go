package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestChildRunnerFreezesLineageAndIntersectsCapabilityCeilings(t *testing.T) {
	store := newLifecycleStore()
	var childAssemblies []RunAssembly
	dependencies := successfulHarnessDependencies(&childAssemblies)
	harness, err := NewRuntimeHarness(store, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	parentScope, err := parent.BeginRun(context.Background(), RunSpec{
		Prompt: "parent", WorkDir: "/workspace", Model: "parent-model", ProviderProtocol: "messages",
		AllowedTools: []string{"delegate_task", "read_file", "write_file"},
	})
	if err != nil {
		t.Fatal(err)
	}
	childRunner, err := parent.NewChildRunner(parentScope)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := &recordingChildCleanup{}
	allowed := []string{"read_file", "write_file", "delegate_task"}
	agentTools := []string{"read_file"}

	result, err := childRunner.Run(context.Background(), ChildRunRequest{
		InvocationID: "invoke-1", DelegationID: "delegate-1", Agent: "general-purpose",
		Task: "inspect files", ReadOnly: false, AllowedTools: allowed,
		AgentAllowedTools: agentTools, Depth: 1, Cleanup: cleanup,
	})
	if err != nil {
		t.Fatal(err)
	}
	allowed[0] = "write_file"
	agentTools[0] = "write_file"
	if result.Status != ChildSucceeded || result.Report != "done" || result.InvocationID != "invoke-1" ||
		result.ParentSessionID != parent.Snapshot().ID || result.ParentRunID != parentScope.Snapshot().RunID ||
		result.SessionID == "" || result.RunID == "" || result.Depth != 1 || result.Agent != "general-purpose" {
		t.Fatalf("child result = %#v", result)
	}
	if cleanup.calls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanup.calls)
	}
	if len(childAssemblies) != 4 {
		t.Fatalf("child assembly calls = %d, want 4", len(childAssemblies))
	}
	for _, assembly := range childAssemblies {
		if assembly.Run.Profile != ChildRun || assembly.Run.Model != "parent-model" ||
			assembly.Spec.ProviderProtocol != "messages" || assembly.Spec.WorkDir != "/workspace" ||
			assembly.Session.ParentSessionID != parent.Snapshot().ID || assembly.Session.ParentRunID != parentScope.Snapshot().RunID {
			t.Fatalf("child assembly = %#v", assembly)
		}
		if !reflect.DeepEqual(assembly.AllowedTools, []string{"read_file"}) {
			t.Fatalf("child tools = %v, want frozen intersection [read_file]", assembly.AllowedTools)
		}
		if assembly.ChildRunner == nil || assembly.ChildRunner.parentRun.RunID != assembly.Run.RunID {
			t.Fatalf("child runner capability does not belong to assembly run: %#v", assembly.ChildRunner)
		}
		if !strings.Contains(assembly.Spec.Prompt, "Effective tools") || !strings.Contains(assembly.Spec.Prompt, "read_file") ||
			!strings.Contains(assembly.Spec.Prompt, "Agent: general-purpose") ||
			strings.Contains(assembly.Spec.Prompt, "delegate_task") || strings.Contains(assembly.Spec.Prompt, "write_file") {
			t.Fatalf("child prompt does not match effective capability snapshot:\n%s", assembly.Spec.Prompt)
		}
	}
	created := store.createOptions()
	if created.Source != session.SOURCESubagent || created.UserID != "subagent-of-"+string(parent.Snapshot().ID) ||
		created.ParentSessionID != parent.Snapshot().ID || created.ParentRunID != parentScope.Snapshot().RunID ||
		created.DelegationID != "delegate-1" || created.Agent != "general-purpose" {
		t.Fatalf("persisted child lineage = %#v", created)
	}
	if err := parent.FinishRun(parentScope); err != nil {
		t.Fatal(err)
	}
}

func TestChildRunnerPreservesExplicitEmptyParentCapabilitySnapshot(t *testing.T) {
	store := newLifecycleStore()
	var childAssemblies []RunAssembly
	harness, err := NewRuntimeHarness(store, successfulHarnessDependencies(&childAssemblies))
	if err != nil {
		t.Fatal(err)
	}
	parent, err := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	parentScope, err := parent.BeginRun(context.Background(), RunSpec{
		Prompt: "parent", WorkDir: "/workspace", AllowedTools: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	childRunner, err := parent.NewChildRunner(parentScope)
	if err != nil {
		t.Fatal(err)
	}
	result, err := childRunner.Run(context.Background(), ChildRunRequest{
		InvocationID: "invoke-empty-parent", Task: "inspect", ReadOnly: false, Depth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ChildSucceeded {
		t.Fatalf("child result = %#v", result)
	}
	for _, assembly := range childAssemblies {
		if assembly.AllowedTools == nil || len(assembly.AllowedTools) != 0 {
			t.Fatalf("child assembly tools = %#v, want explicit empty parent ceiling", assembly.AllowedTools)
		}
		if strings.Contains(assembly.Spec.Prompt, "read_file") || strings.Contains(assembly.Spec.Prompt, "bash") {
			t.Fatalf("child prompt expanded empty parent ceiling:\n%s", assembly.Spec.Prompt)
		}
	}
	if err := parent.FinishRun(parentScope); err != nil {
		t.Fatal(err)
	}
}

func TestChildRunnerFromFrozenParentDoesNotCreateShadowParentState(t *testing.T) {
	store := newLifecycleStore()
	var childAssemblies []RunAssembly
	harness, err := NewRuntimeHarness(store, successfulHarnessDependencies(&childAssemblies))
	if err != nil {
		t.Fatal(err)
	}
	allowed := []string{"delegate_task", "read_file", "write_file"}
	parent := FrozenParentRun{
		Profile: CLIExec, SessionID: "legacy-parent", RunID: "legacy-run",
		WorkDir: "/workspace", ProviderProtocol: "messages", Model: "parent-model",
		AllowedTools: allowed, Context: context.Background(),
	}
	runner, err := harness.NewChildRunnerFromFrozenParent(parent)
	if err != nil {
		t.Fatal(err)
	}
	allowed[1] = "write_file"
	parent.AllowedTools[2] = "bash"

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "legacy-invocation", DelegationID: "legacy-tool-call",
		Task: "inspect", ReadOnly: true, AllowedTools: []string{"read_file"}, Depth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ParentSessionID != "legacy-parent" || result.ParentRunID != "legacy-run" || result.Status != ChildSucceeded {
		t.Fatalf("bridged child result = %#v", result)
	}
	if store.sessionCount() != 1 || store.startCount() != 1 {
		t.Fatalf("bridge persisted sessions/runs = %d/%d, want only one child session/run", store.sessionCount(), store.startCount())
	}
	for _, assembly := range childAssemblies {
		if !reflect.DeepEqual(assembly.AllowedTools, []string{"read_file"}) || assembly.Run.Model != "parent-model" {
			t.Fatalf("bridged child assembly was not frozen: %#v", assembly)
		}
	}
}

func TestChildRunnerRejectsNilReceiverWithoutPanic(t *testing.T) {
	var runner *ChildRunner
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Run() panicked for nil child runner: %v", recovered)
		}
	}()
	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "nil-runner", Task: "inspect", Depth: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "runtime child runner is required") {
		t.Fatalf("Run() error = %v, want missing child runner error", err)
	}
	if result.Status != ChildRejected || result.SessionID != "" || result.RunID != "" {
		t.Fatalf("Run() result = %#v, want rejected result without child identity", result)
	}
}

func TestChildRunnerRejectsNestedDepthBeforeSessionOrCleanup(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	parent, _ := harness.CreateSession(context.Background(), ChildRun, SessionOptions{
		WorkDir: "/workspace", ParentSessionID: "root", ParentRunID: "root-run",
	})
	parentScope, err := parent.BeginRun(context.Background(), RunSpec{
		Prompt: "child", WorkDir: "/workspace", DelegationDepth: intPointer(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := parent.NewChildRunner(parentScope)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := &recordingChildCleanup{}
	sessionsBefore := store.sessionCount()

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "nested", Task: "create descendant", Depth: 2, Cleanup: cleanup,
	})
	if err == nil || result.Status != ChildRejected || result.SessionID != "" || result.RunID != "" {
		t.Fatalf("nested Run() = %#v, %v", result, err)
	}
	if result.ParentSessionID != parent.Snapshot().ID || result.ParentRunID != parentScope.Snapshot().RunID {
		t.Fatalf("nested rejection parent lineage = %s/%s, want %s/%s", result.ParentSessionID, result.ParentRunID, parent.Snapshot().ID, parentScope.Snapshot().RunID)
	}
	if store.sessionCount() != sessionsBefore || cleanup.calls != 0 {
		t.Fatalf("nested rejection sessions/cleanup = %d/%d, want %d/0", store.sessionCount(), cleanup.calls, sessionsBefore)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerRejectsParentProfileWithoutDelegationBeforeSession(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	parent, _ := harness.CreateSession(context.Background(), BenchmarkEval, SessionOptions{WorkDir: "/fixture"})
	parentScope, err := parent.BeginRun(context.Background(), RunSpec{Prompt: "benchmark", WorkDir: "/fixture"})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := parent.NewChildRunner(parentScope)
	if err != nil {
		t.Fatal(err)
	}
	sessionsBefore := store.sessionCount()

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "forbidden-profile", Task: "delegate", Depth: 1,
	})
	if err == nil || result.Status != ChildRejected || result.SessionID != "" || store.sessionCount() != sessionsBefore {
		t.Fatalf("forbidden profile Run() = %#v, %v; sessions=%d", result, err, store.sessionCount())
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerPropagatesCancellationAndCleansBeforeReturning(t *testing.T) {
	store := newLifecycleStore()
	modelStarted := make(chan struct{})
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		return runtimeModelInvokerFunc(func(ctx context.Context, _ engine.RunContext) (engine.ModelResult, error) {
			close(modelStarted)
			<-ctx.Done()
			return engine.ModelResult{}, ctx.Err()
		}), nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)
	cleanup := &recordingChildCleanup{}
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan ChildRunResult, 1)
	errs := make(chan error, 1)

	go func() {
		result, err := runner.Run(ctx, ChildRunRequest{
			InvocationID: "cancelled", Task: "wait", Depth: 1, Cleanup: cleanup,
		})
		returned <- result
		errs <- err
	}()
	<-modelStarted
	cancel()
	result := <-returned
	err := <-errs
	if !errors.Is(err, context.Canceled) || result.Status != ChildCancelled {
		t.Fatalf("cancelled Run() = %#v, %v", result, err)
	}
	if cleanup.calls != 1 || !cleanup.completed {
		t.Fatalf("cleanup state at return = %#v", cleanup)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerPreservesCancellationClassificationWhenCleanupFails(t *testing.T) {
	store := newLifecycleStore()
	modelStarted := make(chan struct{})
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		return runtimeModelInvokerFunc(func(ctx context.Context, _ engine.RunContext) (engine.ModelResult, error) {
			close(modelStarted)
			<-ctx.Done()
			return engine.ModelResult{}, ctx.Err()
		}), nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)
	cleanupErr := errors.New("process cleanup failed")
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan ChildRunResult, 1)
	errs := make(chan error, 1)

	go func() {
		result, err := runner.Run(ctx, ChildRunRequest{
			InvocationID: "cancelled-cleanup-failure", Task: "wait", Depth: 1,
			Cleanup: &recordingChildCleanup{err: cleanupErr},
		})
		returned <- result
		errs <- err
	}()
	<-modelStarted
	cancel()
	result := <-returned
	err := <-errs
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cleanupErr) {
		t.Fatalf("cancelled cleanup Run() error = %v, want cancellation joined with cleanup failure", err)
	}
	if result.Status != ChildCancelled {
		t.Fatalf("cancelled cleanup Run() status = %s, want %s", result.Status, ChildCancelled)
	}
	if result.Runtime.Outcome.ErrorKind != "provider" {
		t.Fatalf("cancelled cleanup runtime outcome = %#v, want original provider cause classification", result.Runtime.Outcome)
	}
	if !errors.Is(result.Runtime.Outcome.Err, context.Canceled) || !errors.Is(result.Runtime.Outcome.Err, cleanupErr) {
		t.Fatalf("cancelled cleanup runtime error = %v, want cancellation joined with cleanup evidence", result.Runtime.Outcome.Err)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerPropagatesParentScopeCancellation(t *testing.T) {
	store := newLifecycleStore()
	modelStarted := make(chan struct{})
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		return runtimeModelInvokerFunc(func(ctx context.Context, _ engine.RunContext) (engine.ModelResult, error) {
			close(modelStarted)
			<-ctx.Done()
			return engine.ModelResult{}, ctx.Err()
		}), nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)
	returned := make(chan ChildRunResult, 1)
	errs := make(chan error, 1)

	go func() {
		result, err := runner.Run(context.Background(), ChildRunRequest{
			InvocationID: "parent-cancelled", Task: "wait", Depth: 1,
		})
		returned <- result
		errs <- err
	}()
	<-modelStarted
	parentScope.Cancel()
	result := <-returned
	err := <-errs
	if !errors.Is(err, context.Canceled) || result.Status != ChildCancelled {
		t.Fatalf("parent-cancelled Run() = %#v, %v", result, err)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerDerivesPermissionFromFrozenParentAfterChildIdentityExists(t *testing.T) {
	store := newLifecycleStore()
	parentPermission := &recordingPermissionScope{}
	childPermission := &recordingPermissionScope{leaf: true}
	parentPermission.child = childPermission
	var assemblies []RunAssembly
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(&assemblies))
	parent, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	parentScope, err := parent.BeginRun(context.Background(), RunSpec{
		Prompt: "parent", WorkDir: "/workspace", Permission: parentPermission,
		AllowedTools: []string{"read_file", "write_file", "delegate_task"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner, _ := parent.NewChildRunner(parentScope)

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "permission-child", DelegationID: "delegate-permission",
		Agent: "general-purpose", Task: "inspect", Depth: 1, ReadOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := parentPermission.snapshot()
	if request.ParentSessionID != parent.Snapshot().ID || request.ParentRunID != parentScope.Snapshot().RunID ||
		request.ChildSessionID != result.SessionID || request.ChildRunID != result.RunID ||
		request.DelegationID != "delegate-permission" || request.Agent != "general-purpose" || !request.ReadOnly {
		t.Fatalf("child permission request = %#v", request)
	}
	for _, assembly := range assemblies {
		if assembly.Permission != childPermission {
			t.Fatalf("child assembly permission = %#v, want derived child scope", assembly.Permission)
		}
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerReadOnlyPermissionAndPromptUseFinalCapabilitySnapshot(t *testing.T) {
	store := newLifecycleStore()
	parentPermission := &recordingPermissionScope{}
	childPermission := &recordingPermissionScope{leaf: true}
	parentPermission.child = childPermission
	var assemblies []RunAssembly
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(&assemblies))
	parent, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	parentScope, err := parent.BeginRun(context.Background(), RunSpec{
		Prompt: "parent", WorkDir: "/workspace", Permission: parentPermission,
		AllowedTools: []string{"delegate_task", "read_file", "write_file"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner, _ := parent.NewChildRunner(parentScope)

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "readonly-child", DelegationID: "delegate-readonly",
		Agent: "general-purpose", Task: "inspect", Depth: 1, ReadOnly: true,
		AllowedTools:      []string{"read_file", "write_file"},
		AgentAllowedTools: []string{"read_file", "write_file"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ChildSucceeded {
		t.Fatalf("child result = %#v", result)
	}
	request := parentPermission.snapshot()
	if !reflect.DeepEqual(request.AllowedTools, []string{"read_file"}) {
		t.Fatalf("permission allowed tools = %v, want final read-only snapshot [read_file]", request.AllowedTools)
	}
	for _, assembly := range assemblies {
		if !reflect.DeepEqual(assembly.AllowedTools, []string{"read_file"}) {
			t.Fatalf("child assembly tools = %v, want [read_file]", assembly.AllowedTools)
		}
		if strings.Contains(assembly.Spec.Prompt, "write_file") || !strings.Contains(assembly.Spec.Prompt, "Effective tools") ||
			!strings.Contains(assembly.Spec.Prompt, "read_file") {
			t.Fatalf("child prompt does not match final read-only capability snapshot:\n%s", assembly.Spec.Prompt)
		}
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerFailsClosedWithoutRequiredParentPermission(t *testing.T) {
	store := newLifecycleStore()
	modelCalls := 0
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		modelCalls++
		return runtimeModelInvokerFunc(func(context.Context, engine.RunContext) (engine.ModelResult, error) {
			return completedModelResult(), nil
		}), nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	parent, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{
		Prompt: "parent", WorkDir: "/workspace", AllowedTools: []string{"delegate_task", "read_file"},
	})
	runner, _ := parent.NewChildRunner(parentScope)

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "missing-permission", Task: "inspect", Depth: 1,
	})
	if err == nil || result.Status != ChildFailed || result.Runtime.Outcome.ErrorKind != "permission" {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
	if modelCalls != 0 {
		t.Fatalf("model calls = %d, want permission rejection before model assembly", modelCalls)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerDoesNotReturnUncommittedAssistantAsPartialReport(t *testing.T) {
	store := newLifecycleStore()
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		return runtimeModelInvokerFunc(func(context.Context, engine.RunContext) (engine.ModelResult, error) {
			store.failNextMessage(errors.New("assistant commit failed"))
			return completedModelResult(), nil
		}), nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "uncommitted", Task: "produce report", Depth: 1,
	})
	if err == nil || result.Status != ChildFailed {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
	if result.Report != "" {
		t.Fatalf("partial report = %q, want no uncommitted assistant content", result.Report)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerReturnsLatestCommittedAssistantAsFailedPartialReport(t *testing.T) {
	store := newLifecycleStore()
	providerErr := errors.New("provider failed after tool result")
	invocations := 0
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewTools = func(context.Context, RunAssembly) (engine.ToolExecutor, error) {
		return artifactToolExecutor{}, nil
	}
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		return runtimeModelInvokerFunc(func(context.Context, engine.RunContext) (engine.ModelResult, error) {
			invocations++
			if invocations == 1 {
				return engine.ModelResult{Message: schema.Message{
					Role: schema.RoleAssistant, Content: "committed partial",
					ToolCalls: []schema.ToolCall{{ID: "call-1", Name: "read_file", Arguments: []byte(`{"path":"README.md"}`)}},
				}, FinishReason: "tool_calls"}, nil
			}
			return engine.ModelResult{}, providerErr
		}), nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "committed-partial", Task: "inspect", Depth: 1,
	})
	if !errors.Is(err, providerErr) || result.Status != ChildFailed || result.Report != "committed partial" {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerClassifiesTurnExhaustionAndRetainsCommittedPartial(t *testing.T) {
	store := newLifecycleStore()
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewTools = func(context.Context, RunAssembly) (engine.ToolExecutor, error) {
		return artifactToolExecutor{}, nil
	}
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		return runtimeModelInvokerFunc(func(context.Context, engine.RunContext) (engine.ModelResult, error) {
			return engine.ModelResult{Message: schema.Message{
				Role: schema.RoleAssistant, Content: "turn-limited partial",
				ToolCalls: []schema.ToolCall{{ID: "call-1", Name: "read_file", Arguments: []byte(`{"path":"README.md"}`)}},
			}, FinishReason: "tool_calls"}, nil
		}), nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)
	maxTurns := 1

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "turn-limit", Task: "inspect", Depth: 1, MaxTurns: &maxTurns,
	})
	if err == nil || result.Status != ChildTurnExhausted || result.Report != "turn-limited partial" ||
		result.Runtime.Outcome.ErrorKind != "turn_limit" || !result.Runtime.Outcome.Partial {
		t.Fatalf("turn-limited Run() = %#v, %v", result, err)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerPanicFinishesRunAndCleansExactlyOnce(t *testing.T) {
	store := newLifecycleStore()
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		return runtimeModelInvokerFunc(func(context.Context, engine.RunContext) (engine.ModelResult, error) {
			panic("provider panic")
		}), nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)
	cleanup := &recordingChildCleanup{}

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "panic", Task: "panic", Depth: 1, Cleanup: cleanup,
	})
	if err == nil || !strings.Contains(err.Error(), "provider panic") || result.Status != ChildFailed || result.RunID == "" {
		t.Fatalf("panic Run() = %#v, %v", result, err)
	}
	if cleanup.calls != 1 || store.finishCount() != 1 {
		t.Fatalf("panic cleanup/finish = %d/%d, want 1/1", cleanup.calls, store.finishCount())
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerTreatsTypedNilCleanupAsAbsent(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)
	var cleanup *recordingChildCleanup

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "typed-nil-cleanup", Task: "finish", Depth: 1, Cleanup: cleanup,
	})
	if err != nil || result.Status != ChildSucceeded || result.Report != "done" {
		t.Fatalf("typed-nil cleanup Run() = %#v, %v; want successful child with absent cleanup", result, err)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerCleanupFailureOverridesSuccessWithoutDiscardingReport(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)
	cleanupErr := errors.New("process cleanup failed")
	cleanup := &recordingChildCleanup{err: cleanupErr}

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "cleanup-failure", Task: "finish", Depth: 1, Cleanup: cleanup,
	})
	if !errors.Is(err, cleanupErr) || result.Status != ChildFailed || result.Report != "done" {
		t.Fatalf("cleanup Run() = %#v, %v", result, err)
	}
	if result.Runtime.Outcome.ErrorKind != "cleanup" || !errors.Is(result.Runtime.Outcome.Err, cleanupErr) {
		t.Fatalf("cleanup runtime outcome = %#v, want matching failed terminal state", result.Runtime.Outcome)
	}
	if cleanup.calls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanup.calls)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerCleanupPanicBecomesFailureAndStillClosesSession(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "cleanup-panic", Task: "finish", Depth: 1,
		Cleanup: ChildCleanupFunc(func(context.Context) error { panic("cleanup panic") }),
	})
	if err == nil || !strings.Contains(err.Error(), "cleanup panic") || result.Status != ChildFailed {
		t.Fatalf("cleanup panic Run() = %#v, %v", result, err)
	}
	if store.finishCount() != 1 {
		t.Fatalf("child finish attempts = %d, want 1 after cleanup panic", store.finishCount())
	}
	reopened, openErr := harness.OpenSession(context.Background(), ChildRun, result.SessionID)
	if openErr != nil {
		t.Fatalf("child session lease survived cleanup panic: %v", openErr)
	}
	if err := reopened.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = parent.FinishRun(parentScope)
}

func TestChildRunnerRecoversHiddenFinishBeforeClosingSession(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	parent, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	parentScope, _ := parent.BeginRun(context.Background(), RunSpec{Prompt: "parent", WorkDir: "/workspace"})
	runner, _ := parent.NewChildRunner(parentScope)
	finishErr := errors.New("transient child finish failure")
	store.failNextFinish(finishErr)

	result, err := runner.Run(context.Background(), ChildRunRequest{
		InvocationID: "finish-recovery", Task: "finish", Depth: 1,
	})
	/* The failed terminal write never fails the child run; the hidden
	 * recovery step still repairs the durable finish before the close. */
	if err != nil || result.Status != ChildSucceeded || result.RunID == "" {
		t.Fatalf("finish recovery Run() = %#v, %v", result, err)
	}
	if store.finishCount() != 2 {
		t.Fatalf("child finish attempts = %d, want initial failure plus hidden recovery", store.finishCount())
	}
	reopened, openErr := harness.OpenSession(context.Background(), ChildRun, result.SessionID)
	if openErr != nil {
		t.Fatalf("reopen recovered child: %v", openErr)
	}
	if err := reopened.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = parent.FinishRun(parentScope)
}

type recordingChildCleanup struct {
	mu        sync.Mutex
	calls     int
	completed bool
	err       error
}

type ChildCleanupFunc func(context.Context) error

func (f ChildCleanupFunc) Cleanup(ctx context.Context) error { return f(ctx) }

type recordingPermissionScope struct {
	mu      sync.Mutex
	request ChildPermissionRequest
	child   PermissionScope
	leaf    bool
}

func (s *recordingPermissionScope) ChildScope(_ context.Context, request ChildPermissionRequest) (PermissionScope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.leaf {
		return nil, errors.New("leaf permission cannot create a descendant")
	}
	s.request = request
	return s.child, nil
}

func (s *recordingPermissionScope) snapshot() ChildPermissionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.request
}

func (c *recordingChildCleanup) Cleanup(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	c.completed = true
	return c.err
}
