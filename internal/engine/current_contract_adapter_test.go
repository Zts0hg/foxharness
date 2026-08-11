package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/metrics"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/testsupport/runtimecontract"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestCurrentProductionContractAdapterRunsToolFreeScenario(t *testing.T) {
	scenario := runtimecontract.Scenario{
		ID: "RT-001",
		Input: runtimecontract.RunInput{
			Profile:       "CLIExec",
			Prompt:        "Return the scripted answer.",
			DisplayPrompt: "Return the scripted answer.",
			SessionChoice: "new",
			Model:         "scripted-model",
			Provider:      "scripted",
			MaxTurns:      3,
		},
		Script: runtimecontract.Script{
			ModelSteps: []runtimecontract.ModelStep{{
				Response: runtimecontract.ModelResponse{
					Content:      "scripted answer",
					FinishReason: "stop",
				},
			}},
		},
		Expected: runtimecontract.Expected{
			Observed: runtimecontract.Observed{
				Requests: []runtimecontract.ModelRequest{{
					Phase: "action",
					Messages: []runtimecontract.Message{
						{Role: "system", Content: "contract system prompt"},
						{Role: "user", Content: "Return the scripted answer."},
					},
					Model: "scripted-model", Provider: "scripted",
				}},
				Facts: []runtimecontract.Fact{
					{Kind: "run_started", Sequence: 1},
					{Kind: "message", Sequence: 2, Content: "scripted answer"},
					{Kind: "run_completed", Sequence: 3, Content: "scripted answer"},
				},
				Outcome: runtimecontract.Outcome{
					FinalMessage: "scripted answer",
					FinishReason: "stop",
					TurnCount:    1,
				},
				Persisted: []runtimecontract.PersistedRecord{
					{Kind: "message", Path: "messages.jsonl", Content: "user:Return the scripted answer.", Order: 1},
					{Kind: "message", Path: "messages.jsonl", Content: "assistant:scripted answer", Order: 2},
				},
				Metrics: []runtimecontract.Metric{
					{Kind: "model_call", Turn: 1, Phase: "action"},
					{Kind: "run_summary", ModelCalls: 1},
				},
			},
		},
	}

	adapter := newCurrentProductionContractAdapter(t)
	if err := runtimecontract.VerifyScenario(context.Background(), adapter, scenario); err != nil {
		t.Fatalf("VerifyScenario() error = %v", err)
	}
}

type currentProductionContractAdapter struct {
	t *testing.T
}

func newCurrentProductionContractAdapter(t *testing.T) runtimecontract.Adapter {
	t.Helper()
	return &currentProductionContractAdapter{t: t}
}

