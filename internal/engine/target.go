package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/Zts0hg/foxharness/internal/schema"
)

/* Message is the narrow model-protocol message value exposed through engine contracts. */
type Message = schema.Message

/* ToolDefinition is the model-visible tool value exposed through engine contracts. */
type ToolDefinition = schema.ToolDefinition

/* ToolCall is the narrow model-protocol invocation value exposed through engine contracts. */
type ToolCall = schema.ToolCall

/* Role is the narrow model-protocol role exposed through engine contracts. */
type Role = schema.Role

const (
	/* RoleSystem identifies a system instruction message. */
	RoleSystem = schema.RoleSystem
	/* RoleUser identifies a user or tool-result message. */
	RoleUser = schema.RoleUser
	/* RoleAssistant identifies an assistant response message. */
	RoleAssistant = schema.RoleAssistant
)

/* ErrPromptTooLong is the normalized model error that permits one context recovery attempt. */
var ErrPromptTooLong = errors.New("model prompt is too long")

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

/* TurnBoundaryToolExecutor updates dynamic capabilities before a turn snapshot is frozen. */
type TurnBoundaryToolExecutor interface {
	BeginTurn(context.Context) error
}

/* ConversationChangeKind identifies an ordered model-visible context change. */
type ConversationChangeKind string

const (
	/* ConversationAppendMessage requests that one message be appended in order. */
	ConversationAppendMessage ConversationChangeKind = "append_message"
	/* ConversationAppendContextMessage requests a non-persisted model-visible message. */
	ConversationAppendContextMessage ConversationChangeKind = "append_context_message"
)

/* ConversationChangeSource identifies the producer of a non-persisted context proposal. */
type ConversationChangeSource string

const (
	/* ConversationSourceThinking identifies private thinking context. */
	ConversationSourceThinking ConversationChangeSource = "thinking"
	/* ConversationSourceRecovery identifies a failed-tool recovery notice. */
	ConversationSourceRecovery ConversationChangeSource = "recovery"
	/* ConversationSourceReminder identifies an ordinary loop, verification, or re-anchor reminder. */
	ConversationSourceReminder ConversationChangeSource = "reminder"
	/* ConversationSourceNextTurnReminder identifies runtime-queued next-turn content. */
	ConversationSourceNextTurnReminder ConversationChangeSource = "next_turn_reminder"
	/* ConversationSourceCompletionGate identifies a completion-gate continuation. */
	ConversationSourceCompletionGate ConversationChangeSource = "completion_gate"
	/* ConversationSourceTODOGate identifies a TODO completion continuation. */
	ConversationSourceTODOGate ConversationChangeSource = "todo_gate"
)

/* ConversationChange describes one ordered context change requested by the engine. */
type ConversationChange struct {
	Kind    ConversationChangeKind
	Source  ConversationChangeSource
	Message schema.Message
}

/* ConversationPreparation identifies why one model-visible projection is requested. */
type ConversationPreparation string

const (
	/* ConversationPrepareNormal requests the next ordinary invocation projection. */
	ConversationPrepareNormal ConversationPreparation = "normal"
	/* ConversationPrepareReactive requests one prompt-too-long recovery projection. */
	ConversationPrepareReactive ConversationPreparation = "reactive"
)

/* ConversationRequest freezes all values used to prepare one model invocation. */
type ConversationRequest struct {
	Input           RunInput
	Turn            int
	Phase           Phase
	ToolDefinitions []schema.ToolDefinition
	Preparation     ConversationPreparation
}

/* ConversationCompaction describes a committed context reduction represented by a projection. */
type ConversationCompaction struct {
	Trigger string
}

/* ConversationProjection contains one model-visible context and its committed preparation effects. */
type ConversationProjection struct {
	Context     RunContext
	Compactions []ConversationCompaction
}

/* Conversation prepares immutable invocation snapshots and accepts ordered change requests. */
type Conversation interface {
	Prepare(context.Context, ConversationRequest) (ConversationProjection, error)
	RequestChanges(context.Context, []ConversationChange) error
}

/* TurnState contains the run-scoped values available to turn policy. */
type TurnState struct {
	Turn  int
	Model ModelResult
}

/* ToolState contains one completed, ordered tool round available to run policy. */
type ToolState struct {
	Turn    int
	Calls   []schema.ToolCall
	Results []ToolExecutionResult
}

/* PolicyChanges contains ordered context proposals from a non-terminal policy phase. */
type PolicyChanges struct {
	Changes []ConversationChange
}

/* TurnDecision describes ordered context proposals and the next run transition. */
type TurnDecision struct {
	Complete bool
	Changes  []ConversationChange
	Terminal *PolicyTerminal
}

/* PolicyTerminal describes a policy-selected terminal error and partial-result semantics. */
type PolicyTerminal struct {
	Err     error
	Partial bool
}

