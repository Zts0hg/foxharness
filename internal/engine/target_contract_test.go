package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/testsupport/runtimecontract"
)

func TestTargetContractAdapterRunsM04Scenarios(t *testing.T) {
	for _, testCase := range []runtimeTurnTestCase{
		{name: "RT-001 tool-free completion", scenario: runtimeTurnToolFreeScenario()},
		{name: "RT-007 provider error", scenario: runtimeTurnProviderErrorScenario()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			adapter := newTargetContractAdapter(t)
			if err := runtimecontract.VerifyScenario(context.Background(), adapter, testCase.scenario); err != nil {
				t.Fatalf("VerifyScenario() error = %v", err)
			}
		})
	}
}

func TestTargetContractAdapterRunsRuntimeTurnCatalog(t *testing.T) {
	for _, testCase := range currentRuntimeTurnScenarios() {
		t.Run(testCase.name, func(t *testing.T) {
			if err := runtimecontract.VerifyScenario(context.Background(), newTargetContractAdapter(t), testCase.scenario); err != nil {
				t.Fatalf("VerifyScenario() error = %v", err)
			}
		})
	}
}

func TestTargetContractAdapterRunsStreamingCatalog(t *testing.T) {
	for _, testCase := range currentStreamingScenarios() {
		t.Run(testCase.name, func(t *testing.T) {
			if err := runtimecontract.VerifyScenario(context.Background(), newTargetContractAdapter(t), testCase.scenario); err != nil {
				t.Fatalf("VerifyScenario() error = %v", err)
			}
		})
	}
}

func TestTargetObserverIsSynchronousAndCanonicallyOrdered(t *testing.T) {
	var observed []Fact
	invoker := modelInvokerFunc(func(context.Context, RunContext) (ModelResult, error) {
		if len(observed) != 1 || observed[0].Kind != FactRunStarted || observed[0].Sequence != 1 {
			t.Fatalf("facts before Invoke() = %#v, want synchronous run_started sequence 1", observed)
		}
		return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"}, nil
	})
	conversation := &targetTestConversation{}
	observer := observerFunc(func(_ context.Context, fact Fact) {
		observed = append(observed, fact)
	})
	eng := NewAgentEngine(invoker, targetTestToolExecutor{}, conversation, targetTestPolicy{}, observer)

	if _, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 1}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantKinds := []FactKind{FactRunStarted, FactMessage, FactRunCompleted}
	if len(observed) != len(wantKinds) {
		t.Fatalf("facts = %#v, want %d facts", observed, len(wantKinds))
	}
	for i, wantKind := range wantKinds {
		if observed[i].Kind != wantKind || observed[i].Sequence != i+1 {
			t.Fatalf("fact[%d] = %#v, want kind %q sequence %d", i, observed[i], wantKind, i+1)
		}
	}
}

func TestTargetRunContextIsImmutableAcrossModelInvocation(t *testing.T) {
	arguments := json.RawMessage(`{"path":"original"}`)
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
	}
	sourceContext := RunContext{
		Messages: []schema.Message{{
			Role:      schema.RoleAssistant,
			Content:   "original",
			ToolCalls: []schema.ToolCall{{ID: "call-1", Name: "inspect", Arguments: arguments}},
		}},
	}
	sourceSnapshot := &targetToolSnapshot{definitions: []schema.ToolDefinition{{Name: "inspect", InputSchema: inputSchema}}}
	conversation := &immutableTargetConversation{runContext: sourceContext}
	toolExecutor := &immutableTargetToolExecutor{snapshot: sourceSnapshot}
	invoker := modelInvokerFunc(func(_ context.Context, runContext RunContext) (ModelResult, error) {
		runContext.Messages[0].Content = "mutated"
		runContext.Messages[0].ToolCalls[0].Arguments[0] = 'X'
		runContext.ToolDefinitions[0].Name = "mutated"
		properties := runContext.ToolDefinitions[0].InputSchema.(map[string]any)["properties"].(map[string]any)
		properties["path"].(map[string]any)["type"] = "number"
		return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"}, nil
	})
	eng := NewAgentEngine(invoker, toolExecutor, conversation, targetTestPolicy{}, nil)

	if _, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 1}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if sourceContext.Messages[0].Content != "original" || string(sourceContext.Messages[0].ToolCalls[0].Arguments) != `{"path":"original"}` {
		t.Fatalf("source messages mutated through RunContext: %#v", sourceContext.Messages)
	}
	if sourceSnapshot.definitions[0].Name != "inspect" {
		t.Fatalf("source tool definition name = %q, want inspect", sourceSnapshot.definitions[0].Name)
	}
	properties := sourceSnapshot.definitions[0].InputSchema.(map[string]any)["properties"].(map[string]any)
	if got := properties["path"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("source nested schema type = %v, want string", got)
	}
}

