package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/testsupport/runtimecontract"
	"github.com/Zts0hg/foxharness/internal/toolresult"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestCurrentProductionContractAdapterRunsSuccessfulToolLifecycle(t *testing.T) {
	input := contractInput(3)
	definition := runtimecontract.ToolDefinition{
		Name:         "inspect",
		Description:  "inspect a fixture",
		InputSchema:  `{"properties":{"path":{"type":"string"}},"required":["path"],"type":"object"}`,
		ParallelSafe: true,
	}
	call := runtimecontract.ToolCall{ID: "call-inspect", Name: "inspect", Arguments: `{"path":"fixture.txt"}`}
	first := runtimecontract.ModelResponse{ToolCalls: []runtimecontract.ToolCall{call}, FinishReason: "tool_calls"}
	second := runtimecontract.ModelResponse{Content: "inspection complete", FinishReason: "stop"}
	initial := contractInitialMessages(input.Prompt)
	followUp := appendContractMessages(initial,
		runtimecontract.Message{Role: "assistant", ToolCalls: []runtimecontract.ToolCall{call}},
		runtimecontract.Message{Role: "user", Content: "fixture contents", ToolCallID: call.ID},
	)
	scenario := runtimecontract.Scenario{
		ID: "TL-001", Input: input,
		Script: runtimecontract.Script{
			ModelSteps: []runtimecontract.ModelStep{{Response: first}, {Response: second}},
			Tools: []runtimecontract.ToolBehavior{{
				Call: call, Definition: definition,
				Result: runtimecontract.ToolResult{Output: "fixture contents", ModelContent: "fixture contents"},
			}},
		},
		Expected: runtimecontract.Expected{Observed: runtimecontract.Observed{
			Requests: []runtimecontract.ModelRequest{
				contractRequest(input, "action", initial, []runtimecontract.ToolDefinition{definition}),
				contractRequest(input, "action", followUp, []runtimecontract.ToolDefinition{definition}),
			},
			Facts: []runtimecontract.Fact{
				{Kind: "run_started", Sequence: 1},
				{Kind: "tool_call", Sequence: 2, CallID: call.ID, Name: call.Name, Content: call.Arguments},
				{Kind: "tool_result", Sequence: 3, CallID: call.ID, Name: call.Name, Content: "fixture contents"},
				{Kind: "message", Sequence: 4, Content: second.Content},
				{Kind: "run_completed", Sequence: 5, Content: second.Content},
			},
			Outcome: runtimecontract.Outcome{FinalMessage: second.Content, FinishReason: "stop", TurnCount: 2},
			Persisted: []runtimecontract.PersistedRecord{
				contractMessageRecord(1, "user:Return the scripted answer."),
				contractMessageRecord(2, `assistant:|tool_call:call-inspect:inspect:{"path":"fixture.txt"}`),
				contractMessageRecord(3, "tool_result:call-inspect:fixture contents"),
				contractMessageRecord(4, "assistant:inspection complete"),
			},
			Metrics: []runtimecontract.Metric{
				{Kind: "model_call", Turn: 1, Phase: "action"},
				{Kind: "tool_call", Turn: 1, ToolName: call.Name, CallID: call.ID},
				{Kind: "model_call", Turn: 2, Phase: "action"},
				{Kind: "run_summary", ModelCalls: 2, ToolCalls: 1},
			},
		}},
	}

	if err := runtimecontract.VerifyScenario(context.Background(), newCurrentProductionContractAdapter(t), scenario); err != nil {
		t.Fatalf("VerifyScenario() error = %v", err)
	}
}

