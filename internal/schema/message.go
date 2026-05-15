// Package schema defines the shared data types used throughout the foxharness
// agent framework, including conversation messages, tool calls, tool results,
// and tool definitions. These types serve as the wire format between the
// engine, providers, and tool implementations.
package schema

import "encoding/json"

// Role enumerates the participant roles in a conversation message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message represents a single turn in the conversation history. Messages with
// a non-empty ToolCalls slice indicate assistant requests to invoke tools;
// messages with a non-empty ToolCallID carry tool execution results.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
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
