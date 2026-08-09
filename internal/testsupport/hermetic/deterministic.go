/* Package hermetic provides deterministic collaborators for mandatory tests. */
package hermetic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

/* Barrier exposes explicit start and release synchronization without timing. */
type Barrier struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

/* NewBarrier returns an unreleased synchronization barrier. */
func NewBarrier() *Barrier {
	return &Barrier{started: make(chan struct{}), release: make(chan struct{})}
}

/* Started closes when a caller reaches the barrier. */
func (b *Barrier) Started() <-chan struct{} { return b.started }

/* Release permits all waiting callers to continue. */
func (b *Barrier) Release() { b.releaseOnce.Do(func() { close(b.release) }) }

func (b *Barrier) wait(ctx context.Context) error {
	b.startedOnce.Do(func() { close(b.started) })
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

/* SequenceClock advances by a fixed step for every observation. */
type SequenceClock struct {
	mu   sync.Mutex
	next time.Time
	step time.Duration
}

/* NewSequenceClock returns a deterministic clock. */
func NewSequenceClock(start time.Time, step time.Duration) *SequenceClock {
	return &SequenceClock{next: start, step: step}
}

/* Now returns the next controlled time. */
func (c *SequenceClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	value := c.next
	c.next = c.next.Add(c.step)
	return value
}

/* IDSequence returns a finite controlled identifier sequence. */
type IDSequence struct {
	mu     sync.Mutex
	values []string
	next   int
}

/* NewIDSequence copies identifiers into an isolated sequence. */
func NewIDSequence(values ...string) *IDSequence {
	return &IDSequence{values: append([]string(nil), values...)}
}

/* Next returns one identifier or an exhaustion error. */
func (s *IDSequence) Next() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next >= len(s.values) {
		return "", errors.New("identifier sequence exhausted")
	}
	value := s.values[s.next]
	s.next++
	return value, nil
}

/* Roots contains all test-owned state roots used by an entry scenario. */
type Roots struct {
	Base      string
	Home      string
	Config    string
	Sessions  string
	Workspace string
}

/* NewRoots creates isolated roots below base. */
func NewRoots(base string) (Roots, error) {
	abs, err := filepath.Abs(base)
	if err != nil {
		return Roots{}, fmt.Errorf("resolve test root: %w", err)
	}
	roots := Roots{
		Base:      filepath.Clean(abs),
		Home:      filepath.Join(abs, "home"),
		Config:    filepath.Join(abs, "config"),
		Sessions:  filepath.Join(abs, "sessions"),
		Workspace: filepath.Join(abs, "workspace"),
	}
	for _, path := range []string{roots.Home, roots.Config, roots.Sessions, roots.Workspace} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return Roots{}, fmt.Errorf("create test root %q: %w", path, err)
		}
	}
	return roots, nil
}

/* Env returns explicit environment values for test-owned state. */
func (r Roots) Env() map[string]string {
	return map[string]string{
		"HOME":                   r.Home,
		"XDG_CONFIG_HOME":        r.Config,
		"FOXHARNESS_CONFIG_DIR":  r.Config,
		"FOXHARNESS_SESSION_DIR": r.Sessions,
	}
}
