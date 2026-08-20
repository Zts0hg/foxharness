package feishu

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/approval"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestUIFEI002GatewayMuxPathsMethodsAndTimeouts(t *testing.T) {
	gateway := NewGateway("token", "encrypt", make(chan Task, 1), approval.NewStore())
	server := gateway.Server(":7777")
	if server.Addr != ":7777" || server.Handler == nil || server.Handler == http.DefaultServeMux {
		t.Fatalf("server ownership = addr %q handler %T", server.Addr, server.Handler)
	}
	if server.ReadHeaderTimeout != defaultReadHeaderTimeout || server.ReadTimeout != defaultReadTimeout || server.WriteTimeout != defaultWriteTimeout || server.IdleTimeout != defaultIdleTimeout {
		t.Fatalf("timeouts = %s/%s/%s/%s", server.ReadHeaderTimeout, server.ReadTimeout, server.WriteTimeout, server.IdleTimeout)
	}

	unknown := httptest.NewRecorder()
	server.Handler.ServeHTTP(unknown, httptest.NewRequest(http.MethodPost, "/unknown", nil))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown path status = %d, want 404", unknown.Code)
	}
	wrongApprovalMethod := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/webhook/approval", nil)
	req.Header.Set("Authorization", "Bearer token")
	server.Handler.ServeHTTP(wrongApprovalMethod, req)
	if wrongApprovalMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("approval GET status = %d, want 405", wrongApprovalMethod.Code)
	}
	unauthorized := postApprovalCallback(server.Handler, "", `{}`)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized approval status = %d", unauthorized.Code)
	}
}

func TestUIFEI003EventTranslationMatrix(t *testing.T) {
	if _, err := taskFromMessageEvent(nil); err == nil {
		t.Fatal("nil event accepted")
	}
	for _, tc := range []struct {
		name    string
		mutate  func(*larkim.P2MessageReceiveV1)
		wantErr string
	}{
		{name: "missing event", mutate: func(e *larkim.P2MessageReceiveV1) { e.Event = nil }, wantErr: "空的飞书消息事件"},
		{name: "missing message", mutate: func(e *larkim.P2MessageReceiveV1) { e.Event.Message = nil }, wantErr: "空的飞书消息事件"},
		{name: "missing chat", mutate: func(e *larkim.P2MessageReceiveV1) { e.Event.Message.ChatId = nil }, wantErr: "消息事件缺少 chat_id 或 message_id"},
		{name: "missing message id", mutate: func(e *larkim.P2MessageReceiveV1) { e.Event.Message.MessageId = nil }, wantErr: "消息事件缺少 chat_id 或 message_id"},
		{name: "missing sender", mutate: func(e *larkim.P2MessageReceiveV1) { e.Event.Sender = nil }, wantErr: "消息事件缺少 sender open_id"},
		{name: "nil content", mutate: func(e *larkim.P2MessageReceiveV1) { e.Event.Message.Content = nil }, wantErr: "消息文本为空"},
		{name: "whitespace", mutate: func(e *larkim.P2MessageReceiveV1) { text := `{"text":"  "}`; e.Event.Message.Content = &text }, wantErr: "消息文本为空"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := messageEvent("event", "message", true)
			tc.mutate(event)
			if task, err := taskFromMessageEvent(event); err == nil || err.Error() != tc.wantErr {
				t.Fatalf("taskFromMessageEvent() = %#v, %v; want %q", task, err, tc.wantErr)
			}
		})
	}

	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "json text", content: `{"text":"  run task  "}`, want: "run task"},
		{name: "raw text", content: "  raw task  ", want: "raw task"},
		{name: "malformed json is raw", content: `{bad`, want: `{bad`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := messageEvent("event", "message", true)
			event.Event.Message.Content = &tc.content
			task, err := taskFromMessageEvent(event)
			if err != nil || task.Text != tc.want || task.ChatID != "chat-1" || task.SenderID != "sender-1" || task.MessageID != "message" {
				t.Fatalf("translated task = %#v, %v", task, err)
			}
			if len(task.TaskID) != 16 {
				t.Fatalf("task ID = %q, want 16 hex chars", task.TaskID)
			}
			if _, err := hex.DecodeString(task.TaskID); err != nil {
				t.Fatalf("task ID is not hex: %v", err)
			}
		})
	}

	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
		wantTask   bool
	}{
		{name: "malformed", body: `{`, wantStatus: http.StatusInternalServerError},
		{name: "unknown event", body: `{"schema":"2.0","header":{"event_id":"e","event_type":"unknown","token":"token"},"event":{}}`, wantStatus: http.StatusOK},
		{name: "unsupported message type remains accepted", body: strings.Replace(messageEventJSON("e", "m", true), `"message_type":"text"`, `"message_type":"image"`, 1), wantStatus: http.StatusOK, wantTask: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tasks := make(chan Task, 1)
			handler := NewGateway("token", "", tasks, approval.NewStore()).Server(":0").Handler
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/webhook/event", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			handler.ServeHTTP(recorder, req)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("event status = %d body=%q, want %d", recorder.Code, recorder.Body.String(), tc.wantStatus)
			}
			if tc.wantTask {
				select {
				case task := <-tasks:
					if task.MessageID != "m" || task.Text != "run task" {
						t.Fatalf("accepted unsupported task = %#v", task)
					}
				default:
					t.Fatal("current unsupported-message delivery was not accepted")
				}
			} else {
				assertNoTask(t, tasks)
			}
		})
	}
}

