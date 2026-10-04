/* Package mcpconfig resolves scoped MCP definitions and atomic configuration edits. */
package mcpconfig

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

/* Scope identifies the explicitly selected configuration file. */
type Scope string

const (
	/* UserScope addresses the user's MCP definitions. */
	UserScope Scope = "user"
	/* ProjectScope addresses definitions in the resolved working directory. */
	ProjectScope Scope = "project"
)

/* Auth describes authentication inputs; OAuth tokens are never stored here. */
type Auth struct {
	Type              string   `json:"type,omitempty"`
	TokenEnv          string   `json:"token_env,omitempty"`
	ClientID          string   `json:"client_id,omitempty"`
	ClientSecretEnv   string   `json:"client_secret_env,omitempty"`
	ClientMetadataURL string   `json:"client_metadata_url,omitempty"`
	Scopes            []string `json:"scopes,omitempty"`
	RedirectPort      int      `json:"redirect_port,omitempty"`
}

/* Definition is a complete server configuration, never field-merged across scopes. */
type Definition struct {
	Transport             string            `json:"transport"`
	Enabled               *bool             `json:"enabled,omitempty"`
	Command               string            `json:"command,omitempty"`
	Args                  []string          `json:"args,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	EnvFrom               map[string]string `json:"env_from,omitempty"`
	URL                   string            `json:"url,omitempty"`
	Headers               map[string]string `json:"headers,omitempty"`
	HeadersFromEnv        map[string]string `json:"headers_from_env,omitempty"`
	Auth                  Auth              `json:"auth,omitempty"`
	StartupTimeoutSeconds float64           `json:"startup_timeout_seconds,omitempty"`
	CallTimeoutSeconds    float64           `json:"call_timeout_seconds,omitempty"`
}

/* IsEnabled reports the default-on enablement of a definition. */
func (d Definition) IsEnabled() bool { return d.Enabled == nil || *d.Enabled }

/* StartupTimeout returns the validated startup deadline duration. */
func (d Definition) StartupTimeout() time.Duration {
	return time.Duration(d.StartupTimeoutSeconds * float64(time.Second))
}

/* CallTimeout returns the validated call and queue deadline duration. */
func (d Definition) CallTimeout() time.Duration {
	return time.Duration(d.CallTimeoutSeconds * float64(time.Second))
}

/* Server preserves effective origin, definition, and an isolated validation failure. */
type Server struct {
	Name       string
	Scope      Scope
	ScopeDir   string
	Definition Definition
	Error      string
}

/* Snapshot is a resolved configuration generation independent of future edits. */
type Snapshot struct {
	Servers map[string]Server
	Errors  []string
}

/* Clone returns a deep copy of a snapshot. */
func (s Snapshot) Clone() Snapshot {
	data, _ := json.Marshal(s)
	var result Snapshot
	_ = json.Unmarshal(data, &result)
	return result
}

/* Redacted returns display-safe configuration with secret-bearing values removed. */
func (s Snapshot) Redacted() Snapshot {
	copy := s.Clone()
	for name, server := range copy.Servers {
		for key := range server.Definition.Env {
			server.Definition.Env[key] = "[redacted]"
		}
		for key := range server.Definition.Headers {
			server.Definition.Headers[key] = "[redacted]"
		}
		server.Definition.Args = nil
		if u, err := url.Parse(server.Definition.URL); err == nil {
			u.User = nil
			u.RawQuery = ""
			u.Fragment = ""
			server.Definition.URL = u.String()
		}
		copy.Servers[name] = server
	}
	return copy
}

/* Load reads both scopes, replaces same-name definitions, and resolves secret references. */
func Load(home, work string, lookup func(string) string) Snapshot {
	if lookup == nil {
		lookup = os.Getenv
	}
	snap := Snapshot{Servers: make(map[string]Server)}
	for _, scope := range []Scope{UserScope, ProjectScope} {
		dir := home
		if scope == ProjectScope {
			dir = work
		}
		document, err := readDocument(filepath.Join(dir, ".foxharness", "mcp.json"))
		if err != nil {
			snap.Errors = append(snap.Errors, fmt.Sprintf("%s MCP configuration: %s", scope, err))
			continue
		}
		for name, raw := range document.servers {
			server := Server{Name: name, Scope: scope, ScopeDir: dir}
			if err := json.Unmarshal(raw, &server.Definition); err != nil {
				server.Error = "invalid server definition"
			} else if err := resolve(&server.Definition, lookup); err != nil {
				server.Error = err.Error()
			}
			if strings.TrimSpace(name) == "" {
				server.Error = "server name is empty"
			}
			snap.Servers[name] = server
		}
	}
	return snap
}

func resolve(d *Definition, lookup func(string) string) error {
	if d.StartupTimeoutSeconds == 0 {
		d.StartupTimeoutSeconds = 30
	}
	if d.CallTimeoutSeconds == 0 {
		d.CallTimeoutSeconds = 120
	}
	if d.StartupTimeoutSeconds <= 0 || d.CallTimeoutSeconds <= 0 || d.StartupTimeoutSeconds > 1e9 || d.CallTimeoutSeconds > 1e9 {
		return fmt.Errorf("timeouts must be positive bounded seconds")
	}
	switch d.Transport {
	case "stdio":
		if strings.TrimSpace(d.Command) == "" || strings.ContainsRune(d.Command, 0) {
			return fmt.Errorf("stdio command is required")
		}
	case "http", "streamable-http", "sse":
		u, err := url.Parse(d.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("remote URL must be HTTP(S) without user credentials or fragment")
		}
	default:
		return fmt.Errorf("unsupported MCP transport")
	}
	if d.Env == nil {
		d.Env = make(map[string]string)
	}
	for key, ref := range d.EnvFrom {
		value := lookup(ref)
		if value == "" {
			return fmt.Errorf("required environment reference is unavailable")
		}
		d.Env[key] = value
	}
	for key, value := range d.Env {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return fmt.Errorf("invalid process environment")
		}
	}
	headers := make(map[string]string)
	for key, value := range d.Headers {
		canonical := http.CanonicalHeaderKey(key)
		if canonical == "" || strings.ContainsAny(key, " \t\r\n:") || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("invalid HTTP header")
		}
		if _, exists := headers[canonical]; exists {
			return fmt.Errorf("duplicate HTTP header")
		}
		headers[canonical] = value
	}
	for key, ref := range d.HeadersFromEnv {
		value := lookup(ref)
		if value == "" {
			return fmt.Errorf("required header reference is unavailable")
		}
		canonical := http.CanonicalHeaderKey(key)
		if canonical == "" || strings.ContainsAny(key, " \t\r\n:") || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("invalid HTTP header")
		}
		if _, exists := headers[canonical]; exists {
			return fmt.Errorf("duplicate HTTP header source")
		}
		headers[canonical] = value
	}
	if d.Auth.Type == "" {
		d.Auth.Type = "none"
	}
	switch d.Auth.Type {
	case "none", "headers":
	case "bearer":
		if headers["Authorization"] != "" {
			return fmt.Errorf("competing authorization sources")
		}
		token := lookup(d.Auth.TokenEnv)
		if token == "" || strings.ContainsAny(token, "\r\n") {
			return fmt.Errorf("bearer token reference is unavailable or invalid")
		}
		headers["Authorization"] = "Bearer " + token
	case "oauth":
		if headers["Authorization"] != "" {
			return fmt.Errorf("competing authorization sources")
		}
	default:
		return fmt.Errorf("unsupported MCP authentication type")
	}
	if d.Auth.RedirectPort < 0 || d.Auth.RedirectPort > 65535 {
		return fmt.Errorf("invalid OAuth redirect port")
	}
	d.Headers = headers
	return nil
}
