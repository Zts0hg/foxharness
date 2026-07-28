package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Zts0hg/foxharness/internal/effort"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAIProvider implements the LLMProvider interface using the OpenAI API.
// It supports any OpenAI-compatible endpoint.
//
// The provider handles:
//   - Message format conversion between schema and OpenAI types
//   - Tool/function calling with proper parameter schemas
//   - Multi-turn conversations with full conversation history
type OpenAIProvider struct {
	// client is the OpenAI SDK client for making API requests.
	client openai.Client
	// model specifies the model identifier to use for generation.
	model string
	// retry controls transient request retries around the SDK request.
	retry RetryConfig
}

// NewOpenAIProvider creates an OpenAI-compatible provider from resolved LLM
// configuration, using retry settings resolved from the environment.
func NewOpenAIProvider(config llmconfig.ResolvedConfig) (*OpenAIProvider, error) {
	return newOpenAIProviderWithRetry(config, retryConfigFromEnv())
}

// newOpenAIProviderWithRetry builds an OpenAI-compatible provider with an
// explicit retry configuration, so callers such as the connectivity probe can
// request a single attempt.
func newOpenAIProviderWithRetry(config llmconfig.ResolvedConfig, retry RetryConfig) (*OpenAIProvider, error) {
	clientOptions := []option.RequestOption{
		option.WithBaseURL(config.BaseURL),
		option.WithMaxRetries(0),
	}
	switch config.Auth {
	case llmconfig.AuthAPIKey:
		if config.APIKey == "" {
			return nil, fmt.Errorf("missing API key for OpenAI-compatible provider")
		}
		clientOptions = append(clientOptions, option.WithAPIKey(config.APIKey))
	case llmconfig.AuthNone:
		clientOptions = append(clientOptions,
			option.WithHeaderDel("Authorization"),
			option.WithHeaderDel("X-Api-Key"),
		)
	default:
		return nil, fmt.Errorf("unsupported auth %q for OpenAI-compatible provider", config.Auth)
	}
	if retry.RequestTimeout > 0 {
		clientOptions = append(clientOptions, option.WithRequestTimeout(retry.RequestTimeout))
	}

	return &OpenAIProvider{
		client: newOpenAIClient(clientOptions...),
		model:  config.Model,
		retry:  retry,
	}, nil
}

func newOpenAIClient(options ...option.RequestOption) openai.Client {
	// openai.NewClient prepends OPENAI_* environment defaults. Build only the
	// chat service from explicit options so foxharness owns provider resolution.
	client := openai.Client{Options: options}
	client.Chat = openai.NewChatService(options...)
	return client
}

func (p *OpenAIProvider) ProviderProtocol() string {
	return ProviderProtocolOpenAI
}

func (p *OpenAIProvider) ModelName() string {
	return p.model
}

// Generate produces a response from the OpenAI-compatible API.
//
// The ctx parameter enables cancellation of long-running requests.
// The messages parameter contains the full conversation history.
// The availableTools parameter lists tools the LLM may invoke.
//
// Returns a schema.Message with the LLM's response, including any tool calls,
// or an error if the API request fails or returns an empty response.
func (p *OpenAIProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, availableTools, GenerateOptions{})
}

// GenerateWithOptions produces a response from the OpenAI-compatible API with
// optional per-call user-run generation settings.
func (p *OpenAIProvider) GenerateWithOptions(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition, options GenerateOptions) (*GenerateResponse, error) {
	params, err := p.chatCompletionParams(messages, availableTools, options)
	if err != nil {
		return nil, err
	}

	resp, err := p.chatCompletionWithRetry(ctx, params)
	if err != nil {
		if options.StructuredOutput != nil {
			err = normalizeStructuredOutputError(err)
		}
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("API 返回了空的 Choices")
	}

	choice := resp.Choices[0].Message
	resultMessage := &schema.Message{
		Role:    schema.RoleAssistant,
		Content: choice.Content,
	}

	for _, toolCall := range choice.ToolCalls {
		if toolCall.Type == "function" {
			resultMessage.ToolCalls = append(resultMessage.ToolCalls, schema.ToolCall{
				ID:        toolCall.ID,
				Name:      toolCall.Function.Name,
				Arguments: schema.NormalizeToolArguments([]byte(toolCall.Function.Arguments)),
			})
		}

	}

	normalized := schema.NormalizeMessage(*resultMessage)
	usage := schema.Usage{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}
	return &GenerateResponse{
		Message: &normalized,
		Usage:   usage,
	}, nil
}

