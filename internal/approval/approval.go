package approval

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Request struct {
	ID        string
	ToolName  string
	Arguments string
	Risk      string
}

type Result struct {
	Approved bool
	Reason   string
}

type Store struct {
	mu      sync.Mutex
	waiting map[string]chan Result
}

func NewStore() *Store {
	return &Store{
		waiting: make(map[string]chan Result),
	}
}

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
