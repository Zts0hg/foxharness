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
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
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

func TestDVCHD001ReadOnlyChildBashRejectsMutationWithoutPermissionExpansion(t *testing.T) {
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
	if !result.IsError {
		t.Fatalf("mutating read-only bash result = %#v, want rejection", result)
	}
	for _, path := range []string{inside, outside} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read-only bash mutated %q: %v", path, err)
		}
	}

	coordinator := permission.NewCoordinator(permission.Config{
		State: permission.NewState(permission.ModeFullAccess, true),
	})
	fullAccessRegistry := NewManager(nil, workDir).WithPermission(coordinator).buildRegistry(true, nil)
	result = fullAccessRegistry.Execute(context.Background(), schema.ToolCall{
		ID:        "full-access-mutating-read-only-bash",
		Name:      "bash",
		Arguments: mustJSON(t, map[string]string{"command": command}),
	})
	if !result.IsError {
		t.Fatalf("full-access mutating read-only bash result = %#v, want immutable ceiling", result)
	}
	for _, path := range []string{inside, outside} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("full access expanded read-only bash and mutated %q: %v", path, err)
		}
	}

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

func TestDVCHD002ChildModelInvocationAndCompactorShareFrozenSnapshot(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := t.TempDir()
	const selectedModel = "claude-3.5-sonnet"
	provider := &childCaptureProvider{response: "done", model: selectedModel}
	mgr := NewManager(provider, workDir)
	provider.model = "glm-4.7"

	result, err := mgr.Run(context.Background(), Request{
		ParentSessionID: "parent",
		Task:            "inspect",
		ReadOnly:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := session.NewManagerWithHome(workDir, homeDir).Open(session.ID(result.SessionID))
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
	selectedConfig.Model = selectedModel
	selectedCompactor, err := compaction.NewCompactor(provider, selectedConfig)
	if err != nil {
		t.Fatal(err)
	}
	if selectedCompactor.ContextWindow() != 200000 {
		t.Fatalf("selected-model compactor window = %d, want 200000", selectedCompactor.ContextWindow())
	}
	if childCompactor.ContextWindow() != selectedCompactor.ContextWindow() {
		t.Fatalf("child/selected compactor windows = %d/%d, want one frozen model snapshot", childCompactor.ContextWindow(), selectedCompactor.ContextWindow())
	}
	if childCompactor.Threshold() != selectedCompactor.Threshold() {
		t.Fatalf("child/selected compaction thresholds = %d/%d, want one frozen model snapshot", childCompactor.Threshold(), selectedCompactor.Threshold())
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
	if used >= childCompactor.Threshold() {
		t.Fatalf("fixture tokens = %d, want below frozen selected-model trigger %d", used, childCompactor.Threshold())
	}
	selectedProjection, err := selectedCompactor.MaybeCompact(context.Background(), history)
	if err != nil {
		t.Fatal(err)
	}
	if len(selectedProjection) != len(history) {
		t.Fatalf("selected-model projection compacted %d messages to %d below its trigger", len(history), len(selectedProjection))
	}
	childProjection, err := childCompactor.MaybeCompact(context.Background(), history)
	if err != nil {
		t.Fatal(err)
	}
	if len(childProjection) != len(history) {
		t.Fatalf("child projection compacted %d messages to %d below its frozen selected-model trigger", len(history), len(childProjection))
	}
}

func TestDVCHD002UnknownChildModelUsesOneExplicitFallback(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := t.TempDir()
	provider := &childCaptureProvider{response: "done", model: "custom-unknown-model"}
	mgr := NewManager(provider, workDir)

	result, err := mgr.Run(context.Background(), Request{
		ParentSessionID: "parent",
		Task:            "inspect",
		ReadOnly:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := session.NewManagerWithHome(workDir, homeDir).Open(session.ID(result.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	childCompactor, err := mgr.newCompactor(sess)
	if err != nil {
		t.Fatal(err)
	}
	if childCompactor.ContextWindow() != compaction.DefaultContextWindow {
		t.Fatalf("unknown-model context window = %d, want explicit fallback %d", childCompactor.ContextWindow(), compaction.DefaultContextWindow)
	}
	tracePaths, err := filepath.Glob(filepath.Join(sess.RunsDir(), "*", "trace.jsonl"))
	if err != nil || len(tracePaths) != 1 {
		t.Fatalf("child run trace paths = %v, error = %v", tracePaths, err)
	}
	trace, err := os.ReadFile(tracePaths[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(trace), `"model":"custom-unknown-model"`) {
		t.Fatalf("unknown child model is not observable in trace: %s", trace)
	}
}

func TestDVCHD005DefaultAgentIdentityPropagatesThroughOutcomeLineageAndTelemetry(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := t.TempDir()
	provider := &childCaptureProvider{response: "done", model: "test-model"}

	result, err := NewManager(provider, workDir).Run(context.Background(), Request{
		ParentSessionID: "parent-session",
		Task:            "inspect",
		ReadOnly:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Agent != AgentGeneralPurpose || result.ParentSessionID != "parent-session" {
		t.Fatalf("child outcome identity = agent %q parent %q", result.Agent, result.ParentSessionID)
	}

	sess, err := session.NewManagerWithHome(workDir, homeDir).Open(session.ID(result.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	if sess.Agent != string(AgentGeneralPurpose) || sess.ParentSessionID != "parent-session" {
		t.Fatalf("child lineage = agent %q parent %q", sess.Agent, sess.ParentSessionID)
	}
	tracePaths, err := filepath.Glob(filepath.Join(sess.RunsDir(), "*", "trace.jsonl"))
	if err != nil || len(tracePaths) != 1 {
		t.Fatalf("child run trace paths = %v, error = %v", tracePaths, err)
	}
	trace, err := os.ReadFile(tracePaths[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"agent":"general-purpose"`, `"parent_session_id":"parent-session"`} {
		if !strings.Contains(string(trace), want) {
			t.Fatalf("child trace missing %s: %s", want, trace)
		}
	}
	messages, _ := provider.snapshot()
	if !strings.Contains(messageText(messages), "Agent: general-purpose") {
		t.Fatalf("default agent identity missing from prompt:\n%s", messageText(messages))
	}
}

func TestDVCHD005UnknownAgentDoesNotCreateChildSession(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := t.TempDir()
	mgr := NewManager(&childCaptureProvider{response: "unexpected", model: "test-model"}, workDir)

	result, err := mgr.Run(context.Background(), Request{
		ParentSessionID: "parent-session",
		Task:            "inspect",
		ReadOnly:        true,
		Agent:           AgentID("unknown-agent"),
	})
	if err == nil || result == nil || result.Status != OutcomeRejected || result.SessionID != "" || result.RunID != "" {
		t.Fatalf("unknown agent result/error = %#v/%v, want correlated rejection without child identity", result, err)
	}
	if !strings.Contains(err.Error(), `unknown agent "unknown-agent"`) {
		t.Fatalf("unknown agent error = %q, want explicit identity", err)
	}
	if _, err := session.NewManagerWithHome(workDir, homeDir).Latest(session.LookupOptions{}); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("unknown agent session lookup error = %v, want ErrNotFound", err)
	}
}

func TestDVCHD003PromptAndEffectiveToolSnapshotsAgree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tests := []struct {
		name          string
		readOnly      bool
		allowedTools  []string
		wantTools     []string
		promptPresent []string
		promptAbsent  []string
	}{
		{
			name:          "read only",
			readOnly:      true,
			wantTools:     []string{"bash", "read_file"},
			promptPresent: []string{"Use bash", "read_file"},
			promptAbsent:  []string{"Use edit_file", "Use write_file", "read_todo", "update_todo", "ask_user_question", "delegate_task", "`skill` tool"},
		},
		{
			name:          "writable",
			wantTools:     []string{"bash", "edit_file", "read_file", "write_file"},
			promptPresent: []string{"Use bash", "Use edit_file", "Use write_file", "read_file"},
			promptAbsent:  []string{"read_todo", "update_todo", "ask_user_question", "delegate_task", "`skill` tool"},
		},
		{
			name:          "caller read file ceiling",
			allowedTools:  []string{"read_file"},
			wantTools:     []string{"read_file"},
			promptPresent: []string{"read_file"},
			promptAbsent:  []string{"Use bash", "Use edit_file", "Use write_file", "read_todo", "update_todo", "ask_user_question", "delegate_task", "`skill` tool"},
		},
		{
			name:         "explicit empty ceiling",
			allowedTools: []string{},
			wantTools:    []string{},
			promptAbsent: []string{"Use bash", "Use edit_file", "Use write_file", "read_file", "read_todo", "update_todo", "ask_user_question", "delegate_task", "`skill` tool"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workDir := t.TempDir()
			provider := &childCaptureProvider{response: "done", model: "test-model"}
			mgr := NewManager(provider, workDir)
			_, err := mgr.Run(context.Background(), Request{
				ParentSessionID: "parent",
				Task:            "inspect only",
				ReadOnly:        tt.readOnly,
				AllowedTools:    tt.allowedTools,
			})
			if err != nil {
				t.Fatal(err)
			}
			messages, definitions := provider.snapshot()
			if got := definitionNames(definitions); !reflect.DeepEqual(got, tt.wantTools) {
				t.Fatalf("model-visible tools = %v, want %v", got, tt.wantTools)
			}
			prompt := messageText(messages)
			for _, want := range tt.promptPresent {
				if !strings.Contains(prompt, want) {
					t.Fatalf("prompt missing available capability %q:\n%s", want, prompt)
				}
			}
			for _, unavailable := range tt.promptAbsent {
				if strings.Contains(prompt, unavailable) {
					t.Fatalf("prompt claims unavailable capability %q:\n%s", unavailable, prompt)
				}
			}

			registry := mgr.buildRegistry(tt.readOnly, tt.allowedTools)
			if got := definitionNames(registry.GetAvailableTools()); !reflect.DeepEqual(got, tt.wantTools) {
				t.Fatalf("executable tools = %v, want %v", got, tt.wantTools)
			}
			for _, unavailable := range []string{"delegate_task", "read_todo", "update_todo", "skill", "ask_user_question"} {
				call := schema.ToolCall{ID: unavailable, Name: unavailable, Arguments: json.RawMessage(`{}`)}
				if result := registry.Execute(context.Background(), call); !result.IsError {
					t.Fatalf("unavailable call %q executed: %#v", unavailable, result)
				}
				if registry.IsParallelSafe(unavailable) {
					t.Fatalf("unavailable call %q is parallel-safe", unavailable)
				}
			}
		})
	}
}

func TestDVCHD004ChildCancellationKillsShellDescendantsAndPendingApproval(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the current ChildRun bash profile requires a Unix shell")
	}
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	mgr := NewManager(nil, workDir)
	supervisor := tools.NewBashProcessSupervisor()
	registry := mgr.buildRegistryWithSupervisor(false, nil, supervisor)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := supervisor.Cleanup(cleanupCtx); err != nil {
			t.Errorf("supervisor cleanup error = %v", err)
		}
	})
	started := filepath.Join(workDir, "started")
	leaked := filepath.Join(workDir, "leaked")
	command := fmt.Sprintf("touch %q; (sleep 0.25; touch %q) & wait", started, leaked)
	arguments := mustJSON(t, map[string]string{"command": command})
	shellResult := registry.Execute(context.Background(), schema.ToolCall{
		ID:        "reject-child-background",
		Name:      "bash",
		Arguments: arguments,
	})
	if !shellResult.IsError || !strings.Contains(shellResult.Output, "rejected background") {
		t.Fatalf("supervised ChildRun Bash result = %#v, want background rejection", shellResult)
	}
	for _, path := range []string{started, leaked} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("rejected ChildRun Bash produced side effect %q: %v", path, err)
		}
	}

	approver := &cancellingApprover{started: make(chan struct{})}
	state := permission.NewState(permission.ModeAsk, false)
	coordinator := permission.NewCoordinator(permission.Config{State: state, Approver: approver})
	permissionRegistry := NewManager(nil, workDir).WithPermission(coordinator).buildRegistry(false, nil)
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
		Run(context.Background(), Request{ParentSessionID: "parent", Task: "start background work", ReadOnly: false})
	if err == nil || result == nil || result.Status != OutcomeTurnExhausted {
		t.Fatalf("turn-exhausted child result/error = %#v/%v, want correlated exhaustion", result, err)
	}
	if !strings.Contains(err.Error(), "超过最大 Turn 数限制: 1") {
		t.Fatalf("turn-exhausted child error = %q, want turn limit", err)
	}
	time.Sleep(350 * time.Millisecond)
	if _, err := os.Stat(exhaustionLeak); !os.IsNotExist(err) {
		t.Fatalf("turn-exhausted child left a delayed side effect: %v", err)
	}
}

func TestDVCHD004CleanupFailureOverridesSuccessWithCorrelatedOutcome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cleanupErr := errors.New("process cleanup failed")
	supervisor := &failingChildSupervisor{cleanupErr: cleanupErr}
	mgr := NewManager(&finalReportProvider{}, t.TempDir())
	mgr.supervisorFactory = func() childRunSupervisor { return supervisor }

	result, err := mgr.Run(context.Background(), Request{ParentSessionID: "parent", Task: "finish", ReadOnly: false})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("cleanup error = %v, want preserved cause", err)
	}
	if result == nil || result.Status != OutcomeFailed || result.SessionID == "" || result.RunID == "" || result.Report != "subagent report" {
		t.Fatalf("cleanup failure outcome = %#v, want correlated failed outcome", result)
	}
	if supervisor.cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want exactly one", supervisor.cleanupCalls)
	}
}

func TestDVCHD004PanicCleansOnceAndRetainsEstablishedIdentity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	supervisor := &failingChildSupervisor{}
	mgr := NewManager(&panicChildProvider{}, t.TempDir())
	mgr.supervisorFactory = func() childRunSupervisor { return supervisor }

	result, err := mgr.Run(context.Background(), Request{ParentSessionID: "parent", Task: "panic", ReadOnly: false})
	if err == nil || !strings.Contains(err.Error(), "provider panic") {
		t.Fatalf("panic error = %v", err)
	}
	if result == nil || result.Status != OutcomeFailed || result.SessionID == "" || result.RunID == "" {
		t.Fatalf("panic outcome = %#v, want failed session/run correlation", result)
	}
	if supervisor.cleanupCalls != 1 {
		t.Fatalf("panic cleanup calls = %d, want exactly one", supervisor.cleanupCalls)
	}
}

type panicChildProvider struct{}

func (*panicChildProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	panic("provider panic")
}

type failingChildSupervisor struct {
	cleanupErr   error
	cleanupCalls int
}

func (*failingChildSupervisor) Run(context.Context, string, string, time.Duration) tools.BashCommandResult {
	return tools.BashCommandResult{}
}

func (s *failingChildSupervisor) Cleanup(context.Context) error {
	s.cleanupCalls++
	return s.cleanupErr
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

func TestDVCHD006EveryInvocationReturnsOneTypedCorrelatedOutcome(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		result, err := runChildOutcome(t, &finalReportProvider{}, 2, context.Background())
		if err != nil {
			t.Fatal(err)
		}
		assertChildOutcome(t, result, OutcomeSucceeded, true, true, "subagent report")
	})

	t.Run("provider failure", func(t *testing.T) {
		terminalErr := errors.New("provider failed")
		result, err := runChildOutcome(t, &errorChildProvider{err: terminalErr}, 2, context.Background())
		if !errors.Is(err, terminalErr) {
			t.Fatalf("provider error = %v, want preserved cause", err)
		}
		assertChildOutcome(t, result, OutcomeFailed, true, true, "")
	})

	t.Run("tool then provider preserves committed assistant text", func(t *testing.T) {
		result, err := runChildOutcome(t, &toolThenErrorChildProvider{}, 3, context.Background())
		if err == nil || !strings.Contains(err.Error(), "provider failed after tool result") {
			t.Fatalf("tool/provider error = %v", err)
		}
		assertChildOutcome(t, result, OutcomeFailed, true, true, "partial before failed tool")
	})

	t.Run("persistence excludes uncommitted assistant text", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		workDir := t.TempDir()
		provider := &messageLogBreakingProvider{homeDir: homeDir}
		result, err := NewManager(provider, workDir).Run(context.Background(), Request{ParentSessionID: "parent", Task: "persist", ReadOnly: true})
		if err == nil {
			t.Fatal("persistence error = nil")
		}
		assertChildOutcome(t, result, OutcomeFailed, true, true, "")
	})

	t.Run("compaction construction is correlated start failure", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		workDir := t.TempDir()
		mgr := NewManager(&finalReportProvider{}, workDir)
		mgr.compactorFactory = func(*session.Session) (*compaction.Compactor, error) {
			return nil, errors.New("compactor failed")
		}
		result, err := mgr.Run(context.Background(), Request{ParentSessionID: "parent", Task: "compact", ReadOnly: true})
		if err == nil {
			t.Fatal("compaction construction error = nil")
		}
		assertChildOutcome(t, result, OutcomeStartFailed, true, false, "")
	})

	t.Run("turn exhaustion preserves latest committed assistant text", func(t *testing.T) {
		result, err := runChildOutcome(t, &partialLoopChildProvider{}, 1, context.Background())
		if err == nil || !strings.Contains(err.Error(), "超过最大 Turn 数限制: 1") {
			t.Fatalf("turn exhaustion error = %v", err)
		}
		var turnLimit *engine.TurnLimitError
		if !errors.As(err, &turnLimit) || turnLimit.MaxTurns != 1 {
			t.Fatalf("turn exhaustion type = %T/%v, want TurnLimitError(1)", err, err)
		}
		assertChildOutcome(t, result, OutcomeTurnExhausted, true, true, "partial child report")
	})

	t.Run("cancellation preserves classification", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := runChildOutcome(t, &errorChildProvider{err: context.Canceled}, 2, ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v, want context.Canceled", err)
		}
		assertChildOutcome(t, result, OutcomeCancelled, true, true, "")
	})

	t.Run("unknown agent is correlated rejection", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		result, err := NewManager(&finalReportProvider{}, t.TempDir()).Run(context.Background(), Request{
			ParentSessionID: "parent",
			Task:            "reject",
			Agent:           "unknown-agent",
		})
		if err == nil {
			t.Fatal("unknown-agent error = nil")
		}
		assertChildOutcome(t, result, OutcomeRejected, false, false, "")
		if result.Agent != "unknown-agent" {
			t.Fatalf("rejected agent = %q, want attempted identity", result.Agent)
		}
	})

	t.Run("session creation is correlated start failure", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		mgr := NewManager(&finalReportProvider{}, t.TempDir())
		mgr.createSession = func(session.CreateOptions) (*session.Session, error) {
			return nil, errors.New("session start failed")
		}
		result, err := mgr.Run(context.Background(), Request{ParentSessionID: "parent", Task: "start"})
		if err == nil || !strings.Contains(err.Error(), "session start failed") {
			t.Fatalf("session start error = %v", err)
		}
		assertChildOutcome(t, result, OutcomeStartFailed, false, false, "")
	})
}

func runChildOutcome(t *testing.T, p provider.LLMProvider, maxTurns int, ctx context.Context) (*Result, error) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return NewManager(p, t.TempDir()).WithMaxTurns(maxTurns).Run(ctx, Request{
		ParentSessionID: "parent",
		Task:            "exercise outcome",
		ReadOnly:        true,
	})
}

func assertChildOutcome(t *testing.T, result *Result, status OutcomeStatus, wantSession, wantRun bool, report string) {
	t.Helper()
	if result == nil {
		t.Fatal("child outcome = nil")
	}
	if result.InvocationID == "" || result.ParentSessionID != "parent" || result.Status != status {
		t.Fatalf("child correlation/status = %#v, want parent and %q", result, status)
	}
	if (result.SessionID != "") != wantSession || (result.RunID != "") != wantRun {
		t.Fatalf("child session/run identity = %q/%q, want presence %t/%t", result.SessionID, result.RunID, wantSession, wantRun)
	}
	if result.Report != report {
		t.Fatalf("child report = %q, want %q", result.Report, report)
	}
}

func TestDVCHD006DelegateAdapterRetainsPartialOutcomeAndTerminalError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tool := NewTool(childManagerWithTestPermission(NewManager(&toolThenErrorChildProvider{}, t.TempDir()).WithMaxTurns(3)), "parent")
	args := json.RawMessage(`{"task":"inspect","read_only":true}`)

	output, err := tool.Execute(context.Background(), args)
	var outcomeErr *OutcomeError
	if !errors.As(err, &outcomeErr) || outcomeErr.Outcome == nil {
		t.Fatalf("delegate error = %T/%v, want typed outcome error", err, err)
	}
	if outcomeErr.Outcome.Status != OutcomeFailed || outcomeErr.Outcome.Report != "partial before failed tool" {
		t.Fatalf("delegate typed outcome = %#v", outcomeErr.Outcome)
	}
	for _, want := range []string{"Status: failed", "Partial Report:", "partial before failed tool", "provider failed after tool result"} {
		if !strings.Contains(output, want) {
			t.Fatalf("delegate failure output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "\nReport:\n") {
		t.Fatalf("delegate failure presents partial content as success:\n%s", output)
	}

	registry := tools.NewRegistry()
	registry.Register(NewTool(childManagerWithTestPermission(NewManager(&toolThenErrorChildProvider{}, t.TempDir()).WithMaxTurns(3)), "parent"))
	result := registry.Execute(context.Background(), schema.ToolCall{ID: "delegate", Name: "delegate_task", Arguments: args})
	if !result.IsError || !strings.Contains(result.Output, "Partial Report:") || !strings.Contains(result.Output, "provider failed after tool result") {
		t.Fatalf("registry delegate outcome = %#v, want partial report and failure", result)
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
