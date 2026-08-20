/*
Package modelinvoke adapts provider transports to the target engine's
run-scoped model invocation contract.
*/
package modelinvoke

import (
	"context"
	"errors"
	"reflect"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

/* Config supplies invocation lifecycle hooks selected by composition. */
type Config struct {
	OnSuccess       func()
	IsPromptTooLong func(error) bool
}

/* Invoker creates isolated target-engine model runs over one provider transport. */
type Invoker struct {
	provider provider.LLMProvider
	config   Config
}

/* New constructs a provider-backed target model invoker. */
func New(modelProvider provider.LLMProvider, config Config) *Invoker {
	return &Invoker{provider: modelProvider, config: config}
}

/* StartRun validates the transport and creates run-local invocation state. */
func (i *Invoker) StartRun(context.Context) (engine.ModelRunInvoker, error) {
	if i == nil || isNilProvider(i.provider) {
		return nil, errors.New("model invocation provider is required")
	}
	return &runInvoker{provider: i.provider, config: i.config}, nil
}

type runInvoker struct {
	provider provider.LLMProvider
	config   Config
}

func (i *runInvoker) Invoke(ctx context.Context, request engine.RunContext, _ engine.ModelFactEmitter) (engine.ModelResult, error) {
	messages := cloneMessages(request.Messages)
	definitions := cloneDefinitions(request.ToolDefinitions)
	var response *provider.GenerateResponse
	var err error
	if optionsProvider, ok := i.provider.(provider.OptionsGenerator); ok {
		response, err = optionsProvider.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{Effort: request.Effort})
	} else {
		response, err = i.provider.Generate(ctx, messages, definitions)
	}
	if err != nil {
		isPromptTooLong := i.config.IsPromptTooLong
		if isPromptTooLong == nil {
			isPromptTooLong = provider.IsPromptTooLong
		}
		if isPromptTooLong(err) {
			return engine.ModelResult{}, &promptTooLongError{cause: err}
		}
		return engine.ModelResult{}, err
	}
	if response == nil || response.Message == nil {
		return engine.ModelResult{}, errors.New("provider returned empty response")
	}
	message := cloneMessage(schema.NormalizeMessage(*response.Message))
	usage := response.Usage
	message.Usage = &usage
	finishReason := "stop"
	if len(message.ToolCalls) > 0 {
		finishReason = "tool_calls"
	}
	if i.config.OnSuccess != nil {
		i.config.OnSuccess()
	}
	return engine.ModelResult{Message: message, FinishReason: finishReason, Usage: response.Usage}, nil
}

type promptTooLongError struct {
	cause error
}

func (e *promptTooLongError) Error() string { return e.cause.Error() }
func (e *promptTooLongError) Unwrap() []error {
	return []error{engine.ErrPromptTooLong, e.cause}
}

func isNilProvider(value provider.LLMProvider) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func cloneMessages(messages []schema.Message) []schema.Message {
	result := make([]schema.Message, len(messages))
	for index, message := range messages {
		result[index] = cloneMessage(message)
	}
	return result
}

func cloneMessage(message schema.Message) schema.Message {
	message.ToolCalls = append([]schema.ToolCall(nil), message.ToolCalls...)
	for index := range message.ToolCalls {
		message.ToolCalls[index].Arguments = append([]byte(nil), message.ToolCalls[index].Arguments...)
	}
	if message.Usage != nil {
		usage := *message.Usage
		message.Usage = &usage
	}
	return message
}

func cloneDefinitions(definitions []schema.ToolDefinition) []schema.ToolDefinition {
	data := make([]schema.ToolDefinition, len(definitions))
	for index, definition := range definitions {
		definition.InputSchema = cloneJSONValue(definition.InputSchema)
		data[index] = definition
	}
	return data
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = cloneJSONValue(item)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index, item := range value {
			result[index] = cloneJSONValue(item)
		}
		return result
	case []string:
		return append([]string(nil), value...)
	case []byte:
		return append([]byte(nil), value...)
	default:
		return value
	}
}

var _ engine.ModelInvoker = (*Invoker)(nil)