func TestCurrentProductionToolFailuresRemainCorrelatedAndContinue(t *testing.T) {
	testCases := []struct {
		name       string
		call       runtimecontract.ToolCall
		behavior   *runtimecontract.ToolBehavior
		wantOutput string
	}{
		{
			name:       "unknown tool",
			call:       runtimecontract.ToolCall{ID: "call-unknown", Name: "missing", Arguments: `{}`},
			wantOutput: "Error: tool 'missing' does not exist in the system",
		},
		{
			name: "invalid arguments",
			call: runtimecontract.ToolCall{ID: "call-invalid", Name: "validate", Arguments: `{"actual":true}`},
			behavior: &runtimecontract.ToolBehavior{
				Call:       runtimecontract.ToolCall{Name: "validate", Arguments: `{"expected":true}`},
				Definition: contractToolDefinition("validate"),
				Result:     runtimecontract.ToolResult{Output: "must not execute"},
			},
			wantOutput: `Error executing validate: invalid arguments: got {"actual":true}, want {"expected":true}`,
		},
		{
			name: "business failure",
			call: runtimecontract.ToolCall{ID: "call-business", Name: "business", Arguments: `{}`},
			behavior: &runtimecontract.ToolBehavior{
				Call:       runtimecontract.ToolCall{Name: "business", Arguments: `{}`},
				Definition: contractToolDefinition("business"),
				Result:     runtimecontract.ToolResult{Output: "business rule rejected request", IsError: true},
			},
			wantOutput: "business rule rejected request",
		},
		{
			name: "infrastructure failure",
			call: runtimecontract.ToolCall{ID: "call-infrastructure", Name: "backend", Arguments: `{}`},
			behavior: &runtimecontract.ToolBehavior{
				Call:       runtimecontract.ToolCall{Name: "backend", Arguments: `{}`},
				Definition: contractToolDefinition("backend"),
				Result:     runtimecontract.ToolResult{ErrorKind: "backend unavailable"},
			},
			wantOutput: "Error executing backend: backend unavailable",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			input := contractInput(3)
			script := runtimecontract.Script{ModelSteps: []runtimecontract.ModelStep{
				{Response: runtimecontract.ModelResponse{ToolCalls: []runtimecontract.ToolCall{testCase.call}, FinishReason: "tool_calls"}},
				{Response: runtimecontract.ModelResponse{Content: "recovered", FinishReason: "stop"}},
			}}
			if testCase.behavior != nil {
				script.Tools = []runtimecontract.ToolBehavior{*testCase.behavior}
			}

			observed, err := newCurrentProductionContractAdapter(t).Run(context.Background(), input, script)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if observed.Outcome.FinalMessage != "recovered" || observed.Outcome.TurnCount != 2 {
				t.Fatalf("outcome = %#v, want continued two-turn completion", observed.Outcome)
			}
			if len(observed.Facts) != 5 {
				t.Fatalf("facts = %#v, want five lifecycle facts", observed.Facts)
			}
			callFact, resultFact := observed.Facts[1], observed.Facts[2]
			if callFact.Kind != "tool_call" || resultFact.Kind != "tool_result" || callFact.CallID != testCase.call.ID || resultFact.CallID != testCase.call.ID {
				t.Fatalf("correlated facts = %#v, %#v", callFact, resultFact)
			}
			if !resultFact.IsError || resultFact.Content != testCase.wantOutput {
				t.Fatalf("tool result fact = %#v, want structured failure %q", resultFact, testCase.wantOutput)
			}
			if len(observed.Requests) != 2 {
				t.Fatalf("requests = %#v, want two", observed.Requests)
			}
			messages := observed.Requests[1].Messages
			if len(messages) < 5 || messages[3].ToolCallID != testCase.call.ID || messages[3].Content != testCase.wantOutput {
				t.Fatalf("follow-up messages = %#v, want correlated failure before recovery", messages)
			}
			if !strings.HasPrefix(messages[4].Content, "[Runtime System Notice]\n\n## Error Recovery Notice") {
				t.Fatalf("follow-up messages = %#v, want recovery notice after tool result", messages)
			}
			wantPersisted := "tool_result:" + testCase.call.ID + ":" + testCase.wantOutput
			if len(observed.Persisted) < 3 || observed.Persisted[2].Content != wantPersisted {
				t.Fatalf("persisted = %#v, want %q", observed.Persisted, wantPersisted)
			}
			if len(observed.Metrics) != 4 || !observed.Metrics[1].IsError || observed.Metrics[1].CallID != testCase.call.ID {
				t.Fatalf("metrics = %#v, want correlated failed tool metric", observed.Metrics)
			}
		})
	}
}