func (a *currentProductionContractAdapter) Run(
	ctx context.Context,
	input runtimecontract.RunInput,
	script runtimecontract.Script,
) (runtimecontract.Observed, error) {
	a.t.Helper()
	if input.SessionChoice != "new" {
		return runtimecontract.Observed{}, fmt.Errorf("current contract adapter session choice %q is unsupported", input.SessionChoice)
	}
	if len(script.Interactions) != 0 {
		return runtimecontract.Observed{}, errors.New("current contract adapter does not support interaction scripts")
	}
	if len(script.ModelSteps) == 0 {
		return runtimecontract.Observed{}, errors.New("current contract adapter requires at least one model step")
	}

	workDir := input.WorkDir
	if workDir == "" {
		workDir = a.t.TempDir()
	}
	manager := session.NewManagerWithHome(workDir, a.t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		return runtimecontract.Observed{}, fmt.Errorf("create current production session: %w", err)
	}

	registry := tools.NewRegistry()
	parallelSafe := make(map[string]bool, len(script.Tools))
	for _, behavior := range script.Tools {
		tool := newCurrentContractTool(behavior)
		registry.Register(tool)
		parallelSafe[tool.Name()] = behavior.Definition.ParallelSafe
	}
	streaming := currentContractScriptStreams(script)
	scripted := &currentContractProvider{
		steps:        script.ModelSteps,
		thinking:     input.Thinking,
		model:        input.Model,
		protocol:     input.Provider,
		parallelSafe: parallelSafe,
		streaming:    streaming,
	}
	reporter := &currentContractReporter{}
	var runReporter Reporter = reporter
	if streaming {
		runReporter = &currentContractStreamingReporter{currentContractReporter: reporter}
	}
	config := DefaultConfig()
	config.DisplayPrompt = input.DisplayPrompt
	config.EnableThinking = input.Thinking
	config.MaxTurns = input.MaxTurns
	config.Model = input.Model
	config.ProviderProtocol = input.Provider
	config.EffortOverride = input.Effort
	current := NewAgentEngine(scripted, registry, workDir, currentContractPromptComposer{}, config)
	result, runErr := current.RunWithReporter(ctx, sess, input.Prompt, runReporter)

	observed := runtimecontract.Observed{Requests: scripted.requests, Facts: reporter.facts}
	if result != nil {
		observed.Outcome.FinalMessage = result.FinalMessage
		if len(result.TelemetryWarnings) > 0 {
			observed.Warnings = make([]runtimecontract.Warning, 0, len(result.TelemetryWarnings))
			for _, warning := range result.TelemetryWarnings {
				observed.Warnings = append(observed.Warnings, runtimecontract.Warning{
					Sink: warning.Sink, Operation: warning.Operation, Error: warning.Error,
				})
			}
		}
	}
	observed.Outcome.TurnCount = scripted.actionCalls
	observed.Outcome.Usage = scripted.usage
	observed.Outcome.FinishReason = scripted.finishReason
	if runErr != nil {
		observed.Outcome.Error = runErr.Error()
		observed.Outcome.Partial = result != nil
		var turnLimit *TurnLimitError
		if errors.As(runErr, &turnLimit) {
			observed.Outcome.ErrorKind = "turn_limit"
		} else {
			observed.Outcome.ErrorKind = "provider"
		}
	}

	records, loadErr := session.NewMessageLog(sess).LoadRecords()
	if loadErr != nil {
		return observed, fmt.Errorf("load current production messages: %w", loadErr)
	}
	for i, record := range records {
		observed.Persisted = append(observed.Persisted, runtimecontract.PersistedRecord{
			Kind:    "message",
			Path:    "messages.jsonl",
			Content: currentContractPersistedMessage(record.Message),
			Order:   i + 1,
		})
	}
	runRoot, rootErr := currentContractRunRoot(sess)
	if rootErr != nil {
		return observed, rootErr
	}
	observed.Metrics, err = currentContractMetrics(filepath.Join(runRoot, "metrics.jsonl"))
	if err != nil {
		return observed, err
	}
	return observed, runErr
}

type currentContractPromptComposer struct{}

func (currentContractPromptComposer) Compose(string) (string, error) {
	return "contract system prompt", nil
}

type currentContractProvider struct {
	steps        []runtimecontract.ModelStep
	callCount    int
	actionCalls  int
	finishReason string
	usage        runtimecontract.Usage
	requests     []runtimecontract.ModelRequest
	thinking     bool
	model        string
	protocol     string
	parallelSafe map[string]bool
	streaming    bool
	fallback     bool
}

func (p *currentContractProvider) Generate(
	ctx context.Context,
	messages []schema.Message,
	definitions []schema.ToolDefinition,
) (*provider.GenerateResponse, error) {
	return p.generate(ctx, messages, definitions, "", "non_stream", nil)
}

func (p *currentContractProvider) GenerateWithOptions(
	ctx context.Context,
	messages []schema.Message,
	definitions []schema.ToolDefinition,
	options provider.GenerateOptions,
) (*provider.GenerateResponse, error) {
	return p.generate(ctx, messages, definitions, options.Effort, "non_stream", nil)
}

