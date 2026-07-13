package tui

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/permission"
	tea "github.com/charmbracelet/bubbletea"
)

type permissionRequest struct {
	approval permission.ApprovalRequest
	reply    chan permission.UserDecision
}

type permissionUserMsg struct {
	req permissionRequest
}

type permissionReviewMsg struct {
	status string
}

// PermissionBridge connects the permission coordinator to the TUI event loop.
type PermissionBridge struct {
	requests chan permissionRequest
	events   chan<- tea.Msg
}

// NewPermissionBridge creates an approval bridge.
func NewPermissionBridge() *PermissionBridge {
	return &PermissionBridge{requests: make(chan permissionRequest, 8)}
}

// SetEvents attaches the model event channel used for review status updates.
func (b *PermissionBridge) SetEvents(events chan<- tea.Msg) {
	if b != nil {
		b.events = events
	}
}

// Approve implements permission.UserApprover.
func (b *PermissionBridge) Approve(ctx context.Context, request permission.ApprovalRequest) (permission.UserDecision, error) {
	req := permissionRequest{approval: request, reply: make(chan permission.UserDecision, 1)}
	select {
	case b.requests <- req:
	case <-ctx.Done():
		return permission.UserDecision{}, ctx.Err()
	}
	select {
	case decision := <-req.reply:
		return decision, nil
	case <-ctx.Done():
		return permission.UserDecision{}, ctx.Err()
	}
}

// OnReviewStart reports transient automatic review progress.
func (b *PermissionBridge) OnReviewStart(request permission.Request) {
	b.send(permissionReviewMsg{status: "Reviewing permission: " + request.ToolName})
}

// OnReviewRetry reports transient reviewer retry progress.
func (b *PermissionBridge) OnReviewRetry(request permission.Request, attempt int) {
	b.send(permissionReviewMsg{status: "Retrying permission review"})
}

// OnAutoApproved reports an automatic approval.
func (b *PermissionBridge) OnAutoApproved(request permission.Request, result permission.ReviewResult) {
	b.send(permissionReviewMsg{status: "Auto-approved: " + request.ToolName + " (" + string(result.Risk) + ")"})
}

// OnEscalated reports escalation to user approval.
func (b *PermissionBridge) OnEscalated(request permission.Request, result permission.ReviewResult) {
	b.send(permissionReviewMsg{status: "Permission review escalated: " + request.ToolName})
}

func (b *PermissionBridge) send(msg tea.Msg) {
	if b == nil || b.events == nil {
		return
	}
	select {
	case b.events <- msg:
	default:
	}
}

func listenForPermissionRequest(ctx context.Context, bridge *PermissionBridge) tea.Cmd {
	return func() tea.Msg {
		if bridge == nil {
			return nil
		}
		select {
		case req := <-bridge.requests:
			return permissionUserMsg{req: req}
		case <-ctx.Done():
			return nil
		}
	}
}
