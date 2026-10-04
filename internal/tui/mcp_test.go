package tui

import (
	"context"
	"github.com/Zts0hg/foxharness/internal/app"
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

type fakeMCP struct{ command app.MCPCommand }

func TestBareMCPShowsStatusThroughSharedPort(t *testing.T) {
	f := &fakeMCP{}
	m := NewModel(context.Background(), &fakeRunner{workDir: t.TempDir()}, Config{MCP: f})
	next, cmd := m.handleSlashCommand("/mcp")
	if cmd == nil {
		t.Fatal("bare /mcp did not request status")
	}
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if f.command.Action != "status" || !entriesContain(m.entries, "command", "management complete") {
		t.Fatal("bare /mcp status missing")
	}
}

func (f *fakeMCP) ExecuteMCP(ctx context.Context, c app.MCPCommand, p app.MCPLoginPrompt) (app.MCPResult, error) {
	f.command = c
	if c.Action == "login" {
		if err := p(ctx, "http://localhost/authorize"); err != nil {
			return app.MCPResult{}, err
		}
	}
	return app.MCPResult{Message: "management complete"}, nil
}
func TestMCPCommandUsesSharedPortAndDisplaysLogin(t *testing.T) {
	f := &fakeMCP{}
	m := NewModel(context.Background(), &fakeRunner{workDir: t.TempDir()}, Config{MCP: f})
	next, cmd := m.handleSlashCommand("/mcp login browser")
	m = next.(Model)
	if cmd == nil {
		t.Fatal("no management command")
	}
	result := cmd()
	url := <-m.events
	next, _ = m.Update(url)
	m = next.(Model)
	next, _ = m.Update(result)
	m = next.(Model)
	if f.command.Name != "browser" || !entriesContain(m.entries, "command", "http://localhost/authorize") || !entriesContain(m.entries, "command", "management complete") {
		t.Fatal("management/login result not displayed")
	}
	if m.mcpCancel != nil {
		t.Fatal("operation left pending")
	}
}

type cancellableMCP struct{ started chan struct{} }

func (f cancellableMCP) ExecuteMCP(ctx context.Context, _ app.MCPCommand, _ app.MCPLoginPrompt) (app.MCPResult, error) {
	close(f.started)
	<-ctx.Done()
	return app.MCPResult{}, ctx.Err()
}
func TestMCPCancelEndsPendingManagementAndShowsFailure(t *testing.T) {
	f := cancellableMCP{started: make(chan struct{})}
	m := NewModel(context.Background(), &fakeRunner{workDir: t.TempDir()}, Config{MCP: f})
	next, cmd := m.handleSlashCommand("/mcp login browser")
	m = next.(Model)
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	<-f.started
	next, _ = m.handleSlashCommand("/mcp cancel")
	m = next.(Model)
	next, _ = m.Update(<-done)
	m = next.(Model)
	if m.mcpCancel != nil || !entriesContain(m.entries, "error", "canceled") {
		t.Fatal("cancellation lost")
	}
}
func TestMCPNewSessionExplainsBrowserStateBoundary(t *testing.T) {
	m := NewModel(context.Background(), &fakeRunner{workDir: t.TempDir()}, Config{MCP: &fakeMCP{}})
	next, _ := m.Update(newSessionFinishedMsg{state: m.runner.State()})
	m = next.(Model)
	if !entriesContain(m.entries, "command", "browser state") {
		t.Fatal("new session silently lost browser state")
	}
}