func TestParallelSafeToolsOverlapAndCommitInModelOrder(t *testing.T) {
	firstRelease := make(chan struct{})
	firstStarted := make(chan string)
	registry := tools.NewRegistry()
	registry.Register(&barrierContractTool{name: "parallel_a", output: "A", started: firstStarted, release: firstRelease, parallel: true})
	registry.Register(&barrierContractTool{name: "parallel_b", output: "B", started: firstStarted, release: firstRelease, parallel: true})
	calls := []schema.ToolCall{
		{ID: "call-a", Name: "parallel_a", Arguments: json.RawMessage(`{}`)},
		{ID: "call-b", Name: "parallel_b", Arguments: json.RawMessage(`{}`)},
	}
	provider := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: calls}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	reporter := &currentContractReporter{}
	engine, sess := newToolContractEngine(t, provider, registry, Config{MaxTurns: 3})
	runDone := runToolContractAsync(engine, sess, reporter)

	started := map[string]bool{awaitToolSignal(t, firstStarted): true, awaitToolSignal(t, firstStarted): true}
	if !started["parallel_a"] || !started["parallel_b"] {
		t.Fatalf("parallel starts = %#v, want both calls before release", started)
	}
	close(firstRelease)
	if err := awaitToolRun(t, runDone); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}

	assertCommittedToolOrder(t, provider.seen[1], []string{"call-a", "call-b"})
	assertToolFactOrder(t, reporter.facts, []string{"call-a", "call-b"})
}

func TestExclusiveToolsSeparateParallelBatches(t *testing.T) {
	firstRelease := make(chan struct{})
	exclusiveRelease := make(chan struct{})
	secondRelease := make(chan struct{})
	started := make(chan string)
	registry := tools.NewRegistry()
	for _, tool := range []*barrierContractTool{
		{name: "parallel_a", output: "A", started: started, release: firstRelease, parallel: true},
		{name: "parallel_b", output: "B", started: started, release: firstRelease, parallel: true},
		{name: "exclusive", output: "X", started: started, release: exclusiveRelease},
		{name: "parallel_c", output: "C", started: started, release: secondRelease, parallel: true},
		{name: "parallel_d", output: "D", started: started, release: secondRelease, parallel: true},
	} {
		registry.Register(tool)
	}
	calls := []schema.ToolCall{
		{ID: "call-a", Name: "parallel_a", Arguments: json.RawMessage(`{}`)},
		{ID: "call-b", Name: "parallel_b", Arguments: json.RawMessage(`{}`)},
		{ID: "call-x", Name: "exclusive", Arguments: json.RawMessage(`{}`)},
		{ID: "call-c", Name: "parallel_c", Arguments: json.RawMessage(`{}`)},
		{ID: "call-d", Name: "parallel_d", Arguments: json.RawMessage(`{}`)},
	}
	provider := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: calls}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	engine, sess := newToolContractEngine(t, provider, registry, Config{MaxTurns: 3})
	runDone := runToolContractAsync(engine, sess, nil)

	first := map[string]bool{awaitToolSignal(t, started): true, awaitToolSignal(t, started): true}
	if !first["parallel_a"] || !first["parallel_b"] {
		t.Fatalf("first batch = %#v, want parallel_a and parallel_b", first)
	}
	assertNoToolSignal(t, started, "exclusive must wait for first batch")
	close(firstRelease)
	if got := awaitToolSignal(t, started); got != "exclusive" {
		t.Fatalf("next start = %q, want exclusive", got)
	}
	assertNoToolSignal(t, started, "second batch must wait for exclusive")
	close(exclusiveRelease)
	second := map[string]bool{awaitToolSignal(t, started): true, awaitToolSignal(t, started): true}
	if !second["parallel_c"] || !second["parallel_d"] {
		t.Fatalf("second batch = %#v, want parallel_c and parallel_d", second)
	}
	close(secondRelease)
	if err := awaitToolRun(t, runDone); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	assertCommittedToolOrder(t, provider.seen[1], []string{"call-a", "call-b", "call-x", "call-c", "call-d"})
}

