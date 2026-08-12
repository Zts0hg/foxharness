package agentops

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestUIAOP004ExactDefaultAndFailurePresentation(t *testing.T) {
	t.Run("empty final uses default and exact artifacts", func(t *testing.T) {
		messenger := &agentOpsTransportMessenger{}
		runner := newAgentOpsProfileRunner(t, agentOpsEmptyFinalProvider{}, messenger)
		task := Task{TaskID: "task", ChatID: "chat", SenderID: "sender", MessageID: "message", Text: "incident"}
		if err := runner.run(context.Background(), task); err != nil {
			t.Fatal(err)
		}
		sess, err := runner.sessions.Latest(session.LookupOptions{Source: session.SOURCEFeishu, UserID: "sender", ChatID: "chat"})
		if err != nil {
			t.Fatal(err)
		}
		records, err := session.NewMessageLog(sess).LoadRecords()
		if err != nil || len(records) != 2 || records[0].RunID == "" {
			t.Fatalf("records = %#v, %v", records, err)
		}
		runID := records[0].RunID
		want := []agentOpsTransportMessage{
			{chatID: "chat", text: "已创建 AgentOps Session: " + sess.ID + "\n开始分析。"},
			{chatID: "chat", text: "任务执行完成。\n\nSession: " + sess.ID + "\nRun: " + runID + "\nTrace: " + filepath.Join(sess.RunsDir(), runID, "trace.jsonl") + "\nMetrics: " + filepath.Join(sess.RunsDir(), runID, "metrics.jsonl")},
		}
		if got := messenger.snapshot(); !equalAgentOpsTransportMessages(got, want) {
			t.Fatalf("messages = %#v, want %#v", got, want)
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

type agentOpsEmptyFinalProvider struct{}

func (agentOpsEmptyFinalProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant}}, nil
}

func (agentOpsEmptyFinalProvider) ProviderProtocol() string { return "scripted" }
func (agentOpsEmptyFinalProvider) ModelName() string        { return "claude-4-sonnet" }

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
