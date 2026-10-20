package main

import (
	"bytes"
	"context"
	"errors"
	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/cli"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/mcpconfig"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/testsupport/mcpfixture"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type captureDrain struct {
	cli.Application
	drainErr error
}

func (c *captureDrain) Drain(ctx context.Context) error {
	c.drainErr = c.Application.Drain(ctx)
	return c.drainErr
}
func TestCLIReportsMCPCleanupFailure(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	s := mcp.NewServer(&mcp.Implementation{Name: "cleanup", Version: "1"}, nil)
	s.AddTool(&mcp.Tool{Name: "x", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			c, _, e := w.(http.Hijacker).Hijack()
			if e == nil {
				c.Close()
			}
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()
	if err := mcpconfig.Add(home, work, mcpconfig.ProjectScope, "cleanup", mcpconfig.Definition{Transport: "streamable-http", URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	a, err := newCLIApplicationWithProvider(context.Background(), foxConfig{WorkDir: work, Model: "fixture", MaxTurns: 3, ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "http://fixture", Model: "fixture"}}, &targetCLIProvider{})
	if err != nil {
		t.Fatal(err)
	}
	capture := &captureDrain{Application: a}
	var logs bytes.Buffer
	err = cli.Run(context.Background(), cli.Config{Prompt: "hello", Application: capture, Stdout: io.Discard, Logger: log.New(&logs, "", 0)})
	if capture.drainErr == nil {
		t.Fatal("fixture did not cause cleanup failure")
	}
	t.Logf("MCP cleanup error=%v; CLI returned=%v; logs=%s", capture.drainErr, err, logs.String())
	if !errors.Is(err, capture.drainErr) {
		t.Fatal("CLI silently discarded MCP cleanup error")
	}
}
func TestAutodevClosesMCPAfterExtractionTimeout(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	f := mcpfixture.Start(t, work)
	if err := os.WriteFile(filepath.Join(work, "fixture.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	model := &targetCLIToolProvider{extractionStarted: make(chan struct{}), releaseExtraction: make(chan struct{})}
	factory := &autodevRuntimeCoreFactory{llmConfig: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "http://fixture", Model: "fixture"}, maxTurns: 3, newProvider: func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return model, nil }}
	core, err := factory.New(context.Background(), work, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close(context.Background())
	outcome := core.Run(context.Background(), autodev.CoreAttempt{AttemptID: "probe", Prompt: "read fixture"}, nil)
	if outcome.Status != autodev.CoreOutcomeSucceeded {
		t.Fatal(outcome.Cause)
	}
	select {
	case <-model.extractionStarted:
	case <-time.After(time.Second):
		t.Fatal("extraction not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = core.Close(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("extraction deadline lost: %v", err)
	}
	t.Logf("core.Close error=%v; active MCP sessions after terminal close=%d", err, f.Active())
	if f.Active() != 0 {
		t.Fatal("MCP connection survived terminal close after extraction deadline")
	}
}
