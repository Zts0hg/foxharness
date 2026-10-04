package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/session"
)

type serviceFixture struct {
	closed, reconnected int
	closeContextErr     error
}

func (s *serviceFixture) Capabilities() []ExternalCapability {
	return []ExternalCapability{{Name: "mcp_browser_click"}}
}
func (s *serviceFixture) Close(ctx context.Context) error {
	s.closed++
	s.closeContextErr = ctx.Err()
	return nil
}
func (s *serviceFixture) Reconnect(context.Context, string) error { s.reconnected++; return nil }

func TestOwnedServicesInitializeOnCreateAndOpenAndExtendBeforeNarrowing(t *testing.T) {
	store := session.NewFileStoreWithHome(t.TempDir(), t.TempDir())
	var services []*serviceFixture
	h, err := NewRuntimeHarness(store, HarnessDependencies{NewSessionServices: func(context.Context, AgentSessionSnapshot) (SessionServices, error) {
		s := &serviceFixture{}
		services = append(services, s)
		return s, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	live, err := h.CreateSession(context.Background(), CLIExec, SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, allowed := range [][]string{nil, {}, {"mcp_browser_click"}} {
		scope, err := live.BeginRun(context.Background(), RunSpec{Prompt: "go", AllowedTools: allowed})
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Join(scope.AllowedTools(), ",")
		if (allowed == nil || len(allowed) > 0) && !strings.Contains(got, "mcp_browser_click") {
			t.Fatalf("external capability filtered: %q", got)
		}
		if allowed != nil && len(allowed) == 0 && got != "" {
			t.Fatal("empty restriction broadened")
		}
		if err := live.FinishRun(scope); err != nil {
			t.Fatal(err)
		}
	}
	unknown, err := live.BeginRun(context.Background(), RunSpec{AllowedTools: []string{"unregistered"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(unknown.AllowedTools()) != 0 {
		t.Fatal("unregistered capability admitted")
	}
	if err := live.FinishRun(unknown); err != nil {
		t.Fatal(err)
	}
	id := live.Snapshot().ID
	if err := live.ReconnectServices(context.Background(), "browser"); err != nil {
		t.Fatal(err)
	}
	if services[0].reconnected != 1 {
		t.Fatal("reconnect not forwarded")
	}
	if err := live.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := live.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	reopened, err := h.OpenSession(context.Background(), CLIExec, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 2 || services[0].closed != 1 || services[0].closeContextErr != nil {
		t.Fatalf("service lifecycle: %#v", services)
	}
	if err := reopened.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPartialServiceFailureClosesAndReleasesSessionLease(t *testing.T) {
	store := session.NewFileStoreWithHome(t.TempDir(), t.TempDir())
	service := &serviceFixture{}
	var id session.ID
	h, err := NewRuntimeHarness(store, HarnessDependencies{NewSessionServices: func(_ context.Context, s AgentSessionSnapshot) (SessionServices, error) {
		id = s.ID
		return service, errors.New("construction failed")
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.CreateSession(context.Background(), CLIExec, SessionOptions{}); err == nil {
		t.Fatal("partial failure accepted")
	}
	if service.closed != 1 {
		t.Fatal("partial service leaked")
	}
	h.dependencies.NewSessionServices = nil
	live, err := h.OpenSession(context.Background(), CLIExec, id)
	if err != nil {
		t.Fatalf("lease leaked: %v", err)
	}
	live.Close(context.Background())
}

func TestExternalChildCapabilitiesPreserveReadOnlyAndEmptyRestrictions(t *testing.T) {
	external := []ExternalCapability{{Name: "mcp_browser_click"}}
	for _, readOnly := range []bool{false, true} {
		got, err := resolveChildAllowedTools(readOnly, []string{"mcp_browser_click"}, external)
		if err != nil {
			t.Fatal(err)
		}
		if readOnly && len(got) != 0 {
			t.Fatal("unknown-effect tool entered read-only child")
		}
		if !readOnly && (len(got) != 1 || got[0] != "mcp_browser_click") {
			t.Fatal("writable authorized child lost external tool")
		}
	}
	got, err := resolveChildAllowedTools(false, []string{}, external)
	if err != nil || len(got) != 0 {
		t.Fatal("empty child restriction broadened")
	}
}