func TestUIFEI003ChallengeAndEncryptedMessageHandling(t *testing.T) {
	tasks := make(chan Task, 1)
	challengeHandler := NewGateway("token", "", tasks, approval.NewStore()).Server(":0").Handler

	challenge := httptest.NewRecorder()
	challengeRequest := httptest.NewRequest(http.MethodPost, "/webhook/event", strings.NewReader(`{"challenge":"challenge-1","token":"token","type":"url_verification"}`))
	challengeRequest.Header.Set("Content-Type", "application/json")
	challengeHandler.ServeHTTP(challenge, challengeRequest)
	if challenge.Code != http.StatusOK || strings.TrimSpace(challenge.Body.String()) != `{"challenge":"challenge-1"}` {
		t.Fatalf("challenge response = %d/%q", challenge.Code, challenge.Body.String())
	}
	assertNoTask(t, tasks)

	encrypted, err := larkcore.EncryptedEventMsg(context.Background(), []byte(messageEventJSON("encrypted-event", "encrypted-message", true)), "encrypt-key")
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{"encrypt": encrypted})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webhook/event", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(larkevent.EventRequestTimestamp, "timestamp")
	request.Header.Set(larkevent.EventRequestNonce, "nonce")
	request.Header.Set(larkevent.EventSignature, larkevent.Signature("timestamp", "nonce", "encrypt-key", string(body)))
	handler := NewGateway("token", "encrypt-key", tasks, approval.NewStore()).Server(":0").Handler
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("encrypted event response = %d/%q", recorder.Code, recorder.Body.String())
	}
	select {
	case task := <-tasks:
		if task.MessageID != "encrypted-message" || task.ChatID != "chat-1" || task.SenderID != "sender-1" {
			t.Fatalf("encrypted task = %#v", task)
		}
	default:
		t.Fatal("encrypted event did not enqueue a task")
	}
}

