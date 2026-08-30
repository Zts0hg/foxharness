package runtimejournal

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/metrics"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tracing"
)

type nilStartModel struct{}

func (nilStartModel) StartRun(context.Context) (engine.ModelRunInvoker, error) {
	return nil, nil
}

type nilSnapshotTools struct{}

func (nilSnapshotTools) Snapshot(context.Context) (engine.ToolSnapshot, error) {
	return nil, nil
}

func (nilSnapshotTools) Execute(context.Context, engine.ToolSnapshot, []schema.ToolCall) (engine.ToolBatch, error) {
	return engine.ToolBatch{}, nil
}

func TestJournalRejectsNilWrappedModelRun(t *testing.T) {
	journal := &Journal{}
	if _, err := journal.WrapModel(nilStartModel{}).StartRun(context.Background()); err == nil {
		t.Fatal("StartRun() error = nil, want nil wrapped model-run error")
	}
}

func TestJournalRejectsNilWrappedToolSnapshot(t *testing.T) {
	journal := &Journal{}
	if _, err := journal.WrapTools(nilSnapshotTools{}).Snapshot(context.Background()); err == nil {
		t.Fatal("Snapshot() error = nil, want nil wrapped tool-snapshot error")
	}
}

type typedNilWrappedModelRun struct{}

func (*typedNilWrappedModelRun) Invoke(context.Context, engine.RunContext, engine.ModelFactEmitter) (engine.ModelResult, error) {
	return engine.ModelResult{}, nil
}

type typedNilWrappedModel struct{}

func (typedNilWrappedModel) StartRun(context.Context) (engine.ModelRunInvoker, error) {
	var run *typedNilWrappedModelRun
	return run, nil
}

func TestJournalRejectsTypedNilWrappedModelRun(t *testing.T) {
	journal := &Journal{}
	if _, err := journal.WrapModel(typedNilWrappedModel{}).StartRun(context.Background()); err == nil {
		t.Fatal("StartRun() error = nil, want typed-nil wrapped model-run error")
	}
}

type typedNilWrappedToolSnapshot struct{}

func (*typedNilWrappedToolSnapshot) ToolDefinitions() []schema.ToolDefinition { return nil }

type typedNilWrappedTools struct{}

func (typedNilWrappedTools) Snapshot(context.Context) (engine.ToolSnapshot, error) {
	var snapshot *typedNilWrappedToolSnapshot
	return snapshot, nil
}

func (typedNilWrappedTools) Execute(context.Context, engine.ToolSnapshot, []schema.ToolCall) (engine.ToolBatch, error) {
	return engine.ToolBatch{}, nil
}

func TestJournalRejectsTypedNilWrappedToolSnapshot(t *testing.T) {
	journal := &Journal{}
	if _, err := journal.WrapTools(typedNilWrappedTools{}).Snapshot(context.Background()); err == nil {
		t.Fatal("Snapshot() error = nil, want typed-nil wrapped tool-snapshot error")
	}
}

func TestJournalRejectsNilDecoratorInputs(t *testing.T) {
	journal := &Journal{}
	t.Run("model", func(t *testing.T) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("StartRun() panicked for nil wrapped model: %v", recovered)
			}
		}()
		if _, err := journal.WrapModel(nil).StartRun(context.Background()); err == nil {
			t.Fatal("StartRun() error = nil, want nil wrapped model error")
		}
	})
	t.Run("tools", func(t *testing.T) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("Snapshot() panicked for nil wrapped tools: %v", recovered)
			}
		}()
		if _, err := journal.WrapTools(nil).Snapshot(context.Background()); err == nil {
			t.Fatal("Snapshot() error = nil, want nil wrapped tools error")
		}
	})
}

type typedNilBoundaryTools struct{}

func (*typedNilBoundaryTools) BeginTurn(context.Context) error {
	panic("typed-nil boundary tools were advanced")
}

func (*typedNilBoundaryTools) Snapshot(context.Context) (engine.ToolSnapshot, error) {
	return staticToolSnapshot{}, nil
}

