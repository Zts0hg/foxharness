/* Package mcpauth owns explicit OAuth login and user-local MCP credentials. */
package mcpauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"golang.org/x/oauth2"
)

type credential struct {
	Config oauth2.Config
	Token  oauth2.Token
}

/* Manager stores credentials separately from shareable project configuration. */
type Manager struct{ home string }

/* NewManager binds credential operations to a user's home directory. */
func NewManager(home string) *Manager { return &Manager{home: home} }

func key(server mcpconfig.Server) string {
	scopes := append([]string(nil), server.Definition.Auth.Scopes...)
	sort.Strings(scopes)
	identity := struct {
		Endpoint, Client, Metadata, SecretReference, Scope, Directory string
		Scopes                                                        []string
	}{server.Definition.URL, server.Definition.Auth.ClientID, server.Definition.Auth.ClientMetadataURL, server.Definition.Auth.ClientSecretEnv, string(server.Scope), filepath.Clean(server.ScopeDir), scopes}
	data, _ := json.Marshal(identity)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) withStore(ctx context.Context, operation func(map[string]credential) (bool, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Join(m.home, ".foxharness")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("credential directory unavailable")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("credential directory unavailable")
	}
	defer root.Close()
	if err := root.Chmod(".", 0700); err != nil {
		return fmt.Errorf("cannot protect credential directory")
	}
	for {
		err = root.Mkdir("mcp-credentials.json.lock", 0700)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("credential lock unavailable")
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("credential lock: %w", ctx.Err())
		case <-timer.C:
		}
	}
	defer root.Remove("mcp-credentials.json.lock")
	records := make(map[string]credential)
	if info, err := root.Lstat("mcp-credentials.json"); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("credential file must be regular and mode 0600")
		}
		f, err := root.Open("mcp-credentials.json")
		if err != nil {
			return fmt.Errorf("credential file unavailable")
		}
		data, readErr := io.ReadAll(io.LimitReader(f, 4*1024*1024+1))
		f.Close()
		if readErr != nil || len(data) > 4*1024*1024 || json.Unmarshal(data, &records) != nil || records == nil {
			return fmt.Errorf("invalid credential store; existing file retained")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("credential file unavailable")
	}
	changed, err := operation(records)
	if err != nil || !changed {
		return err
	}
	data, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("cannot encode credentials")
	}
	tmp := ".credentials-" + rand.Text()
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("cannot write credentials")
	}
	defer root.Remove(tmp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if errors.Join(err, closeErr) != nil {
		return fmt.Errorf("cannot persist credentials")
	}
	if err := root.Rename(tmp, "mcp-credentials.json"); err != nil {
		return fmt.Errorf("cannot replace credential store")
	}
	return nil
}

func (m *Manager) save(ctx context.Context, identity string, record credential) error {
	return m.withStore(ctx, func(records map[string]credential) (bool, error) { records[identity] = record; return true, nil })
}

/* Logout removes local credentials; it does not claim remote token revocation. */
func (m *Manager) Logout(ctx context.Context, server mcpconfig.Server) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return m.withStore(ctx, func(records map[string]credential) (bool, error) { delete(records, key(server)); return true, nil })
}

/* Token reuses or refreshes one service's credential under the file lock. */
func (m *Manager) Token(ctx context.Context, server mcpconfig.Server) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var token string
	err := m.withStore(ctx, func(records map[string]credential) (bool, error) {
		record, ok := records[key(server)]
		if !ok {
			return false, fmt.Errorf("authorization required; run fox mcp login for this server")
		}
		if record.Token.Valid() {
			token = record.Token.AccessToken
			return false, nil
		}
		if record.Token.RefreshToken == "" {
			return false, fmt.Errorf("authorization expired; run fox mcp login for this server")
		}
		refreshCtx := context.WithValue(ctx, oauth2.HTTPClient, oauthClient())
		updated, err := record.Config.TokenSource(refreshCtx, &record.Token).Token()
		if err != nil {
			return false, fmt.Errorf("authorization refresh failed; run fox mcp login for this server")
		}
		record.Token = *updated
		records[key(server)] = record
		token = updated.AccessToken
		return true, nil
	})
	return token, err
}
