package runtimejournal

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/metrics"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tracing"
)

/* Journal is one run-scoped observation bundle shared by runtime journal and mechanism decorators. */
type Journal struct {
	assembly   foxruntime.RunAssembly
	transcript *session.TranscriptLog
	recorder   *metrics.Recorder
	aggregator *metrics.Aggregator
	tracer     *tracing.Tracer
	runSpan    *tracing.Span

	mu          sync.Mutex
	finished    bool
	pendingErrs []error
	stateMu     sync.Mutex
	turns       map[int]*turnObservation
	toolCalls   map[string]*toolObservation
}

/* New constructs a run-local journal using the existing persisted artifact paths. */
func New(assembly foxruntime.RunAssembly) (*Journal, error) {
	if assembly.Session.ID == "" || assembly.Session.RootDir == "" || assembly.Run.RunID == "" || assembly.Run.RootDir == "" {
		return nil, errors.New("runtime journal requires session and run identity paths")
	}
	stored := &session.StoredSession{ID: assembly.Session.ID, RootDir: assembly.Session.RootDir}
	tracer := tracing.NewTracer((&session.StoredRun{RootDir: assembly.Run.RootDir}).TracePath())
	journal := &Journal{
		assembly: assembly, transcript: session.NewTranscriptLog(stored),
		recorder:   metrics.NewRecorder((&session.StoredRun{RootDir: assembly.Run.RootDir}).MetricsPath()),
		aggregator: metrics.NewAggregator(), tracer: tracer,
		turns: make(map[int]*turnObservation), toolCalls: make(map[string]*toolObservation),
	}
	tracer.WithErrorHandler(journal.recordError)
	attributes := map[string]any{
		"session_id": string(assembly.Session.ID), "run_id": string(assembly.Run.RunID),
		"source": assembly.Session.Source, "work_dir": assembly.Session.WorkDir,
	}
	if assembly.Session.ParentSessionID != "" {
		attributes["parent_session_id"] = assembly.Session.ParentSessionID
	}
	if assembly.Session.Agent != "" {
		attributes["agent"] = assembly.Session.Agent
	}
	journal.runSpan = tracer.StartSpan("", "run", attributes)
	return journal, nil
}

/* RecordArtifact persists transcript-compatible facts that are not authoritative session state. */
func (j *Journal) RecordArtifact(_ context.Context, fact foxruntime.RuntimeFact) error {
	if err := j.validateFact(fact); err != nil {
		return err
	}
	switch fact.Fact.Kind {
	case engine.FactRunStarted:
		display := j.assembly.Spec.DisplayPrompt
		if display == "" {
			display = j.assembly.Spec.Prompt
		}
		payload := map[string]string{"prompt": display}
		if display != j.assembly.Spec.Prompt {
			payload["model_prompt"] = j.assembly.Spec.Prompt
		}
		return j.transcript.AppendRun(fact.RunID, "user_prompt", payload)
	case engine.FactContextCompacted:
		payload := map[string]any{}
		switch fact.Fact.Name {
		case "session_history":
			payload["scope"] = "session_history"
		case "reactive":
			payload["turn"] = fact.Fact.Turn
			payload["source"] = "reactive"
		default:
			payload["turn"] = fact.Fact.Turn
		}
		return j.transcript.AppendRun(fact.RunID, "context_compacted", payload)
	case engine.FactSystemReminder:
		payload := map[string]any{
			"turn":    fact.Fact.Turn,
			"message": stripRuntimeNoticePrefix(fact.Fact.Content),
		}
		switch fact.Fact.Name {
		case string(engine.ConversationSourceNextTurnReminder):
			payload["source"] = "next_turn_reminders"
		case string(engine.ConversationSourceCompletionGate):
			payload["source"] = "completion_gate"
		case string(engine.ConversationSourceTODOGate):
			payload["source"] = "todo_completion_gate"
		}
		return j.transcript.AppendRun(fact.RunID, "system_reminder_injected", payload)
	case engine.FactErrorRecovery:
		payload := map[string]any{"prompt": stripRuntimeNoticePrefix(fact.Fact.Content)}
		return j.transcript.AppendRun(fact.RunID, "error_recovery_injected", payload)
	default:
		return nil
	}
}

/* runtimeNoticePrefixes are the presentation prefixes the engine adds to
 * policy-sourced conversation injections; transcript records store the bare
 * notice text. */
