package mcpauth

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

/* LoginPrompt presents an authorization URL through an explicitly available interaction. */
type LoginPrompt func(context.Context, string) error

/* Login performs a bounded authorization-code flow with an owned loopback callback. */
func (m *Manager) Login(ctx context.Context, server mcpconfig.Server, prompt LoginPrompt) error {
	if prompt == nil {
		return fmt.Errorf("OAuth login requires an available interaction to present the authorization URL")
	}
	if server.Definition.Auth.Type != "oauth" {
		return fmt.Errorf("server does not use OAuth")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", server.Definition.Auth.RedirectPort))
	if err != nil {
		return fmt.Errorf("OAuth callback listener unavailable")
	}
	defer listener.Close()
	redirect := "http://" + listener.Addr().String() + "/callback"
	type callback struct {
		result *auth.AuthorizationResult
		err    error
	}
	results := make(chan callback, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		q := r.URL.Query()
		item := callback{result: &auth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss")}}
		if q.Get("error") != "" || item.result.Code == "" {
			item.err = fmt.Errorf("authorization denied or callback missing code")
		}
		select {
		case results <- item:
			io.WriteString(w, "Authorization callback received. Return to Fox.")
		default:
			w.WriteHeader(409)
		}
	})
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer httpServer.Close()
	go httpServer.Serve(listener)
	config := &auth.AuthorizationCodeHandlerConfig{RedirectURL: redirect, Client: oauthClient(), RequestRefreshToken: true,
		AuthorizationCodeFetcher: func(fetchCtx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			if err := prompt(fetchCtx, args.URL); err != nil {
				return nil, fmt.Errorf("authorization interaction failed")
			}
			select {
			case <-fetchCtx.Done():
				return nil, fetchCtx.Err()
			case result := <-results:
				return result.result, result.err
			}
		},
		NewTokenSource: func(_ context.Context, cfg *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			if err := m.save(ctx, key(server), credential{Config: *cfg, Token: *token}); err != nil {
				return nil, err
			}
			return oauth2.StaticTokenSource(token), nil
		},
	}
	if len(server.Definition.Auth.Scopes) > 0 {
		config.ScopeFilter = func([]string) []string { return append([]string(nil), server.Definition.Auth.Scopes...) }
	}
	if server.Definition.Auth.ClientMetadataURL != "" {
		config.ClientIDMetadataDocumentConfig = &auth.ClientIDMetadataDocumentConfig{URL: server.Definition.Auth.ClientMetadataURL}
	}
	if server.Definition.Auth.ClientID != "" {
		config.PreregisteredClient = &oauthex.ClientCredentials{ClientID: server.Definition.Auth.ClientID}
		if ref := server.Definition.Auth.ClientSecretEnv; ref != "" {
			secret := os.Getenv(ref)
			if secret == "" {
				return fmt.Errorf("OAuth client secret reference is unavailable")
			}
			config.PreregisteredClient.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: secret}
		}
	}
	if config.PreregisteredClient == nil && config.ClientIDMetadataDocumentConfig == nil {
		config.DynamicClientRegistrationConfig = &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{ClientName: "Fox", RedirectURIs: []string{redirect}, GrantTypes: []string{"authorization_code", "refresh_token"}, TokenEndpointAuthMethod: "none"}}
	}
	handler, err := auth.NewAuthorizationCodeHandler(config)
	if err != nil {
		return fmt.Errorf("OAuth client configuration is invalid")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", server.Definition.URL, nil)
	if err != nil {
		return fmt.Errorf("invalid OAuth server endpoint")
	}
	resp, err := oauthClient().Do(req)
	if err != nil {
		return fmt.Errorf("OAuth server discovery failed")
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(strings.NewReader(""))
	challenges, err := oauthex.ParseWWWAuthenticate(resp.Header.Values("WWW-Authenticate"))
	if err != nil {
		return fmt.Errorf("invalid OAuth authorization challenge")
	}
	for _, challenge := range challenges {
		if metadataURL := challenge.Params["resource_metadata"]; metadataURL != "" {
			metadata, err := oauthex.GetProtectedResourceMetadata(ctx, metadataURL, server.Definition.URL, oauthClient())
			if err != nil || metadata == nil || len(metadata.AuthorizationServers) == 0 {
				return fmt.Errorf("invalid advertised OAuth resource metadata")
			}
		}
	}
	if resp.StatusCode != 401 && resp.StatusCode != 403 {
		resp.StatusCode = 401
	}
	if err := handler.Authorize(ctx, req, resp); err != nil {
		return fmt.Errorf("OAuth login failed; verify authorization, client registration, and server metadata")
	}
	return nil
}
