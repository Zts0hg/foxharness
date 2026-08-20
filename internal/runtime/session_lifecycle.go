package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/session"
)

var (
	/* ErrSessionClosed indicates that a live AgentSession no longer admits runs. */
	ErrSessionClosed = errors.New("runtime agent session is closed")
	/* ErrRunScopeOwner indicates that a RunScope was finished by another AgentSession. */
	ErrRunScopeOwner = errors.New("runtime run scope belongs to another agent session")
	/* ErrSessionAlreadyOpen indicates that a stored session already has one live owner. */
	ErrSessionAlreadyOpen = errors.New("runtime session is already open")
	/* ErrSessionRecoveryRequired indicates that a recoverable commit must be retried. */
	ErrSessionRecoveryRequired = errors.New("runtime session requires recovery")
)

/* SessionStore is the consumer-owned persistence port needed by RuntimeHarness. */
type SessionStore interface {
	Create(session.CreateOptions) (*session.StoredSession, error)
	Open(session.ID) (*session.StoredSession, error)
	StartRun(*session.StoredSession, string) (*session.StoredRun, error)
	FinishRun(*session.StoredRun) error
	LoadMessageRecords(*session.StoredSession) ([]session.MessageRecord, error)
	AppendMessage(*session.StoredSession, session.RunID, engine.Message, string) (session.MessageRecord, error)
	LoadContextCompactState(*session.StoredSession) (*session.CompactState, error)
	SaveContextCompactState(*session.StoredSession, *session.CompactState) error
	TruncateMessagesBefore(*session.StoredSession, int64) error
}

/* SessionOptions contains persistence metadata for a newly created AgentSession. */
type SessionOptions struct {
	WorkDir         string
	UserID          string
	ChatID          string
	ParentSessionID session.ID
	ParentRunID     session.RunID
	DelegationID    string
	Agent           string
}

/* AgentSessionSnapshot is a copy-only description of one live runtime session. */
type AgentSessionSnapshot struct {
	ID              session.ID
	Profile         ProfileName
	Source          session.Source
	WorkDir         string
	RootDir         string
	ParentSessionID session.ID
	ParentRunID     session.RunID
}

/* RuntimeHarness holds concurrency-safe factories and persistence dependencies. */
type RuntimeHarness struct {
	store SessionStore
	mu    sync.Mutex
	open  map[session.ID]struct{}
}

/* AgentSession coordinates admission and recoverable state for one live session. */
type AgentSession struct {
	store                  SessionStore
	profile                Profile
	record                 session.StoredSession
	gate                   chan struct{}
	stateMu                sync.Mutex
	contextMu              sync.Mutex
	closed                 bool
	recovery               *RunScope
	contextLoaded          bool
	contextRecords         []session.MessageRecord
	contextCompactState    *session.CompactState
	contextInitialPrepared map[session.RunID]bool
	contextPreparedTurns   map[contextTurnKey]bool
	releaseLease           func()
}

/* NewRuntimeHarness constructs a shared harness around the supplied persistence port. */
func NewRuntimeHarness(store SessionStore) (*RuntimeHarness, error) {
	if isNilSessionStore(store) {
		return nil, errors.New("runtime session store is required")
	}
	return &RuntimeHarness{store: store, open: make(map[session.ID]struct{})}, nil
}

/* CreateSession persists and opens a new live session bound to the selected profile. */
func (h *RuntimeHarness) CreateSession(ctx context.Context, name ProfileName, options SessionOptions) (*AgentSession, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	profile, err := ResolveProfile(name)
	if err != nil {
		return nil, err
	}
	stored, err := h.store.Create(session.CreateOptions{
		Source:          profile.Snapshot().SessionSource,
		WorkDir:         options.WorkDir,
		UserID:          options.UserID,
		ChatID:          options.ChatID,
		ParentSessionID: options.ParentSessionID,
		ParentRunID:     options.ParentRunID,
		DelegationID:    options.DelegationID,
		Agent:           options.Agent,
	})
	if err != nil {
		return nil, fmt.Errorf("create runtime session: %w", err)
	}
	return h.newAgentSession(profile, stored)
}

/* OpenSession opens a stored record as a live session under its matching profile. */
func (h *RuntimeHarness) OpenSession(ctx context.Context, name ProfileName, id session.ID) (*AgentSession, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	profile, err := ResolveProfile(name)
	if err != nil {
		return nil, err
	}
	stored, err := h.store.Open(id)
	if err != nil {
		return nil, fmt.Errorf("open runtime session: %w", err)
	}
	if stored == nil {
		return nil, errors.New("open runtime session returned nil")
	}
	if stored.ID != id {
		return nil, fmt.Errorf("opened session ID %q does not match requested ID %q", stored.ID, id)
	}
	return h.newAgentSession(profile, stored)
}