var runtimeNoticePrefixes = []string{
	"[Runtime System Reminder]\n\n",
	"[Runtime System Notice]\n\n",
}

func stripRuntimeNoticePrefix(content string) string {
	for _, prefix := range runtimeNoticePrefixes {
		if strings.HasPrefix(content, prefix) {
			return strings.TrimPrefix(content, prefix)
		}
	}
	return content
}

/* RecordTelemetry finalizes run-level metrics and tracing on the canonical terminal fact. */
func (j *Journal) RecordTelemetry(_ context.Context, fact foxruntime.RuntimeFact) error {
	if err := j.validateFact(fact); err != nil {
		return err
	}
	if fact.Fact.Kind == engine.FactToolCall {
		j.startTool(fact.Fact)
	}
	j.annotateInjection(fact.Fact)
	if fact.Fact.Kind != engine.FactRunCompleted && fact.Fact.Kind != engine.FactRunError {
		return j.takeErrors()
	}
	j.finishOpenTurns(fact.Fact.Kind == engine.FactRunError, fact.Fact.Content)
	j.mu.Lock()
	if j.finished {
		j.mu.Unlock()
		return j.takeErrors()
	}
	j.finished = true
	summary := j.aggregator.Summary(string(j.assembly.Session.ID))
	j.mu.Unlock()
	if err := j.recorder.Append(summary); err != nil {
		j.recordError(err)
	}
	status := "ok"
	attributes := map[string]any{}
	if fact.Fact.Kind == engine.FactRunError {
		status = "error"
		attributes["error"] = fact.Fact.Content
	}
	j.runSpan.End(status, attributes)
	return j.takeErrors()
}

/* WrapModel decorates model calls with the legacy metrics and trace artifact formats. */
func (j *Journal) WrapModel(base engine.ModelInvoker) engine.ModelInvoker {
	return journalModel{journal: j, base: base}
}

/* WrapTools decorates tool batches with the legacy metrics and trace artifact formats. */
func (j *Journal) WrapTools(base engine.ToolExecutor) engine.ToolExecutor {
	return &journalTools{journal: j, base: base}
}

func (j *Journal) validateFact(fact foxruntime.RuntimeFact) error {
	if fact.SessionID != j.assembly.Session.ID || fact.RunID != j.assembly.Run.RunID {
		return fmt.Errorf("runtime journal fact identity %s/%s does not match %s/%s", fact.SessionID, fact.RunID, j.assembly.Session.ID, j.assembly.Run.RunID)
	}
	return nil
}

func (j *Journal) recordError(err error) {
	if err == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.pendingErrs = append(j.pendingErrs, err)
}

func (j *Journal) takeErrors() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	result := errors.Join(j.pendingErrs...)
	j.pendingErrs = nil
	return result
}

type turnObservation struct {
	span  *tracing.Span
	ended bool
}

type toolObservation struct {
	turn    int
	started time.Time
	span    *tracing.Span
}

func (j *Journal) turn(turn int) *turnObservation {
	j.stateMu.Lock()
	defer j.stateMu.Unlock()
	observation := j.turns[turn]
	if observation == nil {
		observation = &turnObservation{span: j.tracer.StartSpan(j.runSpan.ID(), "turn", map[string]any{"turn": turn})}
		j.turns[turn] = observation
	}
	return observation
}

func (j *Journal) finishTurn(turn int, status string, attributes map[string]any) {
	j.stateMu.Lock()
	observation := j.turns[turn]
	if observation == nil || observation.ended {
		j.stateMu.Unlock()
		return
	}
	observation.ended = true
	j.stateMu.Unlock()
	observation.span.End(status, attributes)
}

func (j *Journal) startTool(fact engine.Fact) {
	turn := j.turn(fact.Turn)
	j.stateMu.Lock()
	defer j.stateMu.Unlock()
	if _, exists := j.toolCalls[fact.CallID]; exists {
		return
	}
	j.toolCalls[fact.CallID] = &toolObservation{
		turn: fact.Turn, started: time.Now(),
		span: j.tracer.StartSpan(turn.span.ID(), "tool_call", map[string]any{"tool": fact.Name, "tool_call_id": fact.CallID}),
	}
}

