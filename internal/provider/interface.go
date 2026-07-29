// Package provider provides an abstraction layer for LLM (Large Language Model) providers.
//
// It defines a common interface that different LLM backends can implement,
// allowing the foxharness engine to work with various providers (OpenAI-compatible,
// Anthropic, etc.) through a unified API.
//
// Key Components:
//   - LLMProvider: Interface for LLM generation with tool support
//
// The provider abstraction supports:
//   - Multi-turn conversations with message history
//   - Tool/function calling capabilities
//   - Context cancellation for long-running requests
package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// GenerateResponse wraps the assistant message produced by an LLM provider
// together with the token usage metadata reported by the underlying API.
// Usage carries zero-values when the provider does not surface usage data,
// so callers may safely access fields without nil checks.
type GenerateResponse struct {
	Message *schema.Message
	Usage   schema.Usage
}

// GenerateOptions contains optional per-call provider settings. Empty fields
// preserve the provider default behavior.
type GenerateOptions struct {
	Effort           string
	StructuredOutput *StructuredOutputOptions
}

/*
StreamCallbacks contains optional callbacks invoked while a streaming provider
builds the final assistant message.
*/
type StreamCallbacks struct {
	OnTextDelta func(delta string)
}

/*
EmitTextDelta sends a text delta to the caller when a delta callback is
configured.
*/
func (c StreamCallbacks) EmitTextDelta(delta string) {
	if c.OnTextDelta == nil || delta == "" {
		return
	}
	c.OnTextDelta(delta)
}

// ErrEmptyStream indicates that a streaming request completed without yielding
// any stream events. This usually means a compatible endpoint ignored streaming
// and returned a non-SSE response.
var ErrEmptyStream = errors.New("empty streaming response")

// IsEmptyStream reports whether err indicates a streaming response ended before
// any stream event was received.
func IsEmptyStream(err error) bool {
	return errors.Is(err, ErrEmptyStream)
}

// IsStreamingUnsupported reports whether an API error looks like a provider or
// endpoint rejecting streaming-specific request options.
func IsStreamingUnsupported(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "stream") &&
		!strings.Contains(message, "sse") &&
		!strings.Contains(message, "event-stream") {
		return false
	}
	if statusCode, ok := errorStatusCode(err); ok {
		switch statusCode {
		case http.StatusBadRequest,
			http.StatusNotFound,
			http.StatusMethodNotAllowed,
			http.StatusNotAcceptable,
			http.StatusUnsupportedMediaType,
			http.StatusUnprocessableEntity,
			http.StatusNotImplemented:
		default:
			return false
		}
	}
	for _, token := range []string{
		"not support",
		"not implemented",
		"unsupported",
		"unknown",
		"unrecognized",
		"invalid",
		"not allowed",
	} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

// StructuredOutputOptions requests provider-native JSON schema constrained
// output for a single generation call.
type StructuredOutputOptions struct {
	Name        string
	Description string
	Schema      map[string]any
	Strict      bool
}

// LLMProvider defines the interface for Large Language Model providers.
// Implementations can support various LLM backends (OpenAI, Anthropic, local models, etc.)
// while providing a consistent API for the engine.
//
// The Generate method should handle:
//   - Message history for conversational context
//   - Tool definitions for function calling
//   - Context cancellation for timely termination
type LLMProvider interface {
	// Generate produces a response from the LLM given the message history and available tools.
	//
	// The ctx parameter enables cancellation of long-running requests.
	// The messages parameter contains the conversation history including system, user, and assistant messages.
	// The availableTools parameter lists tools the LLM may invoke; empty means no tools available.
	//
	// Returns a GenerateResponse containing the LLM's response message and token
	// usage metadata. The message may include text content, tool calls, or both.
	// Returns an error if the generation fails.
	Generate(ctx context.Context, message []schema.Message, availableTools []schema.ToolDefinition) (*GenerateResponse, error)
}

// OptionsGenerator is implemented by providers that support explicit
// call-time options for user-run model calls.
type OptionsGenerator interface {
	GenerateWithOptions(ctx context.Context, message []schema.Message, availableTools []schema.ToolDefinition, options GenerateOptions) (*GenerateResponse, error)
}

/*
StreamGenerator is implemented by providers that can stream visible assistant
text while still returning the normalized final message and usage metadata.
*/
type StreamGenerator interface {
	GenerateStream(ctx context.Context, message []schema.Message, availableTools []schema.ToolDefinition, options GenerateOptions, callbacks StreamCallbacks) (*GenerateResponse, error)
}
