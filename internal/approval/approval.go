// Package approval provides a synchronous, in-process approval flow for
// dangerous operations initiated by the agent.  A caller (typically the
// danger middleware) registers a Request with a Store and blocks until a
// human operator resolves it via an external callback (e.g. a Feishu
// approval card).  Pending requests expire after a configurable timeout.
package approval

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	/* ErrNotFound reports an approval ID that is unknown or no longer pending. */
	ErrNotFound = errors.New("approval request not found")
	/* ErrConflict reports a duplicate resolution while the first result is claimed. */
	ErrConflict = errors.New("approval request already resolved")
)

// Request represents a pending approval for a dangerous tool invocation.  It
// carries the unique approval ID, the tool name, its raw arguments, and a
// human-readable risk description.
type Request struct {
	ID        string
	ToolName  string
	Arguments string
	Risk      string
}

// Result captures the operator's decision for an approval Request.
type Result struct {
	Approved bool
	Reason   string
}

// Store is a concurrency-safe registry of pending approval requests. Each
// request is identified by a unique ID and has one mutex-arbitrated terminal
// transition before its waiter removes the record.
type Store struct {
	mu         sync.Mutex
	waiting    map[string]*pendingApproval
	newTimeout func() (<-chan time.Time, func())
}

type pendingApproval struct {
	done     chan struct{}
	result   Result
	resolved bool
}

// NewStore creates an empty Store ready to track approval requests.
func NewStore() *Store {
	return &Store{
		waiting: make(map[string]*pendingApproval),
	}
}

// Wait registers the request, invokes send to notify the operator, and then
// blocks until one of the following occurs: the operator resolves the
// request, the 5-minute timeout expires (returns denied), or ctx is
// cancelled.
func (s *Store) Wait(ctx context.Context, req Request, send func(Request) error) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	pending := &pendingApproval{done: make(chan struct{})}
	s.mu.Lock()
	if _, exists := s.waiting[req.ID]; exists {
		s.mu.Unlock()
		return Result{}, fmt.Errorf("%w: %s", ErrConflict, req.ID)
	}
	s.waiting[req.ID] = pending
	s.mu.Unlock()

	if err := send(req); err != nil {
		return s.finishWait(req.ID, pending, Result{}, err)
	}

	timeout, stopTimeout := s.timeoutSignal()
	defer stopTimeout()

	select {
	case <-pending.done:
		return s.finishWait(req.ID, pending, Result{}, nil)
	case <-timeout:
		return s.finishWait(req.ID, pending, Result{Approved: false, Reason: "审批超时"}, nil)
	case <-ctx.Done():
		return s.finishWait(req.ID, pending, Result{}, ctx.Err())
	}
}

// Resolve delivers the operator's decision to the goroutine blocked in Wait
// for the given approval ID.  It returns an error if no pending request
// exists for the ID.
func (s *Store) Resolve(id string, result Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.waiting[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if pending.resolved {
		return fmt.Errorf("%w: %s", ErrConflict, id)
	}
	pending.result = result
	pending.resolved = true
	close(pending.done)
	return nil
}

func (s *Store) finishWait(id string, pending *pendingApproval, fallback Result, fallbackErr error) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.waiting[id]
	if !ok || current != pending {
		return fallback, fallbackErr
	}
	delete(s.waiting, id)
	if pending.resolved {
		return pending.result, nil
	}
	return fallback, fallbackErr
}

func (s *Store) timeoutSignal() (<-chan time.Time, func()) {
	if s.newTimeout != nil {
		return s.newTimeout()
	}
	timer := time.NewTimer(5 * time.Minute)
	return timer.C, func() { timer.Stop() }
}
