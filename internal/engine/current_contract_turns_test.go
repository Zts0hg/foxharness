package engine

import (
	"context"
	"testing"

	"github.com/Zts0hg/foxharness/internal/testsupport/runtimecontract"
)

func TestCurrentProductionContractAdapterRunsRuntimeTurnCatalog(t *testing.T) {
	for _, testCase := range currentRuntimeTurnScenarios() {
		t.Run(testCase.name, func(t *testing.T) {
			adapter := newCurrentProductionContractAdapter(t)
			if err := runtimecontract.VerifyScenario(context.Background(), adapter, testCase.scenario); err != nil {
				t.Fatalf("VerifyScenario() error = %v", err)
			}
		})
	}
}

type runtimeTurnTestCase struct {
	name     string
	scenario runtimecontract.Scenario
}

func currentRuntimeTurnScenarios() []runtimeTurnTestCase {
	return []runtimeTurnTestCase{
		{name: "RT-001 tool-free completion", scenario: runtimeTurnToolFreeScenario()},
		{name: "RT-002 correlated tool round trip", scenario: runtimeTurnToolScenario(false)},
		{name: "RT-003 mixed text and tool visibility", scenario: runtimeTurnToolScenario(true)},
		{name: "RT-004 thinking and action surfaces", scenario: runtimeTurnThinkingScenario()},
		{name: "RT-005 zero turn limit is unlimited", scenario: runtimeTurnUnlimitedScenario(0)},
		{name: "RT-005 negative turn limit is unlimited", scenario: runtimeTurnUnlimitedScenario(-1)},
		{name: "RT-005 positive turn limit boundary", scenario: runtimeTurnLimitedScenario()},
		{name: "RT-006 invocation snapshot", scenario: runtimeTurnInvocationSnapshotScenario()},
		{name: "RT-007 empty message", scenario: runtimeTurnEmptyScenario(runtimecontract.ModelStep{Response: runtimecontract.ModelResponse{FinishReason: "stop"}}, false)},
		{name: "RT-007 nil response", scenario: runtimeTurnEmptyScenario(runtimecontract.ModelStep{NilResponse: true}, true)},
		{name: "RT-007 nil message", scenario: runtimeTurnEmptyScenario(runtimecontract.ModelStep{NilMessage: true}, true)},
		{name: "RT-007 provider error", scenario: runtimeTurnProviderErrorScenario()},
	}
}

func runtimeTurnToolFreeScenario() runtimecontract.Scenario {
	input := contractInput(3)
	response := runtimecontract.ModelResponse{
		Content:      "scripted answer",
		FinishReason: "stop",
		Usage:        runtimecontract.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6},
	}
	return runtimecontract.Scenario{
		ID:     "RT-001",
		Input:  input,
		Script: runtimecontract.Script{ModelSteps: []runtimecontract.ModelStep{{Response: response}}},
		Expected: runtimecontract.Expected{Observed: runtimecontract.Observed{
			Requests: []runtimecontract.ModelRequest{contractRequest(input, "action", contractInitialMessages(input.Prompt), nil)},
			Facts: []runtimecontract.Fact{
				{Kind: "run_started", Sequence: 1},
				{Kind: "message", Sequence: 2, Content: response.Content},
				{Kind: "run_completed", Sequence: 3, Content: response.Content},
			},
			Outcome: runtimecontract.Outcome{FinalMessage: response.Content, FinishReason: "stop", TurnCount: 1, Usage: response.Usage},
			Persisted: []runtimecontract.PersistedRecord{
				contractMessageRecord(1, "user:Return the scripted answer."),
				contractMessageRecord(2, "assistant:scripted answer"),
			},
			Metrics: []runtimecontract.Metric{
				{Kind: "model_call", Turn: 1, Phase: "action"},
				{Kind: "run_summary", ModelCalls: 1},
			},
		}},
	}
}

