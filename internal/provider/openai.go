package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAIProvider implements the LLMProvider interface using the OpenAI API.
// It supports any OpenAI-compatible endpoint, including Zhipu AI's BigModel platform.
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

// NewZhipuOpenAIProvider creates an OpenAIProvider configured for Zhipu AI's BigModel platform.
//
// The model parameter specifies which model to use (e.g., "glm-4.5-air").
// Reads the ZHIPU_API_KEY environment variable for authentication.
// Panics if ZHIPU_API_KEY is not set.
//
// Returns a configured OpenAIProvider ready for use with the Zhipu API.
func NewZhipuOpenAIProvider(model string) *OpenAIProvider {
	apiKey := os.Getenv("ZHIPU_API_KEY")
	if apiKey == "" {
		panic("ZHIPU_API_KEY environment variable must be set")
	}
	baseUrl := "https://open.bigmodel.cn/api/coding/paas/v4"
	retry := retryConfigFromEnv()
	clientOptions := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseUrl),
		option.WithMaxRetries(0),
	}
	if retry.RequestTimeout > 0 {
		clientOptions = append(clientOptions, option.WithRequestTimeout(retry.RequestTimeout))
	}

	return &OpenAIProvider{
		client: openai.NewClient(clientOptions...),
		model:  model,
		retry:  retry,
	}
}

// Generate produces a response from the OpenAI-compatible API.
//
// The ctx parameter enables cancellation of long-running requests.
// The messages parameter contains the full conversation history.
// The availableTools parameter lists tools the LLM may invoke.
//
// Returns a schema.Message with the LLM's response, including any tool calls,
// or an error if the API request fails or returns an empty response.
func (p *OpenAIProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
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
				Arguments: []byte(toolCall.Function.Arguments),
			})
		}

	}

	return resultMessage, nil
}

func (p *OpenAIProvider) chatCompletionWithRetry(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return chatCompletionWithRetry(ctx, p.client, params, p.retry)
}
