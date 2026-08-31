package agentops

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
)

func TestUIAOP004ExactDefaultAndFailurePresentation(t *testing.T) {
	t.Run("empty final uses default and exact artifacts", func(t *testing.T) {
		messenger := &agentOpsTransportMessenger{}
		application := &agentOpsTransportApplication{outcome: &app.RunOutcome{RunID: "run-1"}}
		runner := NewRunner(agentOpsTransportFactory{prepared: PreparedTaskExecution{
			Session:   app.SessionInfo{ID: "session-1"},
			TracePath: "/session/runs/run-1/trace.jsonl", MetricsPath: "/session/runs/run-1/metrics.jsonl",
			Start: func(context.Context) (TaskApplication, error) { return application, nil },
		}}, messenger)
		task := Task{TaskID: "task", ChatID: "chat", SenderID: "sender", MessageID: "message", Text: "incident"}
		if err := runner.run(context.Background(), task); err != nil {
			t.Fatal(err)
		}
		want := []agentOpsTransportMessage{
			{chatID: "chat", text: "已创建 AgentOps Session: session-1\n开始分析。"},
			{chatID: "chat", text: "任务执行完成。\n\nSession: session-1\nRun: run-1\nTrace: /session/runs/run-1/trace.jsonl\nMetrics: /session/runs/run-1/metrics.jsonl"},
		}
		if got := messenger.snapshot(); !equalAgentOpsTransportMessages(got, want) {
			t.Fatalf("messages = %#v, want %#v", got, want)
		}
		if application.drains != 1 {
			t.Fatalf("application drains = %d, want 1", application.drains)
		}
	})

	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "failure", err: errors.New("provider failed"), want: "AgentOps 任务失败： provider failed"},
		{name: "cancellation", err: context.Canceled, want: "AgentOps 任务失败： context canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messenger := &agentOpsTransportMessenger{}
			runner := &Runner{messenger: messenger, runTask: func(context.Context, Task) error { return tc.err }}
			runner.Run(context.Background(), Task{TaskID: tc.name, ChatID: "target-chat"})
			want := []agentOpsTransportMessage{{chatID: "target-chat", text: tc.want}}
			if got := messenger.snapshot(); !equalAgentOpsTransportMessages(got, want) {
				t.Fatalf("messages = %#v, want %#v", got, want)
			}
		})
	}
}

type agentOpsTransportFactory struct{ prepared PreparedTaskExecution }

func (f agentOpsTransportFactory) PrepareTask(context.Context, TaskExecutionRequest) (PreparedTaskExecution, error) {
	return f.prepared, nil
}

type agentOpsTransportApplication struct {
	outcome *app.RunOutcome
	drains  int
}

func (a *agentOpsTransportApplication) Run(context.Context, app.RunCommand, app.NotificationSink) (*app.RunOutcome, error) {
	return a.outcome, nil
}

func (a *agentOpsTransportApplication) Drain(context.Context) error {
	a.drains++
	return nil
}

type agentOpsTransportMessage struct {
	chatID string
	text   string
}

type agentOpsTransportMessenger struct {
	mu       sync.Mutex
	messages []agentOpsTransportMessage
}

func (m *agentOpsTransportMessenger) SendText(_ context.Context, chatID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, agentOpsTransportMessage{chatID: chatID, text: text})
	return nil
}

func (m *agentOpsTransportMessenger) snapshot() []agentOpsTransportMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agentOpsTransportMessage(nil), m.messages...)
}

func equalAgentOpsTransportMessages(got, want []agentOpsTransportMessage) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
