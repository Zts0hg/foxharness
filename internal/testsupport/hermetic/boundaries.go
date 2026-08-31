package hermetic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sync"
)

/* ProcessRequest identifies a bounded process invocation. */
type ProcessRequest struct {
	Dir  string
	Name string
	Args []string
	Env  map[string]string
}

/* ProcessResponse is a controlled process outcome and process-tree snapshot. */
type ProcessResponse struct {
	PID       int
	ChildPIDs []int
	Stdout    string
	Stderr    string
	ExitCode  int
}

/* ProcessExchange maps an exact process request to a controlled response. */
type ProcessExchange struct {
	Request  ProcessRequest
	Response ProcessResponse
	Error    string
	Barrier  *Barrier
}

/* ProcessRunner is a finite scripted process boundary. */
type ProcessRunner struct {
	mu        sync.Mutex
	exchanges []ProcessExchange
	next      int
	requests  []ProcessRequest
	trees     map[int][]int
}

/* NewProcessRunner copies process exchanges. */
func NewProcessRunner(exchanges []ProcessExchange) *ProcessRunner {
	return &ProcessRunner{exchanges: append([]ProcessExchange(nil), exchanges...), trees: make(map[int][]int)}
}

/* Run consumes one exact process exchange. */
func (r *ProcessRunner) Run(ctx context.Context, request ProcessRequest) (ProcessResponse, error) {
	r.mu.Lock()
	if r.next >= len(r.exchanges) {
		r.mu.Unlock()
		return ProcessResponse{}, errors.New("process script exhausted")
	}
	exchange := r.exchanges[r.next]
	r.next++
	r.requests = append(r.requests, cloneProcessRequest(request))
	r.mu.Unlock()
	if !reflect.DeepEqual(exchange.Request, request) {
		return ProcessResponse{}, fmt.Errorf("process request mismatch: got %#v want %#v", request, exchange.Request)
	}
	if exchange.Barrier != nil {
		if err := exchange.Barrier.wait(ctx); err != nil {
			return ProcessResponse{}, err
		}
	}
	if exchange.Error != "" {
		return exchange.Response, errors.New(exchange.Error)
	}
	r.mu.Lock()
	r.trees[exchange.Response.PID] = append([]int(nil), exchange.Response.ChildPIDs...)
	r.mu.Unlock()
	return exchange.Response, nil
}

/* Descendants returns the controlled child process snapshot. */
func (r *ProcessRunner) Descendants(pid int) []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int(nil), r.trees[pid]...)
}

func cloneProcessRequest(request ProcessRequest) ProcessRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Env = cloneStrings(request.Env)
	return request
}

/* HTTPExchange maps an exact request to an in-memory HTTP response. */
type HTTPExchange struct {
	Method  string
	URL     string
	Status  int
	Headers http.Header
	Body    string
	Error   string
}

/* HTTPRequest is an immutable observed request snapshot. */
type HTTPRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    string
}

/* Transport is a scripted RoundTripper that never opens a network connection. */
type Transport struct {
	mu        sync.Mutex
	exchanges []HTTPExchange
	next      int
	requests  []HTTPRequest
}

/* NewTransport copies HTTP exchanges. */
func NewTransport(exchanges []HTTPExchange) *Transport {
	return &Transport{exchanges: append([]HTTPExchange(nil), exchanges...)}
}

/* RoundTrip consumes one exchange entirely in memory. */
func (t *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.next >= len(t.exchanges) {
		return nil, errors.New("HTTP script exhausted")
	}
	exchange := t.exchanges[t.next]
	t.next++
	t.requests = append(t.requests, HTTPRequest{
		Method: request.Method, URL: request.URL.String(), Headers: request.Header.Clone(), Body: string(body),
	})
	if exchange.Method != request.Method || exchange.URL != request.URL.String() {
		return nil, fmt.Errorf("HTTP request mismatch: got %s %s want %s %s", request.Method, request.URL, exchange.Method, exchange.URL)
	}
	if exchange.Error != "" {
		return nil, errors.New(exchange.Error)
	}
	status := exchange.Status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     exchange.Headers.Clone(),
		Body:       io.NopCloser(bytes.NewBufferString(exchange.Body)),
		Request:    request,
	}, nil
}

/* Requests returns independent HTTP request snapshots. */
func (t *Transport) Requests() []HTTPRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	requests := make([]HTTPRequest, len(t.requests))
	for i, request := range t.requests {
		request.Headers = request.Headers.Clone()
		requests[i] = request
	}
	return requests
}

/* Message is one transport-independent outbound message. */
type Message struct {
	Channel       string
	Content       string
	CorrelationID string
}

/* MessageResult is one controlled delivery result. */
type MessageResult struct {
	ID    string
	Error string
}

/* Messenger records exact messages and returns finite scripted results. */
type Messenger struct {
	mu       sync.Mutex
	results  []MessageResult
	next     int
	messages []Message
}

/* NewMessenger copies delivery results. */
func NewMessenger(results []MessageResult) *Messenger {
	return &Messenger{results: append([]MessageResult(nil), results...)}
}

/* Send records one message and consumes one result. */
func (m *Messenger) Send(ctx context.Context, message Message) (MessageResult, error) {
	select {
	case <-ctx.Done():
		return MessageResult{}, ctx.Err()
	default:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.next >= len(m.results) {
		return MessageResult{}, errors.New("messenger script exhausted")
	}
	result := m.results[m.next]
	m.next++
	m.messages = append(m.messages, message)
	if result.Error != "" {
		return result, errors.New(result.Error)
	}
	return result, nil
}

/* Messages returns observed outbound messages. */
func (m *Messenger) Messages() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message(nil), m.messages...)
}