func TestTargetConversationReceivesExactInvocationSnapshot(t *testing.T) {
	conversation := &invocationRecordingConversation{}
	executor := &immutableTargetToolExecutor{snapshot: targetToolSnapshot{definitions: []schema.ToolDefinition{{Name: "inspect"}}}}
	invoker := modelInvokerFunc(func(context.Context, RunContext) (ModelResult, error) {
		return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"}, nil
	})
	eng := NewAgentEngine(invoker, executor, conversation, targetTestPolicy{}, nil)

	input := RunInput{Prompt: "work", MaxTurns: 1}
	if _, err := eng.Run(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if len(conversation.requests) != 1 {
		t.Fatalf("conversation requests = %#v", conversation.requests)
	}
	got := conversation.requests[0]
	if got.Input != input || got.Turn != 1 || got.Phase != PhaseAction || got.Preparation != ConversationPrepareNormal {
		t.Fatalf("conversation request = %#v", got)
	}
	if len(got.ToolDefinitions) != 1 || got.ToolDefinitions[0].Name != "inspect" {
		t.Fatalf("conversation tool snapshot = %#v", got.ToolDefinitions)
	}
}

func TestTargetToolSnapshotIsFrozenOncePerTurn(t *testing.T) {
	executor := &perTurnTargetToolExecutor{snapshots: []targetToolSnapshot{
		{definitions: []schema.ToolDefinition{{Name: "first"}}},
		{definitions: []schema.ToolDefinition{{Name: "second"}}},
	}}
	invocations := 0
	invoker := modelInvokerFunc(func(_ context.Context, runContext RunContext) (ModelResult, error) {
		invocations++
		if len(runContext.ToolDefinitions) != 1 {
			t.Fatalf("turn %d definitions = %#v", invocations, runContext.ToolDefinitions)
		}
		if invocations == 1 {
			if runContext.ToolDefinitions[0].Name != "first" {
				t.Fatalf("first turn definition = %q", runContext.ToolDefinitions[0].Name)
			}
			return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "call-1", Name: "first"}}}}, nil
		}
		if runContext.ToolDefinitions[0].Name != "second" {
			t.Fatalf("second turn definition = %q", runContext.ToolDefinitions[0].Name)
		}
		return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"}, nil
	})
	eng := NewAgentEngine(invoker, executor, &targetTestConversation{}, targetTestPolicy{}, nil)

	if _, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 2}); err != nil {
		t.Fatal(err)
	}
	if executor.snapshotCalls != 2 {
		t.Fatalf("snapshot calls = %d, want one per turn", executor.snapshotCalls)
	}
	if !reflect.DeepEqual(executor.executedWith, []string{"first"}) {
		t.Fatalf("executed snapshots = %v, want first-turn snapshot", executor.executedWith)
	}
}

func TestTargetPromptTooLongUsesOneReactiveConversationRetry(t *testing.T) {
	conversation := &compactingInvocationConversation{}
	invocations := 0
	invoker := modelInvokerFunc(func(context.Context, RunContext) (ModelResult, error) {
		invocations++
		if invocations == 1 {
			return ModelResult{}, ErrPromptTooLong
		}
		return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"}, nil
	})
	var facts []Fact
	eng := NewAgentEngine(invoker, targetTestToolExecutor{}, conversation, targetTestPolicy{}, observerFunc(func(_ context.Context, fact Fact) {
		facts = append(facts, fact)
	}))

	if _, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 1}); err != nil {
		t.Fatal(err)
	}
	if invocations != 2 || len(conversation.requests) != 2 {
		t.Fatalf("invocations/requests = %d/%d, want one retry", invocations, len(conversation.requests))
	}
	if conversation.requests[0].Preparation != ConversationPrepareNormal || conversation.requests[1].Preparation != ConversationPrepareReactive {
		t.Fatalf("preparation sequence = %#v", conversation.requests)
	}
	if got := factKinds(facts); !reflect.DeepEqual(got, []FactKind{FactRunStarted, FactContextCompacted, FactMessage, FactRunCompleted}) {
		t.Fatalf("reactive fact order = %v", got)
	}
	if facts[1].Name != "reactive" || facts[1].Content != "" {
		t.Fatalf("reactive compaction fact = %#v", facts[1])
	}
}

func TestTargetPromptTooLongDoesNotRetryWithoutChangedProjection(t *testing.T) {
	conversation := &invocationRecordingConversation{}
	invocations := 0
	invoker := modelInvokerFunc(func(context.Context, RunContext) (ModelResult, error) {
		invocations++
		return ModelResult{}, ErrPromptTooLong
	})
	eng := NewAgentEngine(invoker, targetTestToolExecutor{}, conversation, targetTestPolicy{}, nil)

	_, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 1})
	if err == nil || !errors.Is(err, ErrPromptTooLong) {
		t.Fatalf("Run() error = %v, want original prompt-too-long error", err)
	}
	if invocations != 1 || len(conversation.requests) != 2 {
		t.Fatalf("invocations/requests = %d/%d, want one invocation and one recovery proposal request", invocations, len(conversation.requests))
	}
}

func TestTargetThinkingUsesTurnSnapshotForBudgetButHidesToolsFromModel(t *testing.T) {
	conversation := &invocationRecordingConversation{}
	executor := &perTurnTargetToolExecutor{snapshots: []targetToolSnapshot{{definitions: []schema.ToolDefinition{{Name: "inspect"}}}}}
	phases := make([]RunContext, 0, 2)
	invoker := modelInvokerFunc(func(_ context.Context, runContext RunContext) (ModelResult, error) {
		phases = append(phases, runContext)
		if runContext.Phase == PhaseThinking {
			return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "think"}}, nil
		}
		return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"}, nil
	})
	eng := NewAgentEngine(invoker, executor, conversation, targetTestPolicy{}, nil)

	if _, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 1, Thinking: true}); err != nil {
		t.Fatal(err)
	}
	if executor.snapshotCalls != 1 || len(conversation.requests) != 2 || len(phases) != 2 {
		t.Fatalf("snapshots/requests/phases = %d/%d/%d", executor.snapshotCalls, len(conversation.requests), len(phases))
	}
	for index, request := range conversation.requests {
		if len(request.ToolDefinitions) != 1 || request.ToolDefinitions[0].Name != "inspect" {
			t.Fatalf("conversation request %d budget definitions = %#v", index, request.ToolDefinitions)
		}
	}
	if len(phases[0].ToolDefinitions) != 0 || len(phases[1].ToolDefinitions) != 1 || phases[1].ToolDefinitions[0].Name != "inspect" {
		t.Fatalf("model-visible definitions = thinking:%#v action:%#v", phases[0].ToolDefinitions, phases[1].ToolDefinitions)
	}
}