/* annotateInjection records the turn-span annotation for one injected reminder
 * or recovery notice, mirroring the baseline injection tracing. */
func (j *Journal) annotateInjection(fact engine.Fact) {
	switch fact.Kind {
	case engine.FactErrorRecovery:
		attributes := map[string]any{"turn": fact.Turn}
		j.tracer.Annotate(j.turn(fact.Turn).span.ID(), "error_recovery_injected", attributes)
	case engine.FactSystemReminder:
		attributes := map[string]any{"turn": fact.Turn}
		switch fact.Name {
		case string(engine.ConversationSourceReminder), string(engine.ConversationSourceNextTurnReminder):
		default:
			return
		}
		if fact.Name == string(engine.ConversationSourceNextTurnReminder) {
			attributes["source"] = "next_turn_reminders"
		}
		j.tracer.Annotate(j.turn(fact.Turn).span.ID(), "system_reminder_injected", attributes)
	default:
	}
}

func (j *Journal) takeTool(call schema.ToolCall) toolObservation {
	j.stateMu.Lock()
	defer j.stateMu.Unlock()
	if observation := j.toolCalls[call.ID]; observation != nil {
		delete(j.toolCalls, call.ID)
		return *observation
	}
	turn := j.turns[0]
	if turn == nil {
		turn = &turnObservation{span: j.tracer.StartSpan(j.runSpan.ID(), "turn", map[string]any{"turn": 0})}
		j.turns[0] = turn
	}
	return toolObservation{
		started: time.Now(), span: j.tracer.StartSpan(turn.span.ID(), "tool_call", map[string]any{"tool": call.Name, "tool_call_id": call.ID}),
	}
}

func (j *Journal) finishOpenTurns(failed bool, failure string) {
	j.stateMu.Lock()
	open := make(map[int]*turnObservation)
	for turn, observation := range j.turns {
		if !observation.ended {
			observation.ended = true
			open[turn] = observation
		}
	}
	j.stateMu.Unlock()
	status := "ok"
	attributes := map[string]any{}
	if failed {
		status = "error"
		attributes["error"] = failure
	}
	for _, observation := range open {
		observation.span.End(status, attributes)
	}
}

type journalModel struct {
	journal *Journal
	base    engine.ModelInvoker
}

func (m journalModel) StartRun(ctx context.Context) (engine.ModelRunInvoker, error) {
	if isNilJournalCollaborator(m.base) {
		return nil, errors.New("runtime journal model is required")
	}
	run, err := m.base.StartRun(ctx)
	if err != nil {
		return nil, err
	}
	if isNilJournalCollaborator(run) {
		return nil, errors.New("runtime journal model returned nil run invoker")
	}
	return journalModelRun{journal: m.journal, base: run}, nil
}

type journalModelRun struct {
	journal *Journal
	base    engine.ModelRunInvoker
}

func (m journalModelRun) Invoke(ctx context.Context, request engine.RunContext, emit engine.ModelFactEmitter) (engine.ModelResult, error) {
	turn := m.journal.turn(request.Turn)
	attributes := map[string]any{
		"phase": string(request.Phase), "turn": request.Turn,
		"message_len": len(request.Messages), "tools": len(request.ToolDefinitions),
	}
	if request.Provider != "" {
		attributes["provider_protocol"] = request.Provider
	}
	if request.Model != "" {
		attributes["model"] = request.Model
	}
	span := m.journal.tracer.StartSpan(turn.span.ID(), "model_call", attributes)
	started := time.Now()
	result, err := m.base.Invoke(ctx, request, emit)
	duration := time.Since(started)
	estimator := metrics.RoughEstimator{}
	inputTokens := estimator.EstimateMessages(request.Messages) + metrics.EstimateToolDefinitions(estimator, request.ToolDefinitions)
	outputTokens := estimator.EstimateText(result.Message.Content)
	for _, call := range result.Message.ToolCalls {
		outputTokens += estimator.EstimateText(call.Name) + estimator.EstimateText(string(call.Arguments))
	}
	event := metrics.ModelCall{
		Time: time.Now(), Type: metrics.EventModelCall, SessionID: string(m.journal.assembly.Session.ID),
		Turn: request.Turn, Phase: string(request.Phase),
		InputTokens: inputTokens, OutputTokens: outputTokens, DurationMS: duration.Milliseconds(),
	}
	if err != nil {
		event.Error = err.Error()
	}
	if appendErr := m.journal.recorder.Append(event); appendErr != nil {
		m.journal.recordError(appendErr)
	}
	m.journal.mu.Lock()
	m.journal.aggregator.AddModel(inputTokens, outputTokens, err != nil)
	m.journal.mu.Unlock()
	if err != nil {
		span.End("error", map[string]any{"error": err.Error()})
		m.journal.finishTurn(request.Turn, "error", map[string]any{"error": err.Error()})
	} else {
		span.End("ok", map[string]any{"content_bytes": len(result.Message.Content), "tool_calls": len(result.Message.ToolCalls)})
		if request.Phase == engine.PhaseAction && len(result.Message.ToolCalls) == 0 {
			m.journal.finishTurn(request.Turn, "ok", map[string]any{"tool_calls": 0, "final": true})
		}
	}
	return result, err
}

