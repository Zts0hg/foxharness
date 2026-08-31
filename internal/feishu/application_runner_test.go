package feishu

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
)

func TestRunnerExecutesPreparedApplicationWithoutOwningRuntime(t *testing.T) {
	messenger := &recordingTextMessenger{}
	useCase := &recordingRunUseCase{outcome: &app.RunOutcome{
		SessionID: "session-1", RunID: "run-1", FinalMessage: "finished",
	}}
	factory := &recordingTaskExecutionFactory{prepared: PreparedTaskExecution{
		Application: useCase,
		Session:     app.SessionInfo{ID: "session-1"},
		Created:     true,
	}}
	runner := NewRunner(factory, messenger)
	task := Task{TaskID: "task-1", ChatID: "chat-1", SenderID: "sender-1", MessageID: "message-1", Text: " /new inspect "}

	runner.runOne(context.Background(), task)

	wantRequest := TaskExecutionRequest{
		Task:            task,
		Prompt:          "以下任务来自飞书用户 sender-1，消息 ID 为 message-1。\n\ninspect",
		ForceNewSession: true,
	}
	if !reflect.DeepEqual(factory.request, wantRequest) {
		t.Fatalf("execution request = %#v, want %#v", factory.request, wantRequest)
	}
	if got := useCase.command.Prompt; got != wantRequest.Prompt {
		t.Fatalf("application prompt = %q, want %q", got, wantRequest.Prompt)
	}
	wantMessages := []string{
		"已收到任务 task-1，开始执行。",
		"任务已进入新 Session: session-1",
		"任务 task-1：Run run-1 已开始，Session: session-1。",
		"finished",
		"任务 task-1 已完成，Session: session-1，Run: run-1",
	}
	if !reflect.DeepEqual(messenger.texts, wantMessages) {
		t.Fatalf("messages = %#v, want %#v", messenger.texts, wantMessages)
	}
}

func TestRunnerDrainsPreparedApplicationAfterPartialFailure(t *testing.T) {
	runErr := errors.New("provider failed")
	drained := 0
	drainHasDeadline := false
	useCase := &recordingRunUseCase{
		outcome: &app.RunOutcome{SessionID: "session-1", RunID: "run-1", FinalMessage: "partial", Partial: true},
		err:     runErr,
	}
	factory := &recordingTaskExecutionFactory{prepared: PreparedTaskExecution{
		Application: useCase,
		Session:     app.SessionInfo{ID: "session-1"},
		Drain: func(ctx context.Context) error {
			drained++
			_, drainHasDeadline = ctx.Deadline()
			return nil
		},
	}}
	messenger := &recordingTextMessenger{}

	NewRunner(factory, messenger).runOne(context.Background(), Task{TaskID: "task", ChatID: "chat", Text: "inspect"})

	if drained != 1 {
		t.Fatalf("drain calls = %d, want 1", drained)
	}
	if !drainHasDeadline {
		t.Fatal("drain context has no fresh cleanup deadline")
	}
	want := "Session session-1 执行失败：provider failed"
	if got := messenger.texts[len(messenger.texts)-1]; got != want {
		t.Fatalf("terminal message = %q, want %q", got, want)
	}
}

func TestRunnerDrainFailurePreventsSuccessfulTerminalStatus(t *testing.T) {
	for _, test := range []struct {
		name  string
		drain func(context.Context) error
		want  string
	}{
		{name: "error", drain: func(context.Context) error { return errors.New("close failed") }, want: "close failed"},
		{name: "panic", drain: func(context.Context) error { panic("cleanup panic") }, want: "cleanup panic"},
	} {
		t.Run(test.name, func(t *testing.T) {
			messenger := &recordingTextMessenger{}
			factory := &recordingTaskExecutionFactory{prepared: PreparedTaskExecution{
				Application: &recordingRunUseCase{outcome: &app.RunOutcome{
					SessionID: "session-1", RunID: "run-1", FinalMessage: "finished",
				}},
				Session: app.SessionInfo{ID: "session-1"},
				Drain:   test.drain,
			}}

			NewRunner(factory, messenger).runOne(
				context.Background(),
				Task{TaskID: "task", ChatID: "chat", Text: "inspect"},
			)

			last := messenger.texts[len(messenger.texts)-1]
			if !strings.Contains(last, "执行失败") || !strings.Contains(last, test.want) {
				t.Fatalf("terminal message = %q, want cleanup failure containing %q", last, test.want)
			}
			for _, message := range messenger.texts {
				if strings.Contains(message, "任务 task 已完成") {
					t.Fatalf("successful terminal status was published before cleanup completed: %v", messenger.texts)
				}
			}
		})
	}
}