/* Snapshot returns immutable identity and persistence metadata for the live session. */
func (s *AgentSession) Snapshot() AgentSessionSnapshot {
	return AgentSessionSnapshot{
		ID:              s.record.ID,
		Profile:         s.profile.Snapshot().Name,
		Source:          s.record.Source,
		WorkDir:         s.record.WorkDir,
		RootDir:         s.record.RootDir,
		ParentSessionID: s.record.ParentSessionID,
		ParentRunID:     s.record.ParentRunID,
	}
}

/* BeginRun admits one serialized run and freezes its profile-governed scope. */
func (s *AgentSession) BeginRun(ctx context.Context, spec RunSpec) (*RunScope, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	resolved, err := s.profile.Resolve(spec)
	if err != nil {
		return nil, err
	}
	if err := s.acquire(ctx); err != nil {
		return nil, err
	}

	record := s.record
	storedRun, err := s.store.StartRun(&record, spec.Prompt)
	if err != nil {
		s.release()
		return nil, fmt.Errorf("start stored run: %w", err)
	}
	if storedRun == nil {
		s.release()
		return nil, errors.New("start stored run returned nil")
	}
	if storedRun.SessionID != s.record.ID {
		s.release()
		return nil, fmt.Errorf("stored run session %q does not match live session %q", storedRun.SessionID, s.record.ID)
	}

	runContext, cancel := context.WithCancel(ctx)
	runCopy := *storedRun
	return newRunScope(s, runContext, cancel, runCopy, resolved), nil
}

/* FinishRun persists completion, cancels the run context, and releases admission. */
func (s *AgentSession) FinishRun(scope *RunScope) error {
	if scope == nil || scope.owner != s {
		return ErrRunScopeOwner
	}
	return scope.finish()
}

/* Close prevents future runs after any admitted run has completed. */
func (s *AgentSession) Close(ctx context.Context) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		s.release()
		return nil
	}
	if s.recovery != nil {
		s.stateMu.Unlock()
		s.release()
		return ErrSessionRecoveryRequired
	}
	s.closed = true
	s.stateMu.Unlock()
	s.release()
	s.releaseLease()
	return nil
}

func (h *RuntimeHarness) newAgentSession(profile Profile, stored *session.StoredSession) (*AgentSession, error) {
	if stored == nil {
		return nil, errors.New("runtime session store returned nil session")
	}
	if stored.ID == "" {
		return nil, errors.New("runtime session store returned empty session ID")
	}
	profileSnapshot := profile.Snapshot()
	if stored.Source != profileSnapshot.SessionSource {
		return nil, fmt.Errorf("stored session source %q does not match profile %s source %q", stored.Source, profileSnapshot.Name, profileSnapshot.SessionSource)
	}
	releaseLease, err := h.claim(stored.ID)
	if err != nil {
		return nil, err
	}
	copy := *stored
	result := &AgentSession{
		store: h.store, profile: profile, record: copy, gate: make(chan struct{}, 1),
		contextInitialPrepared: make(map[session.RunID]bool),
		contextPreparedTurns:   make(map[contextTurnKey]bool),
		releaseLease:           releaseLease,
	}
	result.gate <- struct{}{}
	return result, nil
}

func (s *AgentSession) acquire(ctx context.Context) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.closed {
		s.release()
		return ErrSessionClosed
	}
	if s.recovery != nil {
		s.release()
		return ErrSessionRecoveryRequired
	}
	return nil
}

func (h *RuntimeHarness) claim(id session.ID) (func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.open[id]; exists {
		return nil, fmt.Errorf("%w: %s", ErrSessionAlreadyOpen, id)
	}
	h.open[id] = struct{}{}
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(h.open, id)
		})
	}, nil
}

func (s *AgentSession) requireRecovery(scope *RunScope) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.recovery = scope
}

func (s *AgentSession) clearRecovery(scope *RunScope) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.recovery == scope {
		s.recovery = nil
	}
}

func (s *AgentSession) lock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.gate:
	}
	return nil
}

func (s *AgentSession) release() {
	s.gate <- struct{}{}
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("runtime context is required")
	}
	return ctx.Err()
}

func isNilSessionStore(store SessionStore) bool {
	if store == nil {
		return true
	}
	value := reflect.ValueOf(store)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