func (p *currentContractProvider) GenerateStream(
	ctx context.Context,
	messages []schema.Message,
	definitions []schema.ToolDefinition,
	options provider.GenerateOptions,
	callbacks provider.StreamCallbacks,
) (*provider.GenerateResponse, error) {
	return p.generate(ctx, messages, definitions, options.Effort, "stream", callbacks.OnTextDelta)
}

func (p *currentContractProvider) generate(
	_ context.Context,
	messages []schema.Message,
	definitions []schema.ToolDefinition,
	effort string,
	transport string,
	onDelta func(string),
) (*provider.GenerateResponse, error) {
	if p.callCount >= len(p.steps) {
		return nil, fmt.Errorf("model script exhausted after %d calls", p.callCount)
	}
	step := p.steps[p.callCount]
	phase := "action"
	if p.thinking && p.callCount%2 == 0 {
		phase = "thinking"
	} else if transport == "stream" || !p.fallback {
		p.actionCalls++
	}
	if transport == "non_stream" && p.fallback {
		p.fallback = false
	}
	observedTransport := ""
	if p.streaming {
		observedTransport = transport
	}
	p.requests = append(p.requests, runtimecontract.ModelRequest{
		Phase:           phase,
		Transport:       observedTransport,
		Messages:        currentContractMessages(messages),
		ToolDefinitions: currentContractDefinitions(definitions, p.parallelSafe),
		Model:           p.model,
		Provider:        p.protocol,
		Effort:          effort,
	})
	p.callCount++
	for _, delta := range step.Deltas {
		if onDelta != nil {
			onDelta(delta)
		}
	}
	if step.Error != "" {
		if transport == "stream" && len(step.Deltas) == 0 && (step.ErrorKind == "empty" || step.ErrorKind == "unsupported" || step.ErrorKind == "retryable") {
			p.fallback = true
		}
		switch step.ErrorKind {
		case "empty":
			return nil, provider.ErrEmptyStream
		case "unsupported":
			return nil, errors.New("streaming is not supported: " + step.Error)
		case "retryable":
			return nil, statusCodeError{StatusCode: 429, message: step.Error}
		default:
			return nil, errors.New(step.Error)
		}
	}
	if step.NilResponse {
		return nil, nil
	}
	if step.NilMessage {
		return &provider.GenerateResponse{}, nil
	}
	p.finishReason = step.Response.FinishReason
	p.usage.InputTokens += step.Response.Usage.InputTokens
	p.usage.OutputTokens += step.Response.Usage.OutputTokens
	p.usage.TotalTokens += step.Response.Usage.TotalTokens
	usage := schema.Usage{
		InputTokens:  int64(step.Response.Usage.InputTokens),
		OutputTokens: int64(step.Response.Usage.OutputTokens),
	}
	message := &schema.Message{
		Role:      schema.RoleAssistant,
		Content:   step.Response.Content,
		Usage:     &usage,
		ToolCalls: currentContractSchemaToolCalls(step.Response.ToolCalls),
	}
	return &provider.GenerateResponse{
		Message: message,
		Usage:   usage,
	}, nil
}

type currentContractTool struct {
	behavior runtimecontract.ToolBehavior
	name     string
	schema   any
}

func newCurrentContractTool(behavior runtimecontract.ToolBehavior) *currentContractTool {
	name := behavior.Definition.Name
	if name == "" {
		name = behavior.Call.Name
	}
	inputSchema := any(map[string]any{})
	if raw := strings.TrimSpace(behavior.Definition.InputSchema); raw != "" {
		if err := json.Unmarshal([]byte(raw), &inputSchema); err != nil {
			inputSchema = raw
		}
	}
	return &currentContractTool{behavior: behavior, name: name, schema: inputSchema}
}

func (t *currentContractTool) Name() string { return t.name }

func (t *currentContractTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: t.name, Description: t.behavior.Definition.Description, InputSchema: t.schema}
}