func TestTargetStreamingFailureStateDoesNotLeakAcrossRuns(t *testing.T) {
	invoker := &targetScriptedInvoker{
		streaming: true,
		steps: []runtimecontract.ModelStep{
			{Deltas: []string{"partial"}, Error: "stream interrupted", ErrorKind: "ordinary"},
			{Deltas: []string{"clean"}, Response: runtimecontract.ModelResponse{Content: "clean", FinishReason: "stop"}},
		},
	}
	observer := &targetTestObserver{}
	eng := NewAgentEngine(invoker, targetTestToolExecutor{}, &immutableTargetConversation{}, targetTestPolicy{}, observer)

	if _, err := eng.Run(context.Background(), RunInput{Prompt: "first", MaxTurns: 1}); err == nil {
		t.Fatal("first Run() error = nil, want streaming failure")
	}
	outcome, err := eng.Run(context.Background(), RunInput{Prompt: "second", MaxTurns: 1})
	if err != nil || outcome.FinalMessage != "clean" {
		t.Fatalf("second Run() outcome/error = %#v, %v", outcome, err)
	}
	if len(invoker.requests) != 2 || invoker.requests[0].Transport != "stream" || invoker.requests[1].Transport != "stream" {
		t.Fatalf("cross-run requests = %#v, want two independent stream starts", invoker.requests)
	}
	want := []runtimecontract.Fact{
		{Kind: "run_started", Sequence: 1},
		{Kind: "message_delta", Sequence: 2, Content: "partial"},
		{Kind: "run_error", Sequence: 3, Content: "模型生成失败: stream interrupted", IsError: true},
		{Kind: "run_started", Sequence: 1},
		{Kind: "message_delta", Sequence: 2, Content: "clean"},
		{Kind: "message", Sequence: 3, Content: "clean"},
		{Kind: "run_completed", Sequence: 4, Content: "clean"},
	}
	if fmt.Sprint(observer.facts) != fmt.Sprint(want) {
		t.Fatalf("cross-run facts = %#v, want %#v", observer.facts, want)
	}
}

func TestTargetConversationCannotMutatePendingToolCalls(t *testing.T) {
	call := schema.ToolCall{ID: "call-1", Name: "inspect", Arguments: json.RawMessage(`{"path":"original"}`)}
	invoker := modelInvokerFunc(func(context.Context, RunContext) (ModelResult, error) {
		return ModelResult{
			Message:      schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{call}},
			FinishReason: "tool_calls",
		}, nil
	})
	conversation := &proposalMutatingConversation{}
	executor := &recordingTargetToolExecutor{}
	eng := NewAgentEngine(invoker, executor, conversation, targetTestPolicy{}, nil)

	if _, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 1}); err == nil {
		t.Fatal("Run() error = nil, want turn limit after the controlled tool round")
	}
	if len(executor.calls) != 1 || executor.calls[0].Name != "inspect" || string(executor.calls[0].Arguments) != `{"path":"original"}` {
		t.Fatalf("executed calls = %#v, want original immutable call", executor.calls)
	}
}

func TestTL006TargetEngineKeepsDistinctToolResultForms(t *testing.T) {
	call := schema.ToolCall{ID: "call-large", Name: "large", Arguments: json.RawMessage(`{}`)}
	invocations := 0
	var secondContext RunContext
	invoker := modelInvokerFunc(func(_ context.Context, runContext RunContext) (ModelResult, error) {
		invocations++
		if invocations == 1 {
			return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{call}}, FinishReason: "tool_calls"}, nil
		}
		secondContext = cloneRunContext(runContext)
		return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"}, nil
	})
	conversation := &targetTestConversation{}
	executor := &distinctResultToolExecutor{}
	var facts []Fact
	eng := NewAgentEngine(invoker, executor, conversation, targetTestPolicy{}, observerFunc(func(_ context.Context, fact Fact) {
		facts = append(facts, fact)
	}))

	if _, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 2}); err != nil {
		t.Fatal(err)
	}
	var resultFact Fact
	for _, fact := range facts {
		if fact.Kind == FactToolResult {
			resultFact = fact
		}
	}
	if resultFact.Content != "reporter preview" || resultFact.FullContent != "full artifact" || resultFact.ArtifactPath != "/fixture/call-large.txt" {
		t.Fatalf("tool result fact = %#v", resultFact)
	}
	if len(secondContext.Messages) == 0 || secondContext.Messages[len(secondContext.Messages)-1].Content != "model preview" {
		t.Fatalf("second context = %#v, want model preview", secondContext.Messages)
	}
}

