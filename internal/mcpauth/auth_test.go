package mcpauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
)

func TestOAuthLoginRefreshRestartLogoutAndScope(t *testing.T) {
	var endpoint, challenge string
	var refreshes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mcp":
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+endpoint+`/.well-known/oauth-protected-resource"`)
			w.WriteHeader(401)
		case "/.well-known/oauth-protected-resource":
			json.NewEncoder(w).Encode(map[string]any{"resource": endpoint + "/mcp", "authorization_servers": []string{endpoint}, "scopes_supported": []string{"tools"}})
		case "/.well-known/oauth-authorization-server":
			json.NewEncoder(w).Encode(map[string]any{"issuer": endpoint, "authorization_endpoint": endpoint + "/authorize", "token_endpoint": endpoint + "/token", "response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"S256"}, "authorization_response_iss_parameter_supported": true})
		case "/token":
			r.ParseForm()
			if r.Form.Get("grant_type") == "refresh_token" {
				refreshes.Add(1)
				if r.Form.Get("resource") != endpoint+"/mcp" {
					w.WriteHeader(400)
					json.NewEncoder(w).Encode(map[string]string{"error": "invalid_target"})
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"access_token": "refreshed", "refresh_token": "refresh2", "token_type": "Bearer", "expires_in": 3600})
				return
			}
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
				t.Error("PKCE not verified")
				w.WriteHeader(400)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "initial", "refresh_token": "refresh1", "token_type": "Bearer", "expires_in": 1})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	endpoint = srv.URL
	home := t.TempDir()
	server := mcpconfig.Server{Name: "service", Scope: mcpconfig.ProjectScope, ScopeDir: t.TempDir(), Definition: mcpconfig.Definition{URL: endpoint + "/mcp", Auth: mcpconfig.Auth{Type: "oauth", ClientID: "client", Scopes: []string{"tools"}}}}
	prompt := func(ctx context.Context, authorizationURL string) error {
		u, err := url.Parse(authorizationURL)
		if err != nil {
			return err
		}
		q := u.Query()
		challenge = q.Get("code_challenge")
		if challenge == "" || q.Get("code_challenge_method") != "S256" {
			t.Fatal("missing PKCE")
		}
		callback, _ := url.Parse(q.Get("redirect_uri"))
		query := callback.Query()
		query.Set("state", q.Get("state"))
		query.Set("code", "authorized")
		query.Set("iss", endpoint)
		callback.RawQuery = query.Encode()
		req, _ := http.NewRequestWithContext(ctx, "GET", callback.String(), nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		return err
	}
	m := NewManager(home)
	if err := m.Login(context.Background(), server, prompt); err != nil {
		t.Fatal(err)
	}
	if err := m.withStore(context.Background(), func(records map[string]credential) (bool, error) {
		if records[key(server)].Resource != endpoint+"/mcp" {
			t.Fatal("login did not persist the authorized resource")
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(home, ".foxharness", "mcp-credentials.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("credential permissions: %v %v", info, err)
	}
	restarted := NewManager(home)
	var wg sync.WaitGroup
	for n := 0; n < 5; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := restarted.Token(context.Background(), server)
			if err != nil || token != "refreshed" {
				t.Errorf("refresh: %q %v", token, err)
			}
		}()
	}
	wg.Wait()
	if refreshes.Load() != 1 {
		t.Fatalf("refresh calls=%d", refreshes.Load())
	}
	for _, change := range []func(*mcpconfig.Server){
		func(s *mcpconfig.Server) { s.ScopeDir = t.TempDir() },
		func(s *mcpconfig.Server) { s.Definition.URL += "/other" },
		func(s *mcpconfig.Server) { s.Definition.Auth.ClientID = "other" },
		func(s *mcpconfig.Server) { s.Definition.Auth.Scopes = []string{"other"} },
	} {
		other := server
		change(&other)
		if _, err := m.Token(context.Background(), other); err == nil {
			t.Fatal("reused another resource/client/scope token")
		}
	}
	if err := m.Logout(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Token(context.Background(), server); err == nil {
		t.Fatal("logout did not invalidate live source")
	}
}

func TestRefreshRetainsAuthorizedResourceAcrossRotationAndRestart(t *testing.T) {
	for _, rotate := range []bool{false, true} {
		t.Run(map[bool]string{false: "reuse", true: "rotate"}[rotate], func(t *testing.T) {
			home := t.TempDir()
			const resource = "https://resource.example/"
			refresh, calls := "refresh-1", 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.ParseForm()
				user, secret, ok := r.BasicAuth()
				if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("resource") != resource || r.Form.Get("refresh_token") != refresh || r.Form.Get("redirect_uri") != "" || !ok || user != "client" || secret != "client-secret" {
					t.Error("refresh lost authorized resource, token, or client authentication")
					w.WriteHeader(400)
					return
				}
				calls++
				response := map[string]any{"access_token": "updated", "token_type": "Bearer", "expires_in": 1}
				if rotate {
					refresh += "-rotated"
					response["refresh_token"] = refresh
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer srv.Close()
			server := mcpconfig.Server{Definition: mcpconfig.Definition{URL: resource + "mcp", Auth: mcpconfig.Auth{Type: "oauth", ClientID: "client"}}}
			record := credential{Resource: resource, Config: oauth2.Config{ClientID: "client", ClientSecret: "client-secret", RedirectURL: "http://127.0.0.1/old-callback", Endpoint: oauth2.Endpoint{TokenURL: srv.URL, AuthStyle: oauth2.AuthStyleInHeader}}, Token: oauth2.Token{AccessToken: "expired", RefreshToken: refresh, Expiry: time.Now().Add(-time.Hour)}}
			if err := NewManager(home).save(context.Background(), key(server), record); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if token, err := NewManager(home).Token(context.Background(), server); err != nil || token != "updated" {
					t.Fatalf("refresh: %q %v", token, err)
				}
			}
			if calls != 2 {
				t.Fatalf("refresh requests=%d", calls)
			}
		})
	}
}

func TestBothHTTPTransportsUseResourceBoundRefreshedCredentials(t *testing.T) {
	for _, transport := range []string{"streamable-http", "sse"} {
		t.Run(transport, func(t *testing.T) {
			home := t.TempDir()
			var endpoint string
			var refreshes, calls atomic.Int32
			server := mcp.NewServer(&mcp.Implementation{Name: "oauth", Version: "1"}, nil)
			server.AddTool(&mcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				calls.Add(1)
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "authorized"}}}, nil
			})
			var protocol http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
			if transport == "sse" {
				protocol = mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return server }, nil)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					r.ParseForm()
					if r.Form.Get("resource") != endpoint+"/mcp" || r.Form.Get("refresh_token") != "refresh" {
						w.WriteHeader(400)
						return
					}
					refreshes.Add(1)
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"access_token": "updated", "token_type": "Bearer", "expires_in": 3600})
					return
				}
				want := "Bearer initial"
				if refreshes.Load() > 0 {
					want = "Bearer updated"
				}
				if r.Header.Get("Authorization") != want {
					w.WriteHeader(401)
					return
				}
				protocol.ServeHTTP(w, r)
			}))
			defer srv.Close()
			endpoint = srv.URL
			definition := mcpconfig.Server{Definition: mcpconfig.Definition{URL: endpoint + "/mcp", Transport: transport, Auth: mcpconfig.Auth{Type: "oauth"}}}
			manager := NewManager(home)
			record := credential{Resource: endpoint + "/mcp", Config: oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{TokenURL: endpoint + "/token", AuthStyle: oauth2.AuthStyleInParams}}, Token: oauth2.Token{AccessToken: "initial", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}}
			ctx := context.Background()
			if err := manager.save(ctx, key(definition), record); err != nil {
				t.Fatal(err)
			}
			client := mcp.NewClient(&mcp.Implementation{Name: "oauth-test", Version: "1"}, nil)
			var wire mcp.Transport = &mcp.StreamableClientTransport{Endpoint: definition.Definition.URL, HTTPClient: NewManager(home).HTTPClient(definition), DisableStandaloneSSE: true, MaxRetries: -1}
			if transport == "sse" {
				wire = &mcp.SSEClientTransport{Endpoint: definition.Definition.URL, HTTPClient: NewManager(home).HTTPClient(definition)}
			}
			session, err := client.Connect(ctx, wire, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			record.Token.Expiry = time.Now().Add(-time.Hour)
			if err := manager.save(ctx, key(definition), record); err != nil {
				t.Fatal(err)
			}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{}})
			if err != nil || result.IsError || calls.Load() != 1 || refreshes.Load() != 1 {
				t.Fatalf("refresh consumer: %+v %v calls=%d refreshes=%d", result, err, calls.Load(), refreshes.Load())
			}
		})
	}
}

