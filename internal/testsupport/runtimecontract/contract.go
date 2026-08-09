/*
Package runtimecontract defines implementation-neutral agent runtime scenarios.
*/
package runtimecontract

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var scenarioIDPattern = regexp.MustCompile(`^[A-Z]+(?:-[A-Z]+)*-[0-9]{3}$`)

/* Adapter executes a scenario through one runtime implementation boundary. */
type Adapter interface {
	Run(context.Context, RunInput, Script) (Observed, error)
}

/* AdapterFunc adapts a function to Adapter. */
type AdapterFunc func(context.Context, RunInput, Script) (Observed, error)

/* Run executes the adapted function. */
func (f AdapterFunc) Run(ctx context.Context, input RunInput, script Script) (Observed, error) {
	return f(ctx, input, script)
}

/* Scenario combines controlled inputs with one authoritative expectation. */
type Scenario struct {
	ID       string
	Input    RunInput
	Script   Script
	Expected Expected
}

/* RunInput contains profile and per-run values without production API types. */
type RunInput struct {
	Profile           string
	Prompt            string
	DisplayPrompt     string
	SessionChoice     string
	SessionID         string
	WorkDir           string
	Model             string
	Provider          string
	Effort            string
	CollaborationMode string
	Thinking          bool
	MaxTurns          int
	ReadOnly          bool
	AllowedTools      []string
}

/* Script contains all deterministic external responses for a scenario. */
type Script struct {
	ModelSteps   []ModelStep
	Tools        []ToolBehavior
	Interactions []InteractionReply
}

/* ModelStep describes one controlled model invocation. */
type ModelStep struct {
	Request  ModelRequest
	Response ModelResponse
	Deltas   []string
	Error    string
}

/* ModelRequest captures request properties whose propagation is observable. */
type ModelRequest struct {
	Phase           string
	Messages        []Message
	ToolDefinitions []ToolDefinition
	Model           string
	Provider        string
	Effort          string
}

/* ModelResponse captures a controlled model response. */
type ModelResponse struct {
	Content      string
	Thinking     string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage
}

/* Message is an implementation-neutral model-visible message. */
type Message struct {
	Role       string
	Content    string
	ToolCallID string
}

/* ToolDefinition is one model-visible tool declaration. */
type ToolDefinition struct {
	Name         string
	Description  string
	InputSchema  string
	ParallelSafe bool
}

/* ToolCall identifies one controlled tool invocation. */
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

/* ToolBehavior maps a controlled call to its execution result. */
type ToolBehavior struct {
	Call   ToolCall
	Result ToolResult
}

/* ToolResult captures full and model-visible tool result forms. */
type ToolResult struct {
	Output       string
	ModelContent string
	ArtifactPath string
	IsError      bool
	ErrorKind    string
}

/* InteractionReply describes a correlated controlled interaction response. */
type InteractionReply struct {
	Kind          string
	CorrelationID string
	Value         string
	Cancelled     bool
	Error         string
}

/* Expected is the authoritative result for one scenario. */
type Expected struct {
	Observed               Observed
	AdapterErrorContains   string
	CompareObservedOnError bool
}

/* Observed contains ordered facts, outcome, persistence, and artifacts. */
type Observed struct {
	Facts      []Fact
	Outcome    Outcome
	Persisted  []PersistedRecord
	Artifacts  []Artifact
	Warnings   []Warning
	DirectText string
}

/* Fact is one canonically ordered runtime observation. */
type Fact struct {
	Kind       string
	Sequence   int
	Turn       int
	Phase      string
	CallID     string
	Name       string
	Content    string
	IsError    bool
	Attributes map[string]string
}

/* Outcome is an implementation-neutral completed or partial run result. */
type Outcome struct {
	FinalMessage string
	FinishReason string
	TurnCount    int
	Usage        Usage
	Partial      bool
	ErrorKind    string
	Error        string
}

/* Usage captures normalized model token accounting. */
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

/* PersistedRecord captures one compatibility-significant durable write. */
type PersistedRecord struct {
	Kind    string
	Path    string
	Content string
	Order   int
}

/* Artifact captures one non-authoritative run artifact. */
type Artifact struct {
	Kind    string
	Path    string
	Content string
}

/* Warning captures one non-fatal observable failure. */
type Warning struct {
	Sink      string
	Operation string
	Error     string
}

/* VerifyScenario executes one adapter and compares it with the expectation. */
func VerifyScenario(ctx context.Context, adapter Adapter, scenario Scenario) error {
	if adapter == nil {
		return fmt.Errorf("runtime contract adapter is required")
	}
	if !scenarioIDPattern.MatchString(scenario.ID) {
		return fmt.Errorf("scenario ID %q must use a stable PREFIX-000 form", scenario.ID)
	}
	if scenario.Input.Profile == "" {
		return fmt.Errorf("scenario %s profile is required", scenario.ID)
	}
	if err := validateFactOrder(scenario.Expected.Observed.Facts); err != nil {
		return fmt.Errorf("scenario %s expectation: %w", scenario.ID, err)
	}

	actual, runErr := adapter.Run(ctx, scenario.Input, scenario.Script)
	if scenario.Expected.AdapterErrorContains != "" {
		if runErr == nil {
			return fmt.Errorf("scenario %s adapter error = nil, want containing %q", scenario.ID, scenario.Expected.AdapterErrorContains)
		}
		if !strings.Contains(runErr.Error(), scenario.Expected.AdapterErrorContains) {
			return fmt.Errorf("scenario %s adapter error = %q, want containing %q", scenario.ID, runErr, scenario.Expected.AdapterErrorContains)
		}
		if !scenario.Expected.CompareObservedOnError {
			return nil
		}
	} else if runErr != nil {
		return fmt.Errorf("scenario %s unexpected adapter error: %w", scenario.ID, runErr)
	}

	if err := compareObserved(scenario.Expected.Observed, actual); err != nil {
		return fmt.Errorf("scenario %s: %w", scenario.ID, err)
	}
	return nil
}

func validateFactOrder(facts []Fact) error {
	previous := 0
	for i, fact := range facts {
		if fact.Sequence <= previous {
			return fmt.Errorf("facts sequence at index %d is %d after %d", i, fact.Sequence, previous)
		}
		previous = fact.Sequence
	}
	return nil
}

func compareObserved(want, got Observed) error {
	checks := []struct {
		name string
		want any
		got  any
	}{
		{name: "facts", want: want.Facts, got: got.Facts},
		{name: "outcome", want: want.Outcome, got: got.Outcome},
		{name: "persisted records", want: want.Persisted, got: got.Persisted},
		{name: "artifacts", want: want.Artifacts, got: got.Artifacts},
		{name: "warnings", want: want.Warnings, got: got.Warnings},
		{name: "direct text", want: want.DirectText, got: got.DirectText},
	}
	for _, check := range checks {
		if !reflect.DeepEqual(check.want, check.got) {
			return fmt.Errorf("%s mismatch: got %#v want %#v", check.name, check.got, check.want)
		}
	}
	return nil
}
