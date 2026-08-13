package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestPolicyOrdersRepeatedRecoveryOrdinaryAndNextTurnReminders(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&structuredFailureTool{name: "repeat_failure"})
	calls := []schema.ToolCall{
		{ID: "call-repeat-1", Name: "repeat_failure", Arguments: json.RawMessage(`{"value":"same"}`)},
		{ID: "call-repeat-2", Name: "repeat_failure", Arguments: json.RawMessage(`{"value":"same"}`)},
		{ID: "call-repeat-3", Name: "repeat_failure", Arguments: json.RawMessage(`{"value":"same"}`)},
	}
	modelProvider := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{calls[0]}}},
		{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{calls[1]}}},
		{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{calls[2]}}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	turn := 0
	engine, sess := newToolContractEngine(t, modelProvider, registry, Config{
		MaxTurns: 5,
		NextTurnReminders: func() []string {
			turn++
			if turn == 4 {
				return []string{"next-turn fixture reminder"}
			}
			return nil
		},
	})
	if _, err := engine.RunWithReporter(context.Background(), sess, "repeat", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if len(modelProvider.seen) != 4 {
		t.Fatalf("model requests = %d, want 4", len(modelProvider.seen))
	}
	for requestIndex, wantCount := range []string{"1", "2", "3"} {
		request := modelProvider.seen[requestIndex+1]
		var recoveryCount int
		for _, message := range request {
			if strings.Contains(message.Content, "## Error Recovery Notice") {
				recoveryCount++
			}
		}
		if recoveryCount != requestIndex+1 {
			t.Fatalf("request %d recovery notices = %d, want %d", requestIndex+2, recoveryCount, requestIndex+1)
		}
		if !messagesContain(request, "Failure count for same tool+arguments: "+wantCount) {
			t.Fatalf("request %d missing current repeated-failure count %s", requestIndex+2, wantCount)
		}
	}

	request := modelProvider.seen[3]
	resultIndex := messageIndexContaining(request, "structured failure")
	recoveryIndex := messageIndexContaining(request, "Failure count for same tool+arguments: 3")
	ordinaryIndex := messageIndexContaining(request, "Possible Loop Detected")
	nextTurnIndex := messageIndexContaining(request, "next-turn fixture reminder")
	if !(resultIndex >= 0 && resultIndex < recoveryIndex && recoveryIndex < ordinaryIndex && ordinaryIndex < nextTurnIndex) {
		t.Fatalf("fourth request policy order = result:%d recovery:%d ordinary:%d next:%d\n%#v", resultIndex, recoveryIndex, ordinaryIndex, nextTurnIndex, request)
	}
	if !strings.Contains(request[recoveryIndex].Content, "禁止再次原样调用") {
		t.Fatalf("third repeated failure lacks strong recovery directive: %q", request[recoveryIndex].Content)
	}
}

func TestPolicyFailedTodoUpdateCannotSatisfyCompletionGate(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	todoPath := filepath.Join(sess.RootDir, "TODO.md")
	if err := os.WriteFile(todoPath, []byte("# TODO\n\n- [ ] Finish policy coverage\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(TODO.md) error = %v", err)
	}
	registry := tools.NewRegistry()
	registry.Register(&structuredFailureTool{name: "update_todo"})
	call := schema.ToolCall{ID: "call-update-failed", Name: "update_todo", Arguments: json.RawMessage(`{"content":"# TODO\n\n- [x] Finish policy coverage\n"}`)}
	modelProvider := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "premature"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{call}}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "still premature"}},
	}}
	reporter := &currentContractReporter{}
	engine := NewLegacyEngine(modelProvider, registry, workDir, staticComposer{}, Config{MaxTurns: 4})
	result, runErr := engine.RunWithReporter(context.Background(), sess, "finish", reporter)
	if runErr == nil || !strings.Contains(runErr.Error(), "TODO.md still has incomplete checklist items after TODO completion reminder") {
		t.Fatalf("RunWithReporter() result/error = %#v, %v", result, runErr)
	}
	if result != nil || modelProvider.call != 3 {
		t.Fatalf("result/provider calls = %#v/%d, want nil/3", result, modelProvider.call)
	}
	if !messagesContain(modelProvider.seen[1], "TODO.md still has incomplete checklist items") {
		t.Fatalf("second request missing initial TODO reminder: %#v", modelProvider.seen[1])
	}
	if !messagesContain(modelProvider.seen[2], "structured failure") || !messagesContain(modelProvider.seen[2], "Error Recovery Notice") {
		t.Fatalf("third request missing failed update result or recovery: %#v", modelProvider.seen[2])
	}
	data, err := os.ReadFile(todoPath)
	if err != nil {
		t.Fatalf("ReadFile(TODO.md) error = %v", err)
	}
	if !strings.Contains(string(data), "- [ ] Finish policy coverage") {
		t.Fatalf("failed update mutated TODO.md:\n%s", data)
	}
	var failedResult bool
	for _, fact := range reporter.facts {
		if fact.Kind == "tool_result" && fact.CallID == call.ID && fact.IsError {
			failedResult = true
		}
	}
	if !failedResult {
		t.Fatalf("reporter facts missing failed correlated update: %#v", reporter.facts)
	}
}

func messageIndexContaining(messages []schema.Message, value string) int {
	for index, message := range messages {
		if strings.Contains(message.Content, value) {
			return index
		}
	}
	return -1
}
