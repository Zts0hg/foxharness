package autodev

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestPFAUT002ActiveAttemptFreezesItemStageAndPromptIdentity(t *testing.T) {
	entered := make(chan CoreAttempt, 1)
	release := make(chan struct{})
	core := &autodevProfileCore{run: func(ctx context.Context, attempt CoreAttempt) CoreOutcome {
		entered <- attempt
		select {
		case <-release:
		case <-ctx.Done():
			return autodevProfileCancelledOutcome(attempt, ctx.Err())
		}
		return autodevProfileSuccessOutcome(attempt, "session-item", "run-stage")
	}}
	sc := &StageContext{ItemID: "item-stable", Slug: "stable", WorkDir: "/worktree/stable", FeatureDir: ".codexspec/specs/stable"}
	stage := Stage{
		Name:   "generate-spec",
		Prompt: func(*StageContext) string { return "stable stage prompt" },
		Verify: func(context.Context, *StageContext) (bool, string) { return true, "" },
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- NewStageMachine(&autodevProfileEngineer{}, nil).RunStep(context.Background(), core, sc, stage)
	}()
	attempt := <-entered
	sc.ItemID = "mutated-item"
	sc.Slug = "mutated"
	sc.Stage = "mutated-stage"
	sc.WorkDir = "/worktree/mutated"
	sc.FeatureDir = ".codexspec/specs/mutated"
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if attempt.AttemptID != "core:item-stable:generate-spec:1" || attempt.CorrelationID != attempt.AttemptID || attempt.Ordinal != 1 || attempt.Prompt != "stable stage prompt" {
		t.Fatalf("active core attempt changed with caller state: %#v", attempt)
	}
}

func TestPFAUT003ItemRunnerKeepsOneSessionAcrossRunsAndIsolatesItems(t *testing.T) {
	factory := &autodevProfileCoreFactory{}
	first := newItemCoreRunner(context.Background(), factory, "/worktree/one", "model-one")
	second := newItemCoreRunner(context.Background(), factory, "/worktree/two", "model-one")

	firstA := first.Run(context.Background(), autodevProfileAttempt("item-one", "stage-a", 1), nil)
	firstB := first.Run(context.Background(), autodevProfileAttempt("item-one", "stage-b", 2), nil)
	secondA := second.Run(context.Background(), autodevProfileAttempt("item-two", "stage-a", 1), nil)
	if firstA.SessionID != firstB.SessionID || firstA.RunID == firstB.RunID {
		t.Fatalf("same item continuity = %#v / %#v", firstA, firstB)
	}
	if firstA.SessionID == secondA.SessionID || firstA.RunID == secondA.RunID {
		t.Fatalf("distinct item runtime leaked identity = %#v / %#v", firstA, secondA)
	}
	got := factory.snapshot()
	if len(got) != 2 || got[0].workDir != "/worktree/one" || got[1].workDir != "/worktree/two" || got[0].model != "model-one" || got[1].model != "model-one" {
		t.Fatalf("item factory snapshots = %#v", got)
	}
	if got[0].runs != 2 || got[1].runs != 1 {
		t.Fatalf("item runner calls = %d/%d, want 2/1", got[0].runs, got[1].runs)
	}
}

func TestPFAUT007EngineerQuestionPortAlwaysReturnsOneVisibleDecision(t *testing.T) {
	reporter := &autodevProfileReporter{TerminalReporter: NewTerminalReporter(io.Discard)}
	asker := NewEngineerAsker(&autodevProfileEngineer{decideErr: errors.New("fixture unavailable")}, reporter, &StageContext{Stage: "implement-tasks"})
	questions := []Question{
		{Prompt: "Choose approach", Options: []Option{{Label: "Conservative", Description: "bounded"}}},
		{Prompt: "Proceed?"},
	}
	answers, err := asker.Ask(context.Background(), questions)
	if err != nil || len(answers) != len(questions) {
		t.Fatalf("Ask() = %#v, %v", answers, err)
	}
	if answers[0].Value != "Conservative" || answers[1].Value != "Proceed with your best judgement." {
		t.Fatalf("fallback answers = %#v", answers)
	}
	if len(reporter.info) != 1 || !strings.Contains(reporter.info[0], "WARNING: engineer decide failed") {
		t.Fatalf("visible conservative fallback = %#v", reporter.info)
	}
}

