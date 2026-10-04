package mcpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

type responseMonitor struct {
	base                  http.RoundTripper
	status                atomic.Int32
	authorizationRequired atomic.Bool
}

func (m *responseMonitor) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := m.base.RoundTrip(req)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "authorization") {
			m.authorizationRequired.Store(true)
		}
		return nil, err
	}
	m.status.Store(int32(resp.StatusCode))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		m.authorizationRequired.Store(true)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		resp.Body = &limitedBody{ReadCloser: resp.Body, remaining: MaxFrameBytes}
	}
	return resp, nil
}

func (m *responseMonitor) legacyFallback() bool {
	status := m.status.Load()
	return status == 400 || status == 404 || status == 405
}

func connectionError(ctx context.Context, m *responseMonitor) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("MCP startup cancelled or timed out: %w", err)
	}
	if m != nil && (m.authorizationRequired.Load() || m.status.Load() == 401 || m.status.Load() == 403) {
		return ErrAuthorizationRequired
	}
	return fmt.Errorf("MCP connection failed; verify command, endpoint, transport, and protocol compatibility")
}

type limitedBody struct {
	io.ReadCloser
	remaining int
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if len(p) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= n
	if b.remaining < 0 {
		return 0, fmt.Errorf("MCP response exceeds frame limit")
	}
	return n, err
}
