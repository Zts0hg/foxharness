package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Zts0hg/foxharness/internal/schema"
)

/* Phase identifies one model-visible phase within a turn. */
type Phase string

const (
	/* PhaseAction identifies a model invocation that may produce a user response or tool calls. */
	PhaseAction Phase = "action"
	/* PhaseThinking identifies a model invocation used for explicit thinking. */
	PhaseThinking Phase = "thinking"
)

/* RunInput contains values that select one target-engine execution. */
type RunInput struct {
	Prompt   string
	MaxTurns int
}

/* RunContext is an immutable model-visible snapshot for one invocation. */
type RunContext struct {
	Phase           Phase
	Messages        []schema.Message
	ToolDefinitions []schema.ToolDefinition
	Model           string
	Provider        string
	Effort          string
}

/* ModelResult contains the normalized result of one model invocation. */
type ModelResult struct {
	Message      schema.Message
	FinishReason string
	Usage        schema.Usage
}

/* ModelInvoker owns provider invocation, streaming, fallback, and normalization. */
type ModelInvoker interface {
	Invoke(context.Context, RunContext) (ModelResult, error)
}

/* ToolSnapshot pairs model-visible definitions with one constrained execution scope. */
type ToolSnapshot interface {
	ToolDefinitions() []schema.ToolDefinition
}

/* ToolBatch contains tool results in the same order as their corresponding calls. */
type ToolBatch struct {
	Results []schema.ToolResult
}

/* ToolExecutor creates constrained snapshots and executes calls against the same snapshot. */
type ToolExecutor interface {
	Snapshot(context.Context) (ToolSnapshot, error)
	Execute(context.Context, ToolSnapshot, []schema.ToolCall) (ToolBatch, error)
}

/* ConversationChangeKind identifies an ordered model-visible context change. */
type ConversationChangeKind string

const (
	/* ConversationAppendMessage requests that one message be appended in order. */
	ConversationAppendMessage ConversationChangeKind = "append_message"
)

/* ConversationChange describes one ordered context change requested by the engine. */
type ConversationChange struct {
	Kind    ConversationChangeKind
	Message schema.Message
}

/* Conversation prepares immutable invocation snapshots and accepts ordered change requests. */
type Conversation interface {
	Prepare(context.Context, RunInput) (RunContext, error)
	Apply(context.Context, []ConversationChange) error
}

/* TurnState contains the run-scoped values available to completion policy. */
type TurnState struct {
	Turn  int
	Model ModelResult
}

/* TurnDecision describes how the engine should transition after a model result. */
type TurnDecision struct {
	Complete bool
}

/* TurnPolicy decides completion and other run-scoped policy transitions. */
type TurnPolicy interface {
	Decide(context.Context, TurnState) (TurnDecision, error)
}

/* FactKind identifies one canonically ordered engine observation. */
type FactKind string

const (
	/* FactRunStarted marks the start of a run. */
	FactRunStarted FactKind = "run_started"
	/* FactMessage carries one complete assistant message. */
	FactMessage FactKind = "message"
	/* FactRunCompleted marks successful terminal completion. */
	FactRunCompleted FactKind = "run_completed"
	/* FactRunError marks failed terminal completion. */
	FactRunError FactKind = "run_error"
)

/* Fact is one typed synchronous observation emitted in canonical order. */
type Fact struct {
	Kind     FactKind
	Sequence int
	Turn     int
	Phase    Phase
	Content  string
	IsError  bool
}

/* Observer synchronously receives each engine fact exactly once. */
type Observer interface {
	Observe(context.Context, Fact)
}

/* RunOutcome contains execution outcome data without runtime or persistence details. */
type RunOutcome struct {
	FinalMessage string
	FinishReason string
	TurnCount    int
	Usage        schema.Usage
	Partial      bool
	ErrorKind    string
	Err          error
}

/* AgentEngine coordinates turn state transitions through immutable collaborators. */
type AgentEngine struct {
	model        ModelInvoker
	tools        ToolExecutor
	conversation Conversation
	policy       TurnPolicy
	observer     Observer
}

/* NewAgentEngine constructs the target turn coordinator without selecting infrastructure. */
func NewAgentEngine(
	model ModelInvoker,
	tools ToolExecutor,
	conversation Conversation,
	policy TurnPolicy,
	observer Observer,
) *AgentEngine {
	return &AgentEngine{
		model:        model,
		tools:        tools,
		conversation: conversation,
		policy:       policy,
		observer:     observer,
	}
}

