package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/testsupport/runtimecontract"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestCurrentProductionContractAdapterRunsStreamingCatalog(t *testing.T) {
	for _, testCase := range currentStreamingScenarios() {
		t.Run(testCase.name, func(t *testing.T) {
			if err := runtimecontract.VerifyScenario(context.Background(), newCurrentProductionContractAdapter(t), testCase.scenario); err != nil {
				t.Fatalf("VerifyScenario() error = %v", err)
			}
		})
	}
}

func currentStreamingScenarios() []runtimeTurnTestCase {
	return []runtimeTurnTestCase{
		{name: "ST-001 successful deltas", scenario: streamingSuccessScenario()},
		{name: "ST-002 unsupported pre-delta fallback", scenario: streamingFallbackScenario("ST-002", "unsupported")},
		{name: "ST-002 empty pre-delta fallback", scenario: streamingFallbackScenario("ST-002", "empty")},
		{name: "ST-003 retryable start fallback", scenario: streamingFallbackScenario("ST-003", "retryable")},
		{name: "ST-004 post-delta failure", scenario: streamingPostDeltaFailureScenario()},
		{name: "ST-005 disabled stream in later turn", scenario: streamingDisabledScenario()},
	}
}

func streamingSuccessScenario() runtimecontract.Scenario {
	input := contractInput(1)
	response := runtimecontract.ModelResponse{Content: "hello world", FinishReason: "stop"}
	return runtimecontract.Scenario{
		ID: "ST-001", Input: input,
		Script: runtimecontract.Script{ModelSteps: []runtimecontract.ModelStep{{Deltas: []string{"hello ", "world"}, Response: response}}},
		Expected: runtimecontract.Expected{Observed: runtimecontract.Observed{
			Requests: []runtimecontract.ModelRequest{streamingRequest(input, "stream", contractInitialMessages(input.Prompt), nil)},
			Facts: []runtimecontract.Fact{
				{Kind: "run_started", Sequence: 1},
				{Kind: "message_delta", Sequence: 2, Content: "hello "},
				{Kind: "message_delta", Sequence: 3, Content: "world"},
				{Kind: "message", Sequence: 4, Content: response.Content},
				{Kind: "run_completed", Sequence: 5, Content: response.Content},
			},
			Outcome:   runtimecontract.Outcome{FinalMessage: response.Content, FinishReason: "stop", TurnCount: 1},
			Persisted: []runtimecontract.PersistedRecord{contractMessageRecord(1, "user:Return the scripted answer."), contractMessageRecord(2, "assistant:hello world")},
			Metrics:   []runtimecontract.Metric{{Kind: "model_call", Turn: 1, Phase: "action"}, {Kind: "run_summary", ModelCalls: 1}},
		}},
	}
}

func streamingFallbackScenario(id, errorKind string) runtimecontract.Scenario {
	input := contractInput(1)
	response := runtimecontract.ModelResponse{Content: "fallback answer", FinishReason: "stop"}
	return runtimecontract.Scenario{
		ID: id, Input: input,
		Script: runtimecontract.Script{ModelSteps: []runtimecontract.ModelStep{
			{Error: "stream start failed", ErrorKind: errorKind},
			{Response: response},
		}},
		Expected: runtimecontract.Expected{Observed: runtimecontract.Observed{
			Requests: []runtimecontract.ModelRequest{
				streamingRequest(input, "stream", contractInitialMessages(input.Prompt), nil),
				streamingRequest(input, "non_stream", contractInitialMessages(input.Prompt), nil),
			},
			Facts:     []runtimecontract.Fact{{Kind: "run_started", Sequence: 1}, {Kind: "message", Sequence: 2, Content: response.Content}, {Kind: "run_completed", Sequence: 3, Content: response.Content}},
			Outcome:   runtimecontract.Outcome{FinalMessage: response.Content, FinishReason: "stop", TurnCount: 1},
			Persisted: []runtimecontract.PersistedRecord{contractMessageRecord(1, "user:Return the scripted answer."), contractMessageRecord(2, "assistant:fallback answer")},
			Metrics:   []runtimecontract.Metric{{Kind: "model_call", Turn: 1, Phase: "action"}, {Kind: "run_summary", ModelCalls: 1}},
		}},
	}
}

func streamingPostDeltaFailureScenario() runtimecontract.Scenario {
	input := contractInput(1)
	errText := "模型生成失败: streaming is not supported: stream interrupted"
	return runtimecontract.Scenario{
		ID: "ST-004", Input: input,
		Script: runtimecontract.Script{ModelSteps: []runtimecontract.ModelStep{{Deltas: []string{"partial"}, Error: "stream interrupted", ErrorKind: "unsupported"}}},
		Expected: runtimecontract.Expected{
			AdapterErrorContains: errText, CompareObservedOnError: true,
			Observed: runtimecontract.Observed{
				Requests: []runtimecontract.ModelRequest{streamingRequest(input, "stream", contractInitialMessages(input.Prompt), nil)},
				Facts: []runtimecontract.Fact{
					{Kind: "run_started", Sequence: 1},
					{Kind: "message_delta", Sequence: 2, Content: "partial"},
					{Kind: "run_error", Sequence: 3, Content: errText, IsError: true},
				},
				Outcome:   runtimecontract.Outcome{TurnCount: 1, ErrorKind: "provider", Error: errText},
				Persisted: []runtimecontract.PersistedRecord{contractMessageRecord(1, "user:Return the scripted answer.")},
				Metrics:   []runtimecontract.Metric{{Kind: "model_call", Turn: 1, Phase: "action", IsError: true}, {Kind: "run_summary", ModelCalls: 1, ErrorCount: 1}},
			},
		},
	}
}