func runtimeTurnToolScenario(mixedText bool) runtimecontract.Scenario {
	input := contractInput(3)
	definition := contractToolDefinition("echo")
	call := runtimecontract.ToolCall{ID: "call-echo-1", Name: "echo", Arguments: `{"value":"hello"}`}
	intermediate := ""
	id := "RT-002"
	if mixedText {
		intermediate = "I will use the echo tool."
		id = "RT-003"
	}
	first := runtimecontract.ModelResponse{Content: intermediate, ToolCalls: []runtimecontract.ToolCall{call}, FinishReason: "tool_calls", Usage: runtimecontract.Usage{InputTokens: 5, OutputTokens: 3, TotalTokens: 8}}
	second := runtimecontract.ModelResponse{Content: "echo complete", FinishReason: "stop", Usage: runtimecontract.Usage{InputTokens: 8, OutputTokens: 2, TotalTokens: 10}}
	facts := []runtimecontract.Fact{{Kind: "run_started", Sequence: 1}}
	sequence := 2
	if mixedText {
		facts = append(facts, runtimecontract.Fact{Kind: "message", Sequence: sequence, Content: intermediate})
		sequence++
	}
	facts = append(facts,
		runtimecontract.Fact{Kind: "tool_call", Sequence: sequence, CallID: call.ID, Name: call.Name, Content: call.Arguments},
		runtimecontract.Fact{Kind: "tool_result", Sequence: sequence + 1, CallID: call.ID, Name: call.Name, Content: "echo:hello"},
		runtimecontract.Fact{Kind: "message", Sequence: sequence + 2, Content: second.Content},
		runtimecontract.Fact{Kind: "run_completed", Sequence: sequence + 3, Content: second.Content},
	)
	initial := contractInitialMessages(input.Prompt)
	followUp := appendContractMessages(initial,
		runtimecontract.Message{Role: "assistant", Content: intermediate, ToolCalls: []runtimecontract.ToolCall{call}},
		runtimecontract.Message{Role: "user", Content: "echo:hello", ToolCallID: call.ID},
	)
	return runtimecontract.Scenario{
		ID:    id,
		Input: input,
		Script: runtimecontract.Script{
			ModelSteps: []runtimecontract.ModelStep{{Response: first}, {Response: second}},
			Tools: []runtimecontract.ToolBehavior{{
				Call:       call,
				Definition: definition,
				Result:     runtimecontract.ToolResult{Output: "echo:hello", ModelContent: "echo:hello"},
			}},
		},
		Expected: runtimecontract.Expected{Observed: runtimecontract.Observed{
			Requests: []runtimecontract.ModelRequest{
				contractRequest(input, "action", initial, []runtimecontract.ToolDefinition{definition}),
				contractRequest(input, "action", followUp, []runtimecontract.ToolDefinition{definition}),
			},
			Facts:   facts,
			Outcome: runtimecontract.Outcome{FinalMessage: second.Content, FinishReason: "stop", TurnCount: 2, Usage: runtimecontract.Usage{InputTokens: 13, OutputTokens: 5, TotalTokens: 18}},
			Persisted: []runtimecontract.PersistedRecord{
				contractMessageRecord(1, "user:Return the scripted answer."),
				contractMessageRecord(2, "assistant:"+intermediate+"|tool_call:call-echo-1:echo:{\"value\":\"hello\"}"),
				contractMessageRecord(3, "tool_result:call-echo-1:echo:hello"),
				contractMessageRecord(4, "assistant:echo complete"),
			},
			Metrics: []runtimecontract.Metric{
				{Kind: "model_call", Turn: 1, Phase: "action"},
				{Kind: "tool_call", Turn: 1, ToolName: "echo", CallID: call.ID},
				{Kind: "model_call", Turn: 2, Phase: "action"},
				{Kind: "run_summary", ModelCalls: 2, ToolCalls: 1},
			},
		}},
	}
}

