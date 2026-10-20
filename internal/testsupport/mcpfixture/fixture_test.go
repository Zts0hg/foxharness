package mcpfixture

import (
	"bytes"
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFixtureServesImageAndTracksSessionLifetime(t *testing.T) {
	f := Start(t, t.TempDir())
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture-test", Version: "1"}, nil)
	s, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: f.Server.URL, DisableStandaloneSSE: true}, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if f.Active() != 1 {
		t.Fatal("negotiated session not recorded")
	}
	for i := 0; i < 2; i++ {
		r, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "screenshot", Arguments: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		image, ok := r.Content[1].(*mcp.ImageContent)
		if !ok || image.MIMEType != "image/png" || !bytes.Equal(image.Data, f.Image) || f.Calls.Load() != int32(i+1) {
			t.Fatalf("fixture response/count mismatch: %+v", r)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if f.Active() != 0 {
		t.Fatal("closed session retained")
	}
}

func TestFixtureExcludesUninitializedProtocolProbes(t *testing.T) {
	f := Start(t, t.TempDir())
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	probe, err := f.protocol.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if got := f.Active(); got != 0 {
		t.Fatalf("uninitialized protocol probe counted as negotiated owner: %d", got)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "probe-test", Version: "1"}, nil)
	negotiated, err := client.Connect(context.Background(), clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if err != nil {
		t.Fatal(err)
	}
	defer negotiated.Close()
	if got := f.Active(); got != 1 {
		t.Fatalf("negotiated owner count = %d, want 1", got)
	}
	if err := negotiated.Close(); err != nil {
		t.Fatal(err)
	}
	if err := probe.Wait(); err != nil {
		t.Fatal(err)
	}
	if got := f.Active(); got != 0 {
		t.Fatalf("closed owner retained: %d", got)
	}
}