/* Run executes target-engine turn transitions without retaining run-scoped state. */
func (e *AgentEngine) Run(ctx context.Context, input RunInput) (RunOutcome, error) {
	if err := e.validate(); err != nil {
		return RunOutcome{ErrorKind: "configuration", Err: err}, err
	}

	sequence := 0
	emit := func(fact Fact) {
		sequence++
		fact.Sequence = sequence
		if e.observer != nil {
			e.observer.Observe(ctx, fact)
		}
	}
	emit(Fact{Kind: FactRunStarted})

	snapshot, err := e.tools.Snapshot(ctx)
	if err != nil {
		return e.fail(ctx, emit, RunOutcome{}, "tool", err)
	}
	runContext, err := e.conversation.Prepare(ctx, input)
	if err != nil {
		return e.fail(ctx, emit, RunOutcome{}, "conversation", err)
	}
	if runContext.Phase == "" {
		runContext.Phase = PhaseAction
	}
	runContext.ToolDefinitions = cloneToolDefinitions(snapshot.ToolDefinitions())

	outcome := RunOutcome{TurnCount: 1}
	modelResult, err := e.model.Invoke(ctx, cloneRunContext(runContext))
	if err != nil {
		return e.fail(ctx, emit, outcome, "provider", fmt.Errorf("模型生成失败: %w", err))
	}
	outcome.FinishReason = modelResult.FinishReason
	outcome.Usage = modelResult.Usage

	decision, err := e.policy.Decide(ctx, TurnState{Turn: 1, Model: cloneModelResult(modelResult)})
	if err != nil {
		return e.fail(ctx, emit, outcome, "policy", err)
	}
	if err := e.conversation.Apply(ctx, []ConversationChange{{
		Kind:    ConversationAppendMessage,
		Message: schema.NormalizeMessage(modelResult.Message),
	}}); err != nil {
		return e.fail(ctx, emit, outcome, "conversation", err)
	}
	if modelResult.Message.Content != "" {
		emit(Fact{Kind: FactMessage, Content: modelResult.Message.Content})
	}
	if decision.Complete {
		outcome.FinalMessage = modelResult.Message.Content
		emit(Fact{Kind: FactRunCompleted, Content: outcome.FinalMessage})
		return outcome, nil
	}

	err = errors.New("target engine requires another turn")
	return e.fail(ctx, emit, outcome, "turn_limit", err)
}

func (e *AgentEngine) validate() error {
	switch {
	case e == nil:
		return errors.New("target engine is required")
	case e.model == nil:
		return errors.New("model invoker is required")
	case e.tools == nil:
		return errors.New("tool executor is required")
	case e.conversation == nil:
		return errors.New("conversation is required")
	case e.policy == nil:
		return errors.New("turn policy is required")
	default:
		return nil
	}
}

func (e *AgentEngine) fail(_ context.Context, emit func(Fact), outcome RunOutcome, kind string, err error) (RunOutcome, error) {
	outcome.ErrorKind = kind
	outcome.Err = err
	emit(Fact{Kind: FactRunError, Content: err.Error(), IsError: true})
	return outcome, err
}

func cloneRunContext(runContext RunContext) RunContext {
	runContext.Messages = cloneMessages(runContext.Messages)
	runContext.ToolDefinitions = cloneToolDefinitions(runContext.ToolDefinitions)
	return runContext
}

func cloneModelResult(result ModelResult) ModelResult {
	result.Message = cloneMessage(result.Message)
	return result
}

func cloneMessages(messages []schema.Message) []schema.Message {
	result := make([]schema.Message, len(messages))
	for index, message := range messages {
		result[index] = cloneMessage(message)
	}
	return result
}

func cloneMessage(message schema.Message) schema.Message {
	message.ToolCalls = append([]schema.ToolCall(nil), message.ToolCalls...)
	for index := range message.ToolCalls {
		message.ToolCalls[index].Arguments = append([]byte(nil), message.ToolCalls[index].Arguments...)
	}
	if message.Usage != nil {
		usage := *message.Usage
		message.Usage = &usage
	}
	return message
}

func cloneToolDefinitions(definitions []schema.ToolDefinition) []schema.ToolDefinition {
	result := make([]schema.ToolDefinition, len(definitions))
	for index, definition := range definitions {
		definition.InputSchema = cloneJSONValue(definition.InputSchema)
		result[index] = definition
	}
	return result
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = cloneJSONValue(item)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index, item := range value {
			result[index] = cloneJSONValue(item)
		}
		return result
	case []string:
		return append([]string(nil), value...)
	case json.RawMessage:
		return append(json.RawMessage(nil), value...)
	case []byte:
		return append([]byte(nil), value...)
	default:
		return value
	}
}
