// Package approval provides a synchronous, in-process approval flow for
// dangerous operations initiated by the agent.  A caller (typically the
// danger middleware) registers a Request with a Store and blocks until a
// human operator resolves it via an external callback (e.g. a Feishu
// approval card).  Pending requests expire after a configurable timeout.
package approval

import (
	"context"
	"fmt"
	"sync"
	"time"
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

// Store is a concurrency-safe registry of pending approval requests.  Each
// request is identified by a unique ID and backed by a buffered channel that
// is closed when the request is resolved or expires.
type Store struct {
	mu      sync.Mutex
	waiting map[string]chan Result
}

// NewStore creates an empty Store ready to track approval requests.
func NewStore() *Store {
	return &Store{
		waiting: make(map[string]chan Result),
	}
}

// Wait registers the request, invokes send to notify the operator, and then
// blocks until one of the following occurs: the operator resolves the
// request, the 5-minute timeout expires (returns denied), or ctx is
// cancelled.
func (s *Store) Wait(ctx context.Context, req Request, send func(Request) error) (Result, error) {
	ch := make(chan Result, 1)
	s.mu.Lock()
	s.waiting[req.ID] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.waiting, req.ID)
		s.mu.Unlock()
	}()

	if err := send(req); err != nil {
		return Result{}, err
	}

	timeout := time.NewTimer(5 * time.Minute)
	defer timeout.Stop()

	select {
	case result := <-ch:
		return result, nil
	case <-timeout.C:
		return Result{Approved: false, Reason: "审批超时"}, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

// Resolve delivers the operator's decision to the goroutine blocked in Wait
// for the given approval ID.  It returns an error if no pending request
// exists for the ID.
func (s *Store) Resolve(id string, result Result) error {
	s.mu.Lock()
	ch, ok := s.waiting[id]
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("审批请求不存在或已过期: %s", id)
	}

	ch <- result
	return nil
}
