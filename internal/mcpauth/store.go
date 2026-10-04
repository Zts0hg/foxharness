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
	Config   oauth2.Config
	Token    oauth2.Token
	Resource string
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
		Issuer                                                        string `json:"Issuer,omitempty"`
	}{server.Definition.URL, server.Definition.Auth.ClientID, server.Definition.Auth.ClientMetadataURL, server.Definition.Auth.ClientSecretEnv, string(server.Scope), filepath.Clean(server.ScopeDir), scopes, server.Definition.Auth.Issuer}
	data, _ := json.Marshal(identity)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) withStore(ctx context.Context, operation func(map[string]credential) (bool, error)) error {
	return m.withLock(ctx, "mcp-credentials.json.lock", func(root *os.Root) error {
		return updateStore(root, operation)
	})
}

/* withCredential serializes one identity across managers and processes, including
 * login and logout, so a completed refresh cannot resurrect removed credentials.
 * The shared file lock is held only for short read or merge/write operations. */
func (m *Manager) withCredential(ctx context.Context, identity string, operation func() error) error {
	sum := sha256.Sum256([]byte(identity))
	return m.withLock(ctx, ".credential-"+hex.EncodeToString(sum[:])+".lock", func(*os.Root) error {
		return operation()
	})
}

func (m *Manager) withLock(ctx context.Context, name string, operation func(*os.Root) error) error {
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
		err = root.Mkdir(name, 0700)
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
	defer root.Remove(name)
	return operation(root)
}

func updateStore(root *os.Root, operation func(map[string]credential) (bool, error)) error {
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
	return m.withCredential(ctx, identity, func() error {
		return m.withStore(ctx, func(records map[string]credential) (bool, error) { records[identity] = record; return true, nil })
	})
}

/* Logout removes local credentials; it does not claim remote token revocation. */
func (m *Manager) Logout(ctx context.Context, server mcpconfig.Server) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	identity := key(server)
	return m.withCredential(ctx, identity, func() error {
		return m.withStore(ctx, func(records map[string]credential) (bool, error) { delete(records, identity); return true, nil })
	})
}

/* Token reuses or refreshes one service's credential under its own identity lock. */
func (m *Manager) Token(ctx context.Context, server mcpconfig.Server) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var token string
	identity := key(server)
	err := m.withCredential(ctx, identity, func() error {
		var record credential
		if err := m.withStore(ctx, func(records map[string]credential) (bool, error) {
			var ok bool
			record, ok = records[identity]
			if !ok {
				return false, fmt.Errorf("authorization required; run fox mcp login for this server")
			}
			return false, nil
		}); err != nil {
			return err
		}
		if record.Token.Valid() {
			token = record.Token.AccessToken
			return nil
		}
		if record.Token.RefreshToken == "" {
			return fmt.Errorf("authorization expired; run fox mcp login for this server")
		}
		refreshCtx := context.WithValue(ctx, oauth2.HTTPClient, oauthClient())
		resource := record.Resource
		if resource == "" {
			resource = server.Definition.URL
		}
		config := record.Config
		config.RedirectURL = ""
		/* TokenSource cannot carry RFC 8707 resource parameters. Exchange supports
		custom grant parameters while retaining client authentication and token
		rotation; an empty code is ignored under RFC 6749 section 3.2. */
		updated, err := config.Exchange(refreshCtx, "",
			oauth2.SetAuthURLParam("grant_type", "refresh_token"),
			oauth2.SetAuthURLParam("refresh_token", record.Token.RefreshToken),
			oauth2.SetAuthURLParam("resource", resource),
		)
		if err != nil {
			return fmt.Errorf("authorization refresh failed; run fox mcp login for this server")
		}
		record.Token = *updated
		if err := m.withStore(ctx, func(records map[string]credential) (bool, error) {
			records[identity] = record
			return true, nil
		}); err != nil {
			return err
		}
		token = updated.AccessToken
		return nil
	})
	return token, err
}