func TestTL008TargetToolResultsPrecedeNextTurnInjections(t *testing.T) {
	call := schema.ToolCall{ID: "call-observe", Name: "observe", Arguments: json.RawMessage(`{}`)}
	invocations := 0
	var secondContext RunContext
	invoker := modelInvokerFunc(func(_ context.Context, runContext RunContext) (ModelResult, error) {
		invocations++
		if invocations == 1 {
			return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{call}}, FinishReason: "tool_calls"}, nil
		}
		secondContext = cloneRunContext(runContext)
		return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"}, nil
	})
	conversation := &injectionOrderingConversation{}
	eng := NewAgentEngine(invoker, &recordingTargetToolExecutor{}, conversation, targetTestPolicy{}, nil)

	if _, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 2}); err != nil {
		t.Fatal(err)
	}
	if len(secondContext.Messages) != 5 {
		t.Fatalf("second context = %#v, want tool call, result, then two injections", secondContext.Messages)
	}
	if secondContext.Messages[2].ToolCallID != call.ID || secondContext.Messages[2].Content != "ok" {
		t.Fatalf("tool result = %#v", secondContext.Messages[2])
	}
	if secondContext.Messages[3].Content != "fixture attachment is ready" || secondContext.Messages[4].Content != "queued user context" {
		t.Fatalf("post-tool injections = %#v", secondContext.Messages[3:])
	}
}

func TestTL004TargetToolFailuresRemainCorrelatedAndContinue(t *testing.T) {
	tests := []struct {
		name       string
		call       runtimecontract.ToolCall
		behavior   *runtimecontract.ToolBehavior
		wantOutput string
	}{
		{name: "unknown", call: runtimecontract.ToolCall{ID: "call-unknown", Name: "missing", Arguments: `{}`}, wantOutput: "Error: tool 'missing' does not exist in the system"},
		{
			name: "invalid arguments", call: runtimecontract.ToolCall{ID: "call-invalid", Name: "validate", Arguments: `{"actual":true}`},
			behavior:   &runtimecontract.ToolBehavior{Call: runtimecontract.ToolCall{Name: "validate", Arguments: `{"expected":true}`}, Definition: contractToolDefinition("validate")},
			wantOutput: `Error executing validate: invalid arguments: got {"actual":true}, want {"expected":true}`,
		},
		{
			name: "business failure", call: runtimecontract.ToolCall{ID: "call-business", Name: "business", Arguments: `{}`},
			behavior:   &runtimecontract.ToolBehavior{Call: runtimecontract.ToolCall{Name: "business", Arguments: `{}`}, Definition: contractToolDefinition("business"), Result: runtimecontract.ToolResult{Output: "business rejected", IsError: true}},
			wantOutput: "business rejected",
		},
		{
			name: "infrastructure failure", call: runtimecontract.ToolCall{ID: "call-backend", Name: "backend", Arguments: `{}`},
			behavior:   &runtimecontract.ToolBehavior{Call: runtimecontract.ToolCall{Name: "backend", Arguments: `{}`}, Definition: contractToolDefinition("backend"), Result: runtimecontract.ToolResult{ErrorKind: "backend unavailable"}},
			wantOutput: "Error executing backend: backend unavailable",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			script := runtimecontract.Script{ModelSteps: []runtimecontract.ModelStep{
				{Response: runtimecontract.ModelResponse{ToolCalls: []runtimecontract.ToolCall{testCase.call}, FinishReason: "tool_calls"}},
				{Response: runtimecontract.ModelResponse{Content: "recovered", FinishReason: "stop"}},
			}}
			if testCase.behavior != nil {
				script.Tools = []runtimecontract.ToolBehavior{*testCase.behavior}
			}
			observed, err := newTargetContractAdapter(t).Run(context.Background(), contractInput(3), script)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if observed.Outcome.FinalMessage != "recovered" || len(observed.Facts) != 5 {
				t.Fatalf("observed = %#v", observed)
			}
			result := observed.Facts[2]
			if result.Kind != "tool_result" || result.CallID != testCase.call.ID || !result.IsError || result.Content != testCase.wantOutput {
				t.Fatalf("tool result = %#v, want %q", result, testCase.wantOutput)
			}
			if got := observed.Requests[1].Messages[3]; got.ToolCallID != testCase.call.ID || got.Content != testCase.wantOutput {
				t.Fatalf("follow-up result = %#v", got)
			}
		})
	}
}

type targetContractAdapter struct {
	t *testing.T
}

func newTargetContractAdapter(t *testing.T) runtimecontract.Adapter {
	t.Helper()
	return &targetContractAdapter{t: t}
}

func (a *targetContractAdapter) Run(
	ctx context.Context,
	input runtimecontract.RunInput,
	script runtimecontract.Script,
) (runtimecontract.Observed, error) {
	a.t.Helper()
	conversation := &targetTestConversation{input: input}
	invoker := &targetScriptedInvoker{
		steps:        script.ModelSteps,
		streaming:    currentContractScriptStreams(script),
		parallelSafe: targetParallelSafety(script.Tools),
	}
	toolExecutor := newTargetScriptedToolExecutor(script.Tools, invoker)
	observer := &targetTestObserver{}
	eng := NewAgentEngine(invoker, toolExecutor, conversation, targetTestPolicy{}, observer)

	outcome, runErr := eng.Run(ctx, RunInput{Prompt: input.Prompt, MaxTurns: input.MaxTurns, Thinking: input.Thinking})
	observed := runtimecontract.Observed{
		Requests:  invoker.requests,
		Facts:     observer.facts,
		Outcome:   targetContractOutcome(outcome),
		Persisted: conversation.persisted,
	}
	observed.Metrics = targetOrderedMetrics(invoker.metrics, toolExecutor.metrics)
	if len(observed.Metrics) > 0 {
		observed.Metrics = append(observed.Metrics, runtimecontract.Metric{
			Kind:       "run_summary",
			ModelCalls: len(invoker.metrics),
			ToolCalls:  len(toolExecutor.metrics),
			ErrorCount: targetMetricErrorCount(invoker.metrics, toolExecutor.metrics),
		})
	}
	return observed, runErr
}

