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
	invoker := &targetScriptedInvoker{steps: script.ModelSteps}
	observer := &targetTestObserver{}
	eng := NewAgentEngine(invoker, targetTestToolExecutor{}, conversation, targetTestPolicy{}, observer)

	outcome, runErr := eng.Run(ctx, RunInput{Prompt: input.Prompt, MaxTurns: input.MaxTurns})
	observed := runtimecontract.Observed{
		Requests:  invoker.requests,
		Facts:     observer.facts,
		Outcome:   targetContractOutcome(outcome),
		Persisted: conversation.persisted,
	}
	if len(invoker.requests) > 0 {
		observed.Metrics = []runtimecontract.Metric{{Kind: "model_call", Turn: 1, Phase: "action", IsError: runErr != nil}}
		observed.Metrics = append(observed.Metrics, runtimecontract.Metric{Kind: "run_summary", ModelCalls: 1, ErrorCount: boolInt(runErr != nil)})
	}
	return observed, runErr
}

type targetScriptedInvoker struct {
	steps    []runtimecontract.ModelStep
	requests []runtimecontract.ModelRequest
}

func (i *targetScriptedInvoker) Invoke(_ context.Context, runContext RunContext) (ModelResult, error) {
	i.requests = append(i.requests, runtimecontract.ModelRequest{
		Phase:           string(runContext.Phase),
		Messages:        targetContractMessages(runContext.Messages),
		ToolDefinitions: targetContractDefinitions(runContext.ToolDefinitions),
		Model:           runContext.Model,
		Provider:        runContext.Provider,
		Effort:          runContext.Effort,
	})
	if len(i.steps) == 0 {
		return ModelResult{}, errors.New("target scripted model exhausted")
	}
	step := i.steps[0]
	i.steps = i.steps[1:]
	if step.Error != "" {
		return ModelResult{}, errors.New(step.Error)
	}
	message := schema.Message{
		Role:      schema.RoleAssistant,
		Content:   step.Response.Content,
		ToolCalls: targetSchemaToolCalls(step.Response.ToolCalls),
	}
	return ModelResult{
		Message:      message,
		FinishReason: step.Response.FinishReason,
		Usage: schema.Usage{
			InputTokens:  int64(step.Response.Usage.InputTokens),
			OutputTokens: int64(step.Response.Usage.OutputTokens),
		},
	}, nil
}

type targetTestConversation struct {
	input     runtimecontract.RunInput
	persisted []runtimecontract.PersistedRecord
}

func (c *targetTestConversation) Prepare(_ context.Context, input RunInput) (RunContext, error) {
	c.persisted = append(c.persisted, contractMessageRecord(len(c.persisted)+1, "user:"+input.Prompt))
	return RunContext{
		Phase:    PhaseAction,
		Messages: targetSchemaMessages(contractInitialMessages(input.Prompt)),
		Model:    c.input.Model,
		Provider: c.input.Provider,
		Effort:   c.input.Effort,
	}, nil
}

func (c *targetTestConversation) Apply(_ context.Context, changes []ConversationChange) error {
	for _, change := range changes {
		if change.Kind != ConversationAppendMessage {
			continue
		}
		c.persisted = append(c.persisted, contractMessageRecord(len(c.persisted)+1, targetPersistedMessage(change.Message)))
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
		Content:  fact.Content,
		IsError:  fact.IsError,
	})
}

type modelInvokerFunc func(context.Context, RunContext) (ModelResult, error)

func (f modelInvokerFunc) Invoke(ctx context.Context, runContext RunContext) (ModelResult, error) {
	return f(ctx, runContext)
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

func targetContractDefinitions(definitions []schema.ToolDefinition) []runtimecontract.ToolDefinition {
	if len(definitions) == 0 {
		return nil
	}
	result := make([]runtimecontract.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		inputSchema, _ := json.Marshal(definition.InputSchema)
		result = append(result, runtimecontract.ToolDefinition{Name: definition.Name, Description: definition.Description, InputSchema: string(inputSchema)})
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
		return "assistant:" + message.Content
	case message.ToolCallID != "":
		return fmt.Sprintf("tool_result:%s:%s", message.ToolCallID, message.Content)
	default:
		return string(message.Role) + ":" + message.Content
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
