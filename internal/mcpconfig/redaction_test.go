package mcpconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactedURLsHandleInvalidAndOAuthConfiguration(t *testing.T) {
	for _, field := range []string{"endpoint", "issuer", "metadata"} {
		for _, tc := range []struct{ name, url, want string }{
			{"credentials", "https://fixture-secret:fixture-secret@example.invalid/client.json?token=fixture-secret#fixture-secret", "https://example.invalid/client.json"},
			{"malformed", "https://example.invalid/%zz?token=fixture-secret", "[redacted invalid URL]"},
			{"opaque", "https:fixture-secret", "[redacted invalid URL]"},
			{"relative", "fixture-secret", "[redacted invalid URL]"},
			{"empty", "", ""},
		} {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				d := Definition{Transport: "http", URL: "https://example.invalid/mcp", Auth: Auth{Type: "oauth", ClientID: "client"}}
				switch field {
				case "endpoint":
					d.URL = tc.url
				case "issuer":
					d.Auth.Issuer = tc.url
				case "metadata":
					d.Auth.ClientMetadataURL = tc.url
				}
				original := Snapshot{Servers: map[string]Server{"configured": {Definition: d}}}
				clean := original.Redacted()
				got := clean.Servers["configured"].Definition
				value := got.URL
				switch field {
				case "issuer":
					value = got.Auth.Issuer
				case "metadata":
					value = got.Auth.ClientMetadataURL
				}
				if value != tc.want {
					t.Fatalf("redacted URL=%q want %q", value, tc.want)
				}
				encoded, _ := json.Marshal(clean)
				if strings.Contains(string(encoded), "fixture-secret") {
					t.Fatal("secret retained in redacted snapshot")
				}
				before, _ := json.Marshal(d)
				after, _ := json.Marshal(original.Servers["configured"].Definition)
				if string(before) != string(after) {
					t.Fatal("redaction changed original configuration")
				}
			})
		}
	}
}
