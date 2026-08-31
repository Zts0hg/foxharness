package turnpolicy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestTargetAdapterPL001PL005OrdersRecoveryOrdinaryAndNextTurn(t *testing.T) {
	calls := []schema.ToolCall{
		{ID: "repeat-1", Name: "repeat", Arguments: json.RawMessage(`{"same":true}`)},
		{ID: "repeat-2", Name: "repeat", Arguments: json.RawMessage(`{"same":true}`)},
		{ID: "repeat-3", Name: "repeat", Arguments: json.RawMessage(`{"same":true}`)},
	}
	invoker := &policyScriptedInvoker{results: []engine.ModelResult{
		toolModelResult(calls[0]), toolModelResult(calls[1]), toolModelResult(calls[2]),
		{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"},
	}}
	conversation := &policyConversation{}
	policy := New(nextTurnConfig(func(turn int) []string {
		if turn == 4 {
			return []string{"next-turn fixture reminder"}
		}
		return nil
	}))
	target := engine.NewAgentEngine(invoker, policyToolExecutor{fail: true}, conversation, policy, nil)

	if _, err := target.Run(context.Background(), engine.RunInput{Prompt: "repeat", MaxTurns: 4}); err != nil {
		t.Fatal(err)
	}
	if len(conversation.requests) != 4 {
		t.Fatalf("requests = %d, want 4", len(conversation.requests))
	}
	for requestIndex, wantCount := range []string{"1", "2", "3"} {
		request := conversation.requests[requestIndex+1]
		if got := countMessageContent(request, "## Error Recovery Notice"); got != requestIndex+1 {
			t.Fatalf("request %d recovery notices = %d, want %d", requestIndex+2, got, requestIndex+1)
		}
		if messageIndex(request, "Failure count for same tool+arguments: "+wantCount) < 0 {
			t.Fatalf("request %d missing recovery count %s: %#v", requestIndex+2, wantCount, request)
		}
	}
	request := conversation.requests[3]
	resultIndex := messageIndex(request, "structured failure")
	recoveryIndex := messageIndex(request, "Failure count for same tool+arguments: 3")
	ordinaryIndex := messageIndex(request, "Possible Loop Detected")
	nextTurnIndex := messageIndex(request, "next-turn fixture reminder")
	if !(resultIndex >= 0 && resultIndex < recoveryIndex && recoveryIndex < ordinaryIndex && ordinaryIndex < nextTurnIndex) {
		t.Fatalf("target policy order = result:%d recovery:%d ordinary:%d next:%d\n%#v", resultIndex, recoveryIndex, ordinaryIndex, nextTurnIndex, request)
	}
	if !strings.Contains(request[recoveryIndex].Content, "禁止再次原样调用") {
		t.Fatalf("third recovery lacks strong directive: %q", request[recoveryIndex].Content)
	}
}

func TestTargetAdapterPL003CommitsTerminalAssistantBeforePolicyError(t *testing.T) {
	invoker := &policyScriptedInvoker{results: []engine.ModelResult{
		{Message: schema.Message{Role: schema.RoleAssistant, Content: "first premature"}, FinishReason: "stop"},
		{Message: schema.Message{Role: schema.RoleAssistant, Content: "second premature"}, FinishReason: "stop"},
	}}
	conversation := &policyConversation{}
	policy := New(completionGateConfig(func() string { return "submit_plan is still required" }))
	target := engine.NewAgentEngine(invoker, policyToolExecutor{}, conversation, policy, nil)

	outcome, err := target.Run(context.Background(), engine.RunInput{Prompt: "finish", MaxTurns: 3})
	if err == nil || !strings.Contains(err.Error(), "completion gate remained unsatisfied") {
		t.Fatalf("outcome/error = %#v, %v", outcome, err)
	}
	if !outcome.Partial {
		t.Fatalf("completion gate outcome partial = false, want legacy partial result: %#v", outcome)
	}
	firstIndex := messageIndex(conversation.messages, "first premature")
	reminderIndex := messageIndex(conversation.messages, "submit_plan is still required")
	secondIndex := messageIndex(conversation.messages, "second premature")
	if !(firstIndex >= 0 && firstIndex < reminderIndex && reminderIndex < secondIndex) {
		t.Fatalf("terminal completion order = first:%d reminder:%d second:%d\n%#v", firstIndex, reminderIndex, secondIndex, conversation.messages)
	}
}

func TestTargetAdapterPL004FailedTodoUpdateRemainsUnsatisfied(t *testing.T) {
	call := schema.ToolCall{ID: "todo-failed", Name: "update_todo", Arguments: json.RawMessage(`{"content":"done"}`)}
	invoker := &policyScriptedInvoker{results: []engine.ModelResult{
		{Message: schema.Message{Role: schema.RoleAssistant, Content: "first premature"}, FinishReason: "stop"},
		toolModelResult(call),
		{Message: schema.Message{Role: schema.RoleAssistant, Content: "still premature"}, FinishReason: "stop"},
	}}
	conversation := &policyConversation{}
	policy := New(todoGateConfig(func() string { return "TODO.md still has incomplete checklist items" }))
	target := engine.NewAgentEngine(invoker, policyToolExecutor{fail: true}, conversation, policy, nil)

	outcome, err := target.Run(context.Background(), engine.RunInput{Prompt: "finish", MaxTurns: 4})
	if err == nil || !strings.Contains(err.Error(), "TODO.md still has incomplete checklist items after TODO completion reminder") {
		t.Fatalf("outcome/error = %#v, %v", outcome, err)
	}
	if outcome.Partial {
		t.Fatalf("TODO gate outcome partial = true, want legacy nil-result semantics: %#v", outcome)
	}
	if len(conversation.requests) != 3 || messageIndex(conversation.requests[1], "TODO.md still has incomplete") < 0 {
		t.Fatalf("second request lacks TODO reminder: %#v", conversation.requests)
	}
	if messageIndex(conversation.requests[2], "structured failure") < 0 || messageIndex(conversation.requests[2], "Error Recovery Notice") < 0 {
		t.Fatalf("third request lacks failed result/recovery: %#v", conversation.requests[2])
	}
	if messageIndex(conversation.messages, "still premature") < 0 {
		t.Fatalf("terminal assistant was not committed: %#v", conversation.messages)
	}
}

type policyScriptedInvoker struct {
	results  []engine.ModelResult
	position int
}

func (i *policyScriptedInvoker) StartRun(context.Context) (engine.ModelRunInvoker, error) {
	return i, nil
}

func (i *policyScriptedInvoker) Invoke(context.Context, engine.RunContext, engine.ModelFactEmitter) (engine.ModelResult, error) {
	result := i.results[i.position]
	i.position++
	return result, nil
}

type policyConversation struct {
	messages []schema.Message
	requests [][]schema.Message
}

func (c *policyConversation) Prepare(_ context.Context, request engine.ConversationRequest) (engine.ConversationProjection, error) {
	input := request.Input
	if c.messages == nil {
		c.messages = []schema.Message{{Role: schema.RoleUser, Content: input.Prompt}}
	}
	messages := clonePolicyMessages(c.messages)
	c.requests = append(c.requests, messages)
	return engine.ConversationProjection{Context: engine.RunContext{Messages: messages}}, nil
}

func (c *policyConversation) RequestChanges(_ context.Context, changes []engine.ConversationChange) error {
	for _, change := range changes {
		c.messages = append(c.messages, clonePolicyMessage(change.Message))
	}
	return nil
}

type policyToolSnapshot struct{}

func (policyToolSnapshot) ToolDefinitions() []schema.ToolDefinition {
	return []schema.ToolDefinition{{Name: "repeat"}, {Name: "update_todo"}}
}

type policyToolExecutor struct {
	fail bool
}

func (policyToolExecutor) Snapshot(context.Context) (engine.ToolSnapshot, error) {
	return policyToolSnapshot{}, nil
}

func (e policyToolExecutor) Execute(_ context.Context, _ engine.ToolSnapshot, calls []schema.ToolCall) (engine.ToolBatch, error) {
	results := make([]engine.ToolExecutionResult, len(calls))
	for index, call := range calls {
		content := "ok"
		if e.fail {
			content = "structured failure"
		}
		results[index] = engine.ToolExecutionResult{
			CallID: call.ID, FullContent: content, ModelContent: content,
			ObserverContent: content, IsError: e.fail,
		}
	}
	return engine.ToolBatch{Results: results}, nil
}

func toolModelResult(call schema.ToolCall) engine.ModelResult {
	return engine.ModelResult{
		Message:      schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{call}},
		FinishReason: "tool_calls",
	}
}