func TestUnattendedAndStaticCredentials(t *testing.T) {
	m := NewManager(t.TempDir())
	server := mcpconfig.Server{Definition: mcpconfig.Definition{URL: "http://localhost/mcp", Auth: mcpconfig.Auth{Type: "oauth"}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.Login(ctx, server, nil); err == nil || !strings.Contains(err.Error(), "interaction") {
		t.Fatalf("unattended login: %v", err)
	}
	if _, err := m.Token(ctx, server); err == nil || !strings.Contains(err.Error(), "login") {
		t.Fatalf("unattended token: %v", err)
	}
	for _, authHeader := range []string{"", "Bearer static"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != authHeader {
				t.Error("wrong static credential")
			}
			io.WriteString(w, "ok")
		}))
		server.Definition = mcpconfig.Definition{URL: srv.URL, Headers: map[string]string{"Authorization": authHeader}}
		resp, err := m.HTTPClient(server).Get(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		srv.Close()
	}
}

func TestRefreshFailureAndStoreFailureAreExplicit(t *testing.T) {
	home := t.TempDir()
	m := NewManager(home)
	server := mcpconfig.Server{Name: "s", Definition: mcpconfig.Definition{URL: "http://localhost/mcp", Auth: mcpconfig.Auth{Type: "oauth"}}}
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"error":"invalid_grant","error_description":"secret-do-not-leak"}`)
	}))
	defer srv.Close()
	record := credential{Config: oauth2.Config{ClientID: "c", Endpoint: oauth2.Endpoint{TokenURL: srv.URL}}, Token: oauth2.Token{AccessToken: "expired", RefreshToken: "r", Expiry: time.Now().Add(-time.Hour)}}
	if err := m.save(ctx, key(server), record); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Token(ctx, server); err == nil || strings.Contains(err.Error(), "secret-do-not-leak") || !strings.Contains(err.Error(), "login") {
		t.Fatalf("unsafe refresh error: %v", err)
	}
	if err := os.Mkdir(filepath.Join(home, ".foxharness", "mcp-credentials.json.lock"), 0700); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithCancel(ctx)
	cancel()
	if err := m.Logout(short, server); err == nil {
		t.Fatal("ignored cancelled lock")
	}
}

func TestInvalidOAuthCallbacksAndCancellationDoNotStoreCredentials(t *testing.T) {
	for _, mode := range []string{"state", "issuer", "denied", "metadata", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			var endpoint, callbackURL string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/mcp":
					w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+endpoint+`/.well-known/oauth-protected-resource"`)
					w.WriteHeader(401)
				case "/.well-known/oauth-protected-resource":
					if mode == "metadata" {
						io.WriteString(w, "invalid")
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"resource": endpoint + "/mcp", "authorization_servers": []string{endpoint}})
				case "/.well-known/oauth-authorization-server":
					json.NewEncoder(w).Encode(map[string]any{"issuer": endpoint, "authorization_endpoint": endpoint + "/authorize", "token_endpoint": endpoint + "/token", "response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"S256"}, "authorization_response_iss_parameter_supported": true})
				case "/token":
					t.Error("invalid authorization reached token endpoint")
					w.WriteHeader(400)
				default:
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			endpoint = srv.URL
			home := t.TempDir()
			m := NewManager(home)
			server := mcpconfig.Server{Definition: mcpconfig.Definition{URL: endpoint + "/mcp", Auth: mcpconfig.Auth{Type: "oauth", ClientID: "c"}}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := m.Login(ctx, server, func(ctx context.Context, link string) error {
				u, _ := url.Parse(link)
				q := u.Query()
				callbackURL = q.Get("redirect_uri")
				if mode == "cancel" {
					cancel()
					return nil
				}
				callback, _ := url.Parse(callbackURL)
				params := url.Values{"state": {q.Get("state")}, "iss": {endpoint}, "code": {"c"}}
				if mode == "state" {
					params.Set("state", "wrong")
				}
				if mode == "issuer" {
					params.Set("iss", "https://wrong.invalid")
				}
				if mode == "denied" {
					params.Set("error", "access_denied")
				}
				callback.RawQuery = params.Encode()
				resp, err := http.Get(callback.String())
				if err == nil {
					resp.Body.Close()
				}
				return err
			})
			if err == nil {
				t.Fatal("invalid login succeeded")
			}
			if _, err := os.Stat(filepath.Join(home, ".foxharness", "mcp-credentials.json")); !os.IsNotExist(err) {
				t.Fatal("invalid flow persisted credentials")
			}
			if callbackURL != "" {
				client := &http.Client{Timeout: time.Second}
				resp, err := client.Get(callbackURL)
				if err == nil {
					resp.Body.Close()
					t.Fatal("callback listener leaked")
				}
			}
		})
	}
}

func TestCredentialsNeverFollowCrossOriginRedirect(t *testing.T) {
	var reached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer source.Close()
	server := mcpconfig.Server{Definition: mcpconfig.Definition{URL: source.URL, Headers: map[string]string{"Authorization": "Bearer secret"}}}
	resp, err := NewManager(t.TempDir()).HTTPClient(server).Get(source.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || reached.Load() {
		t.Fatal("credentials followed cross-origin redirect")
	}
}
