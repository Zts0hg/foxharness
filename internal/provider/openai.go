package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
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

	resp, err := p.chatCompletionWithRetry(ctx, params)
	if err != nil {
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

func (p *OpenAIProvider) chatCompletionWithRetry(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return chatCompletionWithRetry(ctx, p.client, params, p.retry)
}
