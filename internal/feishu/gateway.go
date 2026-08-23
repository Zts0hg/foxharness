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
	"sync"
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
	deliveryStore     DeliveryStore
	server            *http.Server
	admissionMu       sync.Mutex
	admissionClosed   bool
	activeDeliveries  int
	admissionDrained  chan struct{}
	admissionAbort    chan struct{}
	admissionAborted  bool
}

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 60 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	approvalCallbackMaxBytes = 64 << 10
	deliveryRollbackTimeout  = 5 * time.Second
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
		deliveryStore:     newMemoryDeliveryStore(),
		admissionDrained:  make(chan struct{}),
		admissionAbort:    make(chan struct{}),
	}
}

// WithDeliveryStore installs the durable message acceptance authority.
func (g *Gateway) WithDeliveryStore(store DeliveryStore) *Gateway {
	if store != nil {
		g.deliveryStore = store
	}
	return g
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
		abortDelivery, ok := g.beginDelivery()
		if !ok {
			return errors.New("Feishu gateway is shutting down")
		}
		defer g.finishDelivery()

		task, err := taskFromMessageEvent(event)
		if err != nil {
			log.Printf("[Feishu Gateway] ignore message event: %v", err)
			return nil
		}
		deliveryCtx, cancelDelivery := context.WithCancel(ctx)
		defer cancelDelivery()
		go cancelDeliveryOnAbort(deliveryCtx, cancelDelivery, abortDelivery)
		accepted, err := g.reserveDelivery(deliveryCtx, task.MessageID)
		if err != nil && !accepted {
			return fmt.Errorf("reserve Feishu delivery %s: %w", task.MessageID, err)
		}
		if !accepted {
			log.Printf("[Feishu Gateway] duplicate message ignored: %s", task.MessageID)
			return nil
		}
		if err != nil && deliveryCtx.Err() != nil {
			return g.rollbackDelivery(task.MessageID, errors.Join(errors.New("Feishu gateway is shutting down"), err))
		}
		if err != nil {
			log.Printf("[Feishu Gateway] accepted delivery %s with post-commit store error: %v", task.MessageID, err)
		}
		if err := g.rollbackIfDeliveryAborted(abortDelivery, task.MessageID); err != nil {
			return err
		}

		select {
		case g.tasks <- task:
			return nil
		case <-abortDelivery:
			return g.rollbackDelivery(task.MessageID, errors.New("Feishu gateway is shutting down"))
		case <-ctx.Done():
			return g.rollbackDelivery(task.MessageID, ctx.Err())
		default:
			return g.rollbackDelivery(task.MessageID, errors.New("Feishu task queue unavailable"))
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

// StopAccepting prevents future message events from reserving or enqueueing
// work and waits until already-started delivery handlers have returned.
func (g *Gateway) StopAccepting(ctx context.Context) error {
	g.admissionMu.Lock()
	if !g.admissionClosed {
		g.admissionClosed = true
		if g.activeDeliveries == 0 {
			close(g.admissionDrained)
		}
	}
	done := g.admissionDrained
	g.admissionMu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		g.abortDeliveries()
		<-done
		return ctx.Err()
	}
}

func (g *Gateway) abortDeliveries() {
	g.admissionMu.Lock()
	defer g.admissionMu.Unlock()
	if !g.admissionAborted {
		g.admissionAborted = true
		close(g.admissionAbort)
	}
}

func (g *Gateway) beginDelivery() (<-chan struct{}, bool) {
	g.admissionMu.Lock()
	defer g.admissionMu.Unlock()
	if g.admissionClosed {
		return nil, false
	}
	g.activeDeliveries++
	return g.admissionAbort, true
}

func (g *Gateway) finishDelivery() {
	g.admissionMu.Lock()
	defer g.admissionMu.Unlock()
	g.activeDeliveries--
	if g.admissionClosed && g.activeDeliveries == 0 {
		close(g.admissionDrained)
	}
}

func (g *Gateway) rollbackIfDeliveryAborted(abortDelivery <-chan struct{}, messageID string) error {
	select {
	case <-abortDelivery:
		return g.rollbackDelivery(messageID, errors.New("Feishu gateway is shutting down"))
	default:
		return nil
	}
}

func (g *Gateway) rollbackDelivery(messageID string, cause error) error {
	rollbackCtx, cancelRollback := context.WithTimeout(context.Background(), deliveryRollbackTimeout)
	defer cancelRollback()
	if releaseErr := g.releaseDelivery(rollbackCtx, messageID); releaseErr != nil {
		return errors.Join(cause, fmt.Errorf("rollback Feishu delivery %s: %w", messageID, releaseErr))
	}
	return cause
}

func (g *Gateway) reserveDelivery(ctx context.Context, messageID string) (bool, error) {
	if store, ok := g.deliveryStore.(contextDeliveryStore); ok {
		return store.ReserveContext(ctx, messageID)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return g.deliveryStore.Reserve(messageID)
}

func (g *Gateway) releaseDelivery(ctx context.Context, messageID string) error {
	if store, ok := g.deliveryStore.(contextDeliveryStore); ok {
		return store.ReleaseContext(ctx, messageID)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return g.deliveryStore.Release(messageID)
}

func cancelDeliveryOnAbort(ctx context.Context, cancel context.CancelFunc, abort <-chan struct{}) {
	select {
	case <-abort:
		cancel()
	case <-ctx.Done():
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
		if errors.Is(err, approval.ErrConflict) {
			http.Error(w, "approval request already resolved", http.StatusConflict)
			return
		}
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
		senderID = strings.TrimSpace(*event.Event.Sender.SenderId.OpenId)
	}
	if senderID == "" {
		return Task{}, fmt.Errorf("消息事件缺少 sender open_id")
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
