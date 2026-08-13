package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (c *targetTestConversation) Prepare(_ context.Context, input RunInput) (RunContext, error) {
	if c.messages == nil {
		c.messages = targetSchemaMessages(contractInitialMessages(input.Prompt))
		c.persisted = append(c.persisted, contractMessageRecord(len(c.persisted)+1, "user:"+input.Prompt))
	}
	return RunContext{
		Phase:    PhaseAction,
		Messages: cloneMessages(c.messages),
		Model:    c.input.Model,
		Provider: c.input.Provider,
		Effort:   c.input.Effort,
	}, nil
}

func (c *targetTestConversation) Apply(_ context.Context, changes []ConversationChange) error {
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
	}
	return executor
}

func (e *targetScriptedToolExecutor) Snapshot(context.Context) (ToolSnapshot, error) {
	return e.snapshot, nil
}

func (e *targetScriptedToolExecutor) Execute(_ context.Context, _ ToolSnapshot, calls []schema.ToolCall) (ToolBatch, error) {
	batch := ToolBatch{Results: make([]schema.ToolResult, 0, len(calls))}
	turn := 0
	if len(e.invoker.metrics) > 0 {
		turn = e.invoker.metrics[len(e.invoker.metrics)-1].Turn
	}
	for _, call := range calls {
		behavior, ok := e.behaviors[call.ID+"\x00"+call.Name]
		if !ok {
			return ToolBatch{}, fmt.Errorf("target scripted tool behavior missing for %s/%s", call.ID, call.Name)
		}
		content := behavior.Result.ModelContent
		if content == "" {
			content = behavior.Result.Output
		}
		batch.Results = append(batch.Results, schema.ToolResult{ToolCallID: call.ID, Output: content, IsError: behavior.Result.IsError})
		e.metrics = append(e.metrics, runtimecontract.Metric{Kind: "tool_call", Turn: turn, ToolName: call.Name, CallID: call.ID})
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

func (c *immutableTargetConversation) Prepare(context.Context, RunInput) (RunContext, error) {
	return c.runContext, nil
}

func (*immutableTargetConversation) Apply(context.Context, []ConversationChange) error {
	return nil
}

type proposalMutatingConversation struct{}

func (*proposalMutatingConversation) Prepare(context.Context, RunInput) (RunContext, error) {
	return RunContext{}, nil
}

func (*proposalMutatingConversation) Apply(_ context.Context, changes []ConversationChange) error {
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

func (*recordingTargetToolExecutor) Snapshot(context.Context) (ToolSnapshot, error) {
	return targetToolSnapshot{}, nil
}

func (e *recordingTargetToolExecutor) Execute(_ context.Context, _ ToolSnapshot, calls []schema.ToolCall) (ToolBatch, error) {
	e.calls = cloneToolCalls(calls)
	return ToolBatch{Results: []schema.ToolResult{{ToolCallID: calls[0].ID, Output: "ok"}}}, nil
}

type targetTestPolicy struct{}

func (targetTestPolicy) Decide(_ context.Context, state TurnState) (TurnDecision, error) {
	return TurnDecision{Complete: len(state.Model.Message.ToolCalls) == 0}, nil
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
