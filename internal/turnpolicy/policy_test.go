package turnpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestPolicyBindsCallbacksForEachConcurrentRunAndPropagatesErrors(t *testing.T) {
	var bound atomic.Int32
	policy := New(Config{Bind: func(context.Context, engine.RunInput) (Bindings, error) {
		runID := bound.Add(1)
		calls := 0
		return Bindings{CompletionGate: func(context.Context) (string, error) {
			calls++
			if calls == 1 {
				return fmt.Sprintf("run-%d gate", runID), nil
			}
			return "", nil
		}}, nil
	}})
	var group sync.WaitGroup
	for runIndex := 0; runIndex < 8; runIndex++ {
		group.Add(1)
		go func() {
			defer group.Done()
			run, err := policy.StartRun(context.Background(), engine.RunInput{})
			if err != nil {
				t.Errorf("StartRun() error = %v", err)
				return
			}
			state := engine.TurnState{Turn: 1, Model: engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}}}
			decision, err := run.AfterModel(context.Background(), state)
			if err != nil || decision.Complete || len(decision.Changes) != 1 {
				t.Errorf("first decision = %#v, %v", decision, err)
				return
			}
			state.Turn = 2
			decision, err = run.AfterModel(context.Background(), state)
			if err != nil || !decision.Complete {
				t.Errorf("second decision = %#v, %v", decision, err)
			}
		}()
	}
	group.Wait()
	if got := bound.Load(); got != 8 {
		t.Fatalf("Bind() calls = %d, want 8", got)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(Config{Bind: func(ctx context.Context, input engine.RunInput) (Bindings, error) {
		if input.Prompt != "bound input" {
			return Bindings{}, fmt.Errorf("bound prompt = %q", input.Prompt)
		}
		return Bindings{}, ctx.Err()
	}}).StartRun(canceled, engine.RunInput{Prompt: "bound input"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("StartRun() canceled error = %v", err)
	}

	bindErr := errors.New("bind failed")
	if _, err := New(Config{Bind: func(context.Context, engine.RunInput) (Bindings, error) {
		return Bindings{}, bindErr
	}}).StartRun(context.Background(), engine.RunInput{}); !errors.Is(err, bindErr) {
		t.Fatalf("StartRun() error = %v, want bind failure", err)
	}
	queryErr := errors.New("gate query failed")
	run, _ := New(Config{Bind: func(context.Context, engine.RunInput) (Bindings, error) {
		return Bindings{CompletionGate: func(context.Context) (string, error) { return "", queryErr }}, nil
	}}).StartRun(context.Background(), engine.RunInput{})
	if _, err := run.AfterModel(context.Background(), engine.TurnState{Model: engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant}}}); !errors.Is(err, queryErr) {
		t.Fatalf("AfterModel() error = %v, want gate query failure", err)
	}
}

