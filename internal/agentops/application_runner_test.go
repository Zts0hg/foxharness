package agentops

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
)

func TestRunnerStartsPreparedApplicationAfterSessionNoticeAndPreservesFinalPresentation(t *testing.T) {
	var events []string
	messenger := &orderedAgentOpsMessenger{events: &events}
	application := &recordingAgentOpsApplication{
		events: &events,
		outcome: &app.RunOutcome{
			SessionID: "session-1", RunID: "run-1", FinalMessage: "incident resolved",
			TracePath: "/runs/run-1/trace.jsonl", MetricsPath: "/runs/run-1/metrics.jsonl",
		},
	}
	factory := &recordingAgentOpsExecutionFactory{
		events: &events,
		prepared: PreparedTaskExecution{
			Session:   app.SessionInfo{ID: "session-1"},
			TracePath: "/session/trace.jsonl", MetricsPath: "/session/metrics.jsonl",
			Start: func(context.Context) (TaskApplication, error) {
				events = append(events, "start")
				return application, nil
			},
		},
	}
	task := Task{TaskID: "task-1", ChatID: "chat-1", SenderID: "sender-1", MessageID: "message-1", Text: "/new inspect"}
	runner := NewRunner(factory, messenger)

	if err := runner.run(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	wantRequest := TaskExecutionRequest{Task: task, Prompt: BuildPrompt(task)}
	if !reflect.DeepEqual(factory.request, wantRequest) {
		t.Fatalf("execution request = %#v, want %#v", factory.request, wantRequest)
	}
	wantEvents := []string{
		"prepare",
		"message:已创建 AgentOps Session: session-1\n开始分析。",
		"start",
		"run",
		"message:incident resolved\n\nSession: session-1\nRun: run-1\nTrace: /runs/run-1/trace.jsonl\nMetrics: /runs/run-1/metrics.jsonl",
		"drain",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
}

func TestRunnerDrainsPreparedApplicationWithFreshDeadlineAfterPartialFailure(t *testing.T) {
	runErr := errors.New("provider failed")
	drainHasDeadline := false
	application := &recordingAgentOpsApplication{
		outcome: &app.RunOutcome{SessionID: "session", RunID: "run", FinalMessage: "partial", Partial: true},
		err:     runErr,
		drain: func(ctx context.Context) error {
			_, drainHasDeadline = ctx.Deadline()
			return nil
		},
	}
	factory := &recordingAgentOpsExecutionFactory{prepared: PreparedTaskExecution{
		Session: app.SessionInfo{ID: "session"},
		Start: func(context.Context) (TaskApplication, error) {
			return application, nil
		},
	}}
	runner := NewRunner(factory, &orderedAgentOpsMessenger{})

	if err := runner.run(context.Background(), Task{TaskID: "task", ChatID: "chat"}); !errors.Is(err, runErr) {
		t.Fatalf("run() error = %v, want %v", err, runErr)
	}
	if !drainHasDeadline {
		t.Fatal("drain context has no fresh cleanup deadline")
	}
}

func TestRunnerRejectsTypedNilExecutionDependencies(t *testing.T) {
	var nilFactory *recordingAgentOpsExecutionFactory
	runner := NewRunner(nilFactory, &orderedAgentOpsMessenger{})
	if err := runner.run(context.Background(), Task{}); err == nil {
		t.Fatal("typed-nil execution factory was accepted")
	}

	var nilApplication *recordingAgentOpsApplication
	factory := &recordingAgentOpsExecutionFactory{prepared: PreparedTaskExecution{
		Session: app.SessionInfo{ID: "session"},
		Start: func(context.Context) (TaskApplication, error) {
			return nilApplication, nil
		},
	}}
	runner = NewRunner(factory, &orderedAgentOpsMessenger{})
	if err := runner.run(context.Background(), Task{}); err == nil {
		t.Fatal("typed-nil task application was accepted")
	}
}

func TestRunnerDrainPanicCannotReplacePublishedTerminalOutcome(t *testing.T) {
	messenger := &orderedAgentOpsMessenger{}
	application := &recordingAgentOpsApplication{
		outcome: &app.RunOutcome{SessionID: "session", RunID: "run", FinalMessage: "done"},
		drain: func(context.Context) error {
			panic("cleanup panic")
		},
	}
	factory := &recordingAgentOpsExecutionFactory{prepared: PreparedTaskExecution{
		Session: app.SessionInfo{ID: "session"},
		Start: func(context.Context) (TaskApplication, error) {
			return application, nil
		},
	}}

	if err := NewRunner(factory, messenger).run(context.Background(), Task{TaskID: "task", ChatID: "chat"}); err != nil {
		t.Fatalf("run() error = %v, want published success unaffected by cleanup panic", err)
	}
}

type recordingAgentOpsExecutionFactory struct {
	events   *[]string
	request  TaskExecutionRequest
	prepared PreparedTaskExecution
	err      error
}

func (f *recordingAgentOpsExecutionFactory) PrepareTask(_ context.Context, request TaskExecutionRequest) (PreparedTaskExecution, error) {
	f.request = request
	if f.events != nil {
		*f.events = append(*f.events, "prepare")
	}
	return f.prepared, f.err
}

type recordingAgentOpsApplication struct {
	events  *[]string
	outcome *app.RunOutcome
	err     error
	drain   func(context.Context) error
}

func (a *recordingAgentOpsApplication) Run(_ context.Context, _ app.RunCommand, _ app.NotificationSink) (*app.RunOutcome, error) {
	if a.events != nil {
		*a.events = append(*a.events, "run")
	}
	return a.outcome, a.err
}

func (a *recordingAgentOpsApplication) Drain(ctx context.Context) error {
	if a.events != nil {
		*a.events = append(*a.events, "drain")
	}
	if a.drain != nil {
		return a.drain(ctx)
	}
	return nil
}

type orderedAgentOpsMessenger struct {
	events *[]string
}

func (m *orderedAgentOpsMessenger) SendText(_ context.Context, _ string, text string) error {
	if m.events != nil {
		*m.events = append(*m.events, "message:"+text)
	}
	return nil
}
