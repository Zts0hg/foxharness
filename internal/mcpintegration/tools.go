package mcpintegration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
	"github.com/Zts0hg/foxharness/internal/tools"
)

/* ToolName maps an exact server/tool pair to a bounded provider-safe name. */
func ToolName(server, remote string) string { return nameFor("tool", server, remote) }

/* ResourceToolName identifies the authorized per-server linked-resource reader. */
func ResourceToolName(server string) string { return nameFor("resource", server, "read_resource") }

func nameFor(kind, server, remote string) string {
	tuple, _ := json.Marshal([]string{kind, server, remote})
	sum := sha256.Sum256(tuple)
	prefix := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return '_'
	}, server+"_"+remote)
	if len(prefix) > 42 {
		prefix = prefix[:42]
	}
	return "mcp_" + prefix + "_" + hex.EncodeToString(sum[:8])
}

/* Mount validates all identities before registering one immutable tool generation. */
func Mount(registry tools.Registry, services foxruntime.SessionServices) error {
	var host *Host
	switch s := services.(type) {
	case nil:
		return nil
	case *Host:
		host = s
	case *borrowed:
		host = s.host
	default:
		return fmt.Errorf("unknown external tool service")
	}
	host.mu.RLock()
	defer host.mu.RUnlock()
	var adapters []*adapter
	for server, entry := range host.servers {
		if entry.status.State != "ready" {
			continue
		}
		for _, definition := range entry.tools {
			copy := definition
			copy.Name = ToolName(server, definition.Name)
			adapters = append(adapters, &adapter{host: host, entry: entry, generation: entry.generation, definition: copy, remote: definition.Name})
		}
		adapters = append(adapters, &adapter{host: host, entry: entry, generation: entry.generation, resource: true, definition: schema.ToolDefinition{Name: ResourceToolName(server), Description: "Read a resource link returned by an authorized tool on this MCP server.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"uri": map[string]string{"type": "string"}}, "required": []string{"uri"}, "additionalProperties": false}}})
	}
	names := make(map[string]bool)
	for _, tool := range registry.GetAvailableTools() {
		names[tool.Name] = true
	}
	for _, tool := range adapters {
		if names[tool.Name()] {
			return fmt.Errorf("MCP tool name collision: %s", tool.Name())
		}
		names[tool.Name()] = true
	}
	for _, tool := range adapters {
		registry.Register(tool)
	}
	return nil
}

type adapter struct {
	host       *Host
	entry      *serverState
	generation uint64
	definition schema.ToolDefinition
	remote     string
	resource   bool
}

func (a *adapter) Name() string { return a.definition.Name }
func (a *adapter) Definition() schema.ToolDefinition {
	data, _ := json.Marshal(a.definition)
	var copy schema.ToolDefinition
	_ = json.Unmarshal(data, &copy)
	return copy
}
func (a *adapter) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	result, err := a.ExecuteResult(ctx, args)
	return result.Output, err
}
func (a *adapter) ExecuteResult(ctx context.Context, args json.RawMessage) (tools.ExecutionResult, error) {
	if !callerAllows(ctx, a.Name()) {
		return tools.ExecutionResult{}, fmt.Errorf("MCP capability is forbidden")
	}
	return a.host.invoke(ctx, a.entry, a.generation, a.Name(), a.remote, a.resource, args)
}
func (a *adapter) AssessPermission(toolpolicy.Context, json.RawMessage) (toolpolicy.Assessment, error) {
	return toolpolicy.Assessment{Behavior: toolpolicy.BehaviorHumanOnly, Action: a.Name(), Effects: []toolpolicy.Effect{toolpolicy.EffectUnknown}, Scope: toolpolicy.ScopeExternal, RiskHint: toolpolicy.RiskHigh, Reason: "External MCP tool effects require caller authorization; server metadata grants no permission"}, nil
}

func callerAllows(ctx context.Context, name string) bool {
	if !toolprotocol.HasCapabilities(ctx) {
		return true
	}
	for _, allowed := range toolprotocol.CapabilitiesFromContext(ctx) {
		if name == allowed {
			return true
		}
	}
	return false
}
