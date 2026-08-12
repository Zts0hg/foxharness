package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestPFCHD001CurrentProfileSnapshotIsFlatAndCeilingsAreNonRelaxable(t *testing.T) {
	provider := &childCaptureProvider{model: "fixture-child-model"}
	manager := NewManager(provider, t.TempDir())
	provider.model = "mutated-after-resolution"

	if DefaultMaxTurns != 200 {
		t.Fatalf("DefaultMaxTurns = %d, want 200", DefaultMaxTurns)
	}
	if manager.maxTurns != DefaultMaxTurns {
		t.Fatalf("child turn ceiling = %d, want 200", manager.maxTurns)
	}
	if manager.executionSnapshot.model != "fixture-child-model" || manager.executionSnapshot.providerProtocol != "claude" {
		t.Fatalf("child execution snapshot = %#v", manager.executionSnapshot)
	}
	if manager.permissions != nil || manager.PermissionEnforced() {
		t.Fatal("default child profile unexpectedly expanded permission authority")
	}
	readOnly := manager.buildRegistry(true, nil)
	writable := manager.buildRegistry(false, nil)
	if got := definitionNames(readOnly.GetAvailableTools()); !reflect.DeepEqual(got, []string{"bash", "read_file"}) {
		t.Fatalf("read-only ceiling = %v", got)
	}
	if got := definitionNames(writable.GetAvailableTools()); !reflect.DeepEqual(got, []string{"bash", "edit_file", "read_file", "write_file"}) {
		t.Fatalf("writable ceiling = %v", got)
	}
	filtered := manager.buildRegistry(false, []string{"read_file", "delegate_task"})
	if got := definitionNames(filtered.GetAvailableTools()); !reflect.DeepEqual(got, []string{"read_file"}) {
		t.Fatalf("caller restriction expanded child ceiling to %v", got)
	}
}

func TestPFCHD002ActiveRequestFreezesCallerToolAllowlist(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	provider := &childDefinitionCaptureProvider{}
	manager := NewManager(provider, workDir)
	createStarted := make(chan struct{})
	createRelease := make(chan struct{})
	manager.createSession = func(options session.CreateOptions) (*session.Session, error) {
		close(createStarted)
		<-createRelease
		return session.NewManagerWithHome(workDir, home).Create(options)
	}
	allowed := []string{"read_file"}
	resultCh := make(chan *Result, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := manager.Run(context.Background(), Request{
			ParentSessionID: "parent",
			Task:            "inspect files",
			ReadOnly:        true,
			AllowedTools:    allowed,
		})
		resultCh <- result
		errCh <- err
	}()
	<-createStarted
	allowed[0] = "write_file"
	close(createRelease)
	result := <-resultCh
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Status != OutcomeSucceeded {
		t.Fatalf("child result = %#v", result)
	}
	if got := provider.toolNames; len(got) != 1 || got[0] != "read_file" {
		t.Fatalf("active child tool snapshot = %v, want frozen [read_file]", got)
	}
}

func TestPFCHD006ConfiguredTurnBudgetCanOnlyNarrowProfileCeiling(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "minimum narrowing", in: 1, want: 1},
		{name: "exact ceiling", in: DefaultMaxTurns, want: DefaultMaxTurns},
		{name: "above ceiling", in: DefaultMaxTurns + 1, want: DefaultMaxTurns},
		{name: "zero", in: 0, want: DefaultMaxTurns},
		{name: "negative", in: -1, want: DefaultMaxTurns},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := NewManager(&finalReportProvider{}, t.TempDir()).WithMaxTurns(test.in)
			if manager.maxTurns != test.want {
				t.Fatalf("WithMaxTurns(%d) resolved %d, want %d", test.in, manager.maxTurns, test.want)
			}
		})
	}
}

func TestPFCHD011DepthGateRejectsNestedRunBeforeCapacityOrSession(t *testing.T) {
	createCalled := false
	manager := NewManager(&finalReportProvider{}, t.TempDir())
	manager.createSession = func(session.CreateOptions) (*session.Session, error) {
		createCalled = true
		return nil, nil
	}
	result, err := manager.Run(context.Background(), Request{
		ParentSessionID: "already-child",
		Task:            "attempt nested child",
		ReadOnly:        true,
		Depth:           2,
	})
	if err == nil {
		t.Fatalf("Run() error = nil, want nested depth rejection; result=%#v", result)
	}
	if createCalled {
		t.Fatal("nested depth rejection created child session capacity")
	}
	if result == nil || result.Status != OutcomeRejected || result.SessionID != "" || result.RunID != "" {
		t.Fatalf("nested depth outcome = %#v", result)
	}
}

