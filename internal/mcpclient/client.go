/* Package mcpclient isolates MCP transports, discovery, and rich result conversion. */
package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"github.com/Zts0hg/foxharness/internal/processtree"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

/* MaxFrameBytes caps a protocol frame or server-sent event. */
const MaxFrameBytes = 32 * 1024 * 1024

/* Options selects a supported handshake version; empty negotiates from the latest SDK version. */
type Options struct{ ProtocolVersion string }

/* Session owns one negotiated connection and any local process tree. */
type Session struct {
	client   *mcp.ClientSession
	cancel   context.CancelFunc
	tree     processtree.Tree
	wait     <-chan error
	tools    []schema.ToolDefinition
	timeout  time.Duration
	once     sync.Once
	closeErr error
}

/* Connect negotiates and fully discovers a server under its startup deadline. */
func Connect(ctx context.Context, server mcpconfig.Server, httpClient *http.Client, options Options) (*Session, error) {
	if server.Error != "" {
		return nil, fmt.Errorf("MCP server configuration is invalid: %s", server.Error)
	}
	if !server.Definition.IsEnabled() {
		return nil, fmt.Errorf("MCP server is disabled")
	}
	d := server.Definition
	startup := d.StartupTimeout()
	if startup <= 0 {
		startup = 30 * time.Second
	}
	callTimeout := d.CallTimeout()
	if callTimeout <= 0 {
		callTimeout = 120 * time.Second
	}
	deadline, stopDeadline := context.WithTimeout(ctx, startup)
	defer stopDeadline()
	owner, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stopCancel := context.AfterFunc(deadline, cancel)
	s := &Session{cancel: cancel, timeout: callTimeout}
	failed := true
	defer func() {
		stopCancel()
		if failed {
			s.Close()
		}
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "foxharness", Version: "1"}, &mcp.ClientOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Capabilities: &mcp.ClientCapabilities{}})
	version := options.ProtocolVersion
	if version == "" {
		version = "2026-07-28"
	}
	var transport mcp.Transport
	var monitor *responseMonitor
	switch d.Transport {
	case "stdio":
		cmd := exec.Command(d.Command, d.Args...)
		cmd.Dir = server.ScopeDir
		env := map[string]string{}
		for _, entry := range os.Environ() {
			key, value, ok := strings.Cut(entry, "=")
			if ok {
				env[key] = value
			}
		}
		for key, value := range d.Env {
			env[key] = value
		}
		for key, value := range env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("MCP stdout pipe unavailable")
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			stdout.Close()
			return nil, fmt.Errorf("MCP stdin pipe unavailable")
		}
		cmd.Stderr = &boundedStderr{limit: 64 * 1024, cancel: cancel}
		tree, err := processtree.Start(cmd)
		if err != nil {
			stdout.Close()
			stdin.Close()
			return nil, fmt.Errorf("MCP server process failed to start")
		}
		s.tree = tree
		done := make(chan error, 1)
		s.wait = done
		go func() { done <- cmd.Wait() }()
		transport = &mcp.IOTransport{Reader: stdout, Writer: stdin, MaxLineLength: MaxFrameBytes}
	case "http", "streamable-http", "sse":
		if httpClient == nil {
			httpClient = &http.Client{}
		}
		copy := *httpClient
		base := copy.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		monitor = &responseMonitor{base: base}
		copy.Transport = monitor
		if d.Transport == "sse" {
			transport = &mcp.SSEClientTransport{Endpoint: d.URL, HTTPClient: &copy, MaxEventSize: MaxFrameBytes}
		} else {
			transport = &mcp.StreamableClientTransport{Endpoint: d.URL, HTTPClient: &copy, MaxRetries: -1, DisableStandaloneSSE: true, MaxEventSize: MaxFrameBytes}
		}
	default:
		return nil, fmt.Errorf("unsupported MCP transport")
	}
	var err error
	s.client, err = client.Connect(owner, transport, &mcp.ClientSessionOptions{ProtocolVersion: version})
	if err != nil && d.Transport == "http" && monitor.legacyFallback() && owner.Err() == nil {
		copy := *httpClient
		copy.Transport = monitor
		s.client, err = client.Connect(owner, &mcp.SSEClientTransport{Endpoint: d.URL, HTTPClient: &copy, MaxEventSize: MaxFrameBytes}, &mcp.ClientSessionOptions{ProtocolVersion: version})
	}
	if err != nil {
		return nil, connectionError(deadline, monitor)
	}
	seen := map[string]bool{}
	cursor := ""
	names := map[string]bool{}
	for {
		page, err := s.client.ListTools(owner, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("MCP tool discovery failed")
		}
		for _, tool := range page.Tools {
			if tool == nil || tool.Name == "" || names[tool.Name] {
				return nil, fmt.Errorf("MCP discovery returned invalid or duplicate tools")
			}
			names[tool.Name] = true
			s.tools = append(s.tools, schema.ToolDefinition{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
		if seen[cursor] || len(seen) >= 10000 {
			return nil, fmt.Errorf("MCP tool pagination does not terminate")
		}
		seen[cursor] = true
	}
	if deadline.Err() != nil {
		return nil, connectionError(deadline, monitor)
	}
	sort.Slice(s.tools, func(i, j int) bool { return s.tools[i].Name < s.tools[j].Name })
	if !stopCancel() && owner.Err() != nil {
		return nil, fmt.Errorf("MCP startup cancelled")
	}
	failed = false
	return s, nil
}

/* ProtocolVersion returns the negotiated protocol version. */
func (s *Session) ProtocolVersion() string { return s.client.InitializeResult().ProtocolVersion }

/* Tools returns a defensive copy of the fully discovered catalog. */
func (s *Session) Tools() []schema.ToolDefinition {
	data, _ := json.Marshal(s.tools)
	var result []schema.ToolDefinition
	_ = json.Unmarshal(data, &result)
	return result
}

/* Call invokes a tool exactly once and distinguishes tool failure from transport uncertainty. */
func (s *Session) Call(ctx context.Context, name string, arguments json.RawMessage) (toolprotocol.ExecutionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return toolprotocol.ExecutionResult{}, err
	}
	result, err := s.client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return toolprotocol.ExecutionResult{}, fmt.Errorf("MCP tool call failed; side effects may be uncertain, call was not replayed")
	}
	return convertResult(result), nil
}

