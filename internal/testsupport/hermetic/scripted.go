package hermetic

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/Zts0hg/foxharness/internal/testsupport/runtimecontract"
)

/* ModelExchange is one scripted model step with optional synchronization. */
type ModelExchange struct {
	Step    runtimecontract.ModelStep
	Barrier *Barrier
}

/* ScriptedModel consumes model exchanges in deterministic order. */
type ScriptedModel struct {
	mu        sync.Mutex
	exchanges []ModelExchange
	next      int
	requests  []runtimecontract.ModelRequest
}

/* NewScriptedModel copies a model script. */
func NewScriptedModel(exchanges []ModelExchange) *ScriptedModel {
	return &ScriptedModel{exchanges: append([]ModelExchange(nil), exchanges...)}
}

/* Invoke records a request, waits at the controlled barrier, and returns its step. */
func (m *ScriptedModel) Invoke(ctx context.Context, request runtimecontract.ModelRequest, onDelta func(string)) (runtimecontract.ModelResponse, error) {
	m.mu.Lock()
	if m.next >= len(m.exchanges) {
		m.mu.Unlock()
		return runtimecontract.ModelResponse{}, errors.New("model script exhausted")
	}
	exchange := m.exchanges[m.next]
	m.next++
	m.requests = append(m.requests, cloneModelRequest(request))
	m.mu.Unlock()

	if !modelRequestMatches(exchange.Step.Request, request) {
		return runtimecontract.ModelResponse{}, fmt.Errorf("model request mismatch: got %#v want %#v", request, exchange.Step.Request)
	}
	if exchange.Barrier != nil {
		if err := exchange.Barrier.wait(ctx); err != nil {
			return runtimecontract.ModelResponse{}, err
		}
	}
	for _, delta := range exchange.Step.Deltas {
		if onDelta != nil {
			onDelta(delta)
		}
	}
	if exchange.Step.Error != "" {
		return runtimecontract.ModelResponse{}, errors.New(exchange.Step.Error)
	}
	return exchange.Step.Response, nil
}

/* Requests returns independent request snapshots. */
func (m *ScriptedModel) Requests() []runtimecontract.ModelRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]runtimecontract.ModelRequest, len(m.requests))
	for i, request := range m.requests {
		requests[i] = cloneModelRequest(request)
	}
	return requests
}

func modelRequestMatches(want, got runtimecontract.ModelRequest) bool {
	return reflect.DeepEqual(want, runtimecontract.ModelRequest{}) || reflect.DeepEqual(want, got)
}

func cloneModelRequest(request runtimecontract.ModelRequest) runtimecontract.ModelRequest {
	request.Messages = append([]runtimecontract.Message(nil), request.Messages...)
	request.ToolDefinitions = append([]runtimecontract.ToolDefinition(nil), request.ToolDefinitions...)
	return request
}

/* ToolExchange is one correlated tool behavior. */
type ToolExchange struct {
	Behavior     runtimecontract.ToolBehavior
	Aliases      []string
	ParallelSafe bool
	Barrier      *Barrier
}

/* ScriptedTools consumes correlated tool exchanges and preserves call order. */
type ScriptedTools struct {
	mu        sync.Mutex
	exchanges []ToolExchange
	next      int
	calls     []runtimecontract.ToolCall
}

/* NewScriptedTools copies a tool script. */
func NewScriptedTools(exchanges []ToolExchange) *ScriptedTools {
	copyExchanges := append([]ToolExchange(nil), exchanges...)
	for i := range copyExchanges {
		copyExchanges[i].Aliases = append([]string(nil), copyExchanges[i].Aliases...)
	}
	return &ScriptedTools{exchanges: copyExchanges}
}

/* Execute consumes the next exchange after validating correlation and aliasing. */
func (t *ScriptedTools) Execute(ctx context.Context, call runtimecontract.ToolCall) (runtimecontract.ToolResult, error) {
	t.mu.Lock()
	if t.next >= len(t.exchanges) {
		t.mu.Unlock()
		return runtimecontract.ToolResult{}, errors.New("tool script exhausted")
	}
	exchange := t.exchanges[t.next]
	t.next++
	t.calls = append(t.calls, call)
	t.mu.Unlock()

	want := exchange.Behavior.Call
	if call.ID != want.ID || call.Arguments != want.Arguments || !matchesToolName(call.Name, want.Name, exchange.Aliases) {
		return runtimecontract.ToolResult{}, fmt.Errorf("tool call mismatch: got %#v want %#v", call, want)
	}
	if exchange.Barrier != nil {
		if err := exchange.Barrier.wait(ctx); err != nil {
			return runtimecontract.ToolResult{}, err
		}
	}
	return exchange.Behavior.Result, nil
}

/* ParallelSafe reports the frozen execution declaration for a name or alias. */
func (t *ScriptedTools) ParallelSafe(name string) bool {
	for _, exchange := range t.exchanges {
		if matchesToolName(name, exchange.Behavior.Call.Name, exchange.Aliases) {
			return exchange.ParallelSafe
		}
	}
	return false
}

/* Calls returns independent call snapshots. */
func (t *ScriptedTools) Calls() []runtimecontract.ToolCall {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]runtimecontract.ToolCall(nil), t.calls...)
}

func matchesToolName(got, canonical string, aliases []string) bool {
	if got == canonical {
		return true
	}
	for _, alias := range aliases {
		if got == alias {
			return true
		}
	}
	return false
}

/* ScriptedInteractions consumes correlated replies in deterministic order. */
type ScriptedInteractions struct {
	mu      sync.Mutex
	replies []runtimecontract.InteractionReply
	next    int
}

/* NewScriptedInteractions copies controlled replies. */
func NewScriptedInteractions(replies []runtimecontract.InteractionReply) *ScriptedInteractions {
	return &ScriptedInteractions{replies: append([]runtimecontract.InteractionReply(nil), replies...)}
}

/* Reply returns the next matching interaction reply. */
func (s *ScriptedInteractions) Reply(ctx context.Context, kind, correlationID string) (runtimecontract.InteractionReply, error) {
	select {
	case <-ctx.Done():
		return runtimecontract.InteractionReply{}, ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next >= len(s.replies) {
		return runtimecontract.InteractionReply{}, errors.New("interaction script exhausted")
	}
	reply := s.replies[s.next]
	s.next++
	if reply.Kind != kind || reply.CorrelationID != correlationID {
		return runtimecontract.InteractionReply{}, fmt.Errorf("interaction mismatch: got %s/%s want %s/%s", kind, correlationID, reply.Kind, reply.CorrelationID)
	}
	if reply.Error != "" {
		return reply, errors.New(reply.Error)
	}
	return reply, nil
}
