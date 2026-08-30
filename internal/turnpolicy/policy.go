/*
Package turnpolicy implements run-scoped recovery, reminder, and completion
policies for the target engine without owning persistence or runtime state.
*/
package turnpolicy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/recovery"
	"github.com/Zts0hg/foxharness/internal/reminder"
	"github.com/Zts0hg/foxharness/internal/schema"
)

/* Bindings contains runtime-owned queries bound to exactly one engine run. */
type Bindings struct {
	CompletionGate func(context.Context) (string, error)
	TODOGate       func(context.Context) (string, error)
	NextTurn       func(context.Context, int) ([]string, error)
}

/* Config supplies the factory used to create isolated run query bindings. */
type Config struct {
	Bind func(context.Context, engine.RunInput) (Bindings, error)
}

/* Policy is an immutable factory for isolated per-run policy state. */
type Policy struct {
	config Config
}

/* New creates a reusable policy factory from runtime-owned queries. */
func New(config Config) *Policy {
	return &Policy{config: config}
}

/* StartRun creates fresh recovery, reminder, completion, and TODO state. */
func (p *Policy) StartRun(ctx context.Context, input engine.RunInput) (engine.TurnRunPolicy, error) {
	if p == nil {
		return nil, errors.New("turn policy is required")
	}
	bindings := Bindings{}
	if p.config.Bind != nil {
		var err error
		bindings, err = p.config.Bind(ctx, input)
		if err != nil {
			return nil, err
		}
	}
	return &runPolicy{
		bindings: bindings,
		recovery: recovery.NewTracker(),
		reminder: reminder.NewManager(),
	}, nil
}

type runPolicy struct {
	bindings Bindings

	recovery *recovery.Tracker
	reminder *reminder.Manager

	completionReminder string
	todoReminderSent   bool
	todoUpdated        bool
}

func (p *runPolicy) BeforeTurn(ctx context.Context, state engine.TurnState) (engine.PolicyChanges, error) {
	var changes []engine.ConversationChange
	if p.recovery.ShouldInject() {
		if prompt := p.recovery.BuildPrompt(); prompt != "" {
			p.recovery.MarkInject()
			changes = append(changes, contextNotice(prompt))
		}
	}
	if message, ok := p.reminder.MaybeBuild(state.Turn); ok {
		changes = append(changes, contextReminder(engine.ConversationSourceReminder, message))
	}
	if p.bindings.NextTurn != nil {
		messages, err := p.bindings.NextTurn(ctx, state.Turn)
		if err != nil {
			return engine.PolicyChanges{}, err
		}
		for _, message := range messages {
			if strings.TrimSpace(message) == "" {
				continue
			}
			changes = append(changes, contextReminder(engine.ConversationSourceNextTurnReminder, message))
		}
	}
	return engine.PolicyChanges{Changes: changes}, nil
}

func (p *runPolicy) AfterModel(ctx context.Context, state engine.TurnState) (engine.TurnDecision, error) {
	if len(state.Model.Message.ToolCalls) > 0 {
		return engine.TurnDecision{}, nil
	}

	if p.bindings.CompletionGate != nil {
		message, err := p.bindings.CompletionGate(ctx)
		if err != nil {
			return engine.TurnDecision{}, err
		}
		if message = strings.TrimSpace(message); message != "" {
			if p.completionReminder == message {
				return terminalDecision(fmt.Errorf("completion gate remained unsatisfied after reminder: %s", message), true), nil
			}
			p.completionReminder = message
			return engine.TurnDecision{Changes: []engine.ConversationChange{contextReminder(engine.ConversationSourceCompletionGate, message)}}, nil
		}
	}

	if !p.todoUpdated && p.bindings.TODOGate != nil {
		message, err := p.bindings.TODOGate(ctx)
		if err != nil {
			return engine.TurnDecision{}, err
		}
		if message = strings.TrimSpace(message); message != "" {
			if p.todoReminderSent {
				return terminalDecision(errors.New("TODO.md still has incomplete checklist items after TODO completion reminder"), false), nil
			}
			p.todoReminderSent = true
			return engine.TurnDecision{Changes: []engine.ConversationChange{contextReminder(engine.ConversationSourceTODOGate, message)}}, nil
		}
	}

	return engine.TurnDecision{Complete: true}, nil
}

func (p *runPolicy) AfterTools(_ context.Context, state engine.ToolState) (engine.PolicyChanges, error) {
	if len(state.Calls) != len(state.Results) {
		return engine.PolicyChanges{}, fmt.Errorf("policy tool result count = %d, want %d", len(state.Results), len(state.Calls))
	}
	for index, call := range state.Calls {
		result := state.Results[index]
		if result.CallID != "" && result.CallID != call.ID {
			return engine.PolicyChanges{}, fmt.Errorf("policy tool result %d call ID = %q, want %q", index, result.CallID, call.ID)
		}
		policyResult := schema.ToolResult{ToolCallID: call.ID, Output: result.ModelContent, IsError: result.IsError}
		p.reminder.Record(state.Turn, call, policyResult)
		p.recovery.Record(call, policyResult)
		if call.Name == "update_todo" && !result.IsError {
			p.todoUpdated = true
		}
	}
	return engine.PolicyChanges{}, nil
}

func contextNotice(message string) engine.ConversationChange {
	return contextChange(engine.ConversationSourceRecovery, "[Runtime System Notice]\n\n"+message)
}

func contextReminder(source engine.ConversationChangeSource, message string) engine.ConversationChange {
	return contextChange(source, "[Runtime System Reminder]\n\n"+message)
}

func contextChange(source engine.ConversationChangeSource, content string) engine.ConversationChange {
	return engine.ConversationChange{
		Kind:    engine.ConversationAppendContextMessage,
		Source:  source,
		Message: schema.Message{Role: schema.RoleUser, Content: content},
	}
}

func terminalDecision(err error, partial bool) engine.TurnDecision {
	return engine.TurnDecision{Terminal: &engine.PolicyTerminal{Err: err, Partial: partial}}
}
