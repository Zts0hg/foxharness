package app

import (
	"context"
	"encoding/json"
)

/* MCPCommand carries an explicit management action without transport dependencies. */
type MCPCommand struct {
	Action, Name, Scope string
	Definition          json.RawMessage
}

/* MCPServerStatus is a credential-free state snapshot for presentation. */
type MCPServerStatus struct {
	Name, Scope, Transport, State, Protocol, Reason string
	ToolCount                                       int
	Generation                                      uint64
}

/* MCPResult distinguishes a temporary probe from an active session snapshot. */
type MCPResult struct {
	Message string
	Servers []MCPServerStatus
}

/* MCPLoginPrompt presents an explicit login URL; cancellation uses the operation context. */
type MCPLoginPrompt func(context.Context, string) error

/* MCPManager is the presentation-owned management port. */
type MCPManager interface {
	ExecuteMCP(context.Context, MCPCommand, MCPLoginPrompt) (MCPResult, error)
}
