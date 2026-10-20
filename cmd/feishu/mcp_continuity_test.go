package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/feishu"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/testsupport/mcpfixture"
)

func TestFeishuMessagesRetainLiveMCPAndFrozenConfiguration(t *testing.T) {
	ctx := context.Background()
	home, work := t.TempDir(), t.TempDir()
	first := mcpfixture.Start(t, work)
	var idsMu sync.Mutex
	ids := make(map[string]bool)
	handler := first.Server.Config.Handler
	first.Server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := r.Header.Get("Mcp-Session-Id"); id != "" {
			idsMu.Lock()
			ids[id] = true
			idsMu.Unlock()
		}
		handler.ServeHTTP(w, r)
	})
	approvals := approval.NewStore()
	messenger := &mcpfixture.Messenger{Store: approvals, Approved: true}
	factory := newFeishuTaskExecutionFactory(&mcpfixture.Model{}, work, messenger, session.NewFileStoreWithHome(work, home), approvals)
	t.Cleanup(func() { closeFeishuFactory(t, factory) })
	request := feishu.TaskExecutionRequest{Task: feishu.Task{TaskID: "first", ChatID: "chat", SenderID: "user"}, Prompt: "capture"}
	runs := make(map[string]bool)
	run := func(req feishu.TaskExecutionRequest) feishu.PreparedTaskExecution {
		t.Helper()
		taskCtx, cancelTask := context.WithCancel(ctx)
		defer cancelTask()
		p, err := factory.PrepareTask(taskCtx, req)
		if err != nil || p.SetupError != nil {
			t.Fatalf("prepare: %v / %v", err, p.SetupError)
		}
		outcome, err := p.Application.Run(taskCtx, app.RunCommand{Prompt: "capture"}, nil)
		if err != nil || outcome == nil {
			t.Fatalf("run: %v / %+v", err, outcome)
		}
		if outcome.RunID == "" || runs[outcome.RunID] {
			t.Fatalf("run identity reused: %q", outcome.RunID)
		}
		runs[outcome.RunID] = true
		if err := p.Drain(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(outcome.TracePath); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p1 := run(request)
	if first.Active() != 1 {
		t.Fatalf("message drain closed live MCP: active=%d", first.Active())
	}
	second := mcpfixture.Start(t, work)
	request.Task.TaskID = "second"
	p2 := run(request)
	idsMu.Lock()
	connectionCount := len(ids)
	idsMu.Unlock()
	if connectionCount != 1 {
		t.Fatalf("browser connection changed across messages: %d sessions", connectionCount)
	}
	if p2.Created || p2.Session.ID != p1.Session.ID || first.Calls.Load() != 2 || second.Calls.Load() != 0 || messenger.Requests.Load() != 2 {
		t.Fatalf("continuation/configuration/approval mismatch: same=%v first=%d second=%d approvals=%d", p2.Session.ID == p1.Session.ID, first.Calls.Load(), second.Calls.Load(), messenger.Requests.Load())
	}
	request.Task.SenderID = "other"
	other := run(request)
	if other.Session.ID == p1.Session.ID || first.Active() != 1 || second.Active() != 1 {
		t.Fatal("independent user did not own a separate connection")
	}
	request.Task.SenderID = "user"
	request.ForceNewSession = true
	replacement := run(request)
	if !replacement.Created || replacement.Session.ID == p1.Session.ID || first.Active() != 0 || second.Active() != 2 {
		t.Fatal("explicit new session did not replace only its old owner")
	}
	if _, err := p1.Application.Run(ctx, app.RunCommand{Prompt: "capture"}, nil); err == nil {
		t.Fatal("replaced application admitted a run")
	}
	if err := serve(ctx, &immediateGateway{}, feishu.NewRunner(factory, messenger), make(chan feishu.Task), ":0", time.Second); err != nil {
		t.Fatal(err)
	}
	if first.Active() != 0 || second.Active() != 0 {
		t.Fatal("factory shutdown leaked MCP")
	}
	if _, err := factory.PrepareTask(ctx, request); err == nil {
		t.Fatal("closed factory admitted a task")
	}
}

func TestFeishuCancelledAndFailedMessagesRetainSession(t *testing.T) {
	ctx := context.Background()
	home, work := t.TempDir(), t.TempDir()
	fixture := mcpfixture.Start(t, work)
	store := session.NewFileStoreWithHome(work, home)
	approvals := approval.NewStore()
	factory := newFeishuTaskExecutionFactory(&mcpfixture.Model{}, work, &mcpfixture.Messenger{Store: approvals, Approved: true}, store, approvals)
	t.Cleanup(func() { closeFeishuFactory(t, factory) })
	req := feishu.TaskExecutionRequest{Task: feishu.Task{ChatID: "chat", SenderID: "user"}, Prompt: "capture"}
	p, err := factory.PrepareTask(ctx, req)
	if err != nil || p.SetupError != nil {
		t.Fatalf("prepare: %v / %v", err, p.SetupError)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.Application.Run(cancelled, app.RunCommand{Prompt: "capture"}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if err := p.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if fixture.Active() != 1 {
		t.Fatal("cancelled task closed session")
	}
	continued, err := factory.PrepareTask(ctx, req)
	if err != nil || continued.SetupError != nil || continued.Session.ID != p.Session.ID {
		t.Fatal("cancelled session could not continue")
	}
	if _, err := continued.Application.Run(ctx, app.RunCommand{Prompt: "capture", AllowedTools: []string{"read_file"}}, nil); err == nil {
		t.Fatal("expected model failure with unavailable screenshot tool")
	}
	if err := continued.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if fixture.Active() != 1 {
		t.Fatal("failed task closed session")
	}
	last, err := factory.PrepareTask(ctx, req)
	if err != nil || last.SetupError != nil {
		t.Fatalf("prepare: %v / %v", err, last.SetupError)
	}
	if _, err := last.Application.Run(ctx, app.RunCommand{Prompt: "capture"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := last.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if fixture.Calls.Load() != 1 {
		t.Fatalf("calls=%d", fixture.Calls.Load())
	}
	if _, err := os.Stat(filepath.Join(last.Session.Directory, "messages.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func closeFeishuFactory(t *testing.T, factory feishu.TaskExecutionFactory) {
	t.Helper()
	closer, ok := factory.(interface{ Close(context.Context) error })
	if !ok {
		t.Error("factory does not expose service-lifetime cleanup")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := closer.Close(ctx); err != nil {
		t.Error(err)
	}
}

func TestFeishuServeClosesFactoryAndReportsCleanupError(t *testing.T) {
	want := errors.New("fixture MCP shutdown failure")
	factory := &closingFeishuFactory{err: want}
	runner := feishu.NewRunner(factory, discardFeishuMessenger{})
	err := serve(context.Background(), &immediateGateway{}, runner, make(chan feishu.Task), ":0", time.Second)
	if !errors.Is(err, want) || factory.calls != 1 || factory.cancelled {
		t.Fatalf("shutdown: err=%v calls=%d cancelled=%v", err, factory.calls, factory.cancelled)
	}
}

type closingFeishuFactory struct {
	err       error
	calls     int
	cancelled bool
}

func (f *closingFeishuFactory) PrepareTask(context.Context, feishu.TaskExecutionRequest) (feishu.PreparedTaskExecution, error) {
	return feishu.PreparedTaskExecution{}, errors.New("unused")
}
func (f *closingFeishuFactory) Close(ctx context.Context) error {
	f.calls++
	f.cancelled = ctx.Err() != nil
	return f.err
}
