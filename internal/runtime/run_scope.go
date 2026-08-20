package runtime

import (
	"context"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/session"
)

/* RunScopeSnapshot is the immutable diagnostic view of one admitted run. */
type RunScopeSnapshot struct {
	Profile           ProfileName
	SessionID         session.ID
	RunID             session.RunID
	RootDir           string
	Model             string
	Effort            string
	MaxTurns          int
	TaskTimeout       time.Duration
	PermissionPolicy  string
	ReadOnly          bool
	DelegationDepth   int
	CollaborationMode string
}

/* RunScope owns the cancellation, observation, policy, and budget values for one run. */
type RunScope struct {
	owner    *AgentSession
	ctx      context.Context
	cancel   context.CancelFunc
	run      session.StoredRun
	resolved ResolvedRunSpec
	profile  ProfileSnapshot
	finishMu sync.Mutex
	finished bool
	released bool
}

func newRunScope(owner *AgentSession, ctx context.Context, cancel context.CancelFunc, run session.StoredRun, resolved ResolvedRunSpec) *RunScope {
	return &RunScope{
		owner: owner, ctx: ctx, cancel: cancel, run: run,
		resolved: resolved, profile: owner.profile.Snapshot(),
	}
}

/* Snapshot returns stable run identity and profile-governed execution values. */
func (s *RunScope) Snapshot() RunScopeSnapshot {
	run := s.resolved.Snapshot()
	return RunScopeSnapshot{
		Profile: run.Profile, SessionID: s.run.SessionID, RunID: s.run.ID, RootDir: s.run.RootDir,
		Model: run.Model, Effort: run.Effort, MaxTurns: run.MaxTurns, TaskTimeout: run.TaskTimeout,
		PermissionPolicy: s.profile.PermissionPolicy, ReadOnly: run.ReadOnly,
		DelegationDepth: run.DelegationDepth, CollaborationMode: run.CollaborationMode,
	}
}

/* AllowedTools returns a defensive copy of the effective run capabilities. */
func (s *RunScope) AllowedTools() []string {
	return s.resolved.AllowedTools()
}

/* Observer returns the observer frozen when the run was admitted. */
func (s *RunScope) Observer() engine.Observer {
	return s.resolved.Observer()
}

/* Context returns the run-owned cancellation context. */
func (s *RunScope) Context() context.Context {
	return s.ctx
}

/* Cancel signals cancellation without bypassing the required persistence finish. */
func (s *RunScope) Cancel() {
	s.cancel()
}

func (s *RunScope) finish() error {
	s.finishMu.Lock()
	defer s.finishMu.Unlock()
	if s.finished {
		return nil
	}

	s.cancel()
	if err := s.owner.store.FinishRun(&s.run); err != nil {
		s.owner.requireRecovery(s)
		s.releaseAdmission()
		return err
	}

	s.finished = true
	s.owner.clearRecovery(s)
	s.owner.releaseRunContext(s.run.ID)
	s.releaseAdmission()
	return nil
}

func (s *RunScope) releaseAdmission() {
	if s.released {
		return
	}
	s.released = true
	s.owner.release()
}