func runtimeTurnThinkingScenario() runtimecontract.Scenario {
	input := contractInput(2)
	input.Thinking = true
	definition := contractToolDefinition("echo")
	thinking := runtimecontract.ModelResponse{Content: "private plan", FinishReason: "stop", Usage: runtimecontract.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}}
	action := runtimecontract.ModelResponse{Content: "planned answer", FinishReason: "stop", Usage: runtimecontract.Usage{InputTokens: 6, OutputTokens: 2, TotalTokens: 8}}
	initial := contractInitialMessages(input.Prompt)
	actionMessages := appendContractMessages(initial, runtimecontract.Message{Role: "assistant", Content: thinking.Content})
	return runtimecontract.Scenario{
		ID:    "RT-004",
		Input: input,
		Script: runtimecontract.Script{
			ModelSteps: []runtimecontract.ModelStep{{Response: thinking}, {Response: action}},
			Tools:      []runtimecontract.ToolBehavior{{Call: runtimecontract.ToolCall{Name: "echo"}, Definition: definition}},
		},
		Expected: runtimecontract.Expected{Observed: runtimecontract.Observed{
			Requests: []runtimecontract.ModelRequest{
				contractRequest(input, "thinking", initial, nil),
				contractRequest(input, "action", actionMessages, []runtimecontract.ToolDefinition{definition}),
			},
			Facts: []runtimecontract.Fact{
				{Kind: "run_started", Sequence: 1},
				{Kind: "thinking", Sequence: 2, Turn: 1},
				{Kind: "message", Sequence: 3, Content: action.Content},
				{Kind: "run_completed", Sequence: 4, Content: action.Content},
			},
			Outcome: runtimecontract.Outcome{FinalMessage: action.Content, FinishReason: "stop", TurnCount: 1, Usage: runtimecontract.Usage{InputTokens: 9, OutputTokens: 4, TotalTokens: 13}},
			Persisted: []runtimecontract.PersistedRecord{
				contractMessageRecord(1, "user:Return the scripted answer."),
				contractMessageRecord(2, "assistant:planned answer"),
			},
			Metrics: []runtimecontract.Metric{
				{Kind: "model_call", Turn: 1, Phase: "thinking"},
				{Kind: "model_call", Turn: 1, Phase: "action"},
				{Kind: "run_summary", ModelCalls: 2},
			},
		}},
	}
}

func runtimeTurnUnlimitedScenario(maxTurns int) runtimecontract.Scenario {
	scenario := runtimeTurnToolScenario(false)
	scenario.ID = "RT-005"
	scenario.Input.MaxTurns = maxTurns
	for index := range scenario.Expected.Observed.Requests {
		scenario.Expected.Observed.Requests[index].Provider = scenario.Input.Provider
		scenario.Expected.Observed.Requests[index].Model = scenario.Input.Model
	}
	return scenario
}

func runtimeTurnLimitedScenario() runtimecontract.Scenario {
	input := contractInput(2)
	firstCall := runtimecontract.ToolCall{ID: "call-one", Name: "tool_one", Arguments: `{}`}
	secondCall := runtimecontract.ToolCall{ID: "call-two", Name: "tool_two", Arguments: `{}`}
	firstDefinition := contractToolDefinition(firstCall.Name)
	secondDefinition := contractToolDefinition(secondCall.Name)
	definitions := []runtimecontract.ToolDefinition{firstDefinition, secondDefinition}
	first := runtimecontract.ModelResponse{Content: "partial one", ToolCalls: []runtimecontract.ToolCall{firstCall}, FinishReason: "tool_calls"}
	second := runtimecontract.ModelResponse{Content: "partial two", ToolCalls: []runtimecontract.ToolCall{secondCall}, FinishReason: "tool_calls"}
	initial := contractInitialMessages(input.Prompt)
	secondRequest := appendContractMessages(initial,
		runtimecontract.Message{Role: "assistant", Content: first.Content, ToolCalls: []runtimecontract.ToolCall{firstCall}},
		runtimecontract.Message{Role: "user", Content: "one", ToolCallID: firstCall.ID},
	)
	errText := "超过最大 Turn 数限制: 2"
	return runtimecontract.Scenario{
		ID:    "RT-005",
		Input: input,
		Script: runtimecontract.Script{
			ModelSteps: []runtimecontract.ModelStep{{Response: first}, {Response: second}},
			Tools: []runtimecontract.ToolBehavior{
				{Call: firstCall, Definition: firstDefinition, Result: runtimecontract.ToolResult{Output: "one", ModelContent: "one"}},
				{Call: secondCall, Definition: secondDefinition, Result: runtimecontract.ToolResult{Output: "two", ModelContent: "two"}},
			},
		},
		Expected: runtimecontract.Expected{
			AdapterErrorContains:   errText,
			CompareObservedOnError: true,
			Observed: runtimecontract.Observed{
				Requests: []runtimecontract.ModelRequest{
					contractRequest(input, "action", initial, definitions),
					contractRequest(input, "action", secondRequest, definitions),
				},
				Facts: []runtimecontract.Fact{
					{Kind: "run_started", Sequence: 1},
					{Kind: "message", Sequence: 2, Content: first.Content},
					{Kind: "tool_call", Sequence: 3, CallID: firstCall.ID, Name: firstCall.Name, Content: firstCall.Arguments},
					{Kind: "tool_result", Sequence: 4, CallID: firstCall.ID, Name: firstCall.Name, Content: "one"},
					{Kind: "message", Sequence: 5, Content: second.Content},
					{Kind: "tool_call", Sequence: 6, CallID: secondCall.ID, Name: secondCall.Name, Content: secondCall.Arguments},
					{Kind: "tool_result", Sequence: 7, CallID: secondCall.ID, Name: secondCall.Name, Content: "two"},
					{Kind: "run_error", Sequence: 8, Content: errText, IsError: true},
				},
				Outcome: runtimecontract.Outcome{FinalMessage: second.Content, FinishReason: "tool_calls", TurnCount: 2, Partial: true, ErrorKind: "turn_limit", Error: errText},
				Persisted: []runtimecontract.PersistedRecord{
					contractMessageRecord(1, "user:Return the scripted answer."),
					contractMessageRecord(2, "assistant:partial one|tool_call:call-one:tool_one:{}"),
					contractMessageRecord(3, "tool_result:call-one:one"),
					contractMessageRecord(4, "assistant:partial two|tool_call:call-two:tool_two:{}"),
					contractMessageRecord(5, "tool_result:call-two:two"),
				},
				Metrics: []runtimecontract.Metric{
					{Kind: "model_call", Turn: 1, Phase: "action"},
					{Kind: "tool_call", Turn: 1, ToolName: firstCall.Name, CallID: firstCall.ID},
					{Kind: "model_call", Turn: 2, Phase: "action"},
					{Kind: "tool_call", Turn: 2, ToolName: secondCall.Name, CallID: secondCall.ID},
					{Kind: "run_summary", ModelCalls: 2, ToolCalls: 2},
				},
			},
		},
	}
}

