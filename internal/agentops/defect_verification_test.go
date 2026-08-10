package agentops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestDVAOP002RunnerWaitsForAcceptedWork(t *testing.T) {
	for _, test := range []struct {
		name string
		stop func(context.CancelFunc, chan Task)
	}{
		{name: "task channel closed", stop: func(_ context.CancelFunc, tasks chan Task) { close(tasks) }},
		{name: "context cancelled", stop: func(cancel context.CancelFunc, tasks chan Task) {
			cancel()
			close(tasks)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			finished := make(chan struct{})
			runner := &Runner{runTask: func(context.Context, Task) error {
				close(started)
				<-release
				close(finished)
				return nil
			}}
			tasks := make(chan Task)
			ctx, cancel := context.WithCancel(context.Background())
			returned := make(chan struct{})
			go func() {
				runner.Start(ctx, tasks)
				close(returned)
			}()
			tasks <- Task{TaskID: "accepted"}
			<-started
			test.stop(cancel, tasks)
			select {
			case <-returned:
				t.Fatal("Runner.Start returned before accepted work finished")
			case <-time.After(20 * time.Millisecond):
			}
			close(release)
			<-finished
			<-returned
			cancel()
		})
	}
}

func TestDVAOP002AcceptedPermitWaiterReachesCancellationHandling(t *testing.T) {
	firstStarted := make(chan struct{})
	secondContextError := make(chan error, 1)
	runner := &Runner{
		maxConcurrentTasks: 1,
		runTask: func(ctx context.Context, task Task) error {
			if task.TaskID == "first" {
				close(firstStarted)
				<-ctx.Done()
				return ctx.Err()
			}
			secondContextError <- ctx.Err()
			return ctx.Err()
		},
	}
	tasks := make(chan Task)
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan struct{})
	go func() {
		runner.Start(ctx, tasks)
		close(returned)
	}()
	tasks <- Task{TaskID: "first"}
	<-firstStarted
	secondAccepted := make(chan struct{})
	go func() {
		tasks <- Task{TaskID: "second"}
		close(secondAccepted)
	}()
	<-secondAccepted
	cancel()
	close(tasks)
	<-returned
	if err := <-secondContextError; !errors.Is(err, context.Canceled) {
		t.Fatalf("accepted waiter context error = %v, want cancellation handling", err)
	}
}

func TestDVAOP003EveryExecutionPathEmitsOneTypedOutcome(t *testing.T) {
	tests := []struct {
		name         string
		parent       func() context.Context
		runTask      func(context.Context, Task) error
		wantStatus   TaskOutcomeStatus
		wantReason   TaskOutcomeReason
		wantDelivery bool
	}{
		{
			name:   "success",
			parent: context.Background,
			runTask: func(context.Context, Task) error {
				return nil
			},
			wantStatus: TaskOutcomeCompleted,
			wantReason: TaskOutcomeReasonCompleted,
		},
		{
			name:   "ordinary failure",
			parent: context.Background,
			runTask: func(context.Context, Task) error {
				return errors.New("ordinary")
			},
			wantStatus:   TaskOutcomeFailed,
			wantReason:   TaskOutcomeReasonFailure,
			wantDelivery: true,
		},
		{
			name:   "timeout",
			parent: context.Background,
			runTask: func(ctx context.Context, _ Task) error {
				<-ctx.Done()
				return ctx.Err()
			},
			wantStatus:   TaskOutcomeCancelled,
			wantReason:   TaskOutcomeReasonTimeout,
			wantDelivery: true,
		},
		{
			name: "cancellation",
			parent: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			runTask: func(ctx context.Context, _ Task) error {
				return ctx.Err()
			},
			wantStatus:   TaskOutcomeCancelled,
			wantReason:   TaskOutcomeReasonCancellation,
			wantDelivery: true,
		},
		{
			name:   "panic",
			parent: context.Background,
			runTask: func(context.Context, Task) error {
				panic("task panic")
			},
			wantStatus:   TaskOutcomeFailed,
			wantReason:   TaskOutcomeReasonPanic,
			wantDelivery: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messenger := &verificationMessenger{
				contextErrors: make(chan error, 1),
				deadlines:     make(chan bool, 1),
			}
			observer := &recordingAgentOpsTaskOutcomeObserver{}
			runner := &Runner{messenger: messenger, runTask: test.runTask, taskOutcomeObserver: observer}
			if test.name == "timeout" {
				runner.taskTimeout = time.Nanosecond
			}
			runner.Run(test.parent(), Task{TaskID: test.name, ChatID: "chat"})
			outcomes := observer.snapshot()
			if len(outcomes) != 1 {
				t.Fatalf("outcomes = %#v, want exactly one", outcomes)
			}
			outcome := outcomes[0]
			if outcome.TaskID != test.name || outcome.ChatID != "chat" || outcome.Status != test.wantStatus || outcome.Reason != test.wantReason {
				t.Fatalf("outcome = %#v, want task/chat/status/reason %s/chat/%s/%s", outcome, test.name, test.wantStatus, test.wantReason)
			}
			wantCalls := int32(0)
			if test.wantDelivery {
				wantCalls = 1
			}
			if calls := messenger.calls.Load(); calls != wantCalls {
				t.Fatalf("terminal deliveries = %d, want %d", calls, wantCalls)
			}
			if !test.wantDelivery {
				return
			}
			if got := <-messenger.contextErrors; got != nil {
				t.Fatalf("terminal delivery context error = %v, want fresh context", got)
			}
			if hasDeadline := <-messenger.deadlines; !hasDeadline {
				t.Fatal("terminal delivery context has no deadline")
			}
		})
	}
}