func (t *currentContractTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	if err := t.validateArguments(args); err != nil {
		return "", err
	}
	if t.behavior.Result.ErrorKind != "" && !t.behavior.Result.IsError {
		return "", errors.New(t.behavior.Result.ErrorKind)
	}
	return t.behavior.Result.Output, nil
}

func (t *currentContractTool) ExecuteResult(_ context.Context, args json.RawMessage) (tools.ExecutionResult, error) {
	if err := t.validateArguments(args); err != nil {
		return tools.ExecutionResult{}, err
	}
	if t.behavior.Result.ErrorKind != "" && !t.behavior.Result.IsError {
		return tools.ExecutionResult{}, errors.New(t.behavior.Result.ErrorKind)
	}
	return tools.ExecutionResult{Output: t.behavior.Result.Output, Failed: t.behavior.Result.IsError}, nil
}

func (t *currentContractTool) validateArguments(args json.RawMessage) error {
	want := t.behavior.Call.Arguments
	if want == "" || string(args) == want {
		return nil
	}
	return fmt.Errorf("invalid arguments: got %s, want %s", args, want)
}

func (t *currentContractTool) ParallelSafe() bool { return t.behavior.Definition.ParallelSafe }

type currentContractReporter struct {
	facts []runtimecontract.Fact
}

type currentContractStreamingReporter struct {
	*currentContractReporter
}

func (r *currentContractStreamingReporter) OnMessageDelta(_ context.Context, content string) {
	r.append(runtimecontract.Fact{Kind: "message_delta", Content: content})
}

func (r *currentContractReporter) append(fact runtimecontract.Fact) {
	fact.Sequence = len(r.facts) + 1
	r.facts = append(r.facts, fact)
}

func (r *currentContractReporter) OnRunStart(context.Context, string, string) {
	r.append(runtimecontract.Fact{Kind: "run_started"})
}

func (r *currentContractReporter) OnThinking(_ context.Context, turn int) {
	r.append(runtimecontract.Fact{Kind: "thinking", Turn: turn})
}

func (r *currentContractReporter) OnCompaction(_ context.Context, scope string) {
	r.append(runtimecontract.Fact{Kind: "compaction", Name: scope})
}

func (r *currentContractReporter) OnToolCall(_ context.Context, name, args string) {
	r.append(runtimecontract.Fact{Kind: "tool_call", Name: name, Content: args})
}

func (r *currentContractReporter) OnToolResult(_ context.Context, name, result string, isError bool) {
	r.append(runtimecontract.Fact{Kind: "tool_result", Name: name, Content: result, IsError: isError})
}

func (r *currentContractReporter) OnMessage(_ context.Context, content string) {
	r.append(runtimecontract.Fact{Kind: "message", Content: content})
}

func (r *currentContractReporter) OnRunComplete(_ context.Context, result RunResult) {
	r.append(runtimecontract.Fact{Kind: "run_completed", Content: result.FinalMessage})
}

func (r *currentContractReporter) OnRunError(_ context.Context, _ string, _ string, err error) {
	r.append(runtimecontract.Fact{Kind: "run_error", Content: err.Error(), IsError: true})
}

func (r *currentContractReporter) OnToolCallDetail(_ context.Context, call schema.ToolCall) {
	if len(r.facts) == 0 {
		return
	}
	fact := &r.facts[len(r.facts)-1]
	if fact.Kind == "tool_call" && fact.Name == call.Name {
		fact.CallID = call.ID
		fact.Content = string(call.Arguments)
	}
}

func (r *currentContractReporter) OnToolResultDetail(_ context.Context, call schema.ToolCall, _ schema.ToolResult) {
	if len(r.facts) == 0 {
		return
	}
	fact := &r.facts[len(r.facts)-1]
	if fact.Kind == "tool_result" && fact.Name == call.Name {
		fact.CallID = call.ID
	}
}

