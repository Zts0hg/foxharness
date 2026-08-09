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
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

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
	server            *http.Server
}

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 60 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	approvalCallbackMaxBytes = 64 << 10
)

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
	g.server = g.Server(addr)
	err := g.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Server constructs the gateway-owned HTTP server used by Listen.
func (g *Gateway) Server(addr string) *http.Server {
	mux := http.NewServeMux()
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

	mux.HandleFunc("/webhook/event", httpserverext.NewEventHandlerFunc(
		handler,
		larkevent.WithLogLevel(larkcore.LogLevelInfo),
	))
	mux.HandleFunc("/webhook/approval", g.handleApprovalCallback)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
	}
}

type approvalCallbackRequest struct {
	ApprovalID string `json:"approval_id"`
	Approved   bool   `json:"approved"`
	Reason     string `json:"reason,omitempty"`
}

func (g *Gateway) handleApprovalCallback(w http.ResponseWriter, r *http.Request) {
	if !g.approvalCallbackAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, approvalCallbackMaxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request approvalCallbackRequest
	if err := decoder.Decode(&request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	request.ApprovalID = strings.TrimSpace(request.ApprovalID)
	if request.ApprovalID == "" || g.approvalStore == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := g.OnApprovalCallback(request.ApprovalID, request.Approved, request.Reason); err != nil {
		http.Error(w, "approval request not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (g *Gateway) approvalCallbackAuthorized(r *http.Request) bool {
	const bearerPrefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, bearerPrefix) || g.verificationToken == "" {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix))
	return subtle.ConstantTimeCompare([]byte(provided), []byte(g.verificationToken)) == 1
}

// Shutdown gracefully shuts down a server previously created by Listen.
func (g *Gateway) Shutdown(ctx context.Context) error {
	if g.server == nil {
		return nil
	}
	err := g.server.Shutdown(ctx)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
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
