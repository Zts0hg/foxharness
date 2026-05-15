// Package feishu provides Feishu (Lark) integration components for the
// foxharness agent framework.  It implements an HTTP webhook gateway that
// receives message events from the Feishu bot platform, converts them into
// Task values, and dispatches them to a Runner for execution.  It also
// exposes an approval callback endpoint so that human operators can approve
// or reject dangerous tool invocations initiated by the agent.
package feishu

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/Zts0hg/foxharness/internal/approval"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/core/httpserverext"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// Gateway receives Feishu webhook events over HTTP, deserialises message
// payloads into Task values, and pushes them onto the tasks channel for
// consumption by a Runner.  It also handles approval callback resolution.
type Gateway struct {
	verificationToken string
	encryptKey        string
	tasks             chan<- Task
	approvalStore     *approval.Store
}

// NewGateway creates a Gateway that validates incoming events with the given
// verificationToken and encryptKey, dispatches parsed tasks to the tasks
// channel, and resolves approval requests through approvalStore.
func NewGateway(verificationToken, encryptKey string, tasks chan<- Task, approvalStore *approval.Store) *Gateway {
	return &Gateway{
		verificationToken: verificationToken,
		encryptKey:        encryptKey,
		tasks:             tasks,
		approvalStore:     approvalStore,
	}
}

// Listen registers the Feishu event dispatcher on /webhook/event and starts
// an HTTP server bound to addr.  It blocks until the server exits.
func (g *Gateway) Listen(addr string) error {
	handler := dispatcher.NewEventDispatcher(g.verificationToken, g.encryptKey)

	handler.OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
		task, err := taskFromMessageEvent(event)
		if err != nil {
			log.Printf("[Feishu Gateway] ignore message event: %v", err)
			return nil
		}

		select {
		case g.tasks <- task:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	http.HandleFunc("/webhook/event", httpserverext.NewEventHandlerFunc(
		handler,
		larkevent.WithLogLevel(larkcore.LogLevelInfo),
	))

	return http.ListenAndServe(addr, nil)
}

func taskFromMessageEvent(event *larkim.P2MessageReceiveV1) (Task, error) {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return Task{}, fmt.Errorf("空的飞书消息事件")
	}

	msg := event.Event.Message
	if msg.ChatId == nil || msg.MessageId == nil {
		return Task{}, fmt.Errorf("消息事件缺少 chat_id 或 message_id")
	}

	text := extractText(msg.Content)
	text = strings.TrimSpace(text)
	if text == "" {
		return Task{}, fmt.Errorf("消息文本为空")
	}

	senderID := ""
	if event.Event.Sender != nil &&
		event.Event.Sender.SenderId != nil &&
		event.Event.Sender.SenderId.OpenId != nil {
		senderID = *event.Event.Sender.SenderId.OpenId
	}

	return Task{
		TaskID:    newTaskID(),
		ChatID:    *msg.ChatId,
		SenderID:  senderID,
		MessageID: *msg.MessageId,
		Text:      text,
	}, nil

}

func extractText(content *string) string {
	if content == nil {
		return ""
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(*content), &payload); err != nil {
		return *content
	}
	return payload.Text
}

func newTaskID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// OnApprovalCallback resolves a pending approval request identified by
// approvalID with the operator's decision and optional reason.
func (g *Gateway) OnApprovalCallback(approvalID string, approved bool, reason string) error {
	return g.approvalStore.Resolve(approvalID, approval.Result{
		Approved: approved,
		Reason:   reason,
	})
}