func currentContractMessages(messages []schema.Message) []runtimecontract.Message {
	result := make([]runtimecontract.Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, runtimecontract.Message{
			Role:       string(message.Role),
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
			ToolCalls:  currentContractToolCalls(message.ToolCalls),
		})
	}
	return result
}

func currentContractToolCalls(calls []schema.ToolCall) []runtimecontract.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]runtimecontract.ToolCall, 0, len(calls))
	for _, call := range calls {
		result = append(result, runtimecontract.ToolCall{ID: call.ID, Name: call.Name, Arguments: string(call.Arguments)})
	}
	return result
}

func currentContractSchemaToolCalls(calls []runtimecontract.ToolCall) []schema.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]schema.ToolCall, 0, len(calls))
	for _, call := range calls {
		result = append(result, schema.ToolCall{ID: call.ID, Name: call.Name, Arguments: json.RawMessage(call.Arguments)})
	}
	return result
}

func currentContractDefinitions(definitions []schema.ToolDefinition, parallelSafe map[string]bool) []runtimecontract.ToolDefinition {
	if len(definitions) == 0 {
		return nil
	}
	result := make([]runtimecontract.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		encoded, _ := json.Marshal(definition.InputSchema)
		result = append(result, runtimecontract.ToolDefinition{
			Name: definition.Name, Description: definition.Description, InputSchema: string(encoded), ParallelSafe: parallelSafe[definition.Name],
		})
	}
	return result
}

func currentContractPersistedMessage(message schema.Message) string {
	if message.ToolCallID != "" {
		return "tool_result:" + message.ToolCallID + ":" + message.Content
	}
	content := string(message.Role) + ":" + message.Content
	for _, call := range message.ToolCalls {
		content += "|tool_call:" + call.ID + ":" + call.Name + ":" + string(call.Arguments)
	}
	return content
}

func currentContractRunRoot(sess *session.Session) (string, error) {
	entries, err := os.ReadDir(sess.RunsDir())
	if err != nil {
		return "", fmt.Errorf("read current production runs: %w", err)
	}
	var roots []string
	for _, entry := range entries {
		if entry.IsDir() {
			roots = append(roots, filepath.Join(sess.RunsDir(), entry.Name()))
		}
	}
	if len(roots) != 1 {
		return "", fmt.Errorf("current production runs = %d, want 1", len(roots))
	}
	return roots[0], nil
}

func currentContractMetrics(path string) ([]runtimecontract.Metric, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open current production metrics: %w", err)
	}
	defer file.Close()

	var result []runtimecontract.Metric
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var header struct {
			Type metrics.EventType `json:"type"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
			return nil, fmt.Errorf("decode current production metric type: %w", err)
		}
		switch header.Type {
		case metrics.EventModelCall:
			var event metrics.ModelCall
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				return nil, err
			}
			result = append(result, runtimecontract.Metric{Kind: string(event.Type), Turn: event.Turn, Phase: event.Phase, IsError: event.Error != ""})
		case metrics.EventToolCall:
			var event metrics.ToolCall
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				return nil, err
			}
			result = append(result, runtimecontract.Metric{Kind: string(event.Type), Turn: event.Turn, ToolName: event.ToolName, CallID: event.ToolCallID, IsError: event.IsError})
		case metrics.EventRunSummary:
			var event metrics.RunSummary
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				return nil, err
			}
			result = append(result, runtimecontract.Metric{
				Kind: string(event.Type), ModelCalls: event.TotalModelCalls, ToolCalls: event.TotalToolCalls, ErrorCount: event.ErrorCount,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan current production metrics: %w", err)
	}
	return result, nil
}

func currentContractScriptStreams(script runtimecontract.Script) bool {
	for _, step := range script.ModelSteps {
		if len(step.Deltas) > 0 || step.ErrorKind != "" {
			return true
		}
	}
	return false
}
