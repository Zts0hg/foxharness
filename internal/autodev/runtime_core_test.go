package autodev

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
)

type runtimeCoreSessionFake struct {
	specs   []foxruntime.RunSpec
	results []foxruntime.RunResult
	errs    []error
}

func (f *runtimeCoreSessionFake) Run(ctx context.Context, spec foxruntime.RunSpec) (foxruntime.RunResult, error) {
	f.specs = append(f.specs, spec)
	index := len(f.specs) - 1
	if spec.Observer != nil {
		spec.Observer.ObserveRunFact(ctx, foxruntime.RuntimeFact{
			SessionID: "session-1", RunID: "run-1",
			Fact: engine.Fact{Kind: engine.FactRunStarted, Sequence: 1},
		})
		spec.Observer.ObserveRunFact(ctx, foxruntime.RuntimeFact{
			SessionID: "session-1", RunID: "run-1",
			Fact: engine.Fact{Kind: engine.FactToolCall, Sequence: 2, Turn: 1, Name: "read_file", Content: `{"path":"README.md"}`},
		})
		spec.Observer.ObserveRunFact(ctx, foxruntime.RuntimeFact{
			SessionID: "session-1", RunID: "run-1",
			Fact: engine.Fact{Kind: engine.FactToolResult, Sequence: 3, Turn: 1, Name: "read_file", Content: "contents"},
		})
		spec.Observer.ObserveRunFact(ctx, foxruntime.RuntimeFact{
			SessionID: "session-1", RunID: "run-1",
			Fact: engine.Fact{Kind: engine.FactMessage, Sequence: 4, Content: "committed"},
		})
	}
	return f.results[index], f.errs[index]
}

type runtimeCoreReporterFake struct{ events []string }

func (r *runtimeCoreReporterFake) OnRunStart(context.Context, string, string) {
	r.events = append(r.events, "start")
}
func (r *runtimeCoreReporterFake) OnThinking(context.Context, int) {
	r.events = append(r.events, "thinking")
}
func (r *runtimeCoreReporterFake) OnCompaction(context.Context, string) {
	r.events = append(r.events, "compaction")
}
func (r *runtimeCoreReporterFake) OnToolCall(context.Context, string, string) {
	r.events = append(r.events, "tool_call")
}
func (r *runtimeCoreReporterFake) OnToolResult(context.Context, string, string, bool) {
	r.events = append(r.events, "tool_result")
}
func (r *runtimeCoreReporterFake) OnMessage(context.Context, string) {
	r.events = append(r.events, "message")
}
func (r *runtimeCoreReporterFake) OnRunComplete(context.Context, CoreRunResult) {
	r.events = append(r.events, "complete")
}
func (r *runtimeCoreReporterFake) OnRunError(context.Context, string, string, error) {
	r.events = append(r.events, "error")
}