/* TurnRunPolicy owns mutable policy state for exactly one engine run. */
type TurnRunPolicy interface {
	BeforeTurn(context.Context, TurnState) (PolicyChanges, error)
	AfterModel(context.Context, TurnState) (TurnDecision, error)
	AfterTools(context.Context, ToolState) (PolicyChanges, error)
}

/* TurnPolicy creates isolated completion and reminder state for each run. */
type TurnPolicy interface {
	StartRun(context.Context, RunInput) (TurnRunPolicy, error)
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
	/* FactContextCompacted marks a committed context reduction before model invocation. */
	FactContextCompacted FactKind = "compaction"
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
		if !isNilCollaborator(e.observer) {
			e.observer.Observe(ctx, fact)
		}
	}
	emit(Fact{Kind: FactRunStarted})

	policyRun, err := e.policy.StartRun(ctx, input)
	if err != nil {
		return e.fail(emit, RunOutcome{}, "policy", err)
	}
	if isNilCollaborator(policyRun) {
		return e.fail(emit, RunOutcome{}, "policy", errors.New("turn policy returned nil run policy"))
	}
	modelRun, err := e.model.StartRun(ctx)
	if err != nil {
		return e.fail(emit, RunOutcome{}, "provider", fmt.Errorf("模型生成失败: %w", err))
	}
	if isNilCollaborator(modelRun) {
		return e.fail(emit, RunOutcome{}, "provider", errors.New("model invoker returned nil run invoker"))
	}
	outcome := RunOutcome{}
	emitModelFact := func(fact ModelFact) {
		if fact.Kind == ModelFactMessageDelta && fact.Content != "" {
			emit(Fact{Kind: FactMessageDelta, Content: fact.Content})
		}
	}

	for turn := 1; ; turn++ {
		outcome.TurnCount = turn
		if boundary, ok := e.tools.(TurnBoundaryToolExecutor); ok {
			if err := boundary.BeginTurn(ctx); err != nil {
				return e.fail(emit, outcome, "tool", err)
			}
		}
		snapshot, err := e.tools.Snapshot(ctx)
		if err != nil {
			return e.fail(emit, outcome, "tool", err)
		}
		if isNilCollaborator(snapshot) {
			return e.fail(emit, outcome, "tool", errors.New("tool executor returned nil snapshot"))
		}
		definitions := cloneToolDefinitions(snapshot.ToolDefinitions())
		beforeTurn, err := policyRun.BeforeTurn(ctx, TurnState{Turn: turn})
		if err != nil {
			return e.fail(emit, outcome, "policy", err)
		}
		if err := e.requestChanges(ctx, beforeTurn.Changes); err != nil {
			return e.fail(emit, outcome, "conversation", err)
		}
		if input.Thinking {
			emit(Fact{Kind: FactThinking, Turn: turn})
			thinkingContext, compactions, err := e.prepareContext(ctx, input, turn, PhaseThinking, definitions, ConversationPrepareNormal)
			if err != nil {
				return e.fail(emit, outcome, "conversation", err)
			}
			for _, compaction := range compactions {
				emit(Fact{Kind: FactContextCompacted, Turn: turn, Phase: PhaseThinking, Name: compaction.Trigger})
			}
			thinkingContext.ToolDefinitions = nil
			thinking, err := modelRun.Invoke(ctx, thinkingContext, func(ModelFact) {})
			if err != nil {
				return e.fail(emit, outcome, "provider", fmt.Errorf("模型生成失败: %w", err))
			}
			outcome.Usage = addUsage(outcome.Usage, thinking.Usage)
			if err := e.requestChanges(ctx, []ConversationChange{{
				Kind:    ConversationAppendContextMessage,
				Source:  ConversationSourceThinking,
				Message: schema.NormalizeMessage(thinking.Message),
			}}); err != nil {
				return e.fail(emit, outcome, "conversation", err)
			}
		}

		runContext, compactions, err := e.prepareContext(ctx, input, turn, PhaseAction, definitions, ConversationPrepareNormal)
		if err != nil {
			return e.fail(emit, outcome, "conversation", err)
		}
		for _, compaction := range compactions {
			emit(Fact{Kind: FactContextCompacted, Turn: turn, Phase: PhaseAction, Name: compaction.Trigger})
		}
		modelResult, err := modelRun.Invoke(ctx, runContext, emitModelFact)
		if errors.Is(err, ErrPromptTooLong) {
			retryContext, recoveryCompactions, prepareErr := e.prepareContext(ctx, input, turn, PhaseAction, definitions, ConversationPrepareReactive)
			if prepareErr == nil && len(recoveryCompactions) > 0 {
				for _, compaction := range recoveryCompactions {
					emit(Fact{Kind: FactContextCompacted, Turn: turn, Phase: PhaseAction, Name: compaction.Trigger})
				}
				modelResult, err = modelRun.Invoke(ctx, retryContext, emitModelFact)
			}
		}
		if err != nil {
			return e.fail(emit, outcome, "provider", fmt.Errorf("模型生成失败: %w", err))
		}
		outcome.FinalMessage = modelResult.Message.Content
		outcome.FinishReason = modelResult.FinishReason
		outcome.Usage = addUsage(outcome.Usage, modelResult.Usage)

		if err := e.requestChanges(ctx, []ConversationChange{{
			Kind:    ConversationAppendMessage,
			Message: schema.NormalizeMessage(modelResult.Message),
		}}); err != nil {
			return e.fail(emit, outcome, "conversation", err)
		}
		if modelResult.Message.Content != "" {
			emit(Fact{Kind: FactMessage, Content: modelResult.Message.Content})
		}
		decision, err := policyRun.AfterModel(ctx, TurnState{Turn: turn, Model: cloneModelResult(modelResult)})
		if err != nil {
			return e.fail(emit, outcome, "policy", err)
		}
		if err := e.requestChanges(ctx, decision.Changes); err != nil {
			return e.fail(emit, outcome, "conversation", err)
		}
		if decision.Terminal != nil {
			if decision.Terminal.Err == nil {
				return e.fail(emit, outcome, "policy", errors.New("turn policy returned a terminal decision without an error"))
			}
			outcome.Partial = decision.Terminal.Partial
			return e.fail(emit, outcome, "policy", decision.Terminal.Err)
		}

		if len(modelResult.Message.ToolCalls) > 0 {
			toolState, err := e.executeTools(ctx, emit, snapshot, turn, modelResult.Message.ToolCalls)
			if err != nil {
				return e.fail(emit, outcome, "tool", err)
			}
			toolDecision, err := policyRun.AfterTools(ctx, toolState)
			if err != nil {
				return e.fail(emit, outcome, "policy", err)
			}
			if err := e.requestChanges(ctx, toolDecision.Changes); err != nil {
				return e.fail(emit, outcome, "conversation", err)
			}
		}
		if len(modelResult.Message.ToolCalls) == 0 && decision.Complete {
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
	preparation ConversationPreparation,
) (RunContext, []ConversationCompaction, error) {
	request := ConversationRequest{
		Input: input, Turn: turn, Phase: phase,
		ToolDefinitions: cloneToolDefinitions(definitions), Preparation: preparation,
	}
	projection, err := e.conversation.Prepare(ctx, request)
	if err != nil {
		return RunContext{}, nil, err
	}
	runContext := projection.Context
	runContext.Turn = turn
	runContext.Phase = phase
	runContext.ToolDefinitions = cloneToolDefinitions(definitions)
	return cloneRunContext(runContext), append([]ConversationCompaction(nil), projection.Compactions...), nil
}

func (e *AgentEngine) executeTools(
	ctx context.Context,
	emit func(Fact),
	snapshot ToolSnapshot,
	turn int,
	calls []schema.ToolCall,
) (ToolState, error) {
	state := ToolState{Turn: turn, Calls: cloneToolCalls(calls)}
	for _, call := range calls {
		emit(Fact{Kind: FactToolCall, CallID: call.ID, Name: call.Name, Content: string(call.Arguments)})
	}
	batch, err := e.tools.Execute(ctx, snapshot, cloneToolCalls(calls))
	if err != nil {
		return state, err
	}
	if len(batch.Results) != len(calls) {
		return state, fmt.Errorf("tool result count = %d, want %d", len(batch.Results), len(calls))
	}
	changes := make([]ConversationChange, 0, len(batch.Results))
	state.Results = make([]ToolExecutionResult, 0, len(batch.Results))
	for index, result := range batch.Results {
		call := calls[index]
		if result.CallID == "" {
			result.CallID = call.ID
		}
		if result.CallID != call.ID {
			return state, fmt.Errorf("tool result %d call ID = %q, want %q", index, result.CallID, call.ID)
		}
		state.Results = append(state.Results, result)
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
	if err := e.requestChanges(ctx, changes); err != nil {
		return state, err
	}
	return state, ctx.Err()
}

func (e *AgentEngine) requestChanges(ctx context.Context, changes []ConversationChange) error {
	if len(changes) == 0 {
		return nil
	}
	cloned := make([]ConversationChange, len(changes))
	for index, change := range changes {
		change.Message = cloneMessage(change.Message)
		cloned[index] = change
	}
	return e.conversation.RequestChanges(ctx, cloned)
}

func (e *AgentEngine) validate() error {
	switch {
	case e == nil:
		return errors.New("target engine is required")
	case isNilCollaborator(e.model):
		return errors.New("model invoker is required")
	case isNilCollaborator(e.tools):
		return errors.New("tool executor is required")
	case isNilCollaborator(e.conversation):
		return errors.New("conversation is required")
	case isNilCollaborator(e.policy):
		return errors.New("turn policy is required")
	default:
		return nil
	}
}

func isNilCollaborator(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
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