func TestDVAOP003PanicReleasesCapacityBeforeTerminalTransition(t *testing.T) {
	observer := &blockingAgentOpsTaskOutcomeObserver{
		panicObserved: make(chan struct{}),
		releasePanic:  make(chan struct{}),
	}
	afterStarted := make(chan struct{})
	runner := &Runner{
		maxConcurrentTasks:  1,
		taskOutcomeObserver: observer,
		runTask: func(_ context.Context, task Task) error {
			if task.TaskID == "panic" {
				panic("task panic")
			}
			close(afterStarted)
			return nil
		},
	}
	tasks := make(chan Task, 2)
	tasks <- Task{TaskID: "panic", ChatID: "chat"}
	tasks <- Task{TaskID: "after", ChatID: "chat"}
	close(tasks)
	done := make(chan struct{})
	go func() {
		runner.Start(context.Background(), tasks)
		close(done)
	}()
	<-observer.panicObserved
	select {
	case <-afterStarted:
	case <-time.After(time.Second):
		t.Fatal("successor did not start while panic terminal observer was blocked")
	}
	close(observer.releasePanic)
	<-done
	outcomes := observer.snapshot()
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %#v, want one per accepted task", outcomes)
	}
	seen := make(map[string]TaskOutcome)
	for _, outcome := range outcomes {
		seen[outcome.TaskID] = outcome
	}
	if seen["panic"].Reason != TaskOutcomeReasonPanic || seen["after"].Status != TaskOutcomeCompleted {
		t.Fatalf("outcomes = %#v, want correlated panic and successor completion", outcomes)
	}
}

type blockingAgentOpsTaskOutcomeObserver struct {
	recordingAgentOpsTaskOutcomeObserver
	panicObserved chan struct{}
	releasePanic  chan struct{}
}

func (o *blockingAgentOpsTaskOutcomeObserver) ObserveTaskOutcome(outcome TaskOutcome) {
	o.recordingAgentOpsTaskOutcomeObserver.ObserveTaskOutcome(outcome)
	if outcome.Reason == TaskOutcomeReasonPanic {
		close(o.panicObserved)
		<-o.releasePanic
	}
}

func TestDVAOP004OneProviderSnapshotConfiguresEngineCompactorAndChild(t *testing.T) {
	selected := &rotatingAgentOpsNamedProvider{}
	snapshot := snapshotTaskProvider(selected)
	engineConfig := engine.Config{}
	compactionConfig := compaction.DefaultCompactionConfig()
	snapshot.apply(&engineConfig, &compactionConfig)
	if calls := selected.modelCalls.Load(); calls != 1 {
		t.Fatalf("ModelName() calls = %d, want one frozen read", calls)
	}
	if engineConfig.Model != "claude-4-sonnet" || compactionConfig.Model != engineConfig.Model {
		t.Fatalf("model snapshot = engine %q, compactor %q", engineConfig.Model, compactionConfig.Model)
	}
	if got := snapshot.provider.ModelName(); got != engineConfig.Model {
		t.Fatalf("child provider model = %q, want inherited snapshot %q", got, engineConfig.Model)
	}
	if calls := selected.modelCalls.Load(); calls != 1 {
		t.Fatalf("ModelName() calls after child read = %d, want still one", calls)
	}

	compactor, err := compaction.NewCompactor(snapshot.provider, compactionConfig)
	if err != nil {
		t.Fatalf("NewCompactor() error = %v", err)
	}
	wantWindow := compaction.NewModelRegistry().Lookup(engineConfig.Model)
	if compactor.ContextWindow() != wantWindow || compactor.ContextWindow() == compaction.DefaultContextWindow {
		t.Fatalf("compactor window = %d, selected model window = %d", compactor.ContextWindow(), wantWindow)
	}
	source, err := os.ReadFile("runner.go")
	if err != nil {
		t.Fatalf("ReadFile(runner.go) error = %v", err)
	}
	for _, required := range []string{
		"taskProvider := snapshotTaskProvider(r.provider)",
		"r.buildRegistry(task, sess, taskProvider.provider)",
		"engine.NewAgentEngine(\n\t\ttaskProvider.provider,",
		"compaction.NewCompactor(taskProvider.provider, compCfg)",
	} {
		if !strings.Contains(string(source), required) {
			t.Fatalf("Runner does not wire provider snapshot through %q", required)
		}
	}
}

