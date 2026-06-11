// Package tui — autodev_command.go implements the /autodev builtin. The
// actual orchestrator lives behind the Config.Autodev launcher injected by
// internal/app, keeping the tui → autodev dependency one-way (Decision 2).
package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// handleAutodevCommand dispatches the /autodev builtin: it launches the
// orchestrator in a tea.Cmd goroutine with a TUIReporter feeding the
// session area, sharing the run lifecycle (running flag, cancel, events)
// with normal prompt runs (REQ-025, REQ-026).
func (m Model) handleAutodevCommand(backlogPath string) (tea.Model, tea.Cmd) {
	if m.autodevLauncher == nil {
		m.appendEntry("error", "autodev unavailable",
			"autodev is not wired up in this surface; launch the TUI through `fox` to use /autodev.", true)
		m.status = "autodev unavailable"
		return m, nil
	}
	if m.running {
		m.status = "Cannot start autodev while a run is active"
		return m, nil
	}

	runCtx, cancel := context.WithCancel(m.ctx)
	m.cancelRun = cancel
	m.running = true
	m.runStartedAt = m.nowTime()
	m.spinnerFrame = 0
	m.status = "autodev running"
	m.appendCommandEntry("Autodev", "Draining the backlog autonomously. Watch the stream below; Ctrl+C cancels.")

	launcher := m.autodevLauncher
	events := m.events
	return m, func() tea.Msg {
		err := launcher(runCtx, backlogPath, NewTUIReporter(events))
		return runFinishedMsg{err: err}
	}
}