func TestPFAUT014And015CoreReturnsOneNeutralCorrelatedOutcome(t *testing.T) {
	attempt := autodevProfileAttempt("item", "implement-tasks", 4)
	cause := errors.New("provider failed")
	outcome := CoreOutcome{
		Attempt: attempt, Status: CoreOutcomeFailed, SessionID: "session-item", RunID: "run-4",
		PartialMessage: "committed partial", Cause: cause, RetryClass: CoreRetrySameRunner,
		Lifecycle: CoreLifecycleEvidence{RunStarted: true, PostRunEstablished: true},
	}
	if err := outcome.Validate(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(&CoreOutcomeError{Outcome: outcome}, cause) {
		t.Fatal("typed terminal outcome lost its original cause")
	}
	if outcome.Attempt != attempt || outcome.PartialMessage != "committed partial" {
		t.Fatalf("neutral outcome correlation = %#v", outcome)
	}
}

func TestPFAUT016ControlClientDoesNotConstructEngineProviderOrPresentation(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		for _, forbidden := range []string{"\"github.com/Zts0hg/foxharness/internal/app\"", "\"github.com/Zts0hg/foxharness/internal/tui\"", "\"github.com/Zts0hg/foxharness/cmd/fox\"", "provider.NewProvider", "engine.NewLegacyEngine", "fmt.Print", "os.Stdout"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("Autodev control client %s contains runtime/presentation construction token %q", entry.Name(), forbidden)
			}
		}
	}
}

type autodevProfileCore struct {
	run func(context.Context, CoreAttempt) CoreOutcome
}

func (c *autodevProfileCore) Run(ctx context.Context, attempt CoreAttempt, _ CoreReporter) CoreOutcome {
	return c.run(ctx, attempt)
}
func (*autodevProfileCore) Drain(context.Context) error { return nil }
func (*autodevProfileCore) Close(context.Context) error { return nil }
func (*autodevProfileCore) SetUserAsker(QuestionAsker)  {}
func (*autodevProfileCore) SetModel(string) error       { return nil }
func (*autodevProfileCore) WorkDir() string             { return "" }
func (*autodevProfileCore) StagePrompt(context.Context, string, string) (string, error) {
	return "", nil
}

type autodevProfileFactoryRecord struct {
	workDir, model, sessionID string
	runs                      int
}

type autodevProfileCoreFactory struct {
	mu      sync.Mutex
	records []*autodevProfileFactoryRecord
}

func (f *autodevProfileCoreFactory) New(_ context.Context, workDir, model string) (CoreRunner, error) {
	f.mu.Lock()
	record := &autodevProfileFactoryRecord{workDir: workDir, model: model, sessionID: "session-" + workDir}
	f.records = append(f.records, record)
	f.mu.Unlock()
	return &autodevProfileCore{run: func(_ context.Context, attempt CoreAttempt) CoreOutcome {
		f.mu.Lock()
		record.runs++
		runID := record.sessionID + "-run-" + strconv.Itoa(record.runs)
		f.mu.Unlock()
		return autodevProfileSuccessOutcome(attempt, record.sessionID, runID)
	}}, nil
}

func (f *autodevProfileCoreFactory) snapshot() []autodevProfileFactoryRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]autodevProfileFactoryRecord, len(f.records))
	for i, record := range f.records {
		out[i] = *record
	}
	return out
}

type autodevProfileEngineer struct{ decideErr error }

func (e *autodevProfileEngineer) Decide(context.Context, []Question, StageContext) ([]Answer, error) {
	return nil, e.decideErr
}
func (*autodevProfileEngineer) Reply(context.Context, string, StageContext) (string, error) {
	return "", nil
}
func (*autodevProfileEngineer) Review(context.Context, CoreReviewEvidence, string, StageContext) (string, error) {
	return "", nil
}

type autodevProfileReporter struct {
	*TerminalReporter
	info []string
}

func (r *autodevProfileReporter) OnInfo(_ context.Context, message string) {
	r.info = append(r.info, message)
}

func autodevProfileAttempt(item, stage string, ordinal int) CoreAttempt {
	id := "core:" + item + ":" + stage + ":" + strconv.Itoa(ordinal)
	return CoreAttempt{AttemptID: id, CorrelationID: id, Ordinal: ordinal, Prompt: stage}
}

func autodevProfileSuccessOutcome(attempt CoreAttempt, sessionID, runID string) CoreOutcome {
	return CoreOutcome{
		Attempt: attempt, Status: CoreOutcomeSucceeded, SessionID: sessionID, RunID: runID,
		PartialMessage: "done", RetryClass: CoreRetryNever,
		Lifecycle: CoreLifecycleEvidence{RunStarted: true, PostRunEstablished: true},
	}
}

func autodevProfileCancelledOutcome(attempt CoreAttempt, err error) CoreOutcome {
	return CoreOutcome{
		Attempt: attempt, Status: CoreOutcomeCancelled, Cause: err, RetryClass: CoreRetryNever,
	}
}
