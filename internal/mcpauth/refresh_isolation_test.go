package mcpauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"golang.org/x/oauth2"
)

func TestConcurrentRefreshMergesRecordsAndCannotUndoLogout(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := make(chan string, 2)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.URL.Path
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "fresh" + r.URL.Path, "token_type": "Bearer", "expires_in": 3600})
	}))
	defer func() { unblock(); srv.Close() }()
	manager := NewManager(home)
	servers := []mcpconfig.Server{
		{Definition: mcpconfig.Definition{URL: "https://one.example/mcp", Auth: mcpconfig.Auth{Type: "oauth"}}},
		{Definition: mcpconfig.Definition{URL: "https://two.example/mcp", Auth: mcpconfig.Auth{Type: "oauth"}}},
	}
	results := make(chan error, 2)
	for i, s := range servers {
		endpoint := []string{"/one", "/two"}[i]
		if err := manager.save(ctx, key(s), credential{Config: oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: srv.URL + endpoint, AuthStyle: oauth2.AuthStyleInParams}}, Token: oauth2.Token{AccessToken: "old", RefreshToken: "refresh", Expiry: time.Now().Add(-time.Hour)}}); err != nil {
			t.Fatal(err)
		}
		go func(s mcpconfig.Server) { _, err := NewManager(home).Token(ctx, s); results <- err }(s)
	}
	for range servers {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("independent refresh requests did not both reach their endpoints")
		}
	}
	extra := mcpconfig.Server{Definition: mcpconfig.Definition{URL: "https://three.example/mcp"}}
	if err := NewManager(home).save(ctx, key(extra), credential{Token: oauth2.Token{AccessToken: "third", Expiry: time.Now().Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	logout := make(chan error, 1)
	go func() { logout <- NewManager(home).Logout(ctx, servers[0]) }()
	var logoutErr error
	loggedOut := false
	select {
	case logoutErr = <-logout:
		loggedOut = true
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	for range servers {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if !loggedOut {
		logoutErr = <-logout
	}
	if logoutErr != nil {
		t.Fatal(logoutErr)
	}
	if _, err := NewManager(home).Token(ctx, servers[0]); err == nil {
		t.Fatal("in-flight refresh resurrected logged-out credential")
	}
	for _, tc := range []struct {
		s    mcpconfig.Server
		want string
	}{{servers[1], "fresh/two"}, {extra, "third"}} {
		got, err := NewManager(home).Token(ctx, tc.s)
		if err != nil || got != tc.want {
			t.Fatalf("independent credential update lost: %q %v", got, err)
		}
	}
}
