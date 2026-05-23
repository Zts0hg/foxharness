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
