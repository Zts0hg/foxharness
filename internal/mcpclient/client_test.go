package mcpclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/mcpauth"
	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func fixtureServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	s.AddTool(&mcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(request.Params.Arguments)}}}, nil
	})
	s.AddTool(&mcp.Tool{Name: "wait", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	return s
}

func TestStdioFixtureProcess(t *testing.T) {
	if os.Getenv("FOX_MCP_DESCENDANT") == "1" {
		time.Sleep(250 * time.Millisecond)
		_ = os.WriteFile(os.Getenv("FOX_MCP_MARKER"), []byte("survived"), 0600)
		os.Exit(0)
	}
	if os.Getenv("FOX_MCP_FIXTURE") != "1" {
		return
	}
	switch os.Getenv("FOX_MCP_MODE") {
	case "stderr":
		_, _ = os.Stderr.Write([]byte(strings.Repeat("x", 65537)))
		time.Sleep(time.Hour)
	case "frame":
		_, _ = os.Stdout.Write([]byte(strings.Repeat("x", MaxFrameBytes+1)))
		time.Sleep(time.Hour)
	case "descendant":
		child := exec.Command(os.Args[0], "-test.run=^TestStdioFixtureProcess$")
		child.Env = append(os.Environ(), "FOX_MCP_DESCENDANT=1")
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		time.Sleep(time.Hour)
	}
	if err := fixtureServer().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestAllTransportsAndProtocolVersions(t *testing.T) {
	for _, transport := range []string{"stdio", "streamable-http", "sse"} {
		for _, version := range []string{"2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"} {
			t.Run(transport+"/"+version, func(t *testing.T) {
				definition := mcpconfig.Definition{Transport: transport, StartupTimeoutSeconds: 3, CallTimeoutSeconds: 1}
				if transport == "stdio" {
					definition.Command = os.Args[0]
					definition.Args = []string{"-test.run=^TestStdioFixtureProcess$"}
					definition.Env = map[string]string{"FOX_MCP_FIXTURE": "1"}
				} else {
					var handler http.Handler
					if transport == "sse" {
						handler = mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return fixtureServer() }, nil)
					} else {
						handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return fixtureServer() }, &mcp.StreamableHTTPOptions{Stateless: version == "2026-07-28"})
					}
					srv := httptest.NewServer(handler)
					defer srv.Close()
					definition.URL = srv.URL
				}
				startup, cancel := context.WithCancel(context.Background())
				session, err := Connect(startup, mcpconfig.Server{Name: "fixture", Definition: definition}, nil, Options{ProtocolVersion: version})
				if err != nil {
					t.Fatal(err)
				}
				defer session.Close()
				cancel()
				wantVersion := version
				if transport == "sse" && version == "2026-07-28" {
					wantVersion = "2025-11-25"
				}
				if session.ProtocolVersion() != wantVersion {
					t.Fatalf("negotiated %s", session.ProtocolVersion())
				}
				defs := session.Tools()
				if len(defs) != 2 {
					t.Fatalf("tools=%d", len(defs))
				}
				result, err := session.Call(context.Background(), "echo", json.RawMessage(`{"value":"ok"}`))
				if err != nil || !strings.Contains(result.Output, "ok") {
					t.Fatalf("call: %#v %v", result, err)
				}
				callCtx, stop := context.WithCancel(context.Background())
				stop()
				if _, err := session.Call(callCtx, "wait", json.RawMessage(`{}`)); err == nil {
					t.Fatal("cancelled call succeeded")
				}
				if _, err := session.Call(context.Background(), "echo", json.RawMessage(`{}`)); err != nil {
					t.Fatalf("call cancellation killed session: %v", err)
				}
				if err := session.Close(); err != nil {
					t.Fatal(err)
				}
				if err := session.Close(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestHTTPFallbackExcludesAuthorizationFailures(t *testing.T) {
	for _, status := range []int{400, 404, 405, 401, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			gets := 0
			sse := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return fixtureServer() }, nil)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && r.URL.RawQuery == "" {
					w.WriteHeader(status)
					return
				}
				if r.Method == "GET" {
					gets++
				}
				sse.ServeHTTP(w, r)
			}))
			defer srv.Close()
			session, err := Connect(context.Background(), mcpconfig.Server{Name: "fallback", Definition: mcpconfig.Definition{Transport: "http", URL: srv.URL, StartupTimeoutSeconds: 2, CallTimeoutSeconds: 1}}, nil, Options{})
			if status == 400 || status == 404 || status == 405 {
				if err != nil {
					t.Fatal(err)
				}
				session.Close()
				if gets != 1 {
					t.Fatalf("GETs=%d", gets)
				}
			} else {
				if err == nil {
					session.Close()
					t.Fatal("forbidden fallback succeeded")
				}
				if gets != 0 {
					t.Fatal("authorization/server failure switched transports")
				}
			}
		})
	}
}

func TestStartupTimeoutIsBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	start := time.Now()
	_, err := Connect(context.Background(), mcpconfig.Server{Name: "slow", Definition: mcpconfig.Definition{Transport: "sse", URL: srv.URL, StartupTimeoutSeconds: 0.03}}, nil, Options{})
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("startup not bounded: %v %v", err, time.Since(start))
	}
}

