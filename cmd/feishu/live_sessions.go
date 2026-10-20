package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/feishu"
	"github.com/Zts0hg/foxharness/internal/mcpintegration"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/session"
)

type feishuConversationKey struct{ chat, user string }
type feishuLiveSession struct {
	gate    chan struct{}
	stored  *session.StoredSession
	session *foxruntime.AgentSession
}

func (s *feishuLiveSession) lock(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *feishuLiveSession) unlock() { <-s.gate }

/* conversation serializes replacement per chat/user without blocking unrelated
 * session startup. The returned slot is locked until preparation completes. */
func (f *feishuTaskExecutionFactory) conversation(ctx context.Context, request feishu.TaskExecutionRequest) (*feishuLiveSession, error) {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil, foxruntime.ErrSessionClosed
	}
	key := feishuConversationKey{request.Task.ChatID, request.Task.SenderID}
	live := f.sessions[key]
	if live == nil {
		live = &feishuLiveSession{gate: make(chan struct{}, 1)}
		f.sessions[key] = live
	}
	f.mu.Unlock()
	if err := live.lock(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	closed := f.closed
	f.mu.Unlock()
	if closed {
		live.unlock()
		return nil, foxruntime.ErrSessionClosed
	}
	return live, nil
}

/* Close stops admission and releases every conversation's live runtime. It is
 * called after runner cancellation/drain, with a fresh bounded cleanup context. */
func (f *feishuTaskExecutionFactory) Close(ctx context.Context) error {
	f.mu.Lock()
	f.closed = true
	lives := make([]*feishuLiveSession, 0, len(f.sessions))
	for _, live := range f.sessions {
		lives = append(lives, live)
	}
	f.mu.Unlock()
	errs := make(chan error, len(lives))
	var wg sync.WaitGroup
	for _, live := range lives {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := live.lock(ctx); err != nil {
				errs <- err
				return
			}
			defer live.unlock()
			if live.session != nil {
				errs <- closeFeishuSession(ctx, live.session)
			}
		}()
	}
	wg.Wait()
	close(errs)
	var result error
	for err := range errs {
		result = errors.Join(result, err)
	}
	return result
}

func closeFeishuSession(ctx context.Context, live *foxruntime.AgentSession) error {
	recoveryCtx, cancelRecovery := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	recoveryErr := live.RecoverRunFinish(recoveryCtx)
	cancelRecovery()
	closeCtx, cancelClose := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelClose()
	return errors.Join(recoveryErr, live.Close(closeCtx))
}

type feishuTaskDependenciesKey struct{}

/* feishuTaskRuntime binds immutable task factories to a shared live session.
 * Context inheritance keeps each run's approval, journal and tracker independent
 * even when applications are prepared before a preceding run has finished. */
type feishuTaskRuntime struct {
	session      *foxruntime.AgentSession
	dependencies foxruntime.HarnessDependencies
}

func (r feishuTaskRuntime) Run(ctx context.Context, spec foxruntime.RunSpec) (foxruntime.RunResult, error) {
	return r.session.Run(context.WithValue(ctx, feishuTaskDependenciesKey{}, r.dependencies), spec)
}

func feishuTaskDependencies(ctx context.Context) foxruntime.HarnessDependencies {
	return ctx.Value(feishuTaskDependenciesKey{}).(foxruntime.HarnessDependencies)
}

func feishuSessionDependencies(home string) foxruntime.HarnessDependencies {
	return foxruntime.HarnessDependencies{
		NewSessionServices: mcpintegration.Factory(home),
		NewModel: func(ctx context.Context, a foxruntime.RunAssembly) (engine.ModelInvoker, error) {
			return feishuTaskDependencies(ctx).NewModel(ctx, a)
		},
		NewTools: func(ctx context.Context, a foxruntime.RunAssembly) (engine.ToolExecutor, error) {
			return feishuTaskDependencies(ctx).NewTools(ctx, a)
		},
		NewPolicy: func(ctx context.Context, a foxruntime.RunAssembly) (engine.TurnPolicy, error) {
			return feishuTaskDependencies(ctx).NewPolicy(ctx, a)
		},
		NewContext: func(ctx context.Context, a foxruntime.RunAssembly) (foxruntime.ContextCollector, foxruntime.ContextCompactor, error) {
			return feishuTaskDependencies(ctx).NewContext(ctx, a)
		},
		NewArtifactJournal: func(ctx context.Context, a foxruntime.RunAssembly) (foxruntime.SessionArtifactJournal, error) {
			return feishuTaskDependencies(ctx).NewArtifactJournal(ctx, a)
		},
		NewTelemetryJournal: func(ctx context.Context, a foxruntime.RunAssembly) (foxruntime.TelemetryJournal, error) {
			return feishuTaskDependencies(ctx).NewTelemetryJournal(ctx, a)
		},
	}
}