type targetScriptedInvoker struct {
	steps        []runtimecontract.ModelStep
	requests     []runtimecontract.ModelRequest
	metrics      []runtimecontract.Metric
	streaming    bool
	parallelSafe map[string]bool
}

func (i *targetScriptedInvoker) StartRun(context.Context) (ModelRunInvoker, error) {
	return &targetScriptedModelRun{parent: i}, nil
}

type targetScriptedModelRun struct {
	parent            *targetScriptedInvoker
	streamingDisabled bool
}

func (r *targetScriptedModelRun) Invoke(
	_ context.Context,
	runContext RunContext,
	emit ModelFactEmitter,
) (ModelResult, error) {
	metricError := false
	transport := ""
	if r.parent.streaming {
		if r.streamingDisabled {
			transport = "non_stream"
		} else {
			transport = "stream"
		}
	}
	r.appendRequest(runContext, transport)
	defer func() {
		r.parent.metrics = append(r.parent.metrics, runtimecontract.Metric{
			Kind: "model_call", Turn: runContext.Turn, Phase: string(runContext.Phase), IsError: metricError,
		})
	}()

	step, err := r.nextStep()
	if err != nil {
		return ModelResult{}, err
	}
	if transport == "stream" {
		for _, delta := range step.Deltas {
			emit(ModelFact{Kind: ModelFactMessageDelta, Content: delta})
		}
		if step.Error != "" && len(step.Deltas) == 0 && targetStreamingFallbackKind(step.ErrorKind) {
			r.streamingDisabled = true
			r.appendRequest(runContext, "non_stream")
			step, err = r.nextStep()
			if err != nil {
				return ModelResult{}, err
			}
		}
	}
	if step.Error != "" {
		metricError = true
		if transport == "stream" && len(step.Deltas) > 0 && step.ErrorKind == "unsupported" {
			return ModelResult{}, fmt.Errorf("streaming is not supported: %s", step.Error)
		}
		return ModelResult{}, errors.New(step.Error)
	}
	if step.NilResponse || step.NilMessage {
		return ModelResult{}, errors.New("provider returned empty response")
	}
	return targetModelResult(step.Response), nil
}

func (r *targetScriptedModelRun) appendRequest(runContext RunContext, transport string) {
	r.parent.requests = append(r.parent.requests, runtimecontract.ModelRequest{
		Phase:           string(runContext.Phase),
		Transport:       transport,
		Messages:        targetContractMessages(runContext.Messages),
		ToolDefinitions: targetContractDefinitions(runContext.ToolDefinitions, r.parent.parallelSafe),
		Model:           runContext.Model,
		Provider:        runContext.Provider,
		Effort:          runContext.Effort,
	})
}

func (r *targetScriptedModelRun) nextStep() (runtimecontract.ModelStep, error) {
	if len(r.parent.steps) == 0 {
		return runtimecontract.ModelStep{}, errors.New("target scripted model exhausted")
	}
	step := r.parent.steps[0]
	r.parent.steps = r.parent.steps[1:]
	return step, nil
}

func targetModelResult(response runtimecontract.ModelResponse) ModelResult {
	message := schema.Message{
		Role:      schema.RoleAssistant,
		Content:   response.Content,
		ToolCalls: targetSchemaToolCalls(response.ToolCalls),
	}
	return ModelResult{
		Message:      message,
		FinishReason: response.FinishReason,
		Usage: schema.Usage{
			InputTokens:  int64(response.Usage.InputTokens),
			OutputTokens: int64(response.Usage.OutputTokens),
		},
	}
}

func targetStreamingFallbackKind(kind string) bool {
	switch kind {
	case "unsupported", "empty", "retryable":
		return true
	default:
		return false
	}
}

func targetParallelSafety(behaviors []runtimecontract.ToolBehavior) map[string]bool {
	result := make(map[string]bool, len(behaviors))
	for _, behavior := range behaviors {
		result[behavior.Definition.Name] = behavior.Definition.ParallelSafe
	}
	return result
}

func targetMetricErrorCount(groups ...[]runtimecontract.Metric) int {
	count := 0
	for _, metrics := range groups {
		for _, metric := range metrics {
			if metric.IsError {
				count++
			}
		}
	}
	return count
}

func targetOrderedMetrics(modelMetrics, toolMetrics []runtimecontract.Metric) []runtimecontract.Metric {
	maxTurn := 0
	for _, metric := range modelMetrics {
		if metric.Turn > maxTurn {
			maxTurn = metric.Turn
		}
	}
	for _, metric := range toolMetrics {
		if metric.Turn > maxTurn {
			maxTurn = metric.Turn
		}
	}
	result := make([]runtimecontract.Metric, 0, len(modelMetrics)+len(toolMetrics))
	for turn := 1; turn <= maxTurn; turn++ {
		for _, metric := range modelMetrics {
			if metric.Turn == turn {
				result = append(result, metric)
			}
		}
		for _, metric := range toolMetrics {
			if metric.Turn == turn {
				result = append(result, metric)
			}
		}
	}
	return result
}