func (*typedNilBoundaryTools) Execute(context.Context, engine.ToolSnapshot, []schema.ToolCall) (engine.ToolBatch, error) {
	return engine.ToolBatch{}, nil
}

func TestJournalRejectsTypedNilBoundaryDecoratorBeforeSnapshot(t *testing.T) {
	journal := &Journal{}
	var base *typedNilBoundaryTools
	tools := journal.WrapTools(base)
	boundary, ok := tools.(engine.TurnBoundaryToolExecutor)
	if !ok {
		t.Fatal("WrapTools() does not expose TurnBoundaryToolExecutor")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("BeginTurn() panicked for typed-nil wrapped tools: %v", recovered)
		}
	}()
	if err := boundary.BeginTurn(context.Background()); err == nil {
		t.Fatal("BeginTurn() error = nil, want typed-nil wrapped tools error")
	}
}

func TestJournalRejectsTypedNilDecoratorInputs(t *testing.T) {
	journal := &Journal{}
	t.Run("model", func(t *testing.T) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("StartRun() panicked for typed-nil wrapped model: %v", recovered)
			}
		}()
		if _, err := journal.WrapModel((*typedNilWrappedModel)(nil)).StartRun(context.Background()); err == nil {
			t.Fatal("StartRun() error = nil, want typed-nil wrapped model error")
		}
	})
	t.Run("tools", func(t *testing.T) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("Snapshot() panicked for typed-nil wrapped tools: %v", recovered)
			}
		}()
		if _, err := journal.WrapTools((*typedNilWrappedTools)(nil)).Snapshot(context.Background()); err == nil {
			t.Fatal("Snapshot() error = nil, want typed-nil wrapped tools error")
		}
	})
}

func TestJournalRejectsTypedNilExecuteSnapshot(t *testing.T) {
	journal := &Journal{}
	tools := journal.WrapTools(staticTools{})
	var typedNil *journalToolSnapshot
	var frozen engine.ToolSnapshot = typedNil
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Execute() panicked for typed-nil journal snapshot: %v", recovered)
		}
	}()
	if _, err := tools.Execute(context.Background(), frozen, nil); err == nil {
		t.Fatal("Execute() error = nil, want ownership error for typed-nil journal snapshot")
	}
}

