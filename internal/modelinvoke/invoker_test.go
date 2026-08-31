package modelinvoke

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestInvokerNormalizesProviderResponseAndEffort(t *testing.T) {
	transport := &stubProvider{response: &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"},
		Usage:   schema.Usage{InputTokens: 7, OutputTokens: 3},
	}}
	successes := 0
	invoker := New(transport, Config{OnSuccess: func() { successes++ }})
	run, err := invoker.StartRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Invoke(context.Background(), engine.RunContext{
		Messages:        []schema.Message{{Role: schema.RoleUser, Content: "go"}},
		ToolDefinitions: []schema.ToolDefinition{{Name: "read_file"}},
		Effort:          "high",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "done" || result.Message.Usage == nil || result.Message.Usage.InputTokens != 7 || result.Message.Usage.OutputTokens != 3 || result.FinishReason != "stop" || result.Usage.InputTokens != 7 || result.Usage.OutputTokens != 3 {
		t.Fatalf("normalized result = %#v", result)
	}
	if transport.effort != "high" || len(transport.messages) != 1 || len(transport.tools) != 1 || successes != 1 {
		t.Fatalf("provider invocation = effort:%q messages:%#v tools:%#v successes:%d", transport.effort, transport.messages, transport.tools, successes)
	}
}

func TestInvokerDoesNotDropUnsupportedEffortOption(t *testing.T) {
	transport := &generateOnlyProvider{response: &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"},
	}}
	run, err := New(transport, Config{}).StartRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Invoke(context.Background(), engine.RunContext{Effort: "high"}, nil); err == nil || !strings.Contains(err.Error(), "effort") {
		t.Fatalf("Invoke() error = %v, want unsupported effort failure", err)
	}
	if transport.calls != 0 {
		t.Fatalf("Generate() calls = %d, want no option-dropping call", transport.calls)
	}
}

func TestInvokerStreamsAndPreservesPostDeltaFailure(t *testing.T) {
	streamErr := errors.New("unsupported stream_options parameter")
	transport := &streamingStubProvider{
		streamResponses: []*provider.GenerateResponse{{Message: &schema.Message{Role: schema.RoleAssistant, Content: "hello world"}}, nil},
		streamDeltas:    [][]string{{"hello ", "world"}, {"partial"}},
		streamErrors:    []error{nil, streamErr},
	}
	run, err := New(transport, Config{Streaming: true}).StartRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var deltas []string
	result, err := run.Invoke(context.Background(), engine.RunContext{}, func(fact engine.ModelFact) {
		deltas = append(deltas, fact.Content)
	})
	if err != nil || result.Message.Content != "hello world" || strings.Join(deltas, "") != "hello world" {
		t.Fatalf("stream success = %#v/%v deltas=%q", result, err, strings.Join(deltas, ""))
	}
	_, err = run.Invoke(context.Background(), engine.RunContext{}, func(fact engine.ModelFact) {
		deltas = append(deltas, fact.Content)
	})
	if !errors.Is(err, streamErr) || transport.generateCalls != 0 {
		t.Fatalf("post-delta failure = %v generateCalls=%d, want original error without fallback", err, transport.generateCalls)
	}
}

func TestInvokerStreamingFallbackStateIsRunScoped(t *testing.T) {
	unsupported := errors.New("streaming is not supported by this endpoint")
	transport := &streamingStubProvider{
		streamErrors: []error{unsupported, unsupported},
		responses: []*provider.GenerateResponse{
			{Message: &schema.Message{Role: schema.RoleAssistant, Content: "fallback-1"}},
			{Message: &schema.Message{Role: schema.RoleAssistant, Content: "fallback-2"}},
			{Message: &schema.Message{Role: schema.RoleAssistant, Content: "fallback-3"}},
		},
	}
	invoker := New(transport, Config{Streaming: true})
	firstRun, err := invoker.StartRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		if _, err := firstRun.Invoke(context.Background(), engine.RunContext{}, nil); err != nil {
			t.Fatal(err)
		}
	}
	secondRun, err := invoker.StartRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secondRun.Invoke(context.Background(), engine.RunContext{}, nil); err != nil {
		t.Fatal(err)
	}
	if transport.streamCalls != 2 || transport.generateCalls != 3 {
		t.Fatalf("provider calls stream/generate = %d/%d, want 2/3", transport.streamCalls, transport.generateCalls)
	}
}

func TestInvokerDistinguishesDisablingAndRetryablePreDeltaFallback(t *testing.T) {
	for _, test := range []struct {
		name        string
		streamError error
		wantStreams int
	}{
		{name: "empty stream disables later attempts", streamError: provider.ErrEmptyStream, wantStreams: 1},
		{name: "retryable start failure retries next turn", streamError: errors.New("connection reset by peer"), wantStreams: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &streamingStubProvider{
				streamResponses: []*provider.GenerateResponse{nil, {Message: &schema.Message{Role: schema.RoleAssistant, Content: "streamed"}}},
				streamDeltas:    [][]string{nil, {"streamed"}},
				streamErrors:    []error{test.streamError, nil},
				responses: []*provider.GenerateResponse{
					{Message: &schema.Message{Role: schema.RoleAssistant, Content: "fallback-1"}},
					{Message: &schema.Message{Role: schema.RoleAssistant, Content: "fallback-2"}},
				},
			}
			run, err := New(transport, Config{Streaming: true}).StartRun(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for index := 0; index < 2; index++ {
				if _, err := run.Invoke(context.Background(), engine.RunContext{}, nil); err != nil {
					t.Fatal(err)
				}
			}
			if transport.streamCalls != test.wantStreams {
				t.Fatalf("stream calls = %d, want %d", transport.streamCalls, test.wantStreams)
			}
		})
	}
}

