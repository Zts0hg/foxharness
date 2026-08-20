package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestPFAUT001CurrentProfileSnapshotIsFlatAndCeilingsOnlyNarrow(t *testing.T) {
	core, runner := newAutodevProfileRunner(t, t.TempDir(), 0)
	defer closeAutodevProfileCore(t, core)
	runner.SetUserAsker(fakeAutodevAsker{})
	registry := runner.buildRegistry(runner.currentSession, runner.llmProvider, runner.checkpointer, func() string { return "" })

	got := currentAutodevProfileSnapshot{
		name: "AutodevPipeline", source: string(runner.currentSession.Source), workDir: runner.workDir,
		model: runner.model, protocol: runner.providerProtocol, maxTurns: runner.maxTurns,
		serial: true, thinking: runner.enableThinking, effort: runner.effortOverride,
		permission:   runner.permissionCoordinator.State().Snapshot().EffectiveMode,
		questionPort: runner.userAsker != nil, memory: runner.store != nil && runner.autoMemory != nil,
		checkpoint: runner.checkpointer != nil, automaticCompaction: true,
		extraction: "item-owned-drain-close", observation: "typed-core-outcome",
		tools: autodevProfileToolNames(registry.GetAvailableTools()),
	}
	want := currentAutodevProfileSnapshot{
		name: "AutodevPipeline", source: string(session.SOURCECLI), workDir: runner.workDir,
		model: "fixture-model", protocol: "openai", maxTurns: 0,
		serial: true, permission: permission.ModeFullAccess, questionPort: true,
		memory: true, checkpoint: true, automaticCompaction: true,
		extraction: "item-owned-drain-close", observation: "typed-core-outcome",
		tools: "AskUserQuestion,ask_user_question,bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file",
	}
	if got != want {
		t.Fatalf("current Autodev profile = %#v, want %#v", got, want)
	}

	restricted := tools.NewFilteredRegistry(registry, []string{"read_file", "not-in-profile"})
	if names := autodevProfileToolNames(restricted.GetAvailableTools()); names != "read_file" {
		t.Fatalf("restricted tools = %q, want read_file", names)
	}
	if result := restricted.Execute(context.Background(), schema.ToolCall{ID: "expand", Name: "not-in-profile"}); !result.IsError {
		t.Fatalf("restriction expanded Autodev profile: %#v", result)
	}
}