func TestPL001PL005RecoveryOrdinaryAndNextTurnOrder(t *testing.T) {
	policy := New(nextTurnConfig(func(turn int) []string {
		if turn == 4 {
			return []string{"next-turn fixture reminder"}
		}
		return nil
	}))
	run, err := policy.StartRun(context.Background(), engine.RunInput{})
	if err != nil {
		t.Fatal(err)
	}
	call := schema.ToolCall{ID: "call", Name: "repeat", Arguments: json.RawMessage(`{"same":true}`)}
	for turn := 1; turn <= 3; turn++ {
		call.ID = "call-" + strconv.Itoa(turn)
		decision, err := run.AfterTools(context.Background(), engine.ToolState{
			Turn: turn, Calls: []schema.ToolCall{call},
			Results: []engine.ToolExecutionResult{{CallID: call.ID, ModelContent: "structured failure", IsError: true}},
		})
		if err != nil || len(decision.Changes) != 1 || !strings.Contains(decision.Changes[0].Message.Content, "Failure count for same tool+arguments: "+strconv.Itoa(turn)) {
			t.Fatalf("turn %d recovery = %#v, %v", turn, decision, err)
		}
		if decision.Changes[0].Source != engine.ConversationSourceRecovery {
			t.Fatalf("turn %d recovery source = %q", turn, decision.Changes[0].Source)
		}
	}
	decision, err := run.BeforeTurn(context.Background(), engine.TurnState{Turn: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Changes) != 2 || !strings.Contains(decision.Changes[0].Message.Content, "Possible Loop Detected") || !strings.Contains(decision.Changes[1].Message.Content, "next-turn fixture reminder") {
		t.Fatalf("turn-four proposals = %#v", decision.Changes)
	}
	if decision.Changes[0].Source != engine.ConversationSourceReminder || decision.Changes[1].Source != engine.ConversationSourceNextTurnReminder {
		t.Fatalf("turn-four sources = %#v", decision.Changes)
	}
}

func TestPL002ReminderPriorityCooldownReanchorAndVerificationSuppression(t *testing.T) {
	t.Run("loop wins over reanchor and starts cooldown", func(t *testing.T) {
		run, _ := New(Config{}).StartRun(context.Background(), engine.RunInput{})
		call := schema.ToolCall{Name: "bash", Arguments: json.RawMessage(`{"command":"ls"}`)}
		for turn := 1; turn <= 3; turn++ {
			call.ID = "loop-" + strconv.Itoa(turn)
			if _, err := run.AfterTools(context.Background(), successfulToolState(turn, call)); err != nil {
				t.Fatal(err)
			}
		}
		decision, err := run.BeforeTurn(context.Background(), engine.TurnState{Turn: 12})
		if err != nil || len(decision.Changes) != 1 || !strings.Contains(decision.Changes[0].Message.Content, "Possible Loop Detected") || strings.Contains(decision.Changes[0].Message.Content, "Re-anchor") {
			t.Fatalf("priority decision = %#v, %v", decision, err)
		}
		decision, err = run.BeforeTurn(context.Background(), engine.TurnState{Turn: 13})
		if err != nil || len(decision.Changes) != 0 {
			t.Fatalf("cooldown decision = %#v, %v", decision, err)
		}
	})

	t.Run("verification suppresses edit warning but not reanchor", func(t *testing.T) {
		run, _ := New(Config{}).StartRun(context.Background(), engine.RunInput{})
		calls := []schema.ToolCall{
			{ID: "edit", Name: "edit_file", Arguments: json.RawMessage(`{"path":"a.go"}`)},
			{ID: "test", Name: "bash", Arguments: json.RawMessage(`{"command":"go test ./..."}`)},
			{ID: "read-a", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.go"}`)},
			{ID: "read-b", Name: "read_file", Arguments: json.RawMessage(`{"path":"b.go"}`)},
			{ID: "status", Name: "bash", Arguments: json.RawMessage(`{"command":"git status --short"}`)},
		}
		for index, call := range calls {
			if _, err := run.AfterTools(context.Background(), successfulToolState(index+1, call)); err != nil {
				t.Fatal(err)
			}
		}
		decision, err := run.BeforeTurn(context.Background(), engine.TurnState{Turn: 12})
		if err != nil || len(decision.Changes) != 1 || !strings.Contains(decision.Changes[0].Message.Content, "Re-anchor") || strings.Contains(decision.Changes[0].Message.Content, "Verification Needed") {
			t.Fatalf("verified decision = %#v, %v", decision, err)
		}
	})
}

func TestPL003CompletionGateRetriesChangedReminderAndRejectsRepeat(t *testing.T) {
	reminders := []string{"submit_plan is still required", "update_todo is now required", "update_todo is now required"}
	index := 0
	policy := New(completionGateConfig(func() string {
		reminder := reminders[index]
		index++
		return reminder
	}))
	run, _ := policy.StartRun(context.Background(), engine.RunInput{})
	state := engine.TurnState{Model: engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "premature"}}}
	for turn := 1; turn <= 2; turn++ {
		state.Turn = turn
		decision, err := run.AfterModel(context.Background(), state)
		if err != nil || decision.Complete || len(decision.Changes) != 1 || !strings.Contains(decision.Changes[0].Message.Content, reminders[turn-1]) {
			t.Fatalf("turn %d decision = %#v, %v", turn, decision, err)
		}
		if decision.Changes[0].Source != engine.ConversationSourceCompletionGate {
			t.Fatalf("turn %d completion source = %q", turn, decision.Changes[0].Source)
		}
	}
	state.Turn = 3
	decision, err := run.AfterModel(context.Background(), state)
	if err != nil || decision.Terminal == nil || !decision.Terminal.Partial || !strings.Contains(decision.Terminal.Err.Error(), "completion gate remained unsatisfied") {
		t.Fatalf("repeated reminder terminal = %#v, %v", decision.Terminal, err)
	}
}