type journalTools struct {
	journal *Journal
	base    engine.ToolExecutor
}

func (t *journalTools) BeginTurn(ctx context.Context) error {
	if t == nil || isNilJournalCollaborator(t.base) {
		return errors.New("runtime journal tool executor is required")
	}
	if boundary, ok := t.base.(engine.TurnBoundaryToolExecutor); ok {
		return boundary.BeginTurn(ctx)
	}
	return nil
}

func (t *journalTools) Snapshot(ctx context.Context) (engine.ToolSnapshot, error) {
	if t == nil || isNilJournalCollaborator(t.base) {
		return nil, errors.New("runtime journal tool executor is required")
	}
	base, err := t.base.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	if isNilJournalCollaborator(base) {
		return nil, errors.New("runtime journal tool executor returned nil snapshot")
	}
	return &journalToolSnapshot{owner: t, base: base}, nil
}

func (t *journalTools) Execute(ctx context.Context, frozen engine.ToolSnapshot, calls []schema.ToolCall) (engine.ToolBatch, error) {
	snapshot, ok := frozen.(*journalToolSnapshot)
	if !ok || snapshot == nil || snapshot.owner != t {
		return engine.ToolBatch{}, errors.New("runtime journal tool snapshot does not belong to executor")
	}
	batch, err := t.base.Execute(ctx, snapshot.base, calls)
	turnCounts := make(map[int]int)
	for index, call := range calls {
		observation := t.journal.takeTool(call)
		turnCounts[observation.turn]++
		result := engine.ToolExecutionResult{CallID: call.ID, IsError: err != nil}
		if index < len(batch.Results) {
			result = batch.Results[index]
		}
		event := metrics.ToolCall{
			Time: time.Now(), Type: metrics.EventToolCall, SessionID: string(t.journal.assembly.Session.ID),
			Turn: observation.turn, ToolName: call.Name, ToolCallID: call.ID, DurationMS: time.Since(observation.started).Milliseconds(),
			OutputBytes: len(result.FullContent), IsError: result.IsError,
		}
		if appendErr := t.journal.recorder.Append(event); appendErr != nil {
			t.journal.recordError(appendErr)
		}
		t.journal.mu.Lock()
		t.journal.aggregator.AddTool(result.IsError)
		t.journal.mu.Unlock()
		status := "ok"
		if result.IsError {
			status = "error"
		}
		observation.span.End(status, map[string]any{"is_error": result.IsError, "output_bytes": len(result.FullContent)})
	}
	for turn, count := range turnCounts {
		status := "ok"
		attributes := map[string]any{"tool_calls": count}
		if err != nil {
			status = "error"
			attributes["error"] = err.Error()
		}
		t.journal.finishTurn(turn, status, attributes)
	}
	return batch, err
}

type journalToolSnapshot struct {
	owner *journalTools
	base  engine.ToolSnapshot
}

func (s *journalToolSnapshot) ToolDefinitions() []schema.ToolDefinition {
	return s.base.ToolDefinitions()
}

func isNilJournalCollaborator(value any) bool {
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

var _ foxruntime.SessionArtifactJournal = (*Journal)(nil)
var _ foxruntime.TelemetryJournal = (*Journal)(nil)
var _ engine.ModelInvoker = journalModel{}
var _ engine.ToolExecutor = (*journalTools)(nil)
var _ engine.TurnBoundaryToolExecutor = (*journalTools)(nil)