func clonePolicyMessages(messages []schema.Message) []schema.Message {
	cloned := make([]schema.Message, len(messages))
	for index, message := range messages {
		cloned[index] = clonePolicyMessage(message)
	}
	return cloned
}

func clonePolicyMessage(message schema.Message) schema.Message {
	message.ToolCalls = append([]schema.ToolCall(nil), message.ToolCalls...)
	for index := range message.ToolCalls {
		message.ToolCalls[index].Arguments = append([]byte(nil), message.ToolCalls[index].Arguments...)
	}
	return message
}

func countMessageContent(messages []schema.Message, content string) int {
	count := 0
	for _, message := range messages {
		if strings.Contains(message.Content, content) {
			count++
		}
	}
	return count
}

func messageIndex(messages []schema.Message, content string) int {
	for index, message := range messages {
		if strings.Contains(message.Content, content) {
			return index
		}
	}
	return -1
}

/* TestTargetAdapterRecoveryNoticeCarriesTheTurnThatConsumesIt pins the
 * baseline recovery attribution: the notice is produced at the start of the
 * turn that receives it, so its fact lands on the new turn rather than the
 * turn whose tool failed. */
func TestTargetAdapterRecoveryNoticeCarriesTheTurnThatConsumesIt(t *testing.T) {
	calls := []schema.ToolCall{
		{ID: "repeat-1", Name: "repeat", Arguments: json.RawMessage(`{"same":true}`)},
		{ID: "repeat-2", Name: "repeat", Arguments: json.RawMessage(`{"same":true}`)},
	}
	invoker := &policyScriptedInvoker{results: []engine.ModelResult{
		toolModelResult(calls[0]), toolModelResult(calls[1]),
		{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"},
	}}
	conversation := &policyConversation{}
	var recoveryTurns []int
	observer := observerFunc(func(_ context.Context, fact engine.Fact) {
		if fact.Kind == engine.FactErrorRecovery {
			recoveryTurns = append(recoveryTurns, fact.Turn)
		}
	})
	target := engine.NewAgentEngine(invoker, policyToolExecutor{fail: true}, conversation, New(Config{}), observer)

	if _, err := target.Run(context.Background(), engine.RunInput{Prompt: "repeat", MaxTurns: 3}); err != nil {
		t.Fatal(err)
	}
	if len(recoveryTurns) != 2 || recoveryTurns[0] != 2 || recoveryTurns[1] != 3 {
		t.Fatalf("recovery fact turns = %v, want the turns that received the notices", recoveryTurns)
	}
}

type observerFunc func(context.Context, engine.Fact)

func (f observerFunc) Observe(ctx context.Context, fact engine.Fact) { f(ctx, fact) }