func TestPFCHD014PermissionEvidenceCorrelatesCompleteChildLineage(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	reviewer := &recordingPermissionReviewer{}
	coordinator := permission.NewCoordinator(permission.Config{
		State:     permission.NewState(permission.ModeApprove, false),
		Workspace: workDir,
		CWD:       workDir,
		Reviewer:  reviewer,
	})
	manager := NewManager(&childPermissionCallProvider{}, workDir).
		WithPermission(coordinator).
		WithParentEvidence(func(request permission.Request) permission.Evidence {
			return permission.BuildEvidence([]schema.Message{{Role: schema.RoleUser, Content: "trusted parent request"}}, nil, request)
		})
	manager.homeDir = homeDir
	manager.createSession = session.NewManagerWithHome(workDir, homeDir).Create

	result, err := manager.Run(context.Background(), Request{
		ParentSessionID: "parent-session",
		ParentRunID:     "parent-run",
		DelegationID:    "delegate-call",
		Task:            "inspect the workspace",
		ReadOnly:        false,
		Depth:           1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ParentSessionID != "parent-session" || result.ParentRunID != "parent-run" || result.DelegationID != "delegate-call" {
		t.Fatalf("terminal lineage = %#v", result)
	}
	wantCorrelation := permission.EvidenceCorrelation{
		ParentSessionID: "parent-session",
		ParentRunID:     "parent-run",
		ChildSessionID:  result.SessionID,
		ChildRunID:      result.RunID,
		DelegationID:    "delegate-call",
		ToolCallID:      "child-tool-call",
	}
	if reviewer.evidence.Correlation != wantCorrelation {
		t.Fatalf("permission correlation = %#v, want %#v", reviewer.evidence.Correlation, wantCorrelation)
	}
	if reviewer.result.Decision != permission.ReviewApprove || reviewer.result.Risk != permission.RiskLow {
		t.Fatalf("terminal permission result = %#v", reviewer.result)
	}
	stored, openErr := session.NewManagerWithHome(workDir, homeDir).Open(result.SessionID)
	if openErr != nil {
		t.Fatal(openErr)
	}
	if stored.ParentSessionID != "parent-session" || stored.ParentRunID != "parent-run" || stored.DelegationID != "delegate-call" {
		t.Fatalf("persisted child lineage = %#v", stored)
	}
}

func TestPFCHD003FreshSessionsAndRunsRemainIsolatedFromParentAndSiblings(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	sessionManager := session.NewManagerWithHome(workDir, homeDir)
	parent, err := sessionManager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	parentLog := session.NewMessageLog(parent)
	if _, err := parentLog.Append("parent-run", schema.Message{Role: schema.RoleUser, Content: "parent authority"}); err != nil {
		t.Fatal(err)
	}
	parentBefore, err := os.ReadFile(parent.MessagesPath())
	if err != nil {
		t.Fatal(err)
	}

	provider := &sequencedChildProvider{reports: []string{"first report", "second report"}}
	manager := NewManager(provider, workDir)
	results := make([]*Result, 0, 2)
	for _, task := range []string{"first isolated task", "second isolated task"} {
		result, runErr := manager.Run(context.Background(), Request{
			ParentSessionID: parent.ID,
			ParentRunID:     "parent-run",
			DelegationID:    "delegate-" + task,
			Task:            task,
			ReadOnly:        true,
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		results = append(results, result)
	}
	if results[0].SessionID == results[1].SessionID || results[0].RunID == results[1].RunID || results[0].InvocationID == results[1].InvocationID {
		t.Fatalf("child identities leaked across invocations: %#v / %#v", results[0], results[1])
	}
	if results[0].Report != "first report" || results[1].Report != "second report" {
		t.Fatalf("child reports crossed invocations: %q / %q", results[0].Report, results[1].Report)
	}
	for i, result := range results {
		child, openErr := sessionManager.Open(result.SessionID)
		if openErr != nil {
			t.Fatal(openErr)
		}
		if child.Source != session.SOURCESubagent || child.UserID != "subagent-of-"+parent.ID || child.ParentSessionID != parent.ID {
			t.Fatalf("child %d metadata = %#v", i, child)
		}
		messages, loadErr := session.NewMessageLog(child).LoadMessages()
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		text := messageText(messages)
		if !strings.Contains(text, []string{"first isolated task", "second isolated task"}[i]) || strings.Contains(text, []string{"second isolated task", "first isolated task"}[i]) {
			t.Fatalf("child %d conversation is not isolated:\n%s", i, text)
		}
		if child.MemoryPath() == parent.MemoryPath() || child.CompactStatePath() == parent.CompactStatePath() || child.TodoPath() == parent.TodoPath() {
			t.Fatalf("child %d reuses parent state paths", i)
		}
	}
	parentAfter, err := os.ReadFile(parent.MessagesPath())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parentAfter, parentBefore) {
		t.Fatalf("child execution mutated parent conversation:\nbefore=%s\nafter=%s", parentBefore, parentAfter)
	}
}

func TestPFCHD004AcceptedRunWaitsSynchronouslyAndPropagatesCancellation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	provider := &blockingChildProvider{started: make(chan struct{})}
	workDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan *Result, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := NewManager(provider, workDir).Run(ctx, Request{
			ParentSessionID: "parent",
			Task:            "wait for cancellation",
			ReadOnly:        true,
		})
		resultCh <- result
		errCh <- err
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("child provider did not start")
	}
	select {
	case result := <-resultCh:
		t.Fatalf("accepted ChildRun returned before terminal provider state: %#v", result)
	default:
	}
	cancel()
	select {
	case result := <-resultCh:
		err := <-errCh
		if !errors.Is(err, context.Canceled) || result == nil || result.Status != OutcomeCancelled {
			t.Fatalf("cancelled synchronous outcome = %#v / %v", result, err)
		}
	case <-time.After(time.Second):
		t.Fatal("parent cancellation did not terminate synchronous child")
	}
}

func TestPFCHD007ChildUsesOneNonThinkingNonStreamingModelInvocation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	provider := &streamCapableChildProvider{}
	result, err := NewManager(provider, t.TempDir()).Run(context.Background(), Request{
		ParentSessionID: "parent",
		Task:            "finish directly",
		ReadOnly:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != OutcomeSucceeded || provider.generateCalls != 1 || provider.streamCalls != 0 {
		t.Fatalf("child transport = result %#v, generate %d, stream %d", result, provider.generateCalls, provider.streamCalls)
	}
}

func TestPFCHD015To017PromptProjectsOnlyChildContextAndDirectSkillFragment(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("PROJECT-CHILD-INSTRUCTION"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(workDir, ".foxharness", "skills", "audit")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: audit-child\ndescription: inspect carefully\n---\nDIRECT-SKILL-BODY"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := &childCaptureProvider{response: "dense report", model: "test-model"}
	result, err := NewManager(provider, workDir).Run(context.Background(), Request{
		ParentSessionID: "parent-session",
		ParentRunID:     "parent-run",
		DelegationID:    "delegate-call",
		Task:            "use $audit for EXACT-DELEGATED-TASK",
		ReadOnly:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Report != "dense report" {
		t.Fatalf("child report = %q", result.Report)
	}
	messages, definitions := provider.snapshot()
	prompt := messageText(messages)
	for _, want := range []string{
		"PROJECT-CHILD-INSTRUCTION",
		"父 Session: parent-session",
		"EXACT-DELEGATED-TASK",
		"最终只返回高密度报告",
		"## Loaded Skill: audit-child",
		"DIRECT-SKILL-BODY",
		"## Session Working Memory",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("child prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"Available Skills (invoke", "Formal Plan Collaboration Mode", "Asking the User", "delegate_task", "read_todo", "update_todo"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("child prompt contains forbidden parent/capability fragment %q:\n%s", forbidden, prompt)
		}
	}
	if got := definitionNames(definitions); !reflect.DeepEqual(got, []string{"bash", "read_file"}) {
		t.Fatalf("direct skill expanded child tools to %v", got)
	}
}

type childDefinitionCaptureProvider struct {
	toolNames []string
}

type childPermissionCallProvider struct {
	calls int
}

type sequencedChildProvider struct {
	mu      sync.Mutex
	reports []string
	calls   int
}

func (p *sequencedChildProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	report := p.reports[p.calls]
	p.calls++
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: report}}, nil
}

type blockingChildProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *blockingChildProvider) Generate(ctx context.Context, _ []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

type streamCapableChildProvider struct {
	generateCalls int
	streamCalls   int
}

func (p *streamCapableChildProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.generateCalls++
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

func (p *streamCapableChildProvider) GenerateStream(context.Context, []schema.Message, []schema.ToolDefinition, provider.GenerateOptions, provider.StreamCallbacks) (*provider.GenerateResponse, error) {
	p.streamCalls++
	return nil, errors.New("ChildRun must not stream")
}

func (p *childPermissionCallProvider) Generate(_ context.Context, _ []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	if p.calls == 1 {
		return &provider.GenerateResponse{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "child-tool-call",
				Name:      "bash",
				Arguments: json.RawMessage(`{"command":"true"}`),
			}},
		}}, nil
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

func (p *childDefinitionCaptureProvider) Generate(_ context.Context, _ []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.toolNames = p.toolNames[:0]
	for _, definition := range definitions {
		p.toolNames = append(p.toolNames, definition.Name)
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}