type targetTestConversation struct {
	input     runtimecontract.RunInput
	messages  []schema.Message
	persisted []runtimecontract.PersistedRecord
}

func (c *targetTestConversation) Prepare(_ context.Context, request ConversationRequest) (ConversationProjection, error) {
	input := request.Input
	if c.messages == nil {
		c.messages = targetSchemaMessages(contractInitialMessages(input.Prompt))
		c.persisted = append(c.persisted, contractMessageRecord(len(c.persisted)+1, "user:"+input.Prompt))
	}
	return ConversationProjection{Context: RunContext{
		Phase:    PhaseAction,
		Messages: cloneMessages(c.messages),
		Model:    c.input.Model,
		Provider: c.input.Provider,
		Effort:   c.input.Effort,
	}}, nil
}

func (c *targetTestConversation) RequestChanges(_ context.Context, changes []ConversationChange) error {
	for _, change := range changes {
		c.messages = append(c.messages, cloneMessage(change.Message))
		if change.Kind == ConversationAppendMessage {
			c.persisted = append(c.persisted, contractMessageRecord(len(c.persisted)+1, targetPersistedMessage(change.Message)))
		}
	}
	return nil
}

type targetTestToolExecutor struct{}

func (targetTestToolExecutor) Snapshot(context.Context) (ToolSnapshot, error) {
	return targetToolSnapshot{}, nil
}

func (targetTestToolExecutor) Execute(context.Context, ToolSnapshot, []schema.ToolCall) (ToolBatch, error) {
	return ToolBatch{}, nil
}

type perTurnTargetToolExecutor struct {
	snapshots     []targetToolSnapshot
	snapshotCalls int
	executedWith  []string
}

func (e *perTurnTargetToolExecutor) Snapshot(context.Context) (ToolSnapshot, error) {
	index := e.snapshotCalls
	e.snapshotCalls++
	return e.snapshots[index], nil
}

func (e *perTurnTargetToolExecutor) Execute(_ context.Context, snapshot ToolSnapshot, calls []schema.ToolCall) (ToolBatch, error) {
	definitions := snapshot.ToolDefinitions()
	e.executedWith = append(e.executedWith, definitions[0].Name)
	return ToolBatch{Results: []ToolExecutionResult{{CallID: calls[0].ID, ModelContent: "ok"}}}, nil
}

type targetScriptedToolExecutor struct {
	snapshot  targetToolSnapshot
	behaviors map[string]runtimecontract.ToolBehavior
	metrics   []runtimecontract.Metric
	invoker   *targetScriptedInvoker
}

func newTargetScriptedToolExecutor(
	behaviors []runtimecontract.ToolBehavior,
	invoker *targetScriptedInvoker,
) *targetScriptedToolExecutor {
	executor := &targetScriptedToolExecutor{
		behaviors: make(map[string]runtimecontract.ToolBehavior, len(behaviors)),
		invoker:   invoker,
	}
	for _, behavior := range behaviors {
		executor.snapshot.definitions = append(executor.snapshot.definitions, schema.ToolDefinition{
			Name: behavior.Definition.Name, Description: behavior.Definition.Description,
			InputSchema: targetInputSchema(behavior.Definition.InputSchema),
		})
		executor.behaviors[behavior.Call.ID+"\x00"+behavior.Call.Name] = behavior
		executor.behaviors["\x00"+behavior.Call.Name] = behavior
	}
	return executor
}

func (e *targetScriptedToolExecutor) Snapshot(context.Context) (ToolSnapshot, error) {
	return e.snapshot, nil
}

func (e *targetScriptedToolExecutor) Execute(_ context.Context, _ ToolSnapshot, calls []schema.ToolCall) (ToolBatch, error) {
	batch := ToolBatch{Results: make([]ToolExecutionResult, 0, len(calls))}
	turn := 0
	if len(e.invoker.metrics) > 0 {
		turn = e.invoker.metrics[len(e.invoker.metrics)-1].Turn
	}
	for _, call := range calls {
		behavior, ok := e.behaviors[call.ID+"\x00"+call.Name]
		if !ok {
			behavior, ok = e.behaviors["\x00"+call.Name]
		}
		content := ""
		isError := false
		switch {
		case !ok:
			content = fmt.Sprintf("Error: tool '%s' does not exist in the system", call.Name)
			isError = true
		case behavior.Call.Arguments != "" && string(call.Arguments) != behavior.Call.Arguments:
			content = fmt.Sprintf("Error executing %s: invalid arguments: got %s, want %s", call.Name, call.Arguments, behavior.Call.Arguments)
			isError = true
		case behavior.Result.ErrorKind != "":
			content = fmt.Sprintf("Error executing %s: %s", call.Name, behavior.Result.ErrorKind)
			isError = true
		default:
			content = behavior.Result.ModelContent
			if content == "" {
				content = behavior.Result.Output
			}
			isError = behavior.Result.IsError
		}
		batch.Results = append(batch.Results, ToolExecutionResult{
			CallID: call.ID, FullContent: behavior.Result.Output,
			ModelContent: content, ObserverContent: content, IsError: isError,
		})
		e.metrics = append(e.metrics, runtimecontract.Metric{Kind: "tool_call", Turn: turn, ToolName: call.Name, CallID: call.ID, IsError: isError})
	}
	return batch, nil
}

