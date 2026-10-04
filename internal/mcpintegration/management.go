package mcpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/mcpauth"
	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"time"
)

/* Manager implements management for a workspace and an optional active session. */
type Manager struct {
	home, work string
	current    func() *foxruntime.AgentSession
	Connector  Connector
}

/* NewManager uses current for TUI ownership; nil denotes an independent CLI process. */
func NewManager(home, work string, current func() *foxruntime.AgentSession) *Manager {
	return &Manager{home: home, work: work, current: current}
}

/* ExecuteMCP mutates only explicit scopes and never reconnects another process. */
func (m *Manager) ExecuteMCP(ctx context.Context, c app.MCPCommand, prompt app.MCPLoginPrompt) (result app.MCPResult, resultErr error) {
	if c.Action != "status" && c.Name == "" {
		return result, fmt.Errorf("MCP server name is required")
	}
	var err error
	switch c.Action {
	case "add":
		var d mcpconfig.Definition
		if err = json.Unmarshal(c.Definition, &d); err != nil {
			return result, fmt.Errorf("invalid MCP definition JSON")
		}
		err = mcpconfig.Add(m.home, m.work, mcpconfig.Scope(c.Scope), c.Name, d)
	case "remove":
		err = mcpconfig.Remove(m.home, m.work, mcpconfig.Scope(c.Scope), c.Name)
	case "enable", "disable":
		err = mcpconfig.SetEnabled(m.home, m.work, mcpconfig.Scope(c.Scope), c.Name, c.Action == "enable")
	case "status":
		var h *Host
		if m.current != nil {
			if s := m.current(); s != nil {
				h, _ = s.Services().(*Host)
			}
		}
		if h == nil {
			h, err = New(ctx, m.home, m.work, m.Connector)
			if err != nil {
				return result, err
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				resultErr = errors.Join(resultErr, h.Close(cleanup))
			}()
			result.Message = "MCP probe (temporary connections; project definitions replace same-named user definitions)."
		} else {
			result.Message = "Active session MCP connections (configuration edits apply after reconnect or a new session)."
		}
		for _, s := range h.Statuses() {
			result.Servers = append(result.Servers, app.MCPServerStatus{Name: s.Name, Scope: string(s.Scope), Transport: s.Transport, State: s.State, Protocol: s.Protocol, Reason: s.Reason, ToolCount: s.ToolCount, Generation: s.Generation})
		}
		return result, nil
	case "reconnect":
		if m.current == nil {
			return result, fmt.Errorf("reconnect requires /mcp reconnect in the active TUI; this CLI cannot reconnect another process")
		}
		s := m.current()
		if s == nil {
			return result, fmt.Errorf("no active session")
		}
		err = s.ReconnectServices(ctx, c.Name)
		result.Message = "MCP reconnected. Browser state may have been lost."
		return result, err
	case "login", "logout":
		snapshot := mcpconfig.Load(m.home, m.work, nil)
		s, ok := snapshot.Servers[c.Name]
		if !ok {
			return result, fmt.Errorf("MCP server not found")
		}
		if s.Error != "" {
			return result, fmt.Errorf("MCP configuration is invalid: %s", s.Error)
		}
		auth := mcpauth.NewManager(m.home)
		if c.Action == "login" {
			if prompt == nil {
				return result, fmt.Errorf("interactive login requires a URL prompt; run fox mcp login %s", c.Name)
			}
			err = auth.Login(ctx, s, mcpauth.LoginPrompt(prompt))
		} else {
			err = auth.Logout(ctx, s)
		}
		result.Message = "MCP " + c.Action + " completed; reconnect the active session to apply authorization changes."
		return result, err
	default:
		return result, fmt.Errorf("unknown MCP action")
	}
	result.Message = "MCP configuration updated in " + c.Scope + " scope. Project definitions replace same-named user definitions. Existing connections require explicit reconnect or a new session."
	return result, err
}
