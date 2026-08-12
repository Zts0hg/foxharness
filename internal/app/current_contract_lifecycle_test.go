package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestAgentRunnerSerializesRunsWithinOneSession(t *testing.T) {
	modelProvider := &serialLifecycleProvider{
		entered: make(chan string, 2),
		release: make(chan struct{}),
	}
	runner, sess := newLifecycleAgentRunner(t, t.TempDir(), modelProvider, nil)
	run := func(prompt string, started chan<- struct{}, done chan<- serializedLifecycleOutcome) {
		close(started)
		result, err := runner.Run(context.Background(), prompt, nil)
		final := ""
		if result != nil {
			final = result.FinalMessage
		}
		done <- serializedLifecycleOutcome{result: final, err: err}
	}

	firstStarted := make(chan struct{})
	firstDone := make(chan serializedLifecycleOutcome, 1)
	go run("first run", firstStarted, firstDone)
	<-firstStarted
	if got := receiveLifecycleString(t, modelProvider.entered); got != "first run" {
		t.Fatalf("first provider prompt = %q", got)
	}
	if runner.runMu.TryLock() {
		runner.runMu.Unlock()
		t.Fatal("run mutex was available while first run was inside provider")
	}

	secondStarted := make(chan struct{})
	secondDone := make(chan serializedLifecycleOutcome, 1)
	go run("second run", secondStarted, secondDone)
	<-secondStarted
	modelProvider.release <- struct{}{}
	if got := receiveLifecycleOutcome(t, firstDone); got.err != nil || got.result != "done:first run" {
		t.Fatalf("first outcome = %#v", got)
	}
	if got := receiveLifecycleString(t, modelProvider.entered); got != "second run" {
		t.Fatalf("second provider prompt = %q", got)
	}
	modelProvider.release <- struct{}{}
	if got := receiveLifecycleOutcome(t, secondDone); got.err != nil || got.result != "done:second run" {
		t.Fatalf("second outcome = %#v", got)
	}

	records, err := session.NewMessageLog(sess).LoadRecords()
	if err != nil {
		t.Fatalf("LoadRecords() error = %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("message records = %d, want 4", len(records))
	}
	wantContent := []string{"first run", "done:first run", "second run", "done:second run"}
	for i, want := range wantContent {
		if records[i].Message.Content != want {
			t.Fatalf("records[%d] content = %q, want %q", i, records[i].Message.Content, want)
		}
	}
	if records[0].RunID == records[2].RunID || records[0].RunID != records[1].RunID || records[2].RunID != records[3].RunID {
		t.Fatalf("run record correlation = %#v", records)
	}
}

func TestDistinctAgentRunnersExecuteConcurrentlyWithoutStateLeakage(t *testing.T) {
	barrier := &parallelLifecycleBarrier{entered: make(chan string, 2), release: make(chan struct{})}
	providerA := &isolatedLifecycleProvider{marker: "runner-a", barrier: barrier}
	providerB := &isolatedLifecycleProvider{marker: "runner-b", barrier: barrier}
	stateA := permission.NewState(permission.ModeFullAccess, true)
	stateB := permission.NewState(permission.ModeFullAccess, true)
	workDirA := t.TempDir()
	workDirB := t.TempDir()
	runnerA, sessA := newLifecycleAgentRunner(t, workDirA, providerA, permission.NewCoordinator(permission.Config{State: stateA, Workspace: workDirA, CWD: workDirA}))
	runnerB, sessB := newLifecycleAgentRunner(t, workDirB, providerB, permission.NewCoordinator(permission.Config{State: stateB, Workspace: workDirB, CWD: workDirB}))
	runnerA.pendingActivations = []string{"runner-a private reminder"}
	runnerB.pendingActivations = []string{"runner-b private reminder"}

	done := make(chan isolatedLifecycleOutcome, 2)
	start := func(runner *AgentRunner, prompt string) {
		result, err := runner.Run(context.Background(), prompt, nil)
		got := isolatedLifecycleOutcome{err: err}
		if result != nil {
			got.resultSession = result.SessionID
			got.runID = result.RunID
		}
		done <- got
	}
	go start(runnerA, "run a")
	go start(runnerB, "run b")

	seen := map[string]bool{}
	seen[receiveLifecycleString(t, barrier.entered)] = true
	seen[receiveLifecycleString(t, barrier.entered)] = true
	if !seen["runner-a"] || !seen["runner-b"] {
		t.Fatalf("parallel entries = %#v, want both runners", seen)
	}
	close(barrier.release)

	outcomes := []isolatedLifecycleOutcome{receiveIsolatedLifecycleOutcome(t, done), receiveIsolatedLifecycleOutcome(t, done)}
	for _, got := range outcomes {
		if got.err != nil || got.resultSession == "" || got.runID == "" {
			t.Fatalf("isolated outcome = %#v", got)
		}
	}
	if sessA.ID == sessB.ID || sessA.RootDir == sessB.RootDir {
		t.Fatalf("session identity leaked: A=%#v B=%#v", sessA, sessB)
	}
	for _, item := range []struct {
		path string
		want string
	}{
		{path: filepath.Join(workDirA, "isolated.txt"), want: "runner-a"},
		{path: filepath.Join(workDirB, "isolated.txt"), want: "runner-b"},
	} {
		data, err := os.ReadFile(item.path)
		if err != nil || string(data) != item.want {
			t.Fatalf("ReadFile(%s) = %q, %v; want %q", item.path, data, err, item.want)
		}
	}
	assertLifecycleProviderIsolation(t, providerA, "runner-a private reminder", "runner-b private reminder")
	assertLifecycleProviderIsolation(t, providerB, "runner-b private reminder", "runner-a private reminder")
	if got := stateA.Snapshot(); got.EffectiveMode != permission.ModeFullAccess || got.SessionGrantCount != 0 {
		t.Fatalf("runner A permission state = %#v", got)
	}
	stateA.SetSelected(permission.ModeAsk, false)
	if got := stateA.Snapshot(); got.EffectiveMode != permission.ModeAsk || got.SessionGrantCount != 0 {
		t.Fatalf("runner A permission state after local mutation = %#v", got)
	}
	if got := stateB.Snapshot(); got.EffectiveMode != permission.ModeFullAccess || got.SessionGrantCount != 0 {
		t.Fatalf("runner B permission state = %#v", got)
	}
}

func newLifecycleAgentRunner(t *testing.T, workDir string, modelProvider provider.LLMProvider, coordinator *permission.Coordinator) (*AgentRunner, *session.Session) {
	t.Helper()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	store := memory.NewSessionStore(workDir, sess.RootDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	return &AgentRunner{
		workDir:               workDir,
		model:                 "lifecycle-model",
		providerProtocol:      "scripted",
		collaborationMode:     collaboration.ModeDefault,
		maxTurns:              3,
		store:                 store,
		manager:               manager,
		llmProvider:           modelProvider,
		currentSession:        sess,
		permissionCoordinator: coordinator,
	}, sess
}

type serialLifecycleProvider struct {
	entered chan string
	release chan struct{}
}

type serializedLifecycleOutcome struct {
	result string
	err    error
}

type isolatedLifecycleOutcome struct {
	resultSession string
	runID         string
	err           error
}

func (p *serialLifecycleProvider) Generate(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	prompt := lifecycleLastDirectUserMessage(messages)
	p.entered <- prompt
	<-p.release
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done:" + prompt}}, nil
}

type parallelLifecycleBarrier struct {
	entered chan string
	release chan struct{}
}

type isolatedLifecycleProvider struct {
	mu      sync.Mutex
	marker  string
	barrier *parallelLifecycleBarrier
	calls   int
	seen    [][]schema.Message
}

func (p *isolatedLifecycleProvider) Generate(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.seen = append(p.seen, append([]schema.Message(nil), messages...))
	p.mu.Unlock()
	if call == 1 {
		p.barrier.entered <- p.marker
		<-p.barrier.release
		args, err := json.Marshal(map[string]string{"path": "isolated.txt", "content": p.marker})
		if err != nil {
			return nil, err
		}
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "call-" + p.marker, Name: "write_file", Arguments: args}}}}, nil
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done:" + p.marker}}, nil
}