func (p *OpenAIProvider) chatCompletionParams(messages []schema.Message, availableTools []schema.ToolDefinition, options GenerateOptions) (openai.ChatCompletionNewParams, error) {
	var openaiMessages []openai.ChatCompletionMessageParamUnion

	for _, message := range messages {
		switch message.Role {
		case schema.RoleSystem:
			openaiMessages = append(openaiMessages, openai.SystemMessage((message.Content)))

		case schema.RoleUser:
			if message.ToolCallID != "" {
				openaiMessages = append(openaiMessages, openai.ToolMessage(message.Content, message.ToolCallID))
			} else {
				openaiMessages = append(openaiMessages, openai.UserMessage(message.Content))
			}
		case schema.RoleAssistant:
			assistantParam := openai.ChatCompletionAssistantMessageParam{}
			if message.Content != "" {
				assistantParam.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(message.Content),
				}
			}

			if len(message.ToolCalls) > 0 {
				var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
				for _, toolCall := range message.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID:   toolCall.ID,
							Type: "function",
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      toolCall.Name,
								Arguments: string(toolCall.Arguments),
							},
						},
					})
				}

				assistantParam.ToolCalls = toolCalls
			}

			openaiMessages = append(openaiMessages, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &assistantParam,
			})
		}
	}

	var openaiTools []openai.ChatCompletionToolUnionParam
	for _, toolDef := range availableTools {
		var params shared.FunctionParameters

		if m, ok := toolDef.InputSchema.(map[string]interface{}); ok {
			params = shared.FunctionParameters(m)
		} else {
			b, _ := json.Marshal(toolDef.InputSchema)
			_ = json.Unmarshal(b, &params)
		}

		openaiTools = append(openaiTools, openai.ChatCompletionFunctionTool(
			shared.FunctionDefinitionParam{
				Name:        toolDef.Name,
				Description: openai.String(toolDef.Description),
				Parameters:  params,
			},
		))

	}

	params := openai.ChatCompletionNewParams{
		Model:    p.model,
		Messages: openaiMessages,
	}

	if len(openaiTools) > 0 {
		params.Tools = openaiTools
	}
	if explicitEffort, err := effort.ExplicitForProvider(effort.ProtocolOpenAI, options.Effort); err != nil {
		return openai.ChatCompletionNewParams{}, err
	} else if explicitEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(explicitEffort)
	}
	if options.StructuredOutput != nil {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:        options.StructuredOutput.Name,
					Description: param.NewOpt(options.StructuredOutput.Description),
					Schema:      options.StructuredOutput.Schema,
					Strict:      param.NewOpt(options.StructuredOutput.Strict),
				},
			},
		}
	}

	return params, nil
}

func (p *OpenAIProvider) chatCompletionWithRetry(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return chatCompletionWithRetry(ctx, p.client, params, p.retry)
}

type openAIStreamToolCall struct {
	id        string
	name      string
	arguments string
}

// GenerateStream streams visible OpenAI-compatible text deltas and returns the
// normalized final assistant message.
func (p *OpenAIProvider) GenerateStream(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition, options GenerateOptions, callbacks StreamCallbacks) (*GenerateResponse, error) {
	if options.StructuredOutput != nil {
		return p.GenerateWithOptions(ctx, messages, availableTools, options)
	}
	params, err := p.chatCompletionParams(messages, availableTools, options)
	if err != nil {
		return nil, err
	}
	params.StreamOptions.IncludeUsage = param.NewOpt(true)
	resp, receivedChunk, err := p.generateOpenAIStream(ctx, params, callbacks)
	if err != nil && !receivedChunk && isOpenAIStreamOptionsRejected(err) {
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{}
		resp, _, err = p.generateOpenAIStream(ctx, params, callbacks)
	}
	return resp, err
}

func (p *OpenAIProvider) generateOpenAIStream(ctx context.Context, params openai.ChatCompletionNewParams, callbacks StreamCallbacks) (*GenerateResponse, bool, error) {
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	var content string
	toolCalls := map[int]*openAIStreamToolCall{}
	usage := schema.Usage{}
	receivedChunk := false
	for stream.Next() {
		receivedChunk = true
		chunk := stream.Current()
		if chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0 {
			usage.InputTokens = chunk.Usage.PromptTokens
			usage.OutputTokens = chunk.Usage.CompletionTokens
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta
			if delta.Content != "" {
				content += delta.Content
				callbacks.EmitTextDelta(delta.Content)
			}
			for _, toolCall := range delta.ToolCalls {
				index := int(toolCall.Index)
				accumulated := toolCalls[index]
				if accumulated == nil {
					accumulated = &openAIStreamToolCall{}
					toolCalls[index] = accumulated
				}
				if toolCall.ID != "" {
					accumulated.id = toolCall.ID
				}
				if toolCall.Function.Name != "" {
					accumulated.name = toolCall.Function.Name
				}
				if toolCall.Function.Arguments != "" {
					accumulated.arguments += toolCall.Function.Arguments
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, receivedChunk, err
	}
	if !receivedChunk {
		return nil, false, fmt.Errorf("%w: openai stream ended without events", ErrEmptyStream)
	}

	resultMessage := &schema.Message{Role: schema.RoleAssistant, Content: content}
	indexes := make([]int, 0, len(toolCalls))
	for index := range toolCalls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		toolCall := toolCalls[index]
		if toolCall.name == "" {
			continue
		}
		resultMessage.ToolCalls = append(resultMessage.ToolCalls, schema.ToolCall{
			ID:        toolCall.id,
			Name:      toolCall.name,
			Arguments: schema.NormalizeToolArguments([]byte(toolCall.arguments)),
		})
	}

	normalized := schema.NormalizeMessage(*resultMessage)
	return &GenerateResponse{Message: &normalized, Usage: usage}, receivedChunk, nil
}

func isOpenAIStreamOptionsRejected(err error) bool {
	if err == nil || !IsStreamingUnsupported(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "stream_options") || strings.Contains(message, "stream options")
}

var _ StreamGenerator = (*OpenAIProvider)(nil)