func targetInputSchema(encoded string) any {
	if encoded == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(encoded), &value); err != nil {
		return encoded
	}
	return value
}

type immutableTargetToolExecutor struct {
	snapshot ToolSnapshot
}

func (e *immutableTargetToolExecutor) Snapshot(context.Context) (ToolSnapshot, error) {
	return e.snapshot, nil
}

func (*immutableTargetToolExecutor) Execute(context.Context, ToolSnapshot, []schema.ToolCall) (ToolBatch, error) {
	return ToolBatch{}, nil
}

type targetToolSnapshot struct {
	definitions []schema.ToolDefinition
}

func (s targetToolSnapshot) ToolDefinitions() []schema.ToolDefinition {
	return s.definitions
}

type immutableTargetConversation struct {
	runContext RunContext
}

type invocationRecordingConversation struct {
	requests []ConversationRequest
}

type compactingInvocationConversation struct {
	requests []ConversationRequest
}

func (c *compactingInvocationConversation) Prepare(_ context.Context, request ConversationRequest) (ConversationProjection, error) {
	c.requests = append(c.requests, request)
	projection := ConversationProjection{Context: RunContext{Messages: []schema.Message{{Role: schema.RoleUser, Content: request.Input.Prompt}}}}
	if request.Preparation == ConversationPrepareReactive {
		projection.Compactions = []ConversationCompaction{{Trigger: "reactive"}}
	}
	return projection, nil
}

func (*compactingInvocationConversation) RequestChanges(context.Context, []ConversationChange) error {
	return nil
}

func (c *invocationRecordingConversation) Prepare(_ context.Context, request ConversationRequest) (ConversationProjection, error) {
	request.ToolDefinitions = cloneToolDefinitions(request.ToolDefinitions)
	c.requests = append(c.requests, request)
	return ConversationProjection{Context: RunContext{Messages: []schema.Message{{Role: schema.RoleUser, Content: request.Input.Prompt}}}}, nil
}

func (*invocationRecordingConversation) RequestChanges(context.Context, []ConversationChange) error {
	return nil
}

func (c *immutableTargetConversation) Prepare(context.Context, ConversationRequest) (ConversationProjection, error) {
	return ConversationProjection{Context: c.runContext}, nil
}

func (*immutableTargetConversation) RequestChanges(context.Context, []ConversationChange) error {
	return nil
}

type proposalMutatingConversation struct{}

func (*proposalMutatingConversation) Prepare(context.Context, ConversationRequest) (ConversationProjection, error) {
	return ConversationProjection{}, nil
}

func (*proposalMutatingConversation) RequestChanges(_ context.Context, changes []ConversationChange) error {
	for index := range changes {
		if len(changes[index].Message.ToolCalls) == 0 {
			continue
		}
		changes[index].Message.ToolCalls[0].Name = "mutated"
		changes[index].Message.ToolCalls[0].Arguments[0] = 'X'
	}
	return nil
}

type recordingTargetToolExecutor struct {
	calls []schema.ToolCall
}

type distinctResultToolExecutor struct{}

func (*distinctResultToolExecutor) Snapshot(context.Context) (ToolSnapshot, error) {
	return targetToolSnapshot{}, nil
}

func (*distinctResultToolExecutor) Execute(_ context.Context, _ ToolSnapshot, calls []schema.ToolCall) (ToolBatch, error) {
	return ToolBatch{Results: []ToolExecutionResult{{
		CallID: calls[0].ID, FullContent: "full artifact", ModelContent: "model preview",
		ObserverContent: "reporter preview", ArtifactPath: "/fixture/call-large.txt",
	}}}, nil
}

type injectionOrderingConversation struct {
	messages      []schema.Message
	toolCommitted bool
	injected      bool
}

func (c *injectionOrderingConversation) Prepare(_ context.Context, request ConversationRequest) (ConversationProjection, error) {
	input := request.Input
	if c.messages == nil {
		c.messages = []schema.Message{{Role: schema.RoleUser, Content: input.Prompt}}
	}
	if c.toolCommitted && !c.injected {
		c.messages = append(c.messages,
			schema.Message{Role: schema.RoleUser, Content: "fixture attachment is ready"},
			schema.Message{Role: schema.RoleUser, Content: "queued user context"},
		)
		c.injected = true
	}
	return ConversationProjection{Context: RunContext{Messages: cloneMessages(c.messages)}}, nil
}

func factKinds(facts []Fact) []FactKind {
	kinds := make([]FactKind, len(facts))
	for index, fact := range facts {
		kinds[index] = fact.Kind
	}
	return kinds
}

func (c *injectionOrderingConversation) RequestChanges(_ context.Context, changes []ConversationChange) error {
	for _, change := range changes {
		c.messages = append(c.messages, cloneMessage(change.Message))
		if change.Message.ToolCallID != "" {
			c.toolCommitted = true
		}
	}
	return nil
}

func (*recordingTargetToolExecutor) Snapshot(context.Context) (ToolSnapshot, error) {
	return targetToolSnapshot{}, nil
}