func TestCancellationTerminatesEveryConfirmedToolCall(t *testing.T) {
	started := make(chan string)
	finished := make(chan string)
	registry := tools.NewRegistry()
	registry.Register(&barrierContractTool{name: "cancel_a", started: started, finished: finished, parallel: true})
	registry.Register(&barrierContractTool{name: "cancel_b", started: started, finished: finished, parallel: true})
	calls := []schema.ToolCall{
		{ID: "call-cancel-a", Name: "cancel_a", Arguments: json.RawMessage(`{}`)},
		{ID: "call-cancel-b", Name: "cancel_b", Arguments: json.RawMessage(`{}`)},
	}
	provider := &cancellationContractProvider{calls: calls}
	reporter := &currentContractReporter{}
	engine, sess := newToolContractEngine(t, provider, registry, Config{MaxTurns: 3})
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		_, err := engine.RunWithReporter(ctx, sess, "cancel tools", reporter)
		runDone <- err
	}()

	startedSet := map[string]bool{awaitToolSignal(t, started): true, awaitToolSignal(t, started): true}
	if !startedSet["cancel_a"] || !startedSet["cancel_b"] {
		t.Fatalf("started tools = %#v, want both confirmed calls", startedSet)
	}
	cancel()
	finishedSet := map[string]bool{awaitToolSignal(t, finished): true, awaitToolSignal(t, finished): true}
	if !finishedSet["cancel_a"] || !finishedSet["cancel_b"] {
		t.Fatalf("finished tools = %#v, want both canceled calls", finishedSet)
	}
	err := awaitToolRun(t, runDone)
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
	assertToolFactOrder(t, reporter.facts, []string{"call-cancel-a", "call-cancel-b"})
	for _, fact := range reporter.facts {
		if fact.Kind == "tool_result" && (!fact.IsError || !strings.Contains(fact.Content, context.Canceled.Error())) {
			t.Fatalf("terminal tool fact = %#v, want correlated cancellation", fact)
		}
	}
	assertNoToolSignal(t, started, "no tool may start after cancellation")
}

func TestToolResultsPrecedeNextTurnInjections(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&bigOutputTool{name: "observe", output: "observed"})
	call := schema.ToolCall{ID: "call-observe", Name: "observe", Arguments: json.RawMessage(`{}`)}
	provider := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{call}}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	turn := 0
	engine, sess := newToolContractEngine(t, provider, registry, Config{
		MaxTurns: 3,
		NextTurnReminders: func() []string {
			turn++
			if turn == 2 {
				return []string{"fixture attachment is ready", "queued user context"}
			}
			return nil
		},
	})
	if _, err := engine.RunWithReporter(context.Background(), sess, "inspect", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if len(provider.seen) != 2 {
		t.Fatalf("model requests = %d, want 2", len(provider.seen))
	}
	messages := provider.seen[1]
	if len(messages) != 6 {
		t.Fatalf("follow-up messages = %#v, want six", messages)
	}
	if len(messages[2].ToolCalls) != 1 || messages[2].ToolCalls[0].ID != call.ID {
		t.Fatalf("assistant tool-call message = %#v", messages[2])
	}
	if messages[3].ToolCallID != call.ID || messages[3].Content != "observed" {
		t.Fatalf("tool result message = %#v", messages[3])
	}
	for index, want := range []string{"fixture attachment is ready", "queued user context"} {
		message := messages[index+4]
		if message.Role != schema.RoleUser || message.ToolCallID != "" || !strings.Contains(message.Content, want) {
			t.Fatalf("next-turn injection %d = %#v, want %q after tool result", index, message, want)
		}
	}
}

func TestLargeToolResultKeepsDistinctArtifactAndPreviews(t *testing.T) {
	const callID = "call-large"
	largeOutput := strings.Repeat("L", 60_000)
	registry := tools.NewRegistry()
	registry.Register(&bigOutputTool{name: "large_result", output: largeOutput})
	call := schema.ToolCall{ID: callID, Name: "large_result", Arguments: json.RawMessage(`{}`)}
	provider := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{call}}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	reporter := &largeResultContractReporter{}
	engine, sess := newToolContractEngine(t, provider, registry, Config{MaxTurns: 3})
	if _, err := engine.RunWithReporter(context.Background(), sess, "large result", reporter); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}

	artifactPath := filepath.Join(sess.ToolResultsDir(), callID+".txt")
	artifact, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", artifactPath, err)
	}
	if string(artifact) != largeOutput {
		t.Fatalf("artifact bytes = %d, want exact %d-byte result", len(artifact), len(largeOutput))
	}
	if reporter.detail.ToolCallID != callID || reporter.detail.Output != largeOutput || reporter.detail.IsError {
		t.Fatalf("detailed result = call %q bytes %d error %v", reporter.detail.ToolCallID, len(reporter.detail.Output), reporter.detail.IsError)
	}
	wantReporterPreview := strings.Repeat("L", 800) + "\n... (已截断，原始输出约 60000 字节)"
	if reporter.preview != wantReporterPreview {
		t.Fatalf("reporter preview bytes = %d, want exact bounded preview", len(reporter.preview))
	}
	if len(provider.seen) != 2 {
		t.Fatalf("model requests = %d, want 2", len(provider.seen))
	}
	modelPreview := provider.seen[1][3].Content
	if !strings.Contains(modelPreview, "<persisted-output>") || !strings.Contains(modelPreview, artifactPath) || !strings.Contains(modelPreview, strings.Repeat("L", toolresult.PreviewSize)) {
		t.Fatalf("model preview = %q, want persisted marker, path, and 2KB head", modelPreview)
	}
	if len(modelPreview) >= len(largeOutput) {
		t.Fatalf("model preview bytes = %d, want less than full result %d", len(modelPreview), len(largeOutput))
	}
	messages, err := session.NewMessageLog(sess).LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages() error = %v", err)
	}
	var persisted string
	for _, message := range messages {
		if message.ToolCallID == callID {
			persisted = message.Content
		}
	}
	if persisted != modelPreview {
		t.Fatalf("persisted tool message differs from model preview: got %d bytes, want %d", len(persisted), len(modelPreview))
	}
}