func TestJournalPreservesRunArtifactsAcrossRuntimeFactsAndMechanisms(t *testing.T) {
	root := t.TempDir()
	runRoot := filepath.Join(root, "runs", "run-1")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	assembly := foxruntime.RunAssembly{
		Session: foxruntime.AgentSessionSnapshot{ID: "session-1", Source: session.SOURCECLI, WorkDir: "/workspace", RootDir: root},
		Run:     foxruntime.RunScopeSnapshot{RunID: "run-1", RootDir: runRoot, Model: "model-a"},
		Spec:    foxruntime.RunSnapshot{Prompt: "model prompt", DisplayPrompt: "display prompt", ProviderProtocol: "openai"},
	}
	journal, err := New(assembly)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := journal.RecordArtifact(ctx, runtimeFact(assembly, engine.Fact{Kind: engine.FactRunStarted, Sequence: 1})); err != nil {
		t.Fatal(err)
	}
	if err := journal.RecordTelemetry(ctx, runtimeFact(assembly, engine.Fact{Kind: engine.FactRunStarted, Sequence: 1})); err != nil {
		t.Fatal(err)
	}

	modelRun, err := journal.WrapModel(staticModel{}).StartRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	modelResult, err := modelRun.Invoke(ctx, engine.RunContext{
		Turn: 1, Phase: engine.PhaseAction, Model: "model-a", Provider: "openai",
		Messages:        []schema.Message{{Role: schema.RoleUser, Content: "task"}},
		ToolDefinitions: []schema.ToolDefinition{{Name: "read_file"}},
	}, nil)
	if err != nil || modelResult.Message.Content != "done" {
		t.Fatalf("model = %#v/%v", modelResult, err)
	}
	toolCallFact := runtimeFact(assembly, engine.Fact{Kind: engine.FactToolCall, Sequence: 2, Turn: 1, CallID: "call-1", Name: "read_file"})
	if err := journal.RecordTelemetry(ctx, toolCallFact); err != nil {
		t.Fatal(err)
	}

	tools := journal.WrapTools(staticTools{})
	snapshot, err := tools.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := tools.Execute(ctx, snapshot, []schema.ToolCall{{ID: "call-1", Name: "read_file"}})
	if err != nil || len(batch.Results) != 1 || batch.Results[0].FullContent != "tool output" {
		t.Fatalf("tools = %#v/%v", batch, err)
	}
	toolResultFact := runtimeFact(assembly, engine.Fact{Kind: engine.FactToolResult, Sequence: 3, Turn: 1, CallID: "call-1", Name: "read_file", Content: "tool output"})
	if err := journal.RecordTelemetry(ctx, toolResultFact); err != nil {
		t.Fatal(err)
	}

	completed := runtimeFact(assembly, engine.Fact{Kind: engine.FactRunCompleted, Sequence: 4, Content: "done"})
	if err := journal.RecordArtifact(ctx, completed); err != nil {
		t.Fatal(err)
	}
	if err := journal.RecordTelemetry(ctx, completed); err != nil {
		t.Fatal(err)
	}

	transcript := readJSONLines[session.TranscriptEvent](t, filepath.Join(root, "transcript.jsonl"))
	if len(transcript) != 1 || transcript[0].Type != "user_prompt" || transcript[0].RunID != "run-1" {
		t.Fatalf("transcript = %#v", transcript)
	}
	payload, _ := transcript[0].Payload.(map[string]any)
	if payload["prompt"] != "display prompt" || payload["model_prompt"] != "model prompt" {
		t.Fatalf("prompt payload = %#v", payload)
	}

	metricEvents := readUntypedJSONLines(t, filepath.Join(runRoot, "metrics.jsonl"))
	if got, want := eventTypes(metricEvents), []string{"model_call", "tool_call", "run_summary"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("metric event types = %#v, want %#v", got, want)
	}
	if metricEvents[0]["turn"] != float64(1) || metricEvents[0]["phase"] != "action" || metricEvents[0]["model"] != nil || metricEvents[1]["turn"] != float64(1) || metricEvents[1]["tool_call_id"] != "call-1" {
		t.Fatalf("metric events = %#v", metricEvents)
	}
	if metricEvents[2]["total_model_calls"] != float64(1) || metricEvents[2]["total_tool_calls"] != float64(1) {
		t.Fatalf("run summary = %#v", metricEvents[2])
	}

	traceEvents := readJSONLines[tracing.SpanEvent](t, filepath.Join(runRoot, "trace.jsonl"))
	var names []string
	for _, event := range traceEvents {
		if event.Type == tracing.EventSpanStart {
			names = append(names, event.Name)
		}
	}
	if want := []string{"run", "turn", "model_call", "tool_call"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("span names = %#v, want %#v", names, want)
	}
}

func TestJournalRecordsCompactionTranscriptScope(t *testing.T) {
	root := t.TempDir()
	runRoot := filepath.Join(root, "runs", "run-1")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	assembly := foxruntime.RunAssembly{
		Session: foxruntime.AgentSessionSnapshot{ID: "session-1", RootDir: root},
		Run:     foxruntime.RunScopeSnapshot{RunID: "run-1", RootDir: runRoot},
		Spec:    foxruntime.RunSnapshot{Prompt: "task"},
	}
	journal, _ := New(assembly)
	for sequence, scope := range []string{"session_history", "turn_context", "reactive"} {
		fact := engine.Fact{Kind: engine.FactContextCompacted, Sequence: sequence + 1, Turn: sequence, Name: scope}
		if err := journal.RecordArtifact(context.Background(), runtimeFact(assembly, fact)); err != nil {
			t.Fatal(err)
		}
	}
	events := readJSONLines[session.TranscriptEvent](t, filepath.Join(root, "transcript.jsonl"))
	if len(events) != 3 {
		t.Fatalf("transcript events = %#v", events)
	}
	for index, event := range events {
		payload := event.Payload.(map[string]any)
		if event.Type != "context_compacted" {
			t.Fatalf("event %d = %#v", index, event)
		}
		switch index {
		case 0:
			if !reflect.DeepEqual(payload, map[string]any{"scope": "session_history"}) {
				t.Fatalf("initial compaction payload = %#v", payload)
			}
		case 1:
			if !reflect.DeepEqual(payload, map[string]any{"turn": float64(1)}) {
				t.Fatalf("turn compaction payload = %#v", payload)
			}
		case 2:
			if !reflect.DeepEqual(payload, map[string]any{"turn": float64(2), "source": "reactive"}) {
				t.Fatalf("reactive compaction payload = %#v", payload)
			}
		}
	}
}

