package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
)

type childCaptureProvider struct {
	mu       sync.Mutex
	messages [][]schema.Message
	tools    [][]schema.ToolDefinition
	response string
	model    string
}

func (p *childCaptureProvider) Generate(_ context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.messages = append(p.messages, append([]schema.Message(nil), messages...))
	p.tools = append(p.tools, append([]schema.ToolDefinition(nil), availableTools...))
	p.mu.Unlock()
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: p.response}}, nil
}

func (*childCaptureProvider) ProviderProtocol() string { return "claude" }
func (p *childCaptureProvider) ModelName() string      { return p.model }

func (p *childCaptureProvider) snapshot() ([]schema.Message, []schema.ToolDefinition) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.messages) == 0 {
		return nil, nil
	}
	return append([]schema.Message(nil), p.messages[0]...), append([]schema.ToolDefinition(nil), p.tools[0]...)
}

func TestDVCHD001ReadOnlyChildBashCanMutateInsideAndOutsideWorkspace(t *testing.T) {
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	mgr := NewManager(nil, workDir)
	registry := mgr.buildRegistry(true, nil)

	names := definitionNames(registry.GetAvailableTools())
	if !reflect.DeepEqual(names, []string{"bash", "read_file"}) {
		t.Fatalf("read-only definitions = %v, want bash and read_file", names)
	}

	inside := filepath.Join(workDir, "inside.txt")
	outside := filepath.Join(outsideDir, "outside.txt")
	command := fmt.Sprintf("printf inside > %q; bash -c 'printf outside > %q'", inside, outside)
	result := registry.Execute(context.Background(), schema.ToolCall{
		ID:        "mutating-read-only-bash",
		Name:      "bash",
		Arguments: mustJSON(t, map[string]string{"command": command}),
	})
	if result.IsError {
		t.Fatalf("read-only bash result = %#v, want executable", result)
	}
	for path, want := range map[string]string{inside: "inside", outside: "outside"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("mutated file %q = %q/%v, want %q", path, got, err, want)
		}
	}

	coordinator := permission.NewCoordinator(permission.Config{
		State: permission.NewState(permission.ModeFullAccess, true),
	})
	assessment, err := NewTool(NewManager(nil, workDir).WithPermission(coordinator), "parent").AssessPermission(
		toolpolicy.Context{}, json.RawMessage(`{"task":"inspect","read_only":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !assessment.ReadOnly || assessment.RiskHint != toolpolicy.RiskLow {
		t.Fatalf("delegation assessment = %+v, want low-risk read-only classification", assessment)
	}
}

func TestDVCHD002ChildModelInvocationAndCompactorUseDifferentSnapshots(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := t.TempDir()
	provider := &childCaptureProvider{response: "done", model: "claude-3.5-sonnet"}
	mgr := NewManager(provider, workDir)

	result, err := mgr.Run(context.Background(), Request{
		ParentSessionID: "parent",
		Task:            "inspect",
		ReadOnly:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := session.NewManagerWithHome(workDir, homeDir).Open(result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	tracePaths, err := filepath.Glob(filepath.Join(sess.RunsDir(), "*", "trace.jsonl"))
	if err != nil || len(tracePaths) != 1 {
		t.Fatalf("child run trace paths = %v, error = %v", tracePaths, err)
	}
	trace, err := os.ReadFile(tracePaths[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(trace), `"model":"claude-3.5-sonnet"`) {
		t.Fatalf("child model invocation trace does not contain selected model: %s", trace)
	}

	childCompactor, err := mgr.newCompactor(sess)
	if err != nil {
		t.Fatal(err)
	}
	selectedConfig := compaction.DefaultCompactionConfig()
	selectedConfig.Model = provider.model
	selectedCompactor, err := compaction.NewCompactor(provider, selectedConfig)
	if err != nil {
		t.Fatal(err)
	}
	if childCompactor.ContextWindow() != compaction.DefaultContextWindow {
		t.Fatalf("child compactor window = %d, want fallback %d", childCompactor.ContextWindow(), compaction.DefaultContextWindow)
	}
	if selectedCompactor.ContextWindow() != 200000 {
		t.Fatalf("selected-model compactor window = %d, want 200000", selectedCompactor.ContextWindow())
	}
	if childCompactor.Threshold() == selectedCompactor.Threshold() {
		t.Fatalf("compaction trigger thresholds unexpectedly agree at %d", childCompactor.Threshold())
	}
	history := make([]schema.Message, 0, 20)
	for i := 0; i < cap(history); i++ {
		role := schema.RoleUser
		if i == 0 {
			role = schema.RoleSystem
		}
		history = append(history, schema.Message{Role: role, Content: strings.Repeat("x", 18000)})
	}
	used := childCompactor.Estimate(history)
	if used < childCompactor.Threshold() || used >= selectedCompactor.Threshold() {
		t.Fatalf("fixture tokens = %d, want fallback-trigger/selected-model-no-trigger interval [%d,%d)", used, childCompactor.Threshold(), selectedCompactor.Threshold())
	}
	selectedProjection, err := selectedCompactor.MaybeCompact(context.Background(), history)
	if err != nil {
		t.Fatal(err)
	}
	if len(selectedProjection) != len(history) {
		t.Fatalf("selected-model projection compacted %d messages to %d below its trigger", len(history), len(selectedProjection))
	}
	fallbackProjection, err := childCompactor.MaybeCompact(context.Background(), history)
	if err != nil {
		t.Fatal(err)
	}
	if len(fallbackProjection) >= len(history) {
		t.Fatalf("fallback projection retained %d messages, want premature compaction below selected-model trigger", len(fallbackProjection))
	}
}

func TestDVCHD003PromptAndEffectiveToolSnapshotsDisagree(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := t.TempDir()
	provider := &childCaptureProvider{response: "done", model: "test-model"}
	mgr := NewManager(provider, workDir)

	_, err := mgr.Run(context.Background(), Request{
		ParentSessionID: "parent",
		Task:            "inspect only",
		ReadOnly:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	messages, definitions := provider.snapshot()
	if got := definitionNames(definitions); !reflect.DeepEqual(got, []string{"bash", "read_file"}) {
		t.Fatalf("model-visible read-only tools = %v", got)
	}
	prompt := messageText(messages)
	for _, unsupported := range []string{"Use edit_file", "Use write_file", "read_todo", "update_todo"} {
		if !strings.Contains(prompt, unsupported) {
			t.Fatalf("current read-only prompt no longer demonstrates unsupported %q guidance:\n%s", unsupported, prompt)
		}
	}

	for _, readOnly := range []bool{true, false} {
		registry := mgr.buildRegistry(readOnly, nil)
		want := []string{"bash", "read_file"}
		if !readOnly {
			want = []string{"bash", "edit_file", "read_file", "write_file"}
		}
		if got := definitionNames(registry.GetAvailableTools()); !reflect.DeepEqual(got, want) {
			t.Fatalf("readOnly=%t definitions = %v, want %v", readOnly, got, want)
		}
		for _, unavailable := range []string{"delegate_task", "read_todo", "update_todo", "skill"} {
			call := schema.ToolCall{ID: unavailable, Name: unavailable, Arguments: json.RawMessage(`{}`)}
			if result := registry.Execute(context.Background(), call); !result.IsError {
				t.Fatalf("readOnly=%t unavailable call %q executed: %#v", readOnly, unavailable, result)
			}
			if registry.IsParallelSafe(unavailable) {
				t.Fatalf("readOnly=%t unavailable call %q is parallel-safe", readOnly, unavailable)
			}
		}
	}
}

func TestDVCHD004ChildCancellationKillsShellDescendantsAndPendingApproval(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the current ChildRun bash profile requires a Unix shell")
	}
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	mgr := NewManager(nil, workDir)
	registry := mgr.buildRegistry(true, nil)
	started := filepath.Join(workDir, "started")
	leaked := filepath.Join(workDir, "leaked")
	command := fmt.Sprintf("touch %q; (sleep 0.25; touch %q) & wait", started, leaked)
	arguments := mustJSON(t, map[string]string{"command": command})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan schema.ToolResult, 1)
	go func() {
		done <- registry.Execute(ctx, schema.ToolCall{
			ID:        "cancel-child-tree",
			Name:      "bash",
			Arguments: arguments,
		})
	}()
	waitForFile(t, started, time.Second)
	cancel()
	select {
	case result := <-done:
		if !result.IsError {
			t.Fatalf("cancelled Bash result = %#v, want error", result)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled ChildRun Bash did not return")
	}
	time.Sleep(350 * time.Millisecond)
	if _, err := os.Stat(leaked); !os.IsNotExist(err) {
		t.Fatalf("shell descendant produced a late side effect: %v", err)
	}

	approver := &cancellingApprover{started: make(chan struct{})}
	state := permission.NewState(permission.ModeAsk, false)
	coordinator := permission.NewCoordinator(permission.Config{State: state, Approver: approver})
	permissionRegistry := NewManager(nil, workDir).WithPermission(coordinator).buildRegistry(true, nil)
	approvalCtx, approvalCancel := context.WithCancel(context.Background())
	approvalDone := make(chan schema.ToolResult, 1)
	go func() {
		approvalDone <- permissionRegistry.Execute(approvalCtx, schema.ToolCall{
			ID:        "cancel-approval",
			Name:      "bash",
			Arguments: json.RawMessage(`{"command":"touch should-not-run"}`),
		})
	}()
	select {
	case <-approver.started:
	case <-time.After(time.Second):
		t.Fatal("child permission approval did not start")
	}
	approvalCancel()
	select {
	case result := <-approvalDone:
		if !result.IsError {
			t.Fatalf("cancelled approval result = %#v, want denial", result)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled child permission approval did not return")
	}
	if state.Snapshot().SessionGrantCount != 0 {
		t.Fatalf("cancelled child approval retained a session grant: %+v", state.Snapshot())
	}
	if _, err := os.Stat(filepath.Join(workDir, "should-not-run")); !os.IsNotExist(err) {
		t.Fatalf("cancelled child approval executed Bash: %v", err)
	}

	exhaustionLeak := filepath.Join(workDir, "turn-exhaustion-leak")
	backgroundCommand := fmt.Sprintf("nohup bash -c 'sleep 0.25; touch %q' >/dev/null 2>&1 &", exhaustionLeak)
	result, err := NewManager(&backgroundBashChildProvider{command: backgroundCommand}, workDir).
		WithMaxTurns(1).
		Run(context.Background(), Request{ParentSessionID: "parent", Task: "start background work", ReadOnly: true})
	if err == nil || result != nil {
		t.Fatalf("turn-exhausted child result/error = %#v/%v, want current nil outcome and error", result, err)
	}
	if !strings.Contains(err.Error(), "超过最大 Turn 数限制: 1") {
		t.Fatalf("turn-exhausted child error = %q, want turn limit", err)
	}
	waitForFile(t, exhaustionLeak, time.Second)
}

type cancellingApprover struct {
	started chan struct{}
	once    sync.Once
}

func (a *cancellingApprover) Approve(ctx context.Context, _ permission.ApprovalRequest) (permission.UserDecision, error) {
	a.once.Do(func() { close(a.started) })
	<-ctx.Done()
	return permission.UserDecision{}, ctx.Err()
}

func TestDVCHD006ChildWrapperDiscardsCorrelatedOutcomesForEveryFailureClass(t *testing.T) {
	t.Run("provider", func(t *testing.T) {
		assertNilChildOutcome(t, &errorChildProvider{err: errors.New("provider failed")}, 2, nil)
	})

	t.Run("tool then provider", func(t *testing.T) {
		provider := &toolThenErrorChildProvider{}
		assertNilChildOutcome(t, provider, 3, nil)
	})

	t.Run("persistence", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		workDir := t.TempDir()
		provider := &messageLogBreakingProvider{homeDir: homeDir}
		result, err := NewManager(provider, workDir).Run(context.Background(), Request{ParentSessionID: "parent", Task: "persist", ReadOnly: true})
		if err == nil || result != nil {
			t.Fatalf("persistence result/error = %#v/%v, want nil correlated outcome and error", result, err)
		}
	})

	t.Run("compaction construction", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		workDir := t.TempDir()
		mgr := NewManager(&finalReportProvider{}, workDir)
		mgr.compactorFactory = func(*session.Session) (*compaction.Compactor, error) {
			return nil, errors.New("compactor failed")
		}
		result, err := mgr.Run(context.Background(), Request{ParentSessionID: "parent", Task: "compact", ReadOnly: true})
		if err == nil || result != nil {
			t.Fatalf("compaction result/error = %#v/%v, want nil correlated outcome and error", result, err)
		}
		if _, err := session.NewManagerWithHome(workDir, homeDir).Latest(session.LookupOptions{Source: session.SOURCESubagent}); err != nil {
			t.Fatalf("created child session is not recoverable after compactor failure: %v", err)
		}
	})

	t.Run("turn limit with partial report", func(t *testing.T) {
		assertNilChildOutcome(t, &partialLoopChildProvider{}, 1, nil)
	})

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assertNilChildOutcome(t, &errorChildProvider{err: context.Canceled}, 2, ctx)
	})
}

func assertNilChildOutcome(t *testing.T, p provider.LLMProvider, maxTurns int, ctx context.Context) {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	if ctx == nil {
		ctx = context.Background()
	}
	workDir := t.TempDir()
	result, err := NewManager(p, workDir).WithMaxTurns(maxTurns).Run(ctx, Request{
		ParentSessionID: "parent",
		Task:            "exercise failure",
		ReadOnly:        true,
	})
	if err == nil || result != nil {
		t.Fatalf("Run() result/error = %#v/%v, want nil correlated outcome and error", result, err)
	}
	if _, latestErr := session.NewManagerWithHome(workDir, homeDir).Latest(session.LookupOptions{Source: session.SOURCESubagent}); latestErr != nil {
		t.Fatalf("child session persisted but its identity was not returned: %v", latestErr)
	}
}

type errorChildProvider struct{ err error }

func (p *errorChildProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return nil, p.err
}

type toolThenErrorChildProvider struct{ calls int }

func (p *toolThenErrorChildProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	if p.calls == 1 {
		return &provider.GenerateResponse{Message: &schema.Message{
			Role:    schema.RoleAssistant,
			Content: "partial before failed tool",
			ToolCalls: []schema.ToolCall{{
				ID:        "missing",
				Name:      "missing_tool",
				Arguments: json.RawMessage(`{}`),
			}},
		}}, nil
	}
	return nil, errors.New("provider failed after tool result")
}

type partialLoopChildProvider struct{}

func (*partialLoopChildProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return &provider.GenerateResponse{Message: &schema.Message{
		Role:    schema.RoleAssistant,
		Content: "partial child report",
		ToolCalls: []schema.ToolCall{{
			ID:        "read",
			Name:      "read_file",
			Arguments: json.RawMessage(`{"path":"missing"}`),
		}},
	}}, nil
}

type backgroundBashChildProvider struct{ command string }

func (p *backgroundBashChildProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	arguments, err := json.Marshal(map[string]string{"command": p.command})
	if err != nil {
		return nil, err
	}
	return &provider.GenerateResponse{Message: &schema.Message{
		Role: schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{{
			ID:        "detached-background",
			Name:      "bash",
			Arguments: arguments,
		}},
	}}, nil
}

type messageLogBreakingProvider struct{ homeDir string }

func (p *messageLogBreakingProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	pattern := filepath.Join(p.homeDir, ".foxharness", "projects", "*", "sessions", "*", "messages.jsonl")
	paths, err := filepath.Glob(pattern)
	if err != nil || len(paths) != 1 {
		return nil, fmt.Errorf("locate child message log: paths=%v err=%w", paths, err)
	}
	if err := os.Chmod(paths[0], 0o400); err != nil {
		return nil, err
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "unpersisted partial"}}, nil
}

func definitionNames(definitions []schema.ToolDefinition) []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	return names
}

func messageText(messages []schema.Message) string {
	var b strings.Builder
	for _, message := range messages {
		b.WriteString(message.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("file %q was not created before timeout", path)
}