func TestRunnerUsesFreshTerminalContextAfterDrainCancelsRunContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	messenger := &terminalContextTextMessenger{}
	factory := &recordingTaskExecutionFactory{prepared: PreparedTaskExecution{
		Application: &recordingRunUseCase{outcome: &app.RunOutcome{
			SessionID: "session-1", RunID: "run-1", FinalMessage: "finished",
		}},
		Session: app.SessionInfo{ID: "session-1"},
		Drain: func(context.Context) error {
			cancel()
			return nil
		},
	}}

	NewRunner(factory, messenger).runOne(ctx, Task{TaskID: "task", ChatID: "chat", Text: "inspect"})

	if !messenger.terminalCalled || messenger.terminalContextErr != nil {
		t.Fatalf("terminal delivery called/context error = %v/%v, want true/<nil>", messenger.terminalCalled, messenger.terminalContextErr)
	}
}

func TestRunnerDrainsPreparedApplicationBeforePublishingRunPanic(t *testing.T) {
	drained := 0
	observer := &recordingTaskOutcomeObserver{}
	messenger := &recordingTextMessenger{}
	factory := &recordingTaskExecutionFactory{prepared: PreparedTaskExecution{
		Application: &recordingRunUseCase{panicValue: "run panic"},
		Session:     app.SessionInfo{ID: "session-1"},
		Drain: func(context.Context) error {
			drained++
			return nil
		},
	}}
	runner := NewRunner(factory, messenger)
	runner.taskOutcomeObserver = observer
	tasks := make(chan Task, 1)
	tasks <- Task{TaskID: "task", ChatID: "chat", Text: "inspect"}
	close(tasks)

	runner.Start(context.Background(), tasks)

	if drained != 1 {
		t.Fatalf("drain calls = %d, want 1", drained)
	}
	outcomes := observer.snapshot()
	if len(outcomes) != 1 || outcomes[0].Status != TaskOutcomeFailed || !strings.Contains(outcomes[0].Error, "run panic") {
		t.Fatalf("task outcomes = %#v, want one panic failure", outcomes)
	}
}

func TestRunnerReportsRuntimeSetupFailureAfterSelectedSessionNotice(t *testing.T) {
	messenger := &recordingTextMessenger{}
	factory := &recordingTaskExecutionFactory{prepared: PreparedTaskExecution{
		Session:    app.SessionInfo{ID: "session-1"},
		Created:    true,
		SetupError: errors.New("compactor unavailable"),
	}}

	NewRunner(factory, messenger).runOne(context.Background(), Task{TaskID: "task", ChatID: "chat", Text: "inspect"})

	want := []string{
		"已收到任务 task，开始执行。",
		"任务已进入新 Session: session-1",
		"初始化任务执行环境失败：compactor unavailable",
	}
	if !reflect.DeepEqual(messenger.texts, want) {
		t.Fatalf("setup-failure messages = %#v, want %#v", messenger.texts, want)
	}
}

func TestRunnerRejectsTypedNilExecutionDependencies(t *testing.T) {
	var nilFactory *recordingTaskExecutionFactory
	runner := NewRunner(nilFactory, nil)
	if _, err := runner.prepareTask(context.Background(), TaskExecutionRequest{}); err == nil {
		t.Fatal("typed-nil task factory was accepted")
	}

	var nilApplication *recordingRunUseCase
	runner = NewRunner(&recordingTaskExecutionFactory{prepared: PreparedTaskExecution{
		Application: nilApplication,
		Session:     app.SessionInfo{ID: "session"},
	}}, nil)
	if _, err := runner.prepareTask(context.Background(), TaskExecutionRequest{}); err == nil {
		t.Fatal("typed-nil task application was accepted")
	}
}

type recordingTaskExecutionFactory struct {
	request  TaskExecutionRequest
	prepared PreparedTaskExecution
	err      error
}

func (f *recordingTaskExecutionFactory) PrepareTask(_ context.Context, request TaskExecutionRequest) (PreparedTaskExecution, error) {
	f.request = request
	return f.prepared, f.err
}

type recordingRunUseCase struct {
	command    app.RunCommand
	sink       app.NotificationSink
	outcome    *app.RunOutcome
	err        error
	panicValue any
}

func (u *recordingRunUseCase) Run(ctx context.Context, command app.RunCommand, sink app.NotificationSink) (*app.RunOutcome, error) {
	u.command = command
	u.sink = sink
	if u.panicValue != nil {
		panic(u.panicValue)
	}
	if sink != nil && u.outcome != nil {
		sink.Notify(ctx, app.Notification{Kind: app.NotificationRunStarted, SessionID: u.outcome.SessionID, RunID: u.outcome.RunID})
		if u.outcome.FinalMessage != "" {
			sink.Notify(ctx, app.Notification{Kind: app.NotificationMessage, SessionID: u.outcome.SessionID, RunID: u.outcome.RunID, Content: u.outcome.FinalMessage})
		}
	}
	return u.outcome, u.err
}

type recordingTextMessenger struct {
	texts []string
}

func (m *recordingTextMessenger) SendText(_ context.Context, _ string, text string) error {
	m.texts = append(m.texts, text)
	return nil
}

type terminalContextTextMessenger struct {
	terminalCalled     bool
	terminalContextErr error
}

func (m *terminalContextTextMessenger) SendText(ctx context.Context, _ string, text string) error {
	if strings.Contains(text, "任务 task 已完成") {
		m.terminalCalled = true
		m.terminalContextErr = ctx.Err()
	}
	return ctx.Err()
}
