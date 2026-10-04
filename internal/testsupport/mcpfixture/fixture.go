/* Package mcpfixture provides hermetic MCP and scripted-model acceptance fixtures. */
package mcpfixture

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

/* Fixture records actual calls and active sessions. */
type Fixture struct {
	Calls    atomic.Int32
	protocol *mcp.Server
	Image    []byte
	Server   *httptest.Server
}

/* Start configures an isolated project-scoped HTTP fixture serving PNG tool results. */
func Start(t testing.TB, work string) *Fixture {
	t.Helper()
	f := &Fixture{}
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	f.Image = b.Bytes()
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "screenshot", Description: "Capture a fixture screenshot", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		n := f.Calls.Add(1)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("page state %d", n)}, &mcp.ImageContent{MIMEType: "image/png", Data: f.Image}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	f.protocol = server
	f.Server = httptest.NewServer(handler)
	t.Cleanup(f.Server.Close)
	data, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"browser": map[string]any{"transport": "streamable-http", "url": f.Server.URL}}})
	dir := filepath.Join(work, ".foxharness")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return f
}

/* Model calls the discovered screenshot tool and checks hydrated image content on continuation. */
type Model struct {
	mu                       sync.Mutex
	Images, Denied, Requests int
	Disabled                 bool
}

func (m *Model) ImageInput() string {
	if m.Disabled {
		return "disabled"
	}
	return "enabled"
}
func (m *Model) ProviderProtocol() string { return "openai" }
func (m *Model) ModelName() string        { return "fixture" }
func (m *Model) Generate(_ context.Context, messages []schema.Message, defs []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(defs) == 0 {
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "memory"}}, nil
	}
	m.Requests++
	last := messages[len(messages)-1]
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].ToolCallID != "" {
			last = messages[i]
			break
		}
		if messages[i].Role == schema.RoleUser && messages[i].Content == "capture" {
			break
		}
	}

	if last.ToolCallID != "" {
		for _, p := range last.ContentParts {
			if p.Type == "image" && len(p.Data) > 0 {
				m.Images++
			}
		}
		if last.IsError || strings.Contains(last.Content, "denied") {
			m.Denied++
		}
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "MCP complete"}}, nil
	}
	for _, d := range defs {
		if strings.HasPrefix(d.Name, "mcp_browser_screenshot_") {
			return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: fmt.Sprintf("mcp-%d", m.Requests), Name: d.Name, Arguments: json.RawMessage(`{}`)}}}}, nil
		}
	}
	return nil, fmt.Errorf("MCP screenshot tool missing from actual model request")
}

/* Counts returns race-safe observation totals. */
func (m *Model) Counts() (int, int) { m.mu.Lock(); defer m.mu.Unlock(); return m.Images, m.Denied }

/* Active counts actual negotiated sessions rather than attempted HTTP requests. */
func (f *Fixture) Active() int {
	n := 0
	for range f.protocol.Sessions() {
		n++
	}
	return n
}

/* Messenger resolves the actual remote approval callback synchronously in tests. */
type Messenger struct {
	Store    *approval.Store
	Approved bool
	Requests atomic.Int32
}

func (m *Messenger) SendText(_ context.Context, _ string, text string) error {
	_, id, ok := strings.Cut(text, "ApprovalID: ")
	if !ok {
		return nil
	}
	m.Requests.Add(1)
	return m.Store.Resolve(strings.TrimSpace(id), approval.Result{Approved: m.Approved})
}
