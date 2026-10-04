/* Package mcpintegration composes session-owned MCP tools with runtime capabilities. */
package mcpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/mcpauth"
	"github.com/Zts0hg/foxharness/internal/mcpclient"
	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
)

/* Client is the protocol behavior owned by a host's server generation. */
type Client interface {
	Tools() []schema.ToolDefinition
	ProtocolVersion() string
	Call(context.Context, string, json.RawMessage) (toolprotocol.ExecutionResult, error)
	ReadResource(context.Context, string) (toolprotocol.ExecutionResult, error)
	Close() error
}

/* Connector supplies a new independent connection for one resolved server. */
type Connector func(context.Context, mcpconfig.Server) (Client, error)

/* Status describes a server without exposing launch or authorization secrets. */
type Status struct {
	Name                               string
	Scope                              mcpconfig.Scope
	Transport, State, Protocol, Reason string
	ToolCount                          int
	Generation                         uint64
}

type serverState struct {
	gate       chan struct{}
	definition mcpconfig.Server
	client     Client
	generation uint64
	status     Status
	tools      []schema.ToolDefinition
	links      map[string]map[string]bool
}

/* Host owns independent root connections and serializes every server's operations. */
type Host struct {
	mu           sync.RWMutex
	home, work   string
	connector    Connector
	servers      map[string]*serverState
	closed       bool
	configErrors []string
	owner        context.Context
	cancel       context.CancelFunc
}

/* Factory creates session resources for both new and reopened runtime sessions. */
func Factory(home string) func(context.Context, foxruntime.AgentSessionSnapshot) (foxruntime.SessionServices, error) {
	return func(ctx context.Context, s foxruntime.AgentSessionSnapshot) (foxruntime.SessionServices, error) {
		h, err := New(ctx, home, s.WorkDir, nil)
		if h != nil {
			for _, status := range h.Statuses() {
				if status.State != "ready" && status.State != "disabled" {
					log.Printf("[MCP] %q: %s — %s", status.Name, status.State, status.Reason)
				}
			}
		}
		return h, err
	}
}

/* New resolves configuration once and isolates each connection failure in status. */
func New(ctx context.Context, home, work string, connector Connector) (*Host, error) {
	if connector == nil {
		auth := mcpauth.NewManager(home)
		connector = func(ctx context.Context, server mcpconfig.Server) (Client, error) {
			return mcpclient.Connect(ctx, server, auth.HTTPClient(server), mcpclient.Options{})
		}
	}
	owner, cancel := context.WithCancel(context.Background())
	h := &Host{owner: owner, cancel: cancel, home: home, work: work, connector: connector, servers: make(map[string]*serverState)}
	snapshot := mcpconfig.Load(home, work, nil)
	h.configErrors = append([]string(nil), snapshot.Errors...)
	names := make([]string, 0, len(snapshot.Servers))
	for name := range snapshot.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		server := snapshot.Servers[name]
		entry := &serverState{gate: make(chan struct{}, 1)}
		entry.gate <- struct{}{}
		h.servers[name] = entry
		h.connect(ctx, entry, server)
	}
	return h, nil
}

func (h *Host) connect(ctx context.Context, s *serverState, definition mcpconfig.Server) {
	s.definition = definition
	s.generation++
	s.links = make(map[string]map[string]bool)
	s.status = Status{Name: definition.Name, Scope: definition.Scope, Transport: definition.Definition.Transport, State: "connecting", Generation: s.generation}
	s.client = nil
	s.tools = nil
	if !definition.Definition.IsEnabled() {
		s.status.State = "disabled"
		return
	}
	if definition.Error != "" {
		s.status.State = "failed"
		s.status.Reason = definition.Error
		return
	}
	client, err := h.connector(ctx, definition)
	if err != nil {
		if client != nil {
			client.Close()
		}
		s.status.State = "failed"
		s.status.Reason = "MCP connection failed; verify configuration or reconnect"
		if strings.Contains(strings.ToLower(err.Error()), "auth") {
			s.status.State = "authorization-required"
			s.status.Reason = "Authorization required; run fox mcp login"
		}
		return
	}
	if client == nil {
		s.status.State = "failed"
		s.status.Reason = "MCP connector returned no connection"
		return
	}
	s.client = client
	s.tools = client.Tools()
	s.status.State = "ready"
	s.status.Protocol = client.ProtocolVersion()
	s.status.ToolCount = len(s.tools)
}

