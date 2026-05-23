// Package schema defines the shared data types used throughout the foxharness
// agent framework, including conversation messages, tool calls, tool results,
// and tool definitions. These types serve as the wire format between the
// engine, providers, and tool implementations.
package schema

import (
	"bytes"
	"encoding/json"
)

// Role enumerates the participant roles in a conversation message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message represents a single turn in the conversation history. Messages with
// a non-empty ToolCalls slice indicate assistant requests to invoke tools;
// messages with a non-empty ToolCallID carry tool execution results. Usage
// is populated on assistant messages after the provider reports actual token
// consumption; it enables hybrid token counting without a separate tracking
// channel and is omitted from JSON when absent for backward compatibility.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Usage      *Usage     `json:"usage,omitempty"`
}

// Usage reports token consumption attributed to a single LLM API response.
// CacheCreationTokens and CacheReadTokens are zero for providers without
// prompt-caching support. Fields are int64 to match the wire types reported
// by upstream SDKs and to avoid silent truncation on 32-bit platforms.
type Usage struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int64 `json:"cache_read_tokens,omitempty"`
}

// ToolCall describes a single tool invocation requested by the assistant,
// including the call identifier, tool name, and JSON-encoded arguments.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult carries the output of a tool execution back to the conversation,
// correlated by ToolCallID. IsError distinguishes error responses from normal
// output.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error"`
}

// ToolDefinition describes a tool's name, human-readable description, and
// JSON Schema for its input parameters, used to advertise available tools to
// the LLM provider.
type ToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

// NormalizeMessage returns a copy with tool-call arguments guaranteed to be
// JSON-marshalable. Model providers can occasionally return an empty argument
// string for a tool call; encoding/json rejects an empty json.RawMessage.
func NormalizeMessage(msg Message) Message {
	for i := range msg.ToolCalls {
		msg.ToolCalls[i] = NormalizeToolCall(msg.ToolCalls[i])
	}
	return msg
}

// NormalizeToolCall returns a copy with valid JSON arguments. Invalid or empty
// arguments become an empty object so the engine can surface a normal tool
// validation error instead of crashing while persisting session history.
func NormalizeToolCall(call ToolCall) ToolCall {
	call.Arguments = NormalizeToolArguments(call.Arguments)
	return call
}

// NormalizeToolArguments makes a tool-call argument payload safe to marshal.
func NormalizeToolArguments(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return json.RawMessage(`{}`)
	}
	return append(json.RawMessage(nil), trimmed...)
}