func TestPFAUT003And004ProductionFactoryCreatesFreshIsolatedItemSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workOne := t.TempDir()
	workTwo := t.TempDir()
	factory := &appCoreRunnerFactory{llmConfig: testAutodevResolvedLLM("fixture-model"), maxTurns: 1}
	firstCore, err := factory.New(context.Background(), workOne, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	defer closeAutodevProfileCore(t, firstCore)
	secondCore, err := factory.New(context.Background(), workTwo, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	defer closeAutodevProfileCore(t, secondCore)
	firstRunner := firstCore.(*coreRunnerAdapter).runner.(*AgentRunner)
	secondRunner := secondCore.(*coreRunnerAdapter).runner.(*AgentRunner)
	firstProvider := &autodevProfileProvider{}
	secondProvider := &autodevProfileProvider{}
	firstRunner.llmProvider = firstProvider
	secondRunner.llmProvider = secondProvider
	firstRunner.extractionFire = func(*session.Session, string, *automemory.Tracker) {}
	secondRunner.extractionFire = func(*session.Session, string, *automemory.Tracker) {}

	firstA := firstCore.Run(context.Background(), autodevProfileCoreAttempt("item-one", "generate-spec", 1, "first stage"), nil)
	firstB := firstCore.Run(context.Background(), autodevProfileCoreAttempt("item-one", "spec-to-plan", 2, "second stage"), nil)
	secondA := secondCore.Run(context.Background(), autodevProfileCoreAttempt("item-two", "generate-spec", 1, "other item"), nil)
	if firstA.Status != autodev.CoreOutcomeSucceeded || firstB.Status != autodev.CoreOutcomeSucceeded || secondA.Status != autodev.CoreOutcomeSucceeded {
		t.Fatalf("item outcomes = %#v / %#v / %#v", firstA, firstB, secondA)
	}
	if firstA.SessionID != firstB.SessionID || firstA.RunID == firstB.RunID {
		t.Fatalf("same item session continuity = %#v / %#v", firstA, firstB)
	}
	if firstA.SessionID == secondA.SessionID || firstRunner.currentSession.RootDir == secondRunner.currentSession.RootDir {
		t.Fatalf("item sessions are not isolated = %#v / %#v", firstRunner.currentSession, secondRunner.currentSession)
	}
	if firstRunner.currentSession.Source != session.SOURCECLI || secondRunner.currentSession.Source != session.SOURCECLI || firstRunner.workDir != workOne || secondRunner.workDir != workTwo {
		t.Fatalf("item session metadata = %#v / %#v", firstRunner.currentSession, secondRunner.currentSession)
	}
	if firstRunner.model != "fixture-model" || secondRunner.model != "fixture-model" || firstRunner.providerProtocol != "openai" || secondRunner.providerProtocol != "openai" {
		t.Fatalf("frozen provider/model = %s/%s and %s/%s", firstRunner.providerProtocol, firstRunner.model, secondRunner.providerProtocol, secondRunner.model)
	}
	firstRequests := firstProvider.snapshot()
	if len(firstRequests) != 2 || !autodevProfileConversationContains(firstRequests[1].messages, "first stage") || !autodevProfileConversationContains(firstRequests[1].messages, "second stage") {
		t.Fatalf("continuous item conversation = %#v", firstRequests)
	}
	if secondRequests := secondProvider.snapshot(); len(secondRequests) != 1 || autodevProfileConversationContains(secondRequests[0].messages, "first stage") {
		t.Fatalf("later item inherited another conversation = %#v", secondRequests)
	}
}

func TestPFAUT005And006CoreInvocationIsNonThinkingNonStreamingWithExactTools(t *testing.T) {
	core, runner := newAutodevProfileRunner(t, t.TempDir(), 1)
	defer closeAutodevProfileCore(t, core)
	model := &autodevProfileProvider{}
	runner.llmProvider = model
	runner.SetUserAsker(fakeAutodevAsker{})
	runner.extractionFire = func(*session.Session, string, *automemory.Tracker) {}
	downstream := &autodevProfileDeltaReporter{}
	outcome := core.Run(context.Background(), autodevProfileCoreAttempt("item", "implement-tasks", 1, "implement"), downstream)
	if outcome.Status != autodev.CoreOutcomeSucceeded {
		t.Fatalf("core outcome = %#v", outcome)
	}
	requests := model.snapshot()
	if len(requests) != 1 || requests[0].options.Effort != "" {
		t.Fatalf("model invocation count/options = %#v", requests)
	}
	if downstream.deltaCalls != 0 {
		t.Fatalf("Autodev emitted %d model deltas", downstream.deltaCalls)
	}
	if got, want := autodevProfileToolNames(requests[0].definitions), "AskUserQuestion,ask_user_question,bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file"; got != want {
		t.Fatalf("model-visible tool surface = %q, want %q", got, want)
	}
	registry := runner.buildRegistry(runner.currentSession, model, runner.checkpointer, func() string { return "" })
	if got := autodevProfileToolNames(registry.GetAvailableTools()); got != autodevProfileToolNames(requests[0].definitions) {
		t.Fatalf("advertised/executable tools differ: %q / %q", autodevProfileToolNames(requests[0].definitions), got)
	}
}

func TestPFAUT010And012ContextAndStateStayInsideItemSession(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("AUTODEV_PROJECT_INSTRUCTION\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	core, runner := newAutodevProfileRunner(t, workDir, 1)
	defer closeAutodevProfileCore(t, core)
	if err := os.WriteFile(runner.currentSession.MemoryPath(), []byte("AUTODEV_SESSION_MEMORY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &autodevProfileProvider{}
	runner.llmProvider = model
	runner.SetUserAsker(fakeAutodevAsker{})
	runner.extractionFire = func(*session.Session, string, *automemory.Tracker) {}
	outcome := core.Run(context.Background(), autodevProfileCoreAttempt("item", "generate-spec", 1, "AUTODEV_STAGE_PROMPT"), nil)
	if outcome.Status != autodev.CoreOutcomeSucceeded {
		t.Fatalf("outcome = %#v", outcome)
	}
	requests := model.snapshot()
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	joined := autodevProfileMessages(requests[0].messages)
	for _, fragment := range []string{"AUTODEV_PROJECT_INSTRUCTION", "AUTODEV_SESSION_MEMORY", "AUTODEV_STAGE_PROMPT", "Session Plan and Todo Files"} {
		if strings.Count(joined, fragment) != 1 {
			t.Fatalf("context contains %q %d times:\n%s", fragment, strings.Count(joined, fragment), joined)
		}
	}
	for _, forbidden := range []string{"## Formal Plan", "Collaboration Mode"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("Autodev context contains forbidden fragment %q:\n%s", forbidden, joined)
		}
	}
	if filepath.Dir(runner.store.PlanPath()) != runner.currentSession.RootDir || filepath.Dir(runner.store.TodoPath()) != runner.currentSession.RootDir || runner.checkpointer == nil || runner.currentSession.WorkDir != workDir {
		t.Fatalf("item state scope = session %#v plan %q todo %q", runner.currentSession, runner.store.PlanPath(), runner.store.TodoPath())
	}
	coreType := reflect.TypeOf((*autodev.CoreRunner)(nil)).Elem()
	for _, forbidden := range []string{"Resume", "Compact", "Rewind", "PlanReview", "NewSession"} {
		if _, ok := coreType.MethodByName(forbidden); ok {
			t.Fatalf("Autodev CoreRunner exposes user-session operation %q", forbidden)
		}
	}
}

func TestPFAUT014RuntimeReporterForwardsCanonicalFactsWithoutPresentation(t *testing.T) {
	downstream := &autodevProfileDetailedReporter{}
	recorder, reporter := newCoreOutcomeReporter(downstream)
	ctx := context.Background()
	reporter.OnRunStart(ctx, "session", "run")
	reporter.OnToolCall(ctx, "read_file", `{"path":"a"}`)
	reporter.OnToolResult(ctx, "read_file", "content", false)
	reporter.OnCompaction(ctx, "automatic")
	reporter.OnMessage(ctx, "final")
	reporter.OnRunComplete(ctx, engine.RunResult{SessionID: "session", RunID: "run", FinalMessage: "final"})
	if got := strings.Join(downstream.events, ","); got != "run,tool-call,tool-result,compaction,message,complete" {
		t.Fatalf("canonical event order = %q", got)
	}
	if sessionID, runID, partial := recorder.snapshot(); sessionID != "session" || runID != "run" || partial != "final" {
		t.Fatalf("reporter snapshot = %q/%q/%q", sessionID, runID, partial)
	}
	if _, ok := reporter.(engine.MessageDeltaReporter); ok {
		t.Fatal("Autodev runtime reporter exposed presentation-driven streaming")
	}
}

type currentAutodevProfileSnapshot struct {
	name, source, workDir, model, protocol, effort, extraction, observation, tools string
	maxTurns                                                                       int
	serial, thinking, questionPort, memory, checkpoint, automaticCompaction        bool
	permission                                                                     permission.Mode
}

type autodevProfileRequest struct {
	messages    []schema.Message
	definitions []schema.ToolDefinition
	options     provider.GenerateOptions
}

type autodevProfileProvider struct {
	mu       sync.Mutex
	requests []autodevProfileRequest
}

func (p *autodevProfileProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{})
}

func (p *autodevProfileProvider) GenerateWithOptions(_ context.Context, messages []schema.Message, definitions []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, autodevProfileRequest{
		messages: append([]schema.Message(nil), messages...), definitions: append([]schema.ToolDefinition(nil), definitions...), options: options,
	})
	p.mu.Unlock()
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

func (p *autodevProfileProvider) snapshot() []autodevProfileRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]autodevProfileRequest(nil), p.requests...)
}

