package toolexec

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestTL002ParallelCapabilitiesOverlapAndReturnModelOrder(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	executor := New([]Capability{
		barrierCapability("parallel_a", true, "A", started, release),
		barrierCapability("parallel_b", true, "B", started, release),
	})
	snapshot, err := executor.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	done := executeTargetBatch(executor, snapshot, []schema.ToolCall{
		{ID: "call-a", Name: "parallel_a", Arguments: json.RawMessage(`{}`)},
		{ID: "call-b", Name: "parallel_b", Arguments: json.RawMessage(`{}`)},
	})

	gotStarted := map[string]bool{awaitTargetSignal(t, started): true, awaitTargetSignal(t, started): true}
	if !gotStarted["parallel_a"] || !gotStarted["parallel_b"] {
		t.Fatalf("parallel starts = %#v", gotStarted)
	}
	close(release)
	batch := awaitTargetBatch(t, done)
	if got := targetResultIDs(batch); fmt.Sprint(got) != fmt.Sprint([]string{"call-a", "call-b"}) {
		t.Fatalf("result IDs = %v", got)
	}
}

func TestTL003ExclusiveCapabilitiesSeparateParallelBatches(t *testing.T) {
	started := make(chan string, 5)
	firstRelease := make(chan struct{})
	exclusiveRelease := make(chan struct{})
	secondRelease := make(chan struct{})
	executor := New([]Capability{
		barrierCapability("parallel_a", true, "A", started, firstRelease),
		barrierCapability("parallel_b", true, "B", started, firstRelease),
		barrierCapability("exclusive", false, "X", started, exclusiveRelease),
		barrierCapability("parallel_c", true, "C", started, secondRelease),
		barrierCapability("parallel_d", true, "D", started, secondRelease),
	})
	snapshot, _ := executor.Snapshot(context.Background())
	calls := make([]schema.ToolCall, 0, 5)
	for _, name := range []string{"parallel_a", "parallel_b", "exclusive", "parallel_c", "parallel_d"} {
		calls = append(calls, schema.ToolCall{ID: "call-" + name, Name: name, Arguments: json.RawMessage(`{}`)})
	}
	done := executeTargetBatch(executor, snapshot, calls)

	first := map[string]bool{awaitTargetSignal(t, started): true, awaitTargetSignal(t, started): true}
	if !first["parallel_a"] || !first["parallel_b"] {
		t.Fatalf("first batch = %#v", first)
	}
	assertNoTargetSignal(t, started)
	close(firstRelease)
	if got := awaitTargetSignal(t, started); got != "exclusive" {
		t.Fatalf("exclusive start = %q", got)
	}
	assertNoTargetSignal(t, started)
	close(exclusiveRelease)
	second := map[string]bool{awaitTargetSignal(t, started): true, awaitTargetSignal(t, started): true}
	if !second["parallel_c"] || !second["parallel_d"] {
		t.Fatalf("second batch = %#v", second)
	}
	close(secondRelease)
	if got := targetResultIDs(awaitTargetBatch(t, done)); fmt.Sprint(got) != fmt.Sprint([]string{
		"call-parallel_a", "call-parallel_b", "call-exclusive", "call-parallel_c", "call-parallel_d",
	}) {
		t.Fatalf("result IDs = %v", got)
	}
}

