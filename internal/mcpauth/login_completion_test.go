package mcpauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/Zts0hg/foxharness/internal/mcpconfig"
)

func TestExplicitLoginDoesNotClaimSuccessOnPlainForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer srv.Close()
	m := NewManager(t.TempDir())
	s := mcpconfig.Server{Definition: mcpconfig.Definition{URL: srv.URL, Auth: mcpconfig.Auth{Type: "oauth", ClientID: "public-client"}}}
	called := false
	err := m.Login(context.Background(), s, func(context.Context, string) error { called = true; return nil })
	if err == nil {
		t.Fatalf("claimed login success without authorization; prompt=%v", called)
	}
}

func TestConfidentialLoginBindsConfiguredIssuer(t *testing.T) {
	for _, mode := range []string{"missing", "mismatch", "matched", "insufficient-scope"} {
		t.Run(mode, func(t *testing.T) {
			var endpoint string
			var requests, tokens atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/mcp":
					challenge := `Bearer resource_metadata="` + endpoint + `/.well-known/oauth-protected-resource"`
					status := 401
					if mode == "insufficient-scope" {
						challenge += `, error="insufficient_scope", scope="tools"`
						status = 403
					}
					w.Header().Set("WWW-Authenticate", challenge)
					w.WriteHeader(status)
				case "/.well-known/oauth-protected-resource":
					json.NewEncoder(w).Encode(map[string]any{"resource": endpoint + "/mcp", "authorization_servers": []string{endpoint}})
				case "/.well-known/oauth-authorization-server":
					json.NewEncoder(w).Encode(map[string]any{"issuer": endpoint, "authorization_endpoint": endpoint + "/authorize", "token_endpoint": endpoint + "/token", "response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"S256"}, "authorization_response_iss_parameter_supported": true})
				case "/token":
					tokens.Add(1)
					user, secret, ok := r.BasicAuth()
					if !ok || user != "client" || secret != "fixture-only-secret" {
						t.Error("missing client authentication")
					}
					json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-token", "token_type": "Bearer", "expires_in": 3600})
				default:
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			endpoint = srv.URL
			issuer := endpoint
			if mode == "missing" {
				issuer = ""
			}
			if mode == "mismatch" {
				issuer = "https://intended.example"
			}
			t.Setenv("FOX_TEST_CLIENT_SECRET", "fixture-only-secret")
			raw, _ := json.Marshal(map[string]any{"type": "oauth", "client_id": "client", "client_secret_env": "FOX_TEST_CLIENT_SECRET", "issuer": issuer})
			var a mcpconfig.Auth
			if err := json.Unmarshal(raw, &a); err != nil {
				t.Fatal(err)
			}
			server := mcpconfig.Server{Definition: mcpconfig.Definition{URL: endpoint + "/mcp", Auth: a}}
			prompts := 0
			m := NewManager(t.TempDir())
			err := m.Login(context.Background(), server, func(ctx context.Context, target string) error {
				prompts++
				u, err := url.Parse(target)
				if err != nil {
					return err
				}
				q := u.Query()
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
			})
			if mode == "missing" || mode == "mismatch" {
				if err == nil || prompts != 0 || tokens.Load() != 0 {
					t.Fatalf("unbound credentials accepted: err=%v prompts=%d tokens=%d", err, prompts, tokens.Load())
				}
				if mode == "missing" && requests.Load() != 0 {
					t.Fatal("missing issuer performed network discovery")
				}
				if _, err := m.Token(context.Background(), server); err == nil {
					t.Fatal("failed login stored credentials")
				}
				return
			}
			if err != nil || prompts != 1 || tokens.Load() != 1 {
				t.Fatalf("bound login: %v prompts=%d tokens=%d", err, prompts, tokens.Load())
			}
			if token, err := m.Token(context.Background(), server); err != nil || token != "fixture-token" {
				t.Fatalf("saved login: %q %v", token, err)
			}
			raw, _ = json.Marshal(map[string]any{"type": "oauth", "client_id": "client", "client_secret_env": "FOX_TEST_CLIENT_SECRET", "issuer": "https://other.example"})
			json.Unmarshal(raw, &server.Definition.Auth)
			if _, err := m.Token(context.Background(), server); err == nil {
				t.Fatal("changed issuer reused credentials")
			}
		})
	}
}
