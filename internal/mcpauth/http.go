package mcpauth

import (
	"fmt"
	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"net/http"
	"net/url"
	"time"
)

func oauthClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && !sameOrigin(req.URL, via[0].URL) {
			return fmt.Errorf("cross-origin OAuth redirect denied")
		}
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}}
}

/* HTTPClient applies credentials only to the configured origin and never launches login. */
func (m *Manager) HTTPClient(server mcpconfig.Server) *http.Client {
	return &http.Client{Transport: credentialTransport{manager: m, server: server}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && !sameOrigin(req.URL, via[0].URL) {
			return fmt.Errorf("cross-origin MCP redirect denied")
		}
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}}
}

type credentialTransport struct {
	manager *Manager
	server  mcpconfig.Server
}

func (t credentialTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	origin, err := url.Parse(t.server.Definition.URL)
	if err != nil || !sameOrigin(origin, request.URL) {
		return nil, fmt.Errorf("MCP request origin is not authorized")
	}
	req := request.Clone(request.Context())
	for name, value := range t.server.Definition.Headers {
		req.Header.Set(name, value)
	}
	if t.server.Definition.Auth.Type == "oauth" {
		token, err := t.manager.Token(request.Context(), t.server)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func sameOrigin(a, b *url.URL) bool { return a.Scheme == b.Scheme && a.Host == b.Host }