type barrierContractTool struct {
	name     string
	output   string
	started  chan<- string
	finished chan<- string
	release  <-chan struct{}
	parallel bool
}

func (t *barrierContractTool) Name() string { return t.name }

func (t *barrierContractTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: t.name, Description: "barrier contract tool", InputSchema: map[string]any{"type": "object"}}
}

func (t *barrierContractTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	select {
	case t.started <- t.name:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if t.release != nil {
		select {
		case <-t.release:
		case <-ctx.Done():
			if t.finished != nil {
				t.finished <- t.name
			}
			return "", ctx.Err()
		}
	} else {
		<-ctx.Done()
		if t.finished != nil {
			t.finished <- t.name
		}
		return "", ctx.Err()
	}
	if t.finished != nil {
		t.finished <- t.name
	}
	return t.output, nil
}

func (t *barrierContractTool) ParallelSafe() bool { return t.parallel }

type cancellationContractProvider struct {
	calls []schema.ToolCall
	count int
}

type largeResultContractReporter struct {
	recordingReporter
	preview string
	detail  schema.ToolResult
}

func (r *largeResultContractReporter) OnToolResult(ctx context.Context, name, result string, isError bool) {
	r.preview = result
	r.recordingReporter.OnToolResult(ctx, name, result, isError)
}

func (r *largeResultContractReporter) OnToolCallDetail(context.Context, schema.ToolCall) {}

func (r *largeResultContractReporter) OnToolResultDetail(_ context.Context, _ schema.ToolCall, result schema.ToolResult) {
	r.detail = result
}

func (p *cancellationContractProvider) Generate(ctx context.Context, _ []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.count++
	if p.count == 1 {
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: p.calls}}, nil
	}
	return nil, ctx.Err()
}

func newToolContractEngine(t *testing.T, p provider.LLMProvider, registry tools.Registry, config Config) (*AgentEngine, *session.Session) {
	t.Helper()
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return NewAgentEngine(p, registry, workDir, staticComposer{}, config), sess
}

func runToolContractAsync(engine *AgentEngine, sess *session.Session, reporter Reporter) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := engine.RunWithReporter(context.Background(), sess, "tool contract", reporter)
		done <- err
	}()
	return done
}

func awaitToolSignal(t *testing.T, signals <-chan string) string {
	t.Helper()
	select {
	case signal := <-signals:
		return signal
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for synchronized tool signal")
		return ""
	}
}

func assertNoToolSignal(t *testing.T, signals <-chan string, message string) {
	t.Helper()
	select {
	case signal := <-signals:
		t.Fatalf("%s: unexpected start %q", message, signal)
	default:
	}
}

func awaitToolRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tool contract run")
		return errors.New("unreachable timeout")
	}
}

func assertCommittedToolOrder(t *testing.T, messages []schema.Message, want []string) {
	t.Helper()
	var got []string
	for _, message := range messages {
		if message.ToolCallID != "" {
			got = append(got, message.ToolCallID)
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("committed tool order = %v, want %v", got, want)
	}
}

func assertToolFactOrder(t *testing.T, facts []runtimecontract.Fact, want []string) {
	t.Helper()
	var got []string
	for _, fact := range facts {
		if fact.Kind == "tool_result" {
			got = append(got, fact.CallID)
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("tool result fact order = %v, want %v", got, want)
	}
}