func TestDVAOP005DeliveryFailuresCanEndWithoutDeliveredTerminalOutcome(t *testing.T) {
	workDir := t.TempDir()
	messenger := &scriptedAgentOpsMessenger{
		errors: []error{nil, errors.New("final delivery failed"), errors.New("fallback delivery failed")},
	}
	runner := NewRunner(&longFinalProvider{}, workDir, t.TempDir(), messenger, approval.NewStore())
	runner.sessions = session.NewManagerWithHome(workDir, t.TempDir())
	runner.Run(context.Background(), Task{TaskID: "delivery", ChatID: "chat", SenderID: "sender", Text: "incident"})

	texts := messenger.snapshot()
	if len(texts) != 3 {
		t.Fatalf("delivery attempts = %d, want initial, final, fallback", len(texts))
	}
	if len([]rune(texts[1])) <= 5000 {
		t.Fatalf("final delivery runes = %d, want unbounded model content evidence", len([]rune(texts[1])))
	}
	if !strings.Contains(texts[2], "AgentOps 任务失败") {
		t.Fatalf("fallback text = %q", texts[2])
	}
}

func TestDVAOP006LogSearchFollowsSymlinkOutsideLogRoot(t *testing.T) {
	logDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.log")
	if err := os.WriteFile(outside, []byte("SECRET external evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(logDir, "payment.log")); err != nil {
		t.Fatal(err)
	}
	tool := NewLogSearchTool(logDir)
	result, err := tool.Execute(context.Background(), mustAgentOpsJSON(t, map[string]any{
		"service": "payment",
		"query":   "SECRET",
		"limit":   10,
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result, "external evidence") {
		t.Fatalf("symlink result = %q, want outside content", result)
	}
}

func TestDVAOP006LogSearchResourceBoundsRemainEffective(t *testing.T) {
	logDir := t.TempDir()
	lines := make([]string, 250)
	for i := range lines {
		lines[i] = fmt.Sprintf("ERROR line %03d", i)
	}
	if err := os.WriteFile(filepath.Join(logDir, "bounded.log"), []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewLogSearchTool(logDir)
	result, err := tool.Execute(context.Background(), mustAgentOpsJSON(t, map[string]any{
		"service": "bounded",
		"query":   "ERROR",
		"limit":   200,
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := len(strings.Split(result, "\n")); got != 200 {
		t.Fatalf("matched lines = %d, want 200", got)
	}

	oversized := strings.Repeat("x", maxLogSearchLineBytes+1) + " ERROR\n"
	if err := os.WriteFile(filepath.Join(logDir, "oversized.log"), []byte(oversized), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = tool.Execute(context.Background(), mustAgentOpsJSON(t, map[string]any{
		"service": "oversized",
		"query":   "ERROR",
		"limit":   1,
	}))
	if err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("oversized line error = %v, want scanner resource bound", err)
	}
}

type verificationMessenger struct {
	calls         atomic.Int32
	contextErrors chan error
	deadlines     chan bool
}

func (m *verificationMessenger) SendText(ctx context.Context, _ string, _ string) error {
	m.calls.Add(1)
	if m.contextErrors != nil {
		m.contextErrors <- ctx.Err()
	}
	if m.deadlines != nil {
		_, hasDeadline := ctx.Deadline()
		m.deadlines <- hasDeadline
	}
	return nil
}

type recordingAgentOpsTaskOutcomeObserver struct {
	mu       sync.Mutex
	outcomes []TaskOutcome
}

func (o *recordingAgentOpsTaskOutcomeObserver) ObserveTaskOutcome(outcome TaskOutcome) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.outcomes = append(o.outcomes, outcome)
}

func (o *recordingAgentOpsTaskOutcomeObserver) snapshot() []TaskOutcome {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]TaskOutcome(nil), o.outcomes...)
}

type scriptedAgentOpsMessenger struct {
	mu     sync.Mutex
	texts  []string
	errors []error
}

func (m *scriptedAgentOpsMessenger) SendText(_ context.Context, _ string, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, text)
	index := len(m.texts) - 1
	if index < len(m.errors) {
		return m.errors[index]
	}
	return nil
}

func (m *scriptedAgentOpsMessenger) snapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.texts...)
}

type longFinalProvider struct{}

func (*longFinalProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: strings.Repeat("x", 6000)}}, nil
}

type agentOpsNamedProvider struct {
	model string
}

func (*agentOpsNamedProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return nil, errors.New("not called")
}

func (p *agentOpsNamedProvider) ModelName() string      { return p.model }
func (*agentOpsNamedProvider) ProviderProtocol() string { return "scripted" }

type rotatingAgentOpsNamedProvider struct {
	modelCalls atomic.Int32
}

func (*rotatingAgentOpsNamedProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return nil, errors.New("not called")
}

func (p *rotatingAgentOpsNamedProvider) ModelName() string {
	if p.modelCalls.Add(1) == 1 {
		return "claude-4-sonnet"
	}
	return "different-model"
}

func (*rotatingAgentOpsNamedProvider) ProviderProtocol() string { return "scripted" }

func mustAgentOpsJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