func TestPaginatedDiscoveryResourceReadAndUncertainCallNeverReplays(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		var request struct {
			ID     json.RawMessage
			Method string
			Params struct {
				Cursor          string
				URI             string
				ProtocolVersion string
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(400)
			return
		}
		if len(request.ID) == 0 {
			w.WriteHeader(202)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": request.Params.ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}, "resources": map[string]any{}}, "serverInfo": map[string]string{"name": "fixture", "version": "1"}}
		case "tools/list":
			if request.Params.Cursor == "" {
				result = map[string]any{"tools": []any{map[string]any{"name": "one", "inputSchema": map[string]any{"type": "object"}}}, "nextCursor": "two"}
			} else {
				result = map[string]any{"tools": []any{map[string]any{"name": "two", "inputSchema": map[string]any{"type": "object"}}}}
			}
		case "resources/read":
			if request.Params.URI != "test://page" {
				io.WriteString(w, `{"jsonrpc":"2.0","id":`+string(request.ID)+`,"error":{"code":-32002,"message":"resource not found"}}`)
				return
			}
			result = map[string]any{"contents": []any{map[string]any{"uri": "test://page", "mimeType": "text/plain", "text": "page body"}}}
		case "tools/call":
			calls.Add(1)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
			return
		default:
			io.WriteString(w, `{"jsonrpc":"2.0","id":`+string(request.ID)+`,"error":{"code":-32601,"message":"unknown method"}}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer srv.Close()
	s, err := Connect(context.Background(), mcpconfig.Server{Definition: mcpconfig.Definition{Transport: "streamable-http", URL: srv.URL, CallTimeoutSeconds: 0.1}}, nil, Options{ProtocolVersion: "2025-11-25"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(s.Tools()) != 2 {
		t.Fatal("pagination lost tools")
	}
	resource, err := s.ReadResource(context.Background(), "test://page")
	if err != nil || resource.ContentParts[0].URI != "test://page" || resource.Output != "page body" {
		t.Fatalf("read: %#v %v", resource, err)
	}
	if _, err := s.ReadResource(context.Background(), "test://missing"); err == nil {
		t.Fatal("unavailable resource advertised as success")
	}
	_, err = s.Call(context.Background(), "one", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "uncertain") || calls.Load() != 1 {
		t.Fatalf("uncertain call replayed: %d %v", calls.Load(), err)
	}
}

func TestStdioLimitsAndFailedStartupReapDescendants(t *testing.T) {
	for _, mode := range []string{"stderr", "frame", "descendant"} {
		t.Run(mode, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "survived")
			definition := mcpconfig.Definition{Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=^TestStdioFixtureProcess$"}, Env: map[string]string{"FOX_MCP_FIXTURE": "1", "FOX_MCP_MODE": mode, "FOX_MCP_MARKER": marker}, StartupTimeoutSeconds: 0.1}
			start := time.Now()
			_, err := Connect(context.Background(), mcpconfig.Server{Definition: definition}, nil, Options{})
			if err == nil || time.Since(start) > 2*time.Second {
				t.Fatalf("unbounded/accepted %s: %v", mode, err)
			}
			if mode == "descendant" {
				time.Sleep(300 * time.Millisecond)
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatal("descendant survived startup cleanup")
				}
			}
		})
	}
}

func TestMalformedDiscoveryAndUnsupportedProtocolAreRejected(t *testing.T) {
	for _, mode := range []string{"version", "duplicate", "cursor", "empty"} {
		t.Run(mode, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "DELETE" {
					w.WriteHeader(204)
					return
				}
				var q struct {
					ID     json.RawMessage
					Method string
				}
				if json.NewDecoder(r.Body).Decode(&q) != nil {
					w.WriteHeader(400)
					return
				}
				if len(q.ID) == 0 {
					w.WriteHeader(202)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				var result any
				if q.Method == "initialize" {
					version := "2025-11-25"
					if mode == "version" {
						version = "unknown"
					}
					result = map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "fixture", "version": "1"}}
				} else {
					tools := []any{map[string]any{"name": "one", "inputSchema": map[string]any{"type": "object"}}}
					switch mode {
					case "duplicate":
						tools = append(tools, tools[0])
					case "empty":
						tools = []any{map[string]any{"name": "", "inputSchema": map[string]any{"type": "object"}}}
					}
					result = map[string]any{"tools": tools}
					if mode == "cursor" {
						result.(map[string]any)["nextCursor"] = "again"
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": result})
			}))
			defer srv.Close()
			s, err := Connect(context.Background(), mcpconfig.Server{Definition: mcpconfig.Definition{Transport: "streamable-http", URL: srv.URL, StartupTimeoutSeconds: 1}}, nil, Options{})
			if s != nil {
				s.Close()
			}
			if err == nil {
				t.Fatal("invalid peer accepted")
			}
		})
	}
}
func TestCredentialsReachBothHTTPTransports(t *testing.T) {
	for _, transport := range []string{"streamable-http", "sse"} {
		t.Run(transport, func(t *testing.T) {
			var handler http.Handler
			if transport == "sse" {
				handler = mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return fixtureServer() }, nil)
			} else {
				handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return fixtureServer() }, nil)
			}
			var authorized atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-Key") != "key" {
					w.WriteHeader(401)
					return
				}
				authorized.Add(1)
				handler.ServeHTTP(w, r)
			}))
			defer srv.Close()
			definition := mcpconfig.Server{Definition: mcpconfig.Definition{Transport: transport, URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer secret", "X-Key": "key"}}}
			s, err := Connect(context.Background(), definition, mcpauth.NewManager(t.TempDir()).HTTPClient(definition), Options{ProtocolVersion: "2025-11-25"})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if _, err := s.Call(context.Background(), "echo", json.RawMessage(`{}`)); err != nil {
				t.Fatal(err)
			}
			if authorized.Load() < 3 {
				t.Fatal("credentials missing on protocol requests")
			}
		})
	}
}
