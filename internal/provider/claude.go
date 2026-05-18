package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const zhipuClaudeBaseURL = "https://open.bigmodel.cn/api/paas/v4/"

// ClaudeProvider implements LLMProvider using the Anthropic Messages protocol.
// It can talk to Anthropic-compatible endpoints, including Zhipu's Claude-style
// compatibility endpoint.
type ClaudeProvider struct {
	client anthropic.Client
	model  string
	retry  RetryConfig
}

// NewZhipuClaudeProvider creates a ClaudeProvider configured for Zhipu's
// Anthropic-compatible endpoint.
func NewZhipuClaudeProvider(model string) *ClaudeProvider {
	apiKey := os.Getenv("ZHIPU_API_KEY")
	if apiKey == "" {
		panic("ZHIPU_API_KEY environment variable must be set")
	}
	retry := retryConfigFromEnv()
	clientOptions := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(zhipuClaudeBaseURL),
		option.WithMaxRetries(0),
	}
	if retry.RequestTimeout > 0 {
		clientOptions = append(clientOptions, option.WithRequestTimeout(retry.RequestTimeout))
	}

	return &ClaudeProvider{
		client: anthropic.NewClient(clientOptions...),
		model:  model,
		retry:  retry,
	}
}

// Generate translates foxharness messages/tools into Anthropic Messages API
// requests and normalizes text/tool_use response blocks back to schema.Message.
func (p *ClaudeProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	anthropicMessages, systemBlocks := toAnthropicMessages(messages)
	anthropicTools := toAnthropicTools(availableTools)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: 4096,
		Messages:  anthropicMessages,
	}
	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}
	if len(anthropicTools) > 0 {
		params.Tools = anthropicTools
	}

	resp, err := p.messagesNewWithRetry(ctx, params)
	if err != nil {
		return nil, err
	}

	result := &schema.Message{
		Role: schema.RoleAssistant,
	}
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, schema.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: schema.NormalizeToolArguments(block.Input),
			})
		}
	}

	normalized := schema.NormalizeMessage(*result)
	return &normalized, nil
}

func (p *ClaudeProvider) messagesNewWithRetry(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	retry := p.retry.normalized()
	var lastErr error
	for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
		resp, err := p.client.Messages.New(ctx, params)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !shouldRetryProviderError(ctx, err) || attempt == retry.MaxAttempts {
			break
		}

		delay := retry.delay(attempt)
		log.Printf("[Provider] Claude/Zhipu API request failed, retrying attempt %d/%d in %s: %v", attempt+1, retry.MaxAttempts, delay, err)
		if err := sleepWithContext(ctx, delay); err != nil {
			return nil, fmt.Errorf("Claude/Zhipu API 请求取消: %w", err)
		}
	}

	if retry.MaxAttempts > 1 && shouldRetryProviderError(ctx, lastErr) {
		return nil, fmt.Errorf("Claude/Zhipu API 请求失败（已尝试 %d 次）: %w", retry.MaxAttempts, lastErr)
	}
	return nil, fmt.Errorf("Claude/Zhipu API 请求失败: %w", lastErr)
}

func toAnthropicMessages(messages []schema.Message) ([]anthropic.MessageParam, []anthropic.TextBlockParam) {
	anthropicMessages := make([]anthropic.MessageParam, 0, len(messages))
	var systemBlocks []anthropic.TextBlockParam

	for _, msg := range messages {
		switch msg.Role {
		case schema.RoleSystem:
			if msg.Content != "" {
				systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: msg.Content})
			}
		case schema.RoleUser:
			if msg.ToolCallID != "" {
				anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(
					anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false),
				))
				continue
			}
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(msg.Content),
			))
		case schema.RoleAssistant:
			blocks := make([]anthropic.ContentBlockParamUnion, 0, 1+len(msg.ToolCalls))
			if msg.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
			}
			for _, call := range msg.ToolCalls {
				blocks = append(blocks, anthropic.NewToolUseBlock(call.ID, rawJSONToMap(call.Arguments), call.Name))
			}
			if len(blocks) > 0 {
				anthropicMessages = append(anthropicMessages, anthropic.NewAssistantMessage(blocks...))
			}
		}
	}

	return anthropicMessages, systemBlocks
}

func toAnthropicTools(tools []schema.ToolDefinition) []anthropic.ToolUnionParam {
	anthropicTools := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, toolDef := range tools {
		tool := anthropic.ToolParam{
			Name:        toolDef.Name,
			Description: anthropic.String(toolDef.Description),
			InputSchema: toAnthropicToolInputSchema(toolDef.InputSchema),
		}
		anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return anthropicTools
}

func toAnthropicToolInputSchema(inputSchema any) anthropic.ToolInputSchemaParam {
	schemaMap := map[string]any{}
	switch typed := inputSchema.(type) {
	case map[string]any:
		schemaMap = typed
	default:
		data, err := json.Marshal(inputSchema)
		if err == nil {
			_ = json.Unmarshal(data, &schemaMap)
		}
	}

	result := anthropic.ToolInputSchemaParam{}
	if properties, ok := schemaMap["properties"]; ok {
		result.Properties = properties
	}
	result.Required = stringSliceFromAny(schemaMap["required"])

	extra := make(map[string]any)
	for key, value := range schemaMap {
		switch key {
		case "type", "properties", "required":
			continue
		default:
			extra[key] = value
		}
	}
	if len(extra) > 0 {
		result.ExtraFields = extra
	}
	return result
}

func rawJSONToMap(raw json.RawMessage) map[string]any {
	raw = schema.NormalizeToolArguments(raw)
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil || input == nil {
		return map[string]any{}
	}
	return input
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

var _ LLMProvider = (*ClaudeProvider)(nil)