func runtimeTurnInvocationSnapshotScenario() runtimecontract.Scenario {
	input := contractInput(1)
	input.Provider = "openai-compatible"
	input.Model = "model-snapshot"
	input.Effort = "high"
	definition := runtimecontract.ToolDefinition{Name: "inspect", Description: "inspect fixture", InputSchema: `{"type":"object"}`, ParallelSafe: true}
	response := runtimecontract.ModelResponse{Content: "snapshot preserved", FinishReason: "stop"}
	return runtimecontract.Scenario{
		ID:    "RT-006",
		Input: input,
		Script: runtimecontract.Script{
			ModelSteps: []runtimecontract.ModelStep{{Response: response}},
			Tools:      []runtimecontract.ToolBehavior{{Call: runtimecontract.ToolCall{Name: "inspect"}, Definition: definition}},
		},
		Expected: runtimecontract.Expected{Observed: runtimecontract.Observed{
			Requests: []runtimecontract.ModelRequest{contractRequest(input, "action", contractInitialMessages(input.Prompt), []runtimecontract.ToolDefinition{definition})},
			Facts: []runtimecontract.Fact{
				{Kind: "run_started", Sequence: 1},
				{Kind: "message", Sequence: 2, Content: response.Content},
				{Kind: "run_completed", Sequence: 3, Content: response.Content},
			},
			Outcome: runtimecontract.Outcome{FinalMessage: response.Content, FinishReason: "stop", TurnCount: 1},
			Persisted: []runtimecontract.PersistedRecord{
				contractMessageRecord(1, "user:Return the scripted answer."),
				contractMessageRecord(2, "assistant:snapshot preserved"),
			},
			Metrics: []runtimecontract.Metric{{Kind: "model_call", Turn: 1, Phase: "action"}, {Kind: "run_summary", ModelCalls: 1}},
		}},
	}
}