func (e *recordingTargetToolExecutor) Execute(_ context.Context, _ ToolSnapshot, calls []schema.ToolCall) (ToolBatch, error) {
	e.calls = cloneToolCalls(calls)
	return ToolBatch{Results: []ToolExecutionResult{{
		CallID: calls[0].ID, FullContent: "ok", ModelContent: "ok", ObserverContent: "ok",
	}}}, nil
}

type targetTestPolicy struct{}

func (targetTestPolicy) StartRun(context.Context, RunInput) (TurnRunPolicy, error) {
	return targetTestRunPolicy{}, nil
}

type targetTestRunPolicy struct{}

func (targetTestRunPolicy) BeforeTurn(context.Context, TurnState) (PolicyChanges, error) {
	return PolicyChanges{}, nil
}

func (targetTestRunPolicy) AfterModel(_ context.Context, state TurnState) (TurnDecision, error) {
	return TurnDecision{Complete: len(state.Model.Message.ToolCalls) == 0}, nil
}

func (targetTestRunPolicy) AfterTools(context.Context, ToolState) (PolicyChanges, error) {
	return PolicyChanges{}, nil
}

type targetTestObserver struct {
	facts []runtimecontract.Fact
}

func (o *targetTestObserver) Observe(_ context.Context, fact Fact) {
	o.facts = append(o.facts, runtimecontract.Fact{
		Kind:     string(fact.Kind),
		Sequence: fact.Sequence,
		Turn:     fact.Turn,
		Phase:    string(fact.Phase),
		CallID:   fact.CallID,
		Name:     fact.Name,
		Content:  fact.Content,
		IsError:  fact.IsError,
	})
}

type modelInvokerFunc func(context.Context, RunContext) (ModelResult, error)

func (f modelInvokerFunc) StartRun(context.Context) (ModelRunInvoker, error) {
	return modelRunInvokerFunc(func(ctx context.Context, runContext RunContext, _ ModelFactEmitter) (ModelResult, error) {
		return f(ctx, runContext)
	}), nil
}

type modelRunInvokerFunc func(context.Context, RunContext, ModelFactEmitter) (ModelResult, error)

func (f modelRunInvokerFunc) Invoke(ctx context.Context, runContext RunContext, emit ModelFactEmitter) (ModelResult, error) {
	return f(ctx, runContext, emit)
}

type observerFunc func(context.Context, Fact)

func (f observerFunc) Observe(ctx context.Context, fact Fact) {
	f(ctx, fact)
}

func targetContractOutcome(outcome RunOutcome) runtimecontract.Outcome {
	result := runtimecontract.Outcome{
		FinalMessage: outcome.FinalMessage,
		FinishReason: outcome.FinishReason,
		TurnCount:    outcome.TurnCount,
		Usage: runtimecontract.Usage{
			InputTokens:  int(outcome.Usage.InputTokens),
			OutputTokens: int(outcome.Usage.OutputTokens),
			TotalTokens:  int(outcome.Usage.InputTokens + outcome.Usage.OutputTokens),
		},
		Partial:   outcome.Partial,
		ErrorKind: outcome.ErrorKind,
	}
	if outcome.Err != nil {
		result.Error = outcome.Err.Error()
	}
	return result
}

func targetContractMessages(messages []schema.Message) []runtimecontract.Message {
	result := make([]runtimecontract.Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, runtimecontract.Message{
			Role:       string(message.Role),
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			ToolCalls:  targetContractToolCalls(message.ToolCalls),
		})
	}
	return result
}

func targetSchemaMessages(messages []runtimecontract.Message) []schema.Message {
	result := make([]schema.Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, schema.Message{
			Role:       schema.Role(message.Role),
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			ToolCalls:  targetSchemaToolCalls(message.ToolCalls),
		})
	}
	return result
}

func targetContractDefinitions(definitions []schema.ToolDefinition, parallelSafe map[string]bool) []runtimecontract.ToolDefinition {
	if len(definitions) == 0 {
		return nil
	}
	result := make([]runtimecontract.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		inputSchema, _ := json.Marshal(definition.InputSchema)
		result = append(result, runtimecontract.ToolDefinition{
			Name: definition.Name, Description: definition.Description, InputSchema: string(inputSchema), ParallelSafe: parallelSafe[definition.Name],
		})
	}
	return result
}

func targetContractToolCalls(calls []schema.ToolCall) []runtimecontract.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]runtimecontract.ToolCall, 0, len(calls))
	for _, call := range calls {
		result = append(result, runtimecontract.ToolCall{ID: call.ID, Name: call.Name, Arguments: string(call.Arguments)})
	}
	return result
}

func targetSchemaToolCalls(calls []runtimecontract.ToolCall) []schema.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]schema.ToolCall, 0, len(calls))
	for _, call := range calls {
		result = append(result, schema.ToolCall{ID: call.ID, Name: call.Name, Arguments: json.RawMessage(call.Arguments)})
	}
	return result
}

func targetPersistedMessage(message schema.Message) string {
	switch {
	case message.Role == schema.RoleAssistant:
		content := "assistant:" + message.Content
		for _, call := range message.ToolCalls {
			content += "|tool_call:" + call.ID + ":" + call.Name + ":" + string(call.Arguments)
		}
		return content
	case message.ToolCallID != "":
		return fmt.Sprintf("tool_result:%s:%s", message.ToolCallID, message.Content)
	default:
		return string(message.Role) + ":" + message.Content
	}
}