func TestTL004TL005AndTL006SnapshotFailuresAliasesAndResultForms(t *testing.T) {
	executor := New([]Capability{
		{
			Definition:   schema.ToolDefinition{Name: "Observe", Description: "alias", InputSchema: map[string]any{"type": "object"}},
			ParallelSafe: true,
			Execute: func(_ context.Context, call schema.ToolCall) engine.ToolExecutionResult {
				if string(call.Arguments) != `{"valid":true}` {
					return engine.ToolExecutionResult{CallID: call.ID, ModelContent: "invalid arguments", ObserverContent: "invalid arguments", IsError: true}
				}
				return engine.ToolExecutionResult{
					CallID: call.ID, FullContent: "full artifact", ModelContent: "model preview",
					ObserverContent: "reporter preview", ArtifactPath: "/fixture/call-alias.txt",
				}
			},
		},
	})
	snapshot, _ := executor.Snapshot(context.Background())
	definitions := snapshot.ToolDefinitions()
	definitions[0].Name = "mutated"
	if got := snapshot.ToolDefinitions()[0].Name; got != "Observe" {
		t.Fatalf("snapshot definition mutated: %q", got)
	}
	batch, err := executor.Execute(context.Background(), snapshot, []schema.ToolCall{
		{ID: "call-alias", Name: "Observe", Arguments: json.RawMessage(`{"valid":true}`)},
		{ID: "call-invalid", Name: "Observe", Arguments: json.RawMessage(`{}`)},
		{ID: "call-unknown", Name: "missing", Arguments: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Results) != 3 || batch.Results[0].ArtifactPath != "/fixture/call-alias.txt" || batch.Results[0].ModelContent != "model preview" || batch.Results[0].ObserverContent != "reporter preview" || batch.Results[0].FullContent != "full artifact" {
		t.Fatalf("structured results = %#v", batch.Results)
	}
	if !batch.Results[1].IsError || !batch.Results[2].IsError || batch.Results[2].CallID != "call-unknown" {
		t.Fatalf("failure results = %#v", batch.Results[1:])
	}
}

func TestTL007CancellationTerminatesConfirmedCallsInOrder(t *testing.T) {
	started := make(chan string, 2)
	finished := make(chan string, 2)
	executor := New([]Capability{
		cancelCapability("cancel_a", started, finished),
		cancelCapability("cancel_b", started, finished),
	})
	snapshot, _ := executor.Snapshot(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	done := executeTargetBatchContext(ctx, executor, snapshot, []schema.ToolCall{
		{ID: "call-a", Name: "cancel_a", Arguments: json.RawMessage(`{}`)},
		{ID: "call-b", Name: "cancel_b", Arguments: json.RawMessage(`{}`)},
	})
	awaitTargetSignal(t, started)
	awaitTargetSignal(t, started)
	cancel()
	awaitTargetSignal(t, finished)
	awaitTargetSignal(t, finished)
	batch := awaitTargetBatch(t, done)
	if got := targetResultIDs(batch); fmt.Sprint(got) != fmt.Sprint([]string{"call-a", "call-b"}) {
		t.Fatalf("cancel result order = %v", got)
	}
	for _, result := range batch.Results {
		if !result.IsError || result.ModelContent != context.Canceled.Error() {
			t.Fatalf("cancel result = %#v", result)
		}
	}
}

func barrierCapability(name string, parallel bool, output string, started chan<- string, release <-chan struct{}) Capability {
	return Capability{
		Definition: schema.ToolDefinition{Name: name, InputSchema: map[string]any{"type": "object"}}, ParallelSafe: parallel,
		Execute: func(ctx context.Context, call schema.ToolCall) engine.ToolExecutionResult {
			started <- name
			select {
			case <-release:
				return engine.ToolExecutionResult{CallID: call.ID, FullContent: output, ModelContent: output, ObserverContent: output}
			case <-ctx.Done():
				return engine.ToolExecutionResult{CallID: call.ID, FullContent: ctx.Err().Error(), ModelContent: ctx.Err().Error(), ObserverContent: ctx.Err().Error(), IsError: true}
			}
		},
	}
}

func cancelCapability(name string, started, finished chan<- string) Capability {
	capability := barrierCapability(name, true, "", started, nil)
	capability.Execute = func(ctx context.Context, call schema.ToolCall) engine.ToolExecutionResult {
		started <- name
		<-ctx.Done()
		finished <- name
		return engine.ToolExecutionResult{CallID: call.ID, FullContent: ctx.Err().Error(), ModelContent: ctx.Err().Error(), ObserverContent: ctx.Err().Error(), IsError: true}
	}
	return capability
}

type targetBatchOutcome struct {
	batch engine.ToolBatch
	err   error
}

func executeTargetBatch(executor *Executor, snapshot engine.ToolSnapshot, calls []schema.ToolCall) <-chan targetBatchOutcome {
	return executeTargetBatchContext(context.Background(), executor, snapshot, calls)
}

func executeTargetBatchContext(ctx context.Context, executor *Executor, snapshot engine.ToolSnapshot, calls []schema.ToolCall) <-chan targetBatchOutcome {
	done := make(chan targetBatchOutcome, 1)
	go func() {
		batch, err := executor.Execute(ctx, snapshot, calls)
		done <- targetBatchOutcome{batch: batch, err: err}
	}()
	return done
}

func awaitTargetBatch(t *testing.T, done <-chan targetBatchOutcome) engine.ToolBatch {
	t.Helper()
	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatalf("Execute() error = %v", outcome.err)
		}
		return outcome.batch
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for target tool batch")
		return engine.ToolBatch{}
	}
}

func awaitTargetSignal(t *testing.T, signals <-chan string) string {
	t.Helper()
	select {
	case signal := <-signals:
		return signal
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for target tool signal")
		return ""
	}
}

func assertNoTargetSignal(t *testing.T, signals <-chan string) {
	t.Helper()
	select {
	case signal := <-signals:
		t.Fatalf("unexpected tool start %q", signal)
	default:
	}
}

func targetResultIDs(batch engine.ToolBatch) []string {
	result := make([]string, 0, len(batch.Results))
	for _, item := range batch.Results {
		result = append(result, item.CallID)
	}
	return result
}
