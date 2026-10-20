package main

import (
	"context"
	"github.com/Zts0hg/foxharness/internal/benchmark"
	"github.com/Zts0hg/foxharness/internal/testsupport/mcpfixture"
	"strings"
	"testing"
)

func TestBenchmarkMCPFreezesFidelityAndIsolatesRepeats(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FOXHARNESS_LLM_PROTOCOL", "openai")
	t.Setenv("FOXHARNESS_LLM_BASE_URL", "http://fixture")
	t.Setenv("FOXHARNESS_LLM_MODEL", "fixture")
	t.Setenv("FOXHARNESS_LLM_AUTH", "none")
	work := t.TempDir()
	f := mcpfixture.Start(t, work)
	for range 2 {
		h, err := buildHarness(context.Background(), work, &benchmark.Case{ID: "mcp", MaxTurns: 2})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, name := range h.RuntimeFidelity.Spec.ToolSurface {
			if strings.HasPrefix(name, "mcp_browser_screenshot_") {
				found = true
			}
		}
		h.Session.Close(context.Background())
		if !found {
			t.Fatal("fidelity omitted MCP capability")
		}
	}
	if f.Active() != 0 {
		t.Fatal("benchmark connections leaked")
	}
}
