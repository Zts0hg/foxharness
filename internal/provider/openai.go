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

type OpenAIProvider struct {
	client openai.Client
	model  string
}

func NewZhipuOpenAIProvider(model string) *OpenAIProvider {
	apiKey := os.Getenv("ZHIPU_API_KEY")
	if apiKey == "" {
		panic("请设置 ZHIPU_API_AKY 环境变量")
	}
	baseUrl := "https://open.bigmodel.cn/api/coding/paas/v4"

	return &OpenAIProvider{
		client: openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseUrl)),
		model:  model,
	}
}

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

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("OpenAI/Zhipu API 请求失败: %w", err)
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