/* Statuses returns an ordered snapshot of actual connection state. */
func (h *Host) Statuses() []Status {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Status, 0, len(h.servers))
	for _, entry := range h.servers {
		out = append(out, entry.status)
	}
	for _, err := range h.configErrors {
		out = append(out, Status{Name: "configuration", State: "failed", Reason: err})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

/* Capabilities returns host-controlled names; remote annotations grant no read-only privilege. */
func (h *Host) Capabilities() []foxruntime.ExternalCapability {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []foxruntime.ExternalCapability
	for name, entry := range h.servers {
		if entry.status.State != "ready" {
			continue
		}
		for _, tool := range entry.tools {
			out = append(out, foxruntime.ExternalCapability{Name: ToolName(name, tool.Name)})
		}
		out = append(out, foxruntime.ExternalCapability{Name: ResourceToolName(name)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

/* Reconnect drains the selected server before replacing its configuration and generation. */
func (h *Host) Reconnect(ctx context.Context, name string) error {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(h.owner, cancel)
	defer stop()
	defer cancel()
	h.mu.RLock()
	entry := h.servers[name]
	closed := h.closed
	h.mu.RUnlock()
	if closed {
		return fmt.Errorf("MCP host is closed")
	}
	if entry == nil {
		h.mu.Lock()
		if h.closed {
			h.mu.Unlock()
			return fmt.Errorf("MCP host is closed")
		}
		entry = h.servers[name]
		if entry == nil {
			entry = &serverState{gate: make(chan struct{}, 1)}
			entry.gate <- struct{}{}
			h.servers[name] = entry
		}
		h.mu.Unlock()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-entry.gate:
	}
	defer func() { entry.gate <- struct{}{} }()
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return fmt.Errorf("MCP host is closed")
	}
	previous := entry.client
	next := &serverState{generation: entry.generation}
	entry.status.State = "connecting"
	h.mu.Unlock()
	if previous != nil {
		if err := previous.Close(); err != nil {
			h.mu.Lock()
			entry.status.Reason = "Previous connection cleanup failed"
			entry.status.State = "failed"
			h.mu.Unlock()
			return err
		}
	}
	snapshot := mcpconfig.Load(h.home, h.work, nil)
	definition, exists := snapshot.Servers[name]
	if !exists {
		disabled := false
		definition = mcpconfig.Server{Name: name, Definition: mcpconfig.Definition{Enabled: &disabled}}
	}
	h.connect(ctx, next, definition)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		if next.client != nil {
			next.client.Close()
		}
		return fmt.Errorf("MCP host closed during reconnect")
	}
	h.configErrors = append([]string(nil), snapshot.Errors...)
	entry.definition, entry.client, entry.generation = next.definition, next.client, next.generation
	entry.status, entry.tools, entry.links = next.status, next.tools, next.links
	entry.status.Reason += " Browser state may have been lost after reconnect."
	defer h.mu.Unlock()
	if entry.status.State == "failed" || entry.status.State == "authorization-required" {
		return fmt.Errorf("MCP reconnect failed: %s", entry.status.Reason)
	}
	return nil
}

/* Close cancels active operations and starts cleanup for every owned connection. */
func (h *Host) Close(ctx context.Context) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.cancel()
	var clients []Client
	for _, entry := range h.servers {
		if entry.client != nil {
			clients = append(clients, entry.client)
			entry.client = nil
		}
		entry.status.State = "closed"
	}
	h.mu.Unlock()
	done := make(chan error, len(clients))
	for _, client := range clients {
		go func(c Client) { done <- c.Close() }(client)
	}
	var failures []error
	for range clients {
		select {
		case err := <-done:
			failures = append(failures, err)
		case <-ctx.Done():
			return errors.Join(append(failures, ctx.Err())...)
		}
	}
	return errors.Join(failures...)
}

/* Borrow returns tools and capabilities without connection ownership or reconnect authority. */
func (h *Host) Borrow() foxruntime.SessionServices { return &borrowed{host: h} }

type borrowed struct{ host *Host }

func (b *borrowed) Capabilities() []foxruntime.ExternalCapability { return b.host.Capabilities() }
func (b *borrowed) Reconnect(context.Context, string) error {
	return fmt.Errorf("borrowed MCP services cannot reconnect")
}
func (b *borrowed) Close(context.Context) error { return nil }

func (h *Host) invoke(ctx context.Context, entry *serverState, generation uint64, name, remote string, resource bool, args json.RawMessage) (toolprotocol.ExecutionResult, error) {
	h.mu.RLock()
	timeout := entry.definition.Definition.CallTimeout()
	h.mu.RUnlock()
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stop := context.AfterFunc(h.owner, cancel)
	defer stop()
	select {
	case <-ctx.Done():
		return toolprotocol.ExecutionResult{}, ctx.Err()
	case <-entry.gate:
	}
	defer func() { entry.gate <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return toolprotocol.ExecutionResult{}, err
	}
	h.mu.RLock()
	closed := h.closed
	client := entry.client
	current := entry.generation
	h.mu.RUnlock()
	if closed || client == nil {
		return toolprotocol.ExecutionResult{}, fmt.Errorf("MCP server is unavailable")
	}
	if current != generation {
		return toolprotocol.ExecutionResult{}, fmt.Errorf("stale MCP tool adapter; rediscover tools after reconnect")
	}
	if resource {
		var request struct {
			URI string `json:"uri"`
		}
		if json.Unmarshal(args, &request) != nil {
			return toolprotocol.ExecutionResult{}, fmt.Errorf("invalid resource request")
		}
		origins := entry.links[request.URI]
		allowed := false
		for origin := range origins {
			if callerAllows(ctx, origin) {
				allowed = true
				break
			}
		}
		if !allowed {
			return toolprotocol.ExecutionResult{}, fmt.Errorf("resource link is unknown or originating tool is forbidden")
		}
		return client.ReadResource(ctx, request.URI)
	}
	result, err := client.Call(ctx, remote, args)
	if err != nil {
		if ctx.Err() == nil {
			h.mu.Lock()
			if !h.closed {
				entry.status.State = "failed"
				entry.status.Reason = "MCP call failed; side effects or browser state may be uncertain. No replay was attempted; reconnect explicitly."
			}
			h.mu.Unlock()
		}
		return result, err
	}
	for _, part := range result.ContentParts {
		if part.Type == "resource_link" {
			if entry.links[part.URI] == nil {
				entry.links[part.URI] = make(map[string]bool)
			}
			entry.links[part.URI][name] = true
		}
	}
	return result, nil
}