func runtimeFact(assembly foxruntime.RunAssembly, fact engine.Fact) foxruntime.RuntimeFact {
	return foxruntime.RuntimeFact{SessionID: assembly.Session.ID, RunID: assembly.Run.RunID, Fact: fact}
}

type staticModel struct{}

func (staticModel) StartRun(context.Context) (engine.ModelRunInvoker, error) {
	return staticModelRun{}, nil
}

type staticModelRun struct{}

func (staticModelRun) Invoke(context.Context, engine.RunContext, engine.ModelFactEmitter) (engine.ModelResult, error) {
	return engine.ModelResult{
		Message: schema.Message{Role: schema.RoleAssistant, Content: "done", ToolCalls: []schema.ToolCall{{ID: "call-1", Name: "read_file"}}}, FinishReason: "tool_calls",
		Usage: schema.Usage{InputTokens: 7, OutputTokens: 3},
	}, nil
}

type staticTools struct{}

func (staticTools) Snapshot(context.Context) (engine.ToolSnapshot, error) {
	return staticToolSnapshot{}, nil
}
func (staticTools) Execute(context.Context, engine.ToolSnapshot, []schema.ToolCall) (engine.ToolBatch, error) {
	return engine.ToolBatch{Results: []engine.ToolExecutionResult{{CallID: "call-1", FullContent: "tool output", ModelContent: "tool output"}}}, nil
}

type staticToolSnapshot struct{}

func (staticToolSnapshot) ToolDefinitions() []schema.ToolDefinition {
	return []schema.ToolDefinition{{Name: "read_file"}}
}

