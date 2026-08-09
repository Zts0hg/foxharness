package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
	if len(script.Tools) != 0 || len(script.Interactions) != 0 {
		return runtimecontract.Observed{}, errors.New("tool-free current contract adapter received tool or interaction script")
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

	scripted := &currentContractProvider{steps: script.ModelSteps}
	reporter := &currentContractReporter{}
	config := DefaultConfig()
	config.DisplayPrompt = input.DisplayPrompt
	config.EnableThinking = input.Thinking
	config.MaxTurns = input.MaxTurns
	config.Model = input.Model
	config.ProviderProtocol = input.Provider
	config.EffortOverride = input.Effort
	current := NewAgentEngine(scripted, tools.NewRegistry(), workDir, currentContractPromptComposer{}, config)
	result, runErr := current.RunWithReporter(ctx, sess, input.Prompt, reporter)

	observed := runtimecontract.Observed{Facts: reporter.facts}
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
	observed.Outcome.TurnCount = scripted.callCount
	observed.Outcome.Usage = scripted.usage
	observed.Outcome.FinishReason = scripted.finishReason
	if runErr != nil {
		observed.Outcome.Error = runErr.Error()
	}

	records, loadErr := session.NewMessageLog(sess).LoadRecords()
	if loadErr != nil {
		return observed, fmt.Errorf("load current production messages: %w", loadErr)
	}
	for i, record := range records {
		observed.Persisted = append(observed.Persisted, runtimecontract.PersistedRecord{
			Kind:    "message",
			Path:    "messages.jsonl",
			Content: string(record.Message.Role) + ":" + record.Message.Content,
			Order:   i + 1,
		})
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
	finishReason string
	usage        runtimecontract.Usage
}

func (p *currentContractProvider) Generate(
	_ context.Context,
	_ []schema.Message,
	_ []schema.ToolDefinition,
) (*provider.GenerateResponse, error) {
	if p.callCount >= len(p.steps) {
		return nil, fmt.Errorf("model script exhausted after %d calls", p.callCount)
	}
	step := p.steps[p.callCount]
	p.callCount++
	if step.Error != "" {
		return nil, errors.New(step.Error)
	}
	p.finishReason = step.Response.FinishReason
	p.usage.InputTokens += step.Response.Usage.InputTokens
	p.usage.OutputTokens += step.Response.Usage.OutputTokens
	p.usage.TotalTokens += step.Response.Usage.TotalTokens
	usage := schema.Usage{
		InputTokens:  int64(step.Response.Usage.InputTokens),
		OutputTokens: int64(step.Response.Usage.OutputTokens),
	}
	return &provider.GenerateResponse{
		Message: &schema.Message{
			Role:    schema.RoleAssistant,
			Content: step.Response.Content,
			Usage:   &usage,
		},
		Usage: usage,
	}, nil
}

type currentContractReporter struct {
	facts []runtimecontract.Fact
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