/* ReadResource reads through the existing connection; callers must authorize the URI. */
func (s *Session) ReadResource(ctx context.Context, uri string) (toolprotocol.ExecutionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	result, err := s.client.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		return toolprotocol.ExecutionResult{}, fmt.Errorf("MCP resource read failed")
	}
	content := make([]mcp.Content, 0, len(result.Contents))
	for _, resource := range result.Contents {
		content = append(content, &mcp.EmbeddedResource{Resource: resource})
	}
	return convertResult(&mcp.CallToolResult{Content: content}), nil
}

/* Close terminates the owned connection and process tree with a five-second cleanup bound. */
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			var err error
			if s.client != nil {
				err = s.client.Close()
			}
			s.cancel()
			if s.tree != nil {
				err = errors.Join(err, s.tree.Close(time.Second))
			}
			if s.wait != nil {
				select {
				case <-s.wait:
				case <-ctx.Done():
					err = errors.Join(err, fmt.Errorf("MCP process was not reaped"))
				}
			}
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				s.closeErr = fmt.Errorf("MCP connection cleanup failed")
			}
		case <-ctx.Done():
			s.closeErr = fmt.Errorf("MCP connection cleanup timed out")
		}
		s.cancel()
		if ctx.Err() != nil && s.tree != nil {
			go s.tree.Close(time.Second)
		}
	})
	return s.closeErr
}

type boundedStderr struct {
	mu          sync.Mutex
	size, limit int
	cancel      context.CancelFunc
}

func (b *boundedStderr) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.size += len(data)
	if b.size > b.limit {
		b.cancel()
		return 0, fmt.Errorf("MCP stderr limit exceeded")
	}
	return len(data), nil
}