func TestPL003CompletionGateAllowsCompletionWhenConditionClears(t *testing.T) {
	blocked := true
	policy := New(completionGateConfig(func() string {
		if blocked {
			return "submit_plan is still required"
		}
		return ""
	}))
	run, _ := policy.StartRun(context.Background(), engine.RunInput{})
	state := engine.TurnState{Turn: 1, Model: engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "premature"}}}
	decision, err := run.AfterModel(context.Background(), state)
	if err != nil || decision.Complete || len(decision.Changes) != 1 {
		t.Fatalf("blocked decision = %#v, %v", decision, err)
	}
	blocked = false
	state.Turn = 2
	decision, err = run.AfterModel(context.Background(), state)
	if err != nil || !decision.Complete || len(decision.Changes) != 0 {
		t.Fatalf("cleared decision = %#v, %v", decision, err)
	}
}

func TestPL004FailedTodoUpdateCannotSatisfyGateAndStateIsRunScoped(t *testing.T) {
	incomplete := true
	policy := New(todoGateConfig(func() string {
		if incomplete {
			return "TODO.md still has incomplete checklist items"
		}
		return ""
	}))
	run, _ := policy.StartRun(context.Background(), engine.RunInput{})
	call := schema.ToolCall{ID: "todo", Name: "update_todo", Arguments: json.RawMessage(`{}`)}
	if _, err := run.AfterTools(context.Background(), engine.ToolState{Turn: 1, Calls: []schema.ToolCall{call}, Results: []engine.ToolExecutionResult{{CallID: call.ID, IsError: true}}}); err != nil {
		t.Fatal(err)
	}
	state := engine.TurnState{Turn: 2, Model: engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}}}
	decision, err := run.AfterModel(context.Background(), state)
	if err != nil || decision.Complete || len(decision.Changes) != 1 {
		t.Fatalf("failed update decision = %#v, %v", decision, err)
	}
	if decision.Changes[0].Source != engine.ConversationSourceTODOGate {
		t.Fatalf("TODO source = %q", decision.Changes[0].Source)
	}
	decision, err = run.AfterModel(context.Background(), state)
	if err != nil || decision.Terminal == nil || decision.Terminal.Partial || !strings.Contains(decision.Terminal.Err.Error(), "TODO.md still has incomplete") {
		t.Fatalf("repeated TODO gate terminal = %#v, %v", decision.Terminal, err)
	}

	fresh, _ := policy.StartRun(context.Background(), engine.RunInput{})
	decision, err = fresh.AfterModel(context.Background(), state)
	if err != nil || decision.Complete || len(decision.Changes) != 1 {
		t.Fatalf("fresh run inherited prior gate state: %#v, %v", decision, err)
	}
}