func lifecycleLastDirectUserMessage(messages []schema.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == schema.RoleUser && messages[i].ToolCallID == "" && !strings.HasPrefix(messages[i].Content, "[Runtime System") {
			return messages[i].Content
		}
	}
	return ""
}

func assertLifecycleProviderIsolation(t *testing.T, modelProvider *isolatedLifecycleProvider, own, foreign string) {
	t.Helper()
	modelProvider.mu.Lock()
	defer modelProvider.mu.Unlock()
	if modelProvider.calls != 2 || len(modelProvider.seen) != 2 {
		t.Fatalf("provider %s calls/requests = %d/%d", modelProvider.marker, modelProvider.calls, len(modelProvider.seen))
	}
	for _, request := range modelProvider.seen {
		joined := fmt.Sprint(request)
		if !strings.Contains(joined, own) || strings.Contains(joined, foreign) {
			t.Fatalf("provider %s request leaked reminders: %s", modelProvider.marker, joined)
		}
	}
}

func receiveLifecycleString(t *testing.T, values <-chan string) string {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for lifecycle barrier")
		return ""
	}
}

func receiveLifecycleOutcome(t *testing.T, values <-chan serializedLifecycleOutcome) serializedLifecycleOutcome {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for serialized lifecycle outcome")
		return serializedLifecycleOutcome{}
	}
}

func receiveIsolatedLifecycleOutcome(t *testing.T, values <-chan isolatedLifecycleOutcome) isolatedLifecycleOutcome {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for isolated lifecycle outcome")
		return isolatedLifecycleOutcome{}
	}
}
