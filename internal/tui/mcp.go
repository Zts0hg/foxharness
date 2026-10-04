package tui

import (
	"context"
	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/mcpcmd"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

type mcpURLMsg string
type mcpResultMsg struct {
	result app.MCPResult
	err    error
}

func (m Model) handleMCP(text string) (tea.Model, tea.Cmd) {
	args, err := mcpcmd.Split(strings.TrimSpace(strings.TrimPrefix(text, "/mcp")))
	if err != nil {
		m.appendEntry("error", "MCP", err.Error(), true)
		return m, nil
	}
	if len(args) == 1 && args[0] == "cancel" {
		if m.mcpCancel != nil {
			m.mcpCancel()
		}
		m.appendCommandEntry("MCP", "Cancellation requested.")
		return m, nil
	}
	if m.mcpManager == nil {
		m.appendEntry("error", "MCP", "MCP management is unavailable.", true)
		return m, nil
	}
	if m.mcpCancel != nil {
		m.appendEntry("error", "MCP", "An MCP operation is pending; use /mcp cancel.", true)
		return m, nil
	}
	command, err := mcpcmd.Parse(args)
	if err != nil {
		m.appendEntry("error", "MCP", err.Error(), true)
		return m, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.mcpCancel = cancel
	m.appendCommandEntry("MCP", "Running "+command.Action+"; /mcp cancel stops this operation.")
	return m, func() tea.Msg {
		defer cancel()
		result, err := m.mcpManager.ExecuteMCP(ctx, command, func(ctx context.Context, url string) error {
			select {
			case m.events <- mcpURLMsg(url):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		return mcpResultMsg{result, err}
	}
}
