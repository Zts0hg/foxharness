package subagent

import (
	"context"
	"fmt"
	"strings"
)

/* AgentID is the normalized identity of a child agent definition. */
type AgentID string

const (
	/* AgentGeneralPurpose is the built-in ChildRun agent. */
	AgentGeneralPurpose AgentID = "general-purpose"
	/* DefaultMaxTurns is the immutable ChildRun turn ceiling. */
	DefaultMaxTurns = 200
)

type childAgent struct {
	id           AgentID
	persona      string
	allowedTools []string
}

func resolveAgent(raw AgentID) (childAgent, error) {
	id := AgentID(strings.TrimSpace(string(raw)))
	if id == "" {
		id = AgentGeneralPurpose
	}
	if id != AgentGeneralPurpose {
		return childAgent{}, fmt.Errorf("subagent: unknown agent %q", id)
	}
	return childAgent{id: id, persona: "通用编码执行代理，严格服从当前 ChildRun 的任务和能力边界。"}, nil
}

func cloneToolNames(names []string) []string {
	if names == nil {
		return nil
	}
	return append(make([]string, 0, len(names)), names...)
}

/* Request is the normalized invocation protocol accepted by Runner. */
type Request struct {
	ParentSessionID string
	ParentRunID     string
	DelegationID    string
	Task            string
	ReadOnly        bool
	Agent           AgentID
	Depth           int
	AllowedTools    []string
}

/* Result is the single typed terminal outcome for one ChildRun invocation. */
type Result struct {
	InvocationID    string
	SessionID       string
	RunID           string
	ParentSessionID string
	ParentRunID     string
	DelegationID    string
	Agent           AgentID
	Depth           int
	Status          OutcomeStatus
	Report          string
}

/* Agent describes one validated child persona and its optional capability ceiling. */
type Agent struct {
	ID           AgentID
	Instructions string
	AllowedTools []string
}

/* ResolveAgent validates a model-facing child selector without silently falling back. */
func ResolveAgent(raw AgentID) (Agent, error) {
	resolved, err := resolveAgent(raw)
	if err != nil {
		return Agent{}, err
	}
	return Agent{
		ID: resolved.id, Instructions: resolved.persona,
		AllowedTools: cloneToolNames(resolved.allowedTools),
	}, nil
}

/* NewInvocationID creates one adapter-owned child correlation identity. */
func NewInvocationID() string { return newChildInvocationID() }

/* Runner is the consumer-owned child execution capability used by invocation adapters. */
type Runner interface {
	Run(context.Context, Request) (*Result, error)
	DelegationAllowed() bool
	PermissionEnforced() bool
}
