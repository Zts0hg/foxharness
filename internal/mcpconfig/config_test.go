package mcpconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".foxharness"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".foxharness", "mcp.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestScopedConfigWholeOverridesAndSnapshots(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	empty := Load(home, work, nil)
	if len(empty.Servers) != 0 || len(empty.Errors) != 0 {
		t.Fatalf("missing config: %#v", empty)
	}
	writeConfig(t, home, `{"mcpServers":{"same":{"transport":"stdio","command":"user","env":{"SECRET":"do-not-leak"}},"other":{"transport":"stdio","command":"other"}}}`)
	writeConfig(t, work, `{"mcpServers":{"same":{"transport":"stdio","command":"project","enabled":false}}}`)
	snap := Load(home, work, nil)
	if len(snap.Servers) != 2 || snap.Servers["same"].Definition.Command != "project" || snap.Servers["same"].Definition.IsEnabled() || len(snap.Servers["same"].Definition.Env) != 0 || snap.Servers["same"].Scope != ProjectScope {
		t.Fatalf("whole override failed: %#v", snap)
	}
	if snap.Servers["other"].Definition.StartupTimeoutSeconds != 30 || snap.Servers["other"].Definition.CallTimeoutSeconds != 120 {
		t.Fatal("missing timeout defaults")
	}
	writeConfig(t, work, `{"mcpServers":{}}`)
	if snap.Servers["same"].Definition.Command != "project" {
		t.Fatal("snapshot changed after edit")
	}
	copy := snap.Clone()
	d := copy.Servers["other"]
	d.Definition.Args = append(d.Definition.Args, "mutated")
	copy.Servers["other"] = d
	if len(snap.Servers["other"].Definition.Args) != 0 {
		t.Fatal("snapshot aliases copy")
	}
}

func TestInvalidDefinitionsAndSecretsAreIsolated(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	writeConfig(t, home, `{"mcpServers":{
		"ok":{"transport":"stdio","command":"server","env_from":{"TOKEN":"TOKEN"}},
		"bad-shape":1,"transport":{"transport":"bogus"},"command":{"transport":"stdio"},
		"url":{"transport":"http","url":"file:///secret"},"timeout":{"transport":"stdio","command":"s","call_timeout_seconds":-1},
		"secret":{"transport":"http","url":"https://example.invalid/mcp","headers_from_env":{"Authorization":"MISSING"}},
		"oauth-conflict":{"transport":"http","url":"https://example.invalid/mcp","headers":{"authorization":"do-not-leak"},"auth":{"type":"oauth"}}
	}}`)
	snap := Load(home, work, func(name string) string {
		if name == "TOKEN" {
			return "do-not-leak"
		}
		return ""
	})
	if snap.Servers["ok"].Error != "" || snap.Servers["ok"].Definition.Env["TOKEN"] != "do-not-leak" {
		t.Fatal("healthy server unavailable")
	}
	for name, server := range snap.Servers {
		if name != "ok" && server.Error == "" {
			t.Fatalf("accepted invalid %s", name)
		}
	}
	encoded, _ := json.Marshal(snap.Redacted())
	if strings.Contains(string(encoded), "do-not-leak") {
		t.Fatalf("secret display leak: %s", encoded)
	}
}

func TestScopedMutationsPreserveOthersAndRejectInvalidFiles(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	d := Definition{Transport: "stdio", Command: "server", Args: []string{"--flag"}}
	if err := Add(home, work, ProjectScope, "one", d); err != nil {
		t.Fatal(err)
	}
	if err := Add(home, work, ProjectScope, "two", d); err != nil {
		t.Fatal(err)
	}
	if err := Add(home, work, ProjectScope, "one", d); err == nil {
		t.Fatal("overwrote existing server")
	}
	if err := SetEnabled(home, work, ProjectScope, "one", false); err != nil {
		t.Fatal(err)
	}
	if err := SetEnabled(home, work, ProjectScope, "one", true); err != nil {
		t.Fatal(err)
	}
	if err := Remove(home, work, ProjectScope, "one"); err != nil {
		t.Fatal(err)
	}
	if got := Load(home, work, nil); len(got.Servers) != 1 || got.Servers["two"].Error != "" {
		t.Fatalf("unrelated mutation: %#v", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".foxharness", "mcp.json")); !os.IsNotExist(err) {
		t.Fatal("wrong scope written")
	}
	writeConfig(t, work, "broken-json")
	if err := Add(home, work, ProjectScope, "three", d); err == nil {
		t.Fatal("overwrote malformed JSON")
	}
	data, _ := os.ReadFile(filepath.Join(work, ".foxharness", "mcp.json"))
	if string(data) != "broken-json" {
		t.Fatal("malformed file changed")
	}
	if len(Load(home, work, nil).Errors) != 1 {
		t.Fatal("malformed scope hidden")
	}
	if err := Add(home, work, "", "bad", d); err == nil {
		t.Fatal("implicit mutation scope")
	}
}

func TestMutationLockAndInvalidNewDefinition(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	if err := Add(home, work, ProjectScope, "bad", Definition{Transport: "stdio"}); err == nil {
		t.Fatal("invalid definition accepted")
	}
	writeConfig(t, work, `{"mcpServers":{}}`)
	if err := os.Mkdir(filepath.Join(work, ".foxharness", "mcp.json.lock"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := Add(home, work, ProjectScope, "one", Definition{Transport: "stdio", Command: "s"}); err == nil {
		t.Fatal("concurrent writer ignored lock")
	}
	if len(Load(home, work, nil).Servers) != 0 {
		t.Fatal("locked mutation changed config")
	}
}

func TestOAuthIssuerConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, auth string
		invalid    bool
	}{
		{"public", `{"type":"oauth","client_id":"client"}`, false},
		{"confidential", `{"type":"oauth","client_id":"client","client_secret_env":"SECRET","issuer":"https://issuer.example/tenant"}`, false},
		{"missing-issuer", `{"type":"oauth","client_id":"client","client_secret_env":"SECRET"}`, true},
		{"missing-client", `{"type":"oauth","client_secret_env":"SECRET","issuer":"https://issuer.example"}`, true},
		{"unbound-issuer", `{"type":"oauth","issuer":"https://issuer.example"}`, true},
		{"relative", `{"type":"oauth","client_id":"client","issuer":"/issuer"}`, true},
		{"userinfo", `{"type":"oauth","client_id":"client","issuer":"https://secret@issuer.example"}`, true},
		{"query", `{"type":"oauth","client_id":"client","issuer":"https://issuer.example?token=secret"}`, true},
		{"fragment", `{"type":"oauth","client_id":"client","issuer":"https://issuer.example#issuer"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var d Definition
			if err := json.Unmarshal([]byte(`{"transport":"http","url":"https://resource.example/mcp","auth":`+tc.auth+`}`), &d); err != nil {
				t.Fatal(err)
			}
			err := resolve(&d, func(string) string { return "fixture-only-secret" })
			if (err != nil) != tc.invalid {
				t.Fatalf("invalid=%v err=%v", tc.invalid, err)
			}
		})
	}
}