func TestInvokerInfersToolFinishAndEmitsNoSyntheticDelta(t *testing.T) {
	transport := &stubProvider{response: &provider.GenerateResponse{Message: &schema.Message{
		Role:      schema.RoleAssistant,
		ToolCalls: []schema.ToolCall{{ID: "call-1", Name: "read_file", Arguments: []byte(`{"path":"x"}`)}},
	}}}
	run, err := New(transport, Config{}).StartRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	deltas := 0
	result, err := run.Invoke(context.Background(), engine.RunContext{}, func(engine.ModelFact) { deltas++ })
	if err != nil {
		t.Fatal(err)
	}
	if result.FinishReason != "tool_calls" || len(result.Message.ToolCalls) != 1 || deltas != 0 {
		t.Fatalf("tool response = %#v, deltas = %d", result, deltas)
	}
}

func TestInvokerRejectsEmptyResponseAndNormalizesPromptTooLong(t *testing.T) {
	promptErr := errors.New("maximum context length exceeded")
	for _, test := range []struct {
		name      string
		transport *stubProvider
		wantLong  bool
	}{
		{name: "nil response", transport: &stubProvider{}},
		{name: "nil message", transport: &stubProvider{response: &provider.GenerateResponse{}}},
		{name: "prompt too long", transport: &stubProvider{err: promptErr}, wantLong: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, err := New(test.transport, Config{IsPromptTooLong: func(err error) bool { return errors.Is(err, promptErr) }}).StartRun(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			_, err = run.Invoke(context.Background(), engine.RunContext{}, nil)
			if err == nil {
				t.Fatal("Invoke() error = nil")
			}
			if !test.wantLong && err.Error() != "provider returned empty response" {
				t.Fatalf("empty response error = %q", err.Error())
			}
			if got := errors.Is(err, engine.ErrPromptTooLong); got != test.wantLong {
				t.Fatalf("errors.Is(ErrPromptTooLong) = %t, want %t: %v", got, test.wantLong, err)
			}
			if test.wantLong && err.Error() != promptErr.Error() {
				t.Fatalf("prompt-too-long text = %q, want %q", err.Error(), promptErr.Error())
			}
		})
	}
}

func TestInvokerDefensivelyCopiesNestedToolSchemas(t *testing.T) {
	schemaValue := map[string]any{"properties": map[string]any{"path": map[string]any{"type": "string"}}}
	transport := &mutatingProvider{response: &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}}
	run, err := New(transport, Config{}).StartRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = run.Invoke(context.Background(), engine.RunContext{ToolDefinitions: []schema.ToolDefinition{{Name: "read_file", InputSchema: schemaValue}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := schemaValue["properties"].(map[string]any)["path"].(map[string]any)
	if path["type"] != "string" {
		t.Fatalf("provider mutated caller schema = %#v", schemaValue)
	}
}

type stubProvider struct {
	response *provider.GenerateResponse
	err      error
	messages []schema.Message
	tools    []schema.ToolDefinition
	effort   string
}

type mutatingProvider struct {
	response *provider.GenerateResponse
}

type generateOnlyProvider struct {
	response *provider.GenerateResponse
	calls    int
}

func (p *generateOnlyProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	return p.response, nil
}

type streamingStubProvider struct {
	streamResponses []*provider.GenerateResponse
	streamDeltas    [][]string
	streamErrors    []error
	responses       []*provider.GenerateResponse
	streamCalls     int
	generateCalls   int
}

func (p *streamingStubProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	index := p.generateCalls
	p.generateCalls++
	if index >= len(p.responses) {
		return nil, errors.New("unexpected non-streaming call")
	}
	return p.responses[index], nil
}

func (p *streamingStubProvider) GenerateStream(_ context.Context, _ []schema.Message, _ []schema.ToolDefinition, _ provider.GenerateOptions, callbacks provider.StreamCallbacks) (*provider.GenerateResponse, error) {
	index := p.streamCalls
	p.streamCalls++
	if index < len(p.streamDeltas) {
		for _, delta := range p.streamDeltas[index] {
			callbacks.EmitTextDelta(delta)
		}
	}
	var response *provider.GenerateResponse
	if index < len(p.streamResponses) {
		response = p.streamResponses[index]
	}
	var err error
	if index < len(p.streamErrors) {
		err = p.streamErrors[index]
	}
	return response, err
}

func (p *mutatingProvider) Generate(_ context.Context, _ []schema.Message, tools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	tools[0].InputSchema.(map[string]any)["properties"].(map[string]any)["path"].(map[string]any)["type"] = "mutated"
	return p.response, nil
}

func (p *stubProvider) Generate(ctx context.Context, messages []schema.Message, tools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, tools, provider.GenerateOptions{})
}

func (p *stubProvider) GenerateWithOptions(_ context.Context, messages []schema.Message, tools []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.messages = append([]schema.Message(nil), messages...)
	p.tools = append([]schema.ToolDefinition(nil), tools...)
	p.effort = options.Effort
	return p.response, p.err
}