func TestUIFEI005ReporterAndTerminalMessagesAreExactAndCorrelated(t *testing.T) {
	client := &recordingLarkClient{}
	messenger := newRecordingMessenger(client)
	reporter := NewReporter(messenger, "chat-1", "task-1")
	ctx := context.Background()
	reporter.Notify(ctx, app.Notification{Kind: app.NotificationRunStarted, SessionID: "session-1", RunID: "run-1"})
	reporter.Notify(ctx, app.Notification{Kind: app.NotificationThinking, Turn: 2})
	reporter.Notify(ctx, app.Notification{Kind: app.NotificationContextCompacted, Name: "turn"})
	reporter.Notify(ctx, app.Notification{Kind: app.NotificationToolCall, Name: "read_file", Content: `{"path":"a.txt"}`})
	reporter.Notify(ctx, app.Notification{Kind: app.NotificationToolResult, Name: "read_file", Content: "contents"})
	reporter.Notify(ctx, app.Notification{Kind: app.NotificationToolResult, Name: "bash", Content: "failed", IsError: true})
	reporter.Notify(ctx, app.Notification{Kind: app.NotificationMessage, Content: "final assistant"})
	reporter.Notify(ctx, app.Notification{Kind: app.NotificationRunCompleted})
	reporter.Notify(ctx, app.Notification{Kind: app.NotificationRunError, Content: errors.New("ignored").Error()})

	runner := &Runner{messenger: messenger}
	task := Task{TaskID: "task-1", ChatID: "chat-1"}
	runner.deliverTaskText(ctx, task, DeliveryStageReceipt, "已收到任务 task-1，开始执行。")
	runner.deliverTaskText(ctx, task, DeliveryStageSession, "任务已进入新 Session: session-1")
	runner.deliverTaskText(ctx, task, DeliveryStageFinal, "任务 task-1 已完成，Session: session-1，Run: run-1")
	runner.deliverCancellation(task, context.DeadlineExceeded)

	want := []recordedLarkMessage{
		{chatID: "chat-1", text: "任务 task-1：Run run-1 已开始，Session: session-1。"},
		{chatID: "chat-1", text: "任务 task-1：第 2 轮正在规划。"},
		{chatID: "chat-1", text: "任务 task-1：上下文已压缩（turn）。"},
		{chatID: "chat-1", text: "任务 task-1：准备执行工具 read_file。\n参数：{\"path\":\"a.txt\"}"},
		{chatID: "chat-1", text: "任务 task-1：工具 read_file 执行成功。\n输出摘要：contents"},
		{chatID: "chat-1", text: "任务 task-1：工具 bash 执行失败。\nfailed"},
		{chatID: "chat-1", text: "final assistant"},
		{chatID: "chat-1", text: "已收到任务 task-1，开始执行。"},
		{chatID: "chat-1", text: "任务已进入新 Session: session-1"},
		{chatID: "chat-1", text: "任务 task-1 已完成，Session: session-1，Run: run-1"},
		{chatID: "chat-1", text: "任务 task-1 已取消：context deadline exceeded"},
	}
	if got := client.snapshot(); !equalRecordedMessages(got, want) {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}
}

func TestUIFEI006UnicodeBoundsEmptySuppressionAndTransportErrors(t *testing.T) {
	client := &recordingLarkClient{}
	messenger := newRecordingMessenger(client)
	reporter := NewReporter(messenger, "chat", "task")
	reporter.Notify(context.Background(), app.Notification{Kind: app.NotificationMessage, Content: "   "})
	if got := client.snapshot(); len(got) != 0 {
		t.Fatalf("empty message delivered: %#v", got)
	}

	input := strings.Repeat("界", 1900)
	if err := messenger.SendText(context.Background(), "chat", input); err != nil {
		t.Fatal(err)
	}
	got := client.snapshot()
	if len(got) != 1 || got[0].chatID != "chat" || len([]rune(got[0].text)) > maxFeishuTextRunes || !strings.Contains(got[0].text, "已截断") || strings.ContainsRune(got[0].text, '\uFFFD') {
		t.Fatalf("bounded Unicode delivery = %#v", got)
	}

	transportErr := errors.New("sdk failed")
	failing := newRecordingMessenger(&recordingLarkClient{err: transportErr})
	if err := failing.SendText(context.Background(), "chat", "text"); err == nil || !strings.Contains(err.Error(), "发送飞书消息失败") || !errors.Is(err, transportErr) {
		t.Fatalf("SDK transport error = %v", err)
	}
	apiFailure := newRecordingMessenger(&recordingLarkClient{response: `{"code":999,"msg":"rejected"}`})
	if err := apiFailure.SendText(context.Background(), "chat", "text"); err == nil || !strings.Contains(err.Error(), "code=999") {
		t.Fatalf("API response error = %v", err)
	}
}

type recordedLarkMessage struct {
	chatID string
	text   string
}

type recordingLarkClient struct {
	mu       sync.Mutex
	messages []recordedLarkMessage
	err      error
	response string
}

func (c *recordingLarkClient) Do(request *http.Request) (*http.Response, error) {
	if c.err != nil {
		return nil, c.err
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		ReceiveID string `json:"receive_id"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if envelope.ReceiveID == "" {
		envelope.ReceiveID = request.URL.Query().Get("receive_id")
	}
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(envelope.Content), &content); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.messages = append(c.messages, recordedLarkMessage{chatID: envelope.ReceiveID, text: content.Text})
	c.mu.Unlock()
	response := c.response
	if response == "" {
		response = `{"code":0}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
}

func (c *recordingLarkClient) snapshot() []recordedLarkMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]recordedLarkMessage(nil), c.messages...)
}

func newRecordingMessenger(client *recordingLarkClient) *Messenger {
	return &Messenger{client: lark.NewClient("app", "secret", lark.WithEnableTokenCache(false), lark.WithHttpClient(client))}
}

func equalRecordedMessages(a, b []recordedLarkMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