func runtimeTurnEmptyScenario(step runtimecontract.ModelStep, wantError bool) runtimecontract.Scenario {
	input := contractInput(1)
	scenario := runtimecontract.Scenario{ID: "RT-007", Input: input, Script: runtimecontract.Script{ModelSteps: []runtimecontract.ModelStep{step}}}
	initial := contractInitialMessages(input.Prompt)
	observed := runtimecontract.Observed{
		Requests: []runtimecontract.ModelRequest{contractRequest(input, "action", initial, nil)},
		Facts:    []runtimecontract.Fact{{Kind: "run_started", Sequence: 1}},
		Outcome:  runtimecontract.Outcome{TurnCount: 1},
		Persisted: []runtimecontract.PersistedRecord{
			contractMessageRecord(1, "user:Return the scripted answer."),
		},
		Metrics: []runtimecontract.Metric{{Kind: "model_call", Turn: 1, Phase: "action"}, {Kind: "run_summary", ModelCalls: 1}},
	}
	if !wantError {
		observed.Facts = append(observed.Facts, runtimecontract.Fact{Kind: "run_completed", Sequence: 2})
		observed.Outcome.FinishReason = "stop"
		observed.Persisted = append(observed.Persisted, contractMessageRecord(2, "assistant:"))
	} else {
		errText := "模型生成失败: provider returned empty response"
		observed.Facts = append(observed.Facts, runtimecontract.Fact{Kind: "run_error", Sequence: 2, Content: errText, IsError: true})
		observed.Outcome.ErrorKind = "provider"
		observed.Outcome.Error = errText
		scenario.Expected.AdapterErrorContains = errText
		scenario.Expected.CompareObservedOnError = true
	}
	scenario.Expected.Observed = observed
	return scenario
}

func runtimeTurnProviderErrorScenario() runtimecontract.Scenario {
	input := contractInput(1)
	errText := "模型生成失败: provider unavailable"
	return runtimecontract.Scenario{
		ID:     "RT-007",
		Input:  input,
		Script: runtimecontract.Script{ModelSteps: []runtimecontract.ModelStep{{Error: "provider unavailable"}}},
		Expected: runtimecontract.Expected{
			AdapterErrorContains:   errText,
			CompareObservedOnError: true,
			Observed: runtimecontract.Observed{
				Requests: []runtimecontract.ModelRequest{contractRequest(input, "action", contractInitialMessages(input.Prompt), nil)},
				Facts: []runtimecontract.Fact{
					{Kind: "run_started", Sequence: 1},
					{Kind: "run_error", Sequence: 2, Content: errText, IsError: true},
				},
				Outcome:   runtimecontract.Outcome{TurnCount: 1, ErrorKind: "provider", Error: errText},
				Persisted: []runtimecontract.PersistedRecord{contractMessageRecord(1, "user:Return the scripted answer.")},
				Metrics: []runtimecontract.Metric{
					{Kind: "model_call", Turn: 1, Phase: "action", IsError: true},
					{Kind: "run_summary", ModelCalls: 1, ErrorCount: 1},
				},
			},
		},
	}
}

func contractInput(maxTurns int) runtimecontract.RunInput {
	return runtimecontract.RunInput{
		Profile:       "CLIExec",
		Prompt:        "Return the scripted answer.",
		DisplayPrompt: "Return the scripted answer.",
		SessionChoice: "new",
		Model:         "scripted-model",
		Provider:      "scripted",
		MaxTurns:      maxTurns,
	}
}

func contractInitialMessages(prompt string) []runtimecontract.Message {
	return []runtimecontract.Message{{Role: "system", Content: "contract system prompt"}, {Role: "user", Content: prompt}}
}

func appendContractMessages(messages []runtimecontract.Message, additional ...runtimecontract.Message) []runtimecontract.Message {
	result := append([]runtimecontract.Message(nil), messages...)
	return append(result, additional...)
}

func contractRequest(input runtimecontract.RunInput, phase string, messages []runtimecontract.Message, definitions []runtimecontract.ToolDefinition) runtimecontract.ModelRequest {
	return runtimecontract.ModelRequest{
		Phase: phase, Messages: messages, ToolDefinitions: definitions,
		Model: input.Model, Provider: input.Provider, Effort: input.Effort,
	}
}

func contractToolDefinition(name string) runtimecontract.ToolDefinition {
	return runtimecontract.ToolDefinition{Name: name, Description: name + " scripted tool", InputSchema: `{}`}
}

func contractMessageRecord(order int, content string) runtimecontract.PersistedRecord {
	return runtimecontract.PersistedRecord{Kind: "message", Path: "messages.jsonl", Content: content, Order: order}
}
