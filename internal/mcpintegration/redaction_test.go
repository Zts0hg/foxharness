package mcpintegration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/mcpcmd"
	"github.com/Zts0hg/foxharness/internal/mcpconfig"
)

func TestListRedactsSecretsEvenWhenConfigurationIsInvalid(t *testing.T) {
	for _, scope := range []string{"user", "project"} {
		t.Run(scope, func(t *testing.T) {
			home, work := t.TempDir(), t.TempDir()
			dir := home
			if scope == "project" {
				dir = work
			}
			dir = filepath.Join(dir, ".foxharness")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			body := `{"mcpServers":{"invalid":{"transport":"http","url":"https://example.invalid/%zz?token=fixture-secret","auth":{"type":"oauth","client_id":"client","issuer":"https://fixture-secret@example.invalid/issuer","client_metadata_url":"https://example.invalid/client.json?token=fixture-secret"}}}}`
			if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			m := NewManager(home, work, nil)
			m.Connector = func(context.Context, mcpconfig.Server) (Client, error) {
				t.Fatal("list connected to invalid server")
				return nil, nil
			}
			result, err := m.ExecuteMCP(context.Background(), app.MCPCommand{Action: "list"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			display := mcpcmd.Format(result)
			if strings.Contains(display, "fixture-secret") {
				t.Fatal("management output exposed secret")
			}
			if !strings.Contains(display, "invalid") || !strings.Contains(display, scope) || !strings.Contains(display, "remote URL must be HTTP(S)") {
				t.Fatal("management output lost invalid configuration diagnostic")
			}
		})
	}
}