func readJSONLines[T any](t *testing.T, path string) []T {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var result []T
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var item T
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
		result = append(result, item)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func readUntypedJSONLines(t *testing.T, path string) []map[string]any {
	return readJSONLines[map[string]any](t, path)
}

func eventTypes(events []map[string]any) []string {
	result := make([]string, len(events))
	for index, event := range events {
		result[index], _ = event["type"].(string)
	}
	return result
}

var _ = metrics.EventRunSummary

/* TestJournalRecordsReminderAndRecoveryTranscriptEvents verifies that injected
 * reminders and recovery notices regain their baseline transcript records and
 * turn-span annotations. */
func TestJournalRecordsReminderAndRecoveryTranscriptEvents(t *testing.T) {
	root := t.TempDir()
	runRoot := filepath.Join(root, "runs", "run-1")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	assembly := foxruntime.RunAssembly{
		Session: foxruntime.AgentSessionSnapshot{ID: "session-1", RootDir: root},
		Run:     foxruntime.RunScopeSnapshot{RunID: "run-1", RootDir: runRoot},
		Spec:    foxruntime.RunSnapshot{Prompt: "task"},
	}
	journal, err := New(assembly)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cases := []struct {
		source      string
		content     string
		wantPayload map[string]any
		annotate    bool
		annotateExt map[string]any
	}{
		{
			source:      string(engine.ConversationSourceReminder),
			content:     "[Runtime System Reminder]\n\ngeneral reminder body",
			wantPayload: map[string]any{"turn": 2, "message": "general reminder body"},
			annotate:    true,
		},
		{
			source:      string(engine.ConversationSourceNextTurnReminder),
			content:     "[Runtime System Reminder]\n\nqueued reminder body",
			wantPayload: map[string]any{"turn": 3, "message": "queued reminder body", "source": "next_turn_reminders"},
			annotate:    true,
			annotateExt: map[string]any{"source": "next_turn_reminders"},
		},
		{
			source:      string(engine.ConversationSourceCompletionGate),
			content:     "[Runtime System Reminder]\n\ngate body",
			wantPayload: map[string]any{"turn": 4, "message": "gate body", "source": "completion_gate"},
		},
		{
			source:      string(engine.ConversationSourceTODOGate),
			content:     "[Runtime System Reminder]\n\ntodo gate body",
			wantPayload: map[string]any{"turn": 5, "message": "todo gate body", "source": "todo_completion_gate"},
		},
	}
	for index, testCase := range cases {
		fact := engine.Fact{
			Kind: engine.FactSystemReminder, Sequence: index + 2,
			Turn: testCase.wantPayload["turn"].(int), Name: testCase.source, Content: testCase.content,
		}
		if err := journal.RecordArtifact(ctx, runtimeFact(assembly, fact)); err != nil {
			t.Fatal(err)
		}
		if err := journal.RecordTelemetry(ctx, runtimeFact(assembly, fact)); err != nil {
			t.Fatal(err)
		}
	}
	recovery := engine.Fact{
		Kind: engine.FactErrorRecovery, Sequence: 7,
		Turn: 3, Content: "[Runtime System Notice]\n\nrecovery prompt body",
	}
	if err := journal.RecordArtifact(ctx, runtimeFact(assembly, recovery)); err != nil {
		t.Fatal(err)
	}
	if err := journal.RecordTelemetry(ctx, runtimeFact(assembly, recovery)); err != nil {
		t.Fatal(err)
	}

	events := readJSONLines[session.TranscriptEvent](t, filepath.Join(root, "transcript.jsonl"))
	var reminderEvents []session.TranscriptEvent
	for _, event := range events {
		if event.Type == "system_reminder_injected" || event.Type == "error_recovery_injected" {
			reminderEvents = append(reminderEvents, event)
		}
	}
	if len(reminderEvents) != len(cases)+1 {
		t.Fatalf("injection transcript events = %d, want %d: %#v", len(reminderEvents), len(cases)+1, reminderEvents)
	}
	for index, want := range cases {
		event := reminderEvents[index]
		if event.Type != "system_reminder_injected" {
			t.Fatalf("event %d type = %q", index, event.Type)
		}
		payload, _ := event.Payload.(map[string]any)
		for key, value := range want.wantPayload {
			if numeric, ok := value.(int); ok {
				value = float64(numeric)
			}
			if payload[key] != value {
				t.Fatalf("event %d payload[%q] = %#v, want %#v (full: %#v)", index, key, payload[key], value, payload)
			}
		}
	}
	recoveryEvent := reminderEvents[len(cases)]
	if recoveryEvent.Type != "error_recovery_injected" {
		t.Fatalf("recovery event type = %q", recoveryEvent.Type)
	}
	if payload, _ := recoveryEvent.Payload.(map[string]any); payload["prompt"] != "recovery prompt body" {
		t.Fatalf("recovery payload = %#v", payload)
	}

	annotations := readJSONLines[tracing.SpanEvent](t, filepath.Join(runRoot, "trace.jsonl"))
	annotateNames := map[string][]map[string]any{}
	for _, event := range annotations {
		if event.Type == tracing.EventAnnotation {
			annotateNames[event.Name] = append(annotateNames[event.Name], event.Attrs)
		}
	}
	if got := len(annotateNames["system_reminder_injected"]); got != 2 {
		t.Fatalf("system_reminder_injected annotations = %d, want 2", got)
	}
	if got := len(annotateNames["error_recovery_injected"]); got != 1 {
		t.Fatalf("error_recovery_injected annotations = %d, want 1", got)
	}
	if got := annotateNames["error_recovery_injected"][0]["turn"]; got != float64(3) {
		t.Fatalf("recovery annotation turn = %#v, want 3", got)
	}
	if got := annotateNames["system_reminder_injected"][1]["source"]; got != "next_turn_reminders" {
		t.Fatalf("next-turn annotation source = %#v", got)
	}
}