func streamingDisabledScenario() runtimecontract.Scenario {
	input := contractInput(3)
	definition := contractToolDefinition("echo")
	call := runtimecontract.ToolCall{ID: "call-stream", Name: "echo", Arguments: `{}`}
	toolResponse := runtimecontract.ModelResponse{ToolCalls: []runtimecontract.ToolCall{call}, FinishReason: "tool_calls"}
	finalResponse := runtimecontract.ModelResponse{Content: "done", FinishReason: "stop"}
	initial := contractInitialMessages(input.Prompt)
	followUp := appendContractMessages(initial, runtimecontract.Message{Role: "assistant", ToolCalls: []runtimecontract.ToolCall{call}}, runtimecontract.Message{Role: "user", Content: "ok", ToolCallID: call.ID})
	return runtimecontract.Scenario{
		ID: "ST-005", Input: input,
		Script: runtimecontract.Script{
			ModelSteps: []runtimecontract.ModelStep{{Error: "empty", ErrorKind: "empty"}, {Response: toolResponse}, {Response: finalResponse}},
			Tools:      []runtimecontract.ToolBehavior{{Call: call, Definition: definition, Result: runtimecontract.ToolResult{Output: "ok", ModelContent: "ok"}}},
		},
		Expected: runtimecontract.Expected{Observed: runtimecontract.Observed{
			Requests: []runtimecontract.ModelRequest{
				streamingRequest(input, "stream", initial, []runtimecontract.ToolDefinition{definition}),
				streamingRequest(input, "non_stream", initial, []runtimecontract.ToolDefinition{definition}),
				streamingRequest(input, "non_stream", followUp, []runtimecontract.ToolDefinition{definition}),
			},
			Facts: []runtimecontract.Fact{
				{Kind: "run_started", Sequence: 1}, {Kind: "tool_call", Sequence: 2, CallID: call.ID, Name: call.Name, Content: call.Arguments},
				{Kind: "tool_result", Sequence: 3, CallID: call.ID, Name: call.Name, Content: "ok"},
				{Kind: "message", Sequence: 4, Content: "done"}, {Kind: "run_completed", Sequence: 5, Content: "done"},
			},
			Outcome: runtimecontract.Outcome{FinalMessage: "done", FinishReason: "stop", TurnCount: 2},
			Persisted: []runtimecontract.PersistedRecord{
				contractMessageRecord(1, "user:Return the scripted answer."), contractMessageRecord(2, "assistant:|tool_call:call-stream:echo:{}"),
				contractMessageRecord(3, "tool_result:call-stream:ok"), contractMessageRecord(4, "assistant:done"),
			},
			Metrics: []runtimecontract.Metric{
				{Kind: "model_call", Turn: 1, Phase: "action"}, {Kind: "tool_call", Turn: 1, ToolName: "echo", CallID: call.ID},
				{Kind: "model_call", Turn: 2, Phase: "action"}, {Kind: "run_summary", ModelCalls: 2, ToolCalls: 1},
			},
		}},
	}
}

func streamingRequest(input runtimecontract.RunInput, transport string, messages []runtimecontract.Message, definitions []runtimecontract.ToolDefinition) runtimecontract.ModelRequest {
	request := contractRequest(input, "action", messages, definitions)
	request.Transport = transport
	return request
}

func TestCurrentProductionStreamingFailureDoesNotLeakAcrossRuns(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	provider := &currentContractProvider{
		steps: []runtimecontract.ModelStep{
			{Deltas: []string{"partial"}, Error: "stream interrupted", ErrorKind: "ordinary"},
			{Deltas: []string{"clean"}, Response: runtimecontract.ModelResponse{Content: "clean", FinishReason: "stop"}},
		},
		model: "scripted-model", protocol: "scripted", streaming: true,
	}
	eng := NewAgentEngine(provider, tools.NewRegistry(), workDir, currentContractPromptComposer{}, Config{MaxTurns: 1})
	firstReporter := &currentContractStreamingReporter{currentContractReporter: &currentContractReporter{}}
	if _, err := eng.RunWithReporter(context.Background(), sess, "first", firstReporter); err == nil || !strings.Contains(err.Error(), "stream interrupted") {
		t.Fatalf("first run error = %v", err)
	}
	secondReporter := &currentContractStreamingReporter{currentContractReporter: &currentContractReporter{}}
	result, err := eng.RunWithReporter(context.Background(), sess, "second", secondReporter)
	if err != nil || result.FinalMessage != "clean" {
		t.Fatalf("second run result/error = %#v, %v", result, err)
	}
	if got := provider.requests; len(got) != 2 || got[0].Transport != "stream" || got[1].Transport != "stream" {
		t.Fatalf("cross-run requests = %#v", got)
	}
	if len(secondReporter.facts) != 4 || secondReporter.facts[1].Kind != "message_delta" || secondReporter.facts[1].Content != "clean" {
		t.Fatalf("second run facts = %#v", secondReporter.facts)
	}
}
