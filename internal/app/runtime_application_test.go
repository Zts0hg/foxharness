package app

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestRuntimeApplicationMapsCommandNotificationsOutcomeAndLifecycle(t *testing.T) {
	order := []string{}
	runner := &runtimeSessionStub{
		order: &order,
		result: foxruntime.RunResult{
			SessionID: "session-1", RunID: "run-2",
			Outcome: engine.RunOutcome{
				FinalMessage: "done", FinishReason: "stop", TurnCount: 2,
				Usage: schema.Usage{InputTokens: 7, OutputTokens: 3},
			},
		},
	}
	sink := &recordingNotificationSink{}
	application, err := NewRuntimeApplication(RuntimeApplicationConfig{
		Session: runner,
		Info:    SessionInfo{ID: "session-1", Directory: "/sessions/session-1", TranscriptPath: "/sessions/session-1/transcript.jsonl"},
		RunSpec: foxruntime.RunSpec{
			ProviderProtocol: "openai", WorkDir: "/workspace", Model: "base-model", Effort: "medium",
		},
		BeforeRun: func(_ context.Context, command RunCommand) error {
			order = append(order, "before:"+command.Prompt)
			return nil
		},
		AfterRun: func(_ context.Context, result foxruntime.RunResult, runErr error) {
			order = append(order, "after:"+string(result.RunID))
			if runErr != nil {
				t.Fatalf("AfterRun error = %v", runErr)
			}
		},
		Drain: func(context.Context) error {
			order = append(order, "drain")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	command := RunCommand{
		Prompt: "task", DisplayPrompt: "Task", AllowedTools: []string{"read_file"},
		CollaborationMode: "default", Model: "override-model", Effort: "high",
	}
	outcome, err := application.Run(context.Background(), command, sink)
	if err != nil {
		t.Fatal(err)
	}
	wantSpec := foxruntime.RunSpec{
		Prompt: "task", DisplayPrompt: "Task", AllowedTools: []string{"read_file"},
		CollaborationMode: "default", ProviderProtocol: "openai", WorkDir: "/workspace",
		Model: "override-model", Effort: "high", Observer: runner.observer,
	}
	if !reflect.DeepEqual(runner.spec, wantSpec) {
		t.Fatalf("runtime spec = %#v, want %#v", runner.spec, wantSpec)
	}
	wantOutcome := &RunOutcome{
		SessionID: "session-1", RunID: "run-2", FinalMessage: "done", FinishReason: "stop", TurnCount: 2,
		Usage:       Usage{InputTokens: 7, OutputTokens: 3},
		MetricsPath: filepath.Join("/sessions/session-1", "runs", "run-2", "metrics.jsonl"),
		TracePath:   filepath.Join("/sessions/session-1", "runs", "run-2", "trace.jsonl"),
	}
	if !reflect.DeepEqual(outcome, wantOutcome) {
		t.Fatalf("outcome = %#v, want %#v", outcome, wantOutcome)
	}
	if got, want := order, []string{"before:task", "runtime", "after:run-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %#v, want %#v", got, want)
	}
	if got := application.Session(); got.ID != "session-1" || got.Directory != "/sessions/session-1" {
		t.Fatalf("session info = %#v", got)
	}
	if err := application.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := order[len(order)-1]; got != "drain" {
		t.Fatalf("last lifecycle event = %q", got)
	}
}

func TestRuntimeApplicationPreservesBaseSpecAndDefensiveAllowedTools(t *testing.T) {
	allowed := []string{"read_file", "bash"}
	runner := &runtimeSessionStub{result: foxruntime.RunResult{SessionID: "s", RunID: "r"}}
	application, err := NewRuntimeApplication(RuntimeApplicationConfig{
		Session: runner, Info: SessionInfo{ID: "s", Directory: "/s"},
		RunSpec: foxruntime.RunSpec{Model: "model", Effort: "medium", AllowedTools: allowed},
	})
	if err != nil {
		t.Fatal(err)
	}
	allowed[0] = "mutated"
	command := RunCommand{Prompt: "task"}
	if _, err := application.Run(context.Background(), command, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := runner.spec.AllowedTools, []string{"read_file", "bash"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("allowed tools = %#v, want %#v", got, want)
	}
	if runner.spec.Model != "model" || runner.spec.Effort != "medium" {
		t.Fatalf("base spec was cleared: %#v", runner.spec)
	}
}

func TestRuntimeApplicationPreservesExplicitEmptyAllowedTools(t *testing.T) {
	runner := &runtimeSessionStub{result: foxruntime.RunResult{SessionID: "s", RunID: "r"}}
	application, err := NewRuntimeApplication(RuntimeApplicationConfig{
		Session: runner, Info: SessionInfo{ID: "s", Directory: "/s"},
		RunSpec: foxruntime.RunSpec{Model: "model", Effort: "medium"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Run(context.Background(), RunCommand{Prompt: "task", AllowedTools: []string{}}, nil); err != nil {
		t.Fatal(err)
	}
	if runner.spec.AllowedTools == nil || len(runner.spec.AllowedTools) != 0 {
		t.Fatalf("allowed tools = %#v, want explicit empty deny-all slice", runner.spec.AllowedTools)
	}
}

func TestRuntimeApplicationFailureAndDrainContracts(t *testing.T) {
	beforeErr := errors.New("before failed")
	runner := &runtimeSessionStub{}
	application, err := NewRuntimeApplication(RuntimeApplicationConfig{
		Session: runner, Info: SessionInfo{ID: "s", Directory: "/s"},
		BeforeRun: func(context.Context, RunCommand) error { return beforeErr },
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := application.Run(context.Background(), RunCommand{Prompt: "task"}, nil); outcome != nil || !errors.Is(err, beforeErr) {
		t.Fatalf("before failure = %#v/%v", outcome, err)
	}
	if runner.calls != 0 {
		t.Fatalf("runtime calls = %d, want 0", runner.calls)
	}

	runErr := errors.New("runtime failed")
	runner.runErr = runErr
	runner.result = foxruntime.RunResult{SessionID: session.ID("s"), RunID: session.RunID("partial"), Outcome: engine.RunOutcome{Partial: true, FinalMessage: "partial", Err: runErr}}
	application.config.BeforeRun = nil
	outcome, err := application.Run(context.Background(), RunCommand{Prompt: "task"}, nil)
	if !errors.Is(err, runErr) || outcome == nil || outcome.RunID != "partial" || !outcome.Partial {
		t.Fatalf("runtime failure = %#v/%v", outcome, err)
	}
}

func TestNewRuntimeApplicationRejectsMissingSession(t *testing.T) {
	if _, err := NewRuntimeApplication(RuntimeApplicationConfig{}); err == nil || err.Error() != "runtime application session is required" {
		t.Fatalf("NewRuntimeApplication error = %v", err)
	}
}

type runtimeSessionStub struct {
	result   foxruntime.RunResult
	runErr   error
	spec     foxruntime.RunSpec
	observer foxruntime.RunObserver
	calls    int
	order    *[]string
}

func (s *runtimeSessionStub) Run(ctx context.Context, spec foxruntime.RunSpec) (foxruntime.RunResult, error) {
	s.calls++
	s.spec = spec
	s.observer = spec.Observer
	if s.order != nil {
		*s.order = append(*s.order, "runtime")
	}
	if spec.Observer != nil {
		spec.Observer.ObserveRunFact(ctx, foxruntime.RuntimeFact{
			SessionID: "session-1", RunID: "run-2",
			Fact: engine.Fact{Kind: engine.FactRunStarted, Sequence: 1},
		})
	}
	return s.result, s.runErr
}
