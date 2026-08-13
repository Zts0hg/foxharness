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
	Thinking bool
}

/* RunContext is an immutable model-visible snapshot for one invocation. */
type RunContext struct {
	Turn            int
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

/* ModelFactKind identifies one normalized fact produced during model invocation. */
type ModelFactKind string

const (
	/* ModelFactMessageDelta carries one visible assistant text delta. */
	ModelFactMessageDelta ModelFactKind = "message_delta"
)

/* ModelFact is one synchronous normalized model-invocation observation. */
type ModelFact struct {
	Kind    ModelFactKind
	Content string
}

/* ModelFactEmitter synchronously returns normalized invocation facts to the engine. */
type ModelFactEmitter func(ModelFact)

/* ModelRunInvoker owns mutable provider fallback state for exactly one engine run. */
type ModelRunInvoker interface {
	Invoke(context.Context, RunContext, ModelFactEmitter) (ModelResult, error)
}

/* ModelInvoker creates isolated provider invocation state for each engine run. */
type ModelInvoker interface {
	StartRun(context.Context) (ModelRunInvoker, error)
}

/* ToolSnapshot pairs model-visible definitions with one constrained execution scope. */
type ToolSnapshot interface {
	ToolDefinitions() []schema.ToolDefinition
}

/* ToolExecutionResult contains distinct full, model-visible, and observer result forms. */
type ToolExecutionResult struct {
	CallID          string
	FullContent     string
	ModelContent    string
	ObserverContent string
	ArtifactPath    string
	IsError         bool
}

/* ToolBatch contains tool results in the same order as their corresponding calls. */
type ToolBatch struct {
	Results []ToolExecutionResult
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
	/* ConversationAppendContextMessage requests a non-persisted model-visible message. */
	ConversationAppendContextMessage ConversationChangeKind = "append_context_message"
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
	/* FactMessageDelta carries one streamed assistant text delta. */
	FactMessageDelta FactKind = "message_delta"
	/* FactThinking marks the thinking phase for one turn. */
	FactThinking FactKind = "thinking"
	/* FactToolCall carries one ordered model-requested tool invocation. */
	FactToolCall FactKind = "tool_call"
	/* FactToolResult carries one ordered correlated tool result. */
	FactToolResult FactKind = "tool_result"
	/* FactRunCompleted marks successful terminal completion. */
	FactRunCompleted FactKind = "run_completed"
	/* FactRunError marks failed terminal completion. */
	FactRunError FactKind = "run_error"
)

/* Fact is one typed synchronous observation emitted in canonical order. */
type Fact struct {
	Kind         FactKind
	Sequence     int
	Turn         int
	Phase        Phase
	CallID       string
	Name         string
	Content      string
	FullContent  string
	ArtifactPath string
	IsError      bool
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

	modelRun, err := e.model.StartRun(ctx)
	if err != nil {
		return e.fail(emit, RunOutcome{}, "provider", fmt.Errorf("模型生成失败: %w", err))
	}
	snapshot, err := e.tools.Snapshot(ctx)
	if err != nil {
		return e.fail(emit, RunOutcome{}, "tool", err)
	}

	definitions := cloneToolDefinitions(snapshot.ToolDefinitions())
	outcome := RunOutcome{}
	emitModelFact := func(fact ModelFact) {
		if fact.Kind == ModelFactMessageDelta && fact.Content != "" {
			emit(Fact{Kind: FactMessageDelta, Content: fact.Content})
		}
	}

	for turn := 1; ; turn++ {
		outcome.TurnCount = turn
		if input.Thinking {
			emit(Fact{Kind: FactThinking, Turn: turn})
			thinkingContext, err := e.prepareContext(ctx, input, turn, PhaseThinking, nil)
			if err != nil {
				return e.fail(emit, outcome, "conversation", err)
			}
			thinking, err := modelRun.Invoke(ctx, thinkingContext, emitModelFact)
			if err != nil {
				return e.fail(emit, outcome, "provider", fmt.Errorf("模型生成失败: %w", err))
			}
			outcome.Usage = addUsage(outcome.Usage, thinking.Usage)
			if err := e.applyChanges(ctx, []ConversationChange{{
				Kind:    ConversationAppendContextMessage,
				Message: schema.NormalizeMessage(thinking.Message),
			}}); err != nil {
				return e.fail(emit, outcome, "conversation", err)
			}
		}

		runContext, err := e.prepareContext(ctx, input, turn, PhaseAction, definitions)
		if err != nil {
			return e.fail(emit, outcome, "conversation", err)
		}
		modelResult, err := modelRun.Invoke(ctx, runContext, emitModelFact)
		if err != nil {
			return e.fail(emit, outcome, "provider", fmt.Errorf("模型生成失败: %w", err))
		}
		outcome.FinalMessage = modelResult.Message.Content
		outcome.FinishReason = modelResult.FinishReason
		outcome.Usage = addUsage(outcome.Usage, modelResult.Usage)

		decision, err := e.policy.Decide(ctx, TurnState{Turn: turn, Model: cloneModelResult(modelResult)})
		if err != nil {
			return e.fail(emit, outcome, "policy", err)
		}
		if err := e.applyChanges(ctx, []ConversationChange{{
			Kind:    ConversationAppendMessage,
			Message: schema.NormalizeMessage(modelResult.Message),
		}}); err != nil {
			return e.fail(emit, outcome, "conversation", err)
		}
		if modelResult.Message.Content != "" {
			emit(Fact{Kind: FactMessage, Content: modelResult.Message.Content})
		}

		if len(modelResult.Message.ToolCalls) > 0 {
			if err := e.executeTools(ctx, emit, snapshot, modelResult.Message.ToolCalls); err != nil {
				return e.fail(emit, outcome, "tool", err)
			}
		}
		if decision.Complete {
			emit(Fact{Kind: FactRunCompleted, Content: outcome.FinalMessage})
			return outcome, nil
		}
		if input.MaxTurns > 0 && turn >= input.MaxTurns {
			outcome.Partial = true
			err := fmt.Errorf("超过最大 Turn 数限制: %d", input.MaxTurns)
			return e.fail(emit, outcome, "turn_limit", err)
		}
	}
}

func (e *AgentEngine) prepareContext(
	ctx context.Context,
	input RunInput,
	turn int,
	phase Phase,
	definitions []schema.ToolDefinition,
) (RunContext, error) {
	runContext, err := e.conversation.Prepare(ctx, input)
	if err != nil {
		return RunContext{}, err
	}
	runContext.Turn = turn
	runContext.Phase = phase
	runContext.ToolDefinitions = cloneToolDefinitions(definitions)
	return cloneRunContext(runContext), nil
}

func (e *AgentEngine) executeTools(
	ctx context.Context,
	emit func(Fact),
	snapshot ToolSnapshot,
	calls []schema.ToolCall,
) error {
	for _, call := range calls {
		emit(Fact{Kind: FactToolCall, CallID: call.ID, Name: call.Name, Content: string(call.Arguments)})
	}
	batch, err := e.tools.Execute(ctx, snapshot, cloneToolCalls(calls))
	if err != nil {
		return err
	}
	if len(batch.Results) != len(calls) {
		return fmt.Errorf("tool result count = %d, want %d", len(batch.Results), len(calls))
	}
	changes := make([]ConversationChange, 0, len(batch.Results))
	for index, result := range batch.Results {
		call := calls[index]
		if result.CallID == "" {
			result.CallID = call.ID
		}
		if result.CallID != call.ID {
			return fmt.Errorf("tool result %d call ID = %q, want %q", index, result.CallID, call.ID)
		}
		observerContent := result.ObserverContent
		if observerContent == "" {
			observerContent = result.ModelContent
		}
		emit(Fact{
			Kind: FactToolResult, CallID: call.ID, Name: call.Name,
			Content: observerContent, FullContent: result.FullContent,
			ArtifactPath: result.ArtifactPath, IsError: result.IsError,
		})
		changes = append(changes, ConversationChange{
			Kind: ConversationAppendMessage,
			Message: schema.Message{
				Role: schema.RoleUser, Content: result.ModelContent, ToolCallID: call.ID,
			},
		})
	}
	if err := e.applyChanges(ctx, changes); err != nil {
		return err
	}
	return ctx.Err()
}

func (e *AgentEngine) applyChanges(ctx context.Context, changes []ConversationChange) error {
	cloned := make([]ConversationChange, len(changes))
	for index, change := range changes {
		change.Message = cloneMessage(change.Message)
		cloned[index] = change
	}
	return e.conversation.Apply(ctx, cloned)
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

func (e *AgentEngine) fail(emit func(Fact), outcome RunOutcome, kind string, err error) (RunOutcome, error) {
	outcome.ErrorKind = kind
	outcome.Err = err
	emit(Fact{Kind: FactRunError, Content: err.Error(), IsError: true})
	return outcome, err
}

func addUsage(left, right schema.Usage) schema.Usage {
	left.InputTokens += right.InputTokens
	left.OutputTokens += right.OutputTokens
	left.CacheCreationTokens += right.CacheCreationTokens
	left.CacheReadTokens += right.CacheReadTokens
	return left
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
	message.ToolCalls = cloneToolCalls(message.ToolCalls)
	if message.Usage != nil {
		usage := *message.Usage
		message.Usage = &usage
	}
	return message
}

func cloneToolCalls(calls []schema.ToolCall) []schema.ToolCall {
	result := append([]schema.ToolCall(nil), calls...)
	for index := range result {
		result[index].Arguments = append([]byte(nil), result[index].Arguments...)
	}
	return result
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