func TestM18RuntimeCoreRunnerMapsCanonicalFactsAndCommittedOutcome(t *testing.T) {
	sessionFake := &runtimeCoreSessionFake{
		results: []foxruntime.RunResult{{
			SessionID: "session-1", RunID: "run-1", CommittedMessage: "committed",
			Outcome: engine.RunOutcome{FinalMessage: "final"},
		}},
		errs: []error{nil},
	}
	reporter := &runtimeCoreReporterFake{}
	runner, err := NewRuntimeCoreRunner(RuntimeCoreRunnerConfig{
		Session:  sessionFake,
		BaseSpec: foxruntime.RunSpec{Model: "model-a", WorkDir: "/wt/item"},
		WorkDir:  "/wt/item",
		AfterRun: func(context.Context, foxruntime.RunResult, error) {
			reporter.events = append(reporter.events, "after")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt := CoreAttempt{AttemptID: "attempt-1", CorrelationID: "attempt-1", Ordinal: 1, Prompt: "implement"}
	outcome := runner.Run(context.Background(), attempt, reporter)

	if outcome.Status != CoreOutcomeSucceeded || outcome.SessionID != "session-1" || outcome.RunID != "run-1" || outcome.PartialMessage != "final" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if got, want := reporter.events, []string{"start", "tool_call", "tool_result", "message", "complete", "after"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if len(sessionFake.specs) != 1 || sessionFake.specs[0].Prompt != "implement" || sessionFake.specs[0].Model != "model-a" {
		t.Fatalf("runtime specs = %#v", sessionFake.specs)
	}
	if sessionFake.specs[0].Observer == nil {
		t.Fatal("runtime run did not receive the Autodev core observer")
	}
}

func TestM18RuntimeCoreRunnerPreservesTurnLimitPartialAndLifecycle(t *testing.T) {
	cause := errors.New("超过最大 Turn 数限制: 2")
	sessionFake := &runtimeCoreSessionFake{
		results: []foxruntime.RunResult{{
			SessionID: "session-1", RunID: "run-1", CommittedMessage: "committed partial",
			Outcome: engine.RunOutcome{FinalMessage: "uncommitted", Partial: true, ErrorKind: "turn_limit", Err: cause},
		}},
		errs: []error{cause},
	}
	runner, err := NewRuntimeCoreRunner(RuntimeCoreRunnerConfig{Session: sessionFake, WorkDir: "/wt/item"})
	if err != nil {
		t.Fatal(err)
	}
	outcome := runner.Run(context.Background(), CoreAttempt{AttemptID: "a", CorrelationID: "a", Ordinal: 1, Prompt: "work"}, nil)
	if outcome.Status != CoreOutcomeTurnExhausted || outcome.RetryClass != CoreRetrySameRunner {
		t.Fatalf("status/retry = %s/%s", outcome.Status, outcome.RetryClass)
	}
	if outcome.PartialMessage != "committed partial" || !outcome.Lifecycle.RunStarted || !outcome.Lifecycle.PostRunEstablished {
		t.Fatalf("partial/lifecycle = %#v", outcome)
	}
}

func TestM18RuntimeCoreRunnerDelegatesMutableAndLifecyclePorts(t *testing.T) {
	sessionFake := &runtimeCoreSessionFake{}
	var asker QuestionAsker
	var model string
	var drained, closed int
	runner, err := NewRuntimeCoreRunner(RuntimeCoreRunnerConfig{
		Session: sessionFake, WorkDir: "/wt/item",
		SetQuestionAsker: func(value QuestionAsker) { asker = value },
		SetModel:         func(value string) error { model = value; return nil },
		StagePrompt: func(_ context.Context, command, args string) (string, error) {
			return command + ":" + args, nil
		},
		Drain: func(context.Context) error { drained++; return nil },
		Close: func(context.Context) error { closed++; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	questionAsker := questionAskerFunc(func(context.Context, []Question) ([]Answer, error) { return nil, nil })
	runner.SetUserAsker(questionAsker)
	if asker == nil {
		t.Fatal("question asker was not installed")
	}
	if err := runner.SetModel("model-b"); err != nil || model != "model-b" {
		t.Fatalf("SetModel = %q, %v", model, err)
	}
	prompt, err := runner.StagePrompt(context.Background(), "codexspec:specify", "req")
	if err != nil || prompt != "codexspec:specify:req" {
		t.Fatalf("StagePrompt = %q, %v", prompt, err)
	}
	if err := runner.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if drained != 1 || closed != 1 || runner.WorkDir() != "/wt/item" {
		t.Fatalf("lifecycle/workdir = %d/%d/%q", drained, closed, runner.WorkDir())
	}
}

type questionAskerFunc func(context.Context, []Question) ([]Answer, error)

func (f questionAskerFunc) Ask(ctx context.Context, questions []Question) ([]Answer, error) {
	return f(ctx, questions)
}

func TestM18RuntimeCoreRunnerRejectsMissingSession(t *testing.T) {
	_, err := NewRuntimeCoreRunner(RuntimeCoreRunnerConfig{})
	if err == nil || err.Error() != "autodev runtime session is required" {
		t.Fatalf("error = %v", err)
	}
}

func TestM18RuntimeCoreRunnerFreezesCallerOwnedBaseSpec(t *testing.T) {
	maxTurns := 4
	allowed := []string{"read_file"}
	sessionFake := &runtimeCoreSessionFake{
		results: []foxruntime.RunResult{{SessionID: "session-1", RunID: "run-1"}},
		errs:    []error{nil},
	}
	runner, err := NewRuntimeCoreRunner(RuntimeCoreRunnerConfig{
		Session: sessionFake, BaseSpec: foxruntime.RunSpec{MaxTurns: &maxTurns, AllowedTools: allowed},
	})
	if err != nil {
		t.Fatal(err)
	}
	maxTurns = 99
	allowed[0] = "bash"
	runner.Run(context.Background(), CoreAttempt{AttemptID: "a", CorrelationID: "a", Ordinal: 1, Prompt: "work"}, nil)
	got := sessionFake.specs[0]
	if got.MaxTurns == nil || *got.MaxTurns != 4 || !reflect.DeepEqual(got.AllowedTools, []string{"read_file"}) {
		t.Fatalf("frozen spec = %#v", got)
	}
}

var _ RuntimeCoreSession = (*runtimeCoreSessionFake)(nil)
var _ CoreReporter = (*runtimeCoreReporterFake)(nil)
