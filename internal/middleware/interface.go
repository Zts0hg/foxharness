// Package middleware defines the interception layer for tool execution in the
// foxharness agent framework. Middleware implementations inspect tool calls
// before execution and return a Decision to allow or deny them, enabling
// approval workflows, policy enforcement, and risk classification.
package middleware

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// DecisionType enumerates the possible outcomes of a middleware check.
type DecisionType string

const (
	DecisionAllow DecisionType = "allow"
	DecisionDeny  DecisionType = "deny"
)

// Decision represents the result of a middleware check, indicating whether a
// tool call may proceed and, if denied, the reason for rejection.
type Decision struct {
	Type   DecisionType
	Reason string
}

// Middleware inspects a tool call before it is executed and returns a Decision.
// Implementations may use the context for cancellation or to interact with
// external approval systems.
type Middleware interface {
	BeforeExecute(ctx context.Context, call schema.ToolCall) (Decision, error)
}

// Allow returns a Decision that permits the tool call to proceed.
func Allow() Decision {
	return Decision{Type: DecisionAllow}
}

// Deny returns a Decision that blocks the tool call, with the given reason
// surfaced to the agent.
func Deny(reason string) Decision {
	return Decision{Type: DecisionDeny, Reason: reason}
}
