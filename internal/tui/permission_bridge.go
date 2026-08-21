package tui

import (
	"context"
	"strconv"

	"github.com/Zts0hg/foxharness/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

type permissionRequest struct {
	approval app.PermissionRequest
	reply    chan app.PermissionResponse
}

type permissionUserMsg struct {
	req permissionRequest
}

type permissionReviewMsg struct {
	status string
}

// permissionAutoApprovedMsg records an auto-reviewer approval so the TUI can
// leave a persistent transcript note, mirroring codex's approval-decision cell.
type permissionAutoApprovedMsg struct {
	action string
}

type permissionStateChangedMsg struct{}

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

// RequestPermission implements app.PermissionPort.
func (b *PermissionBridge) RequestPermission(ctx context.Context, request app.PermissionRequest) (app.PermissionResponse, error) {
	req := permissionRequest{approval: request, reply: make(chan app.PermissionResponse, 1)}
	select {
	case b.requests <- req:
	case <-ctx.Done():
		return app.PermissionResponse{}, ctx.Err()
	}
	select {
	case decision := <-req.reply:
		return decision, nil
	case <-ctx.Done():
		return app.PermissionResponse{}, ctx.Err()
	}
}

// NotifyInteraction maps application interaction progress onto the TUI loop.
func (b *PermissionBridge) NotifyInteraction(_ context.Context, notice app.InteractionNotice) {
	switch notice.Kind {
	case app.InteractionPermissionReviewStarted:
		b.send(permissionReviewMsg{status: "Reviewing permission: " + notice.ToolName})
	case app.InteractionPermissionReviewRetry:
		b.send(permissionReviewMsg{status: "Retrying permission review (attempt " + strconv.Itoa(notice.Attempt) + ")"})
	case app.InteractionPermissionAutoApproved:
		b.send(permissionAutoApprovedMsg{action: notice.Action})
	case app.InteractionPermissionEscalated:
		b.send(permissionReviewMsg{status: "Permission review escalated: " + notice.ToolName})
	case app.InteractionPermissionStateChanged:
		b.send(permissionStateChangedMsg{})
	}
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

var _ app.PermissionPort = (*PermissionBridge)(nil)
var _ app.InteractionNoticeSink = (*PermissionBridge)(nil)

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