func TestPL004SuccessfulTodoUpdateSatisfiesGate(t *testing.T) {
	policy := New(todoGateConfig(func() string { return "TODO.md still has incomplete checklist items" }))
	run, _ := policy.StartRun(context.Background(), engine.RunInput{})
	state := engine.TurnState{Turn: 1, Model: engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "premature"}}}
	decision, err := run.AfterModel(context.Background(), state)
	if err != nil || decision.Complete || len(decision.Changes) != 1 {
		t.Fatalf("initial TODO decision = %#v, %v", decision, err)
	}
	call := schema.ToolCall{ID: "todo-success", Name: "update_todo", Arguments: json.RawMessage(`{}`)}
	if _, err := run.AfterTools(context.Background(), successfulToolState(2, call)); err != nil {
		t.Fatal(err)
	}
	state.Turn = 3
	decision, err = run.AfterModel(context.Background(), state)
	if err != nil || !decision.Complete || len(decision.Changes) != 0 {
		t.Fatalf("successful TODO decision = %#v, %v", decision, err)
	}
}

func TestPolicyFactoryIsolatesRecoveryReminderAndCompletionState(t *testing.T) {
	policy := New(completionGateConfig(func() string { return "still required" }))
	first, _ := policy.StartRun(context.Background(), engine.RunInput{})
	call := schema.ToolCall{ID: "first", Name: "repeat", Arguments: json.RawMessage(`{"same":true}`)}
	for turn := 1; turn <= 3; turn++ {
		call.ID = "first-" + strconv.Itoa(turn)
		if _, err := first.AfterTools(context.Background(), failedToolState(turn, call)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := first.BeforeTurn(context.Background(), engine.TurnState{Turn: 4}); err != nil {
		t.Fatal(err)
	}
	final := engine.TurnState{Turn: 4, Model: engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}}}
	if _, err := first.AfterModel(context.Background(), final); err != nil {
		t.Fatal(err)
	}
	terminal, err := first.AfterModel(context.Background(), final)
	if err != nil || terminal.Terminal == nil {
		t.Fatalf("first run repeated completion terminal = %#v, %v", terminal, err)
	}

	second, _ := policy.StartRun(context.Background(), engine.RunInput{})
	call.ID = "second-1"
	recoveryDecision, err := second.AfterTools(context.Background(), failedToolState(1, call))
	if err != nil || len(recoveryDecision.Changes) != 1 || !strings.Contains(recoveryDecision.Changes[0].Message.Content, "Failure count for same tool+arguments: 1") {
		t.Fatalf("second run recovery inherited state: %#v, %v", recoveryDecision, err)
	}
	reminderDecision, err := second.BeforeTurn(context.Background(), engine.TurnState{Turn: 2})
	if err != nil || len(reminderDecision.Changes) != 0 {
		t.Fatalf("second run reminder inherited state: %#v, %v", reminderDecision, err)
	}
	completionDecision, err := second.AfterModel(context.Background(), final)
	if err != nil || len(completionDecision.Changes) != 1 {
		t.Fatalf("second run completion inherited state: %#v, %v", completionDecision, err)
	}
}

func successfulToolState(turn int, call schema.ToolCall) engine.ToolState {
	return engine.ToolState{
		Turn: turn, Calls: []schema.ToolCall{call},
		Results: []engine.ToolExecutionResult{{CallID: call.ID, ModelContent: "ok"}},
	}
}

func failedToolState(turn int, call schema.ToolCall) engine.ToolState {
	state := successfulToolState(turn, call)
	state.Results[0].ModelContent = "structured failure"
	state.Results[0].IsError = true
	return state
}

func completionGateConfig(query func() string) Config {
	return boundConfig(Bindings{CompletionGate: func(context.Context) (string, error) {
		return query(), nil
	}})
}

func todoGateConfig(query func() string) Config {
	return boundConfig(Bindings{TODOGate: func(context.Context) (string, error) {
		return query(), nil
	}})
}

func nextTurnConfig(query func(int) []string) Config {
	return boundConfig(Bindings{NextTurn: func(_ context.Context, turn int) ([]string, error) {
		return query(turn), nil
	}})
}

func boundConfig(bindings Bindings) Config {
	return Config{Bind: func(context.Context, engine.RunInput) (Bindings, error) {
		return bindings, nil
	}}
}
