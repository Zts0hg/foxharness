package subagent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// OutcomeStatus identifies the terminal state of one ChildRun invocation.
type OutcomeStatus string

const (
	// OutcomeSucceeded indicates that the child produced a final report.
	OutcomeSucceeded OutcomeStatus = "succeeded"
	// OutcomeFailed indicates that an admitted child run failed after starting.
	OutcomeFailed OutcomeStatus = "failed"
	// OutcomeCancelled indicates parent cancellation or deadline expiry.
	OutcomeCancelled OutcomeStatus = "cancelled"
	// OutcomeTurnExhausted indicates that the child consumed its turn budget.
	OutcomeTurnExhausted OutcomeStatus = "turn_exhausted"
	// OutcomeRejected indicates that invocation admission rejected the request.
	OutcomeRejected OutcomeStatus = "rejected"
	// OutcomeStartFailed indicates failure before an engine run was established.
	OutcomeStartFailed OutcomeStatus = "start_failed"
)

// OutcomeError retains a typed ChildRun outcome while preserving the original
// terminal error for errors.Is and errors.As callers.
type OutcomeError struct {
	Outcome *Result
	Err     error
}

// Error returns the original terminal error text for compatibility.
func (e *OutcomeError) Error() string {
	if e == nil || e.Err == nil {
		return "subagent failed"
	}
	return e.Err.Error()
}

// Unwrap exposes the original terminal cause.
func (e *OutcomeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// FormatFailureOutcome renders a non-success outcome without labeling partial
// assistant content as a successful final report.
func FormatFailureOutcome(outcome *Result, terminalErr error) string {
	if outcome == nil {
		if terminalErr == nil {
			return "Subagent failed without a correlated outcome."
		}
		return "Subagent failed: " + terminalErr.Error()
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Subagent Invocation: %s\nStatus: %s", outcome.InvocationID, outcome.Status)
	if outcome.SessionID != "" {
		fmt.Fprintf(&text, "\nSession: %s", outcome.SessionID)
	}
	if outcome.RunID != "" {
		fmt.Fprintf(&text, "\nRun: %s", outcome.RunID)
	}
	if outcome.Report != "" {
		fmt.Fprintf(&text, "\n\nPartial Report:\n%s", outcome.Report)
	}
	if terminalErr != nil {
		fmt.Fprintf(&text, "\n\nError:\n%s", terminalErr.Error())
	}
	return text.String()
}

func newChildOutcome(req Request) *Result {
	agent := AgentID(strings.TrimSpace(string(req.Agent)))
	if agent == "" {
		agent = AgentGeneralPurpose
	}
	return &Result{
		InvocationID:    newChildInvocationID(),
		ParentSessionID: req.ParentSessionID,
		ParentRunID:     req.ParentRunID,
		DelegationID:    req.DelegationID,
		Agent:           agent,
		Depth:           req.Depth,
		Status:          OutcomeStartFailed,
	}
}

func newChildInvocationID() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err == nil {
		return "child-" + hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("child-%d", time.Now().UnixNano())
}