type autodevProfileDeltaReporter struct {
	autodevProfileDetailedReporter
	deltaCalls int
}

func (r *autodevProfileDeltaReporter) OnMessageDelta(context.Context, string) { r.deltaCalls++ }

type autodevProfileDetailedReporter struct{ events []string }

func (r *autodevProfileDetailedReporter) OnRunStart(context.Context, string, string) {
	r.events = append(r.events, "run")
}
func (r *autodevProfileDetailedReporter) OnThinking(context.Context, int) {
	r.events = append(r.events, "thinking")
}
func (r *autodevProfileDetailedReporter) OnCompaction(context.Context, string) {
	r.events = append(r.events, "compaction")
}
func (r *autodevProfileDetailedReporter) OnToolCall(context.Context, string, string) {
	r.events = append(r.events, "tool-call")
}
func (r *autodevProfileDetailedReporter) OnToolResult(context.Context, string, string, bool) {
	r.events = append(r.events, "tool-result")
}
func (r *autodevProfileDetailedReporter) OnMessage(context.Context, string) {
	r.events = append(r.events, "message")
}
func (r *autodevProfileDetailedReporter) OnRunComplete(context.Context, autodev.CoreRunResult) {
	r.events = append(r.events, "complete")
}
func (r *autodevProfileDetailedReporter) OnRunError(context.Context, string, string, error) {
	r.events = append(r.events, "error")
}
func (*autodevProfileDetailedReporter) OnToolCallDetail(context.Context, schema.ToolCall) {}
func (*autodevProfileDetailedReporter) OnToolResultDetail(context.Context, schema.ToolCall, schema.ToolResult) {
}

func newAutodevProfileRunner(t *testing.T, workDir string, maxTurns int) (autodev.CoreRunner, *AgentRunner) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	factory := &appCoreRunnerFactory{llmConfig: testAutodevResolvedLLM("fixture-model"), maxTurns: maxTurns}
	core, err := factory.New(context.Background(), workDir, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	adapter := core.(*coreRunnerAdapter)
	return core, adapter.runner.(*AgentRunner)
}

func closeAutodevProfileCore(t *testing.T, core autodev.CoreRunner) {
	t.Helper()
	if err := core.Close(context.Background()); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func autodevProfileCoreAttempt(item, stage string, ordinal int, prompt string) autodev.CoreAttempt {
	id := "core:" + item + ":" + stage
	return autodev.CoreAttempt{AttemptID: id, CorrelationID: id, Ordinal: ordinal, Prompt: prompt}
}

func autodevProfileToolNames(definitions []schema.ToolDefinition) string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func autodevProfileConversationContains(messages []schema.Message, content string) bool {
	return strings.Contains(autodevProfileMessages(messages), content)
}

func autodevProfileMessages(messages []schema.Message) string {
	var contents []string
	for _, message := range messages {
		contents = append(contents, message.Content)
	}
	return strings.Join(contents, "\n")
}
