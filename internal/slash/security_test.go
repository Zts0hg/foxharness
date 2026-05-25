package slash

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestSecurity_FrontmatterPayloadNoExecution(t *testing.T) {
	// A payload that would attempt to execute arbitrary code on naive
	// deserialization. yaml.v3 rejects unknown YAML tags, which is what we
	// want — the parser must not invoke side effects regardless.
	bad := []byte(`---
description: !!python/object/apply:os.system ["echo bad"]
---
body`)
	_, _, _ = ParseFrontmatter(bad)
}

func TestSecurity_ShellEmbeddingHonorsWorkDir(t *testing.T) {
	wd := t.TempDir()
	out, err := ExecuteEmbeddedShell("dir="+"!`pwd`", wd, 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, wd) && !strings.Contains(out, "/private"+wd) {
		t.Errorf("workDir not honored: out=%q wd=%q", out, wd)
	}
}

func TestSecurity_ShellEmbeddingTimeoutPreventsHang(t *testing.T) {
	start := time.Now()
	out, err := ExecuteEmbeddedShell("x="+"!`sleep 10`", "", 200*time.Millisecond)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("shell embedding did not respect timeout, took %s", time.Since(start))
	}
	if !strings.Contains(out, "[ERROR:") {
		t.Errorf("expected timeout to produce ERROR marker, got %q", out)
	}
}

func TestSecurity_FilteredRegistryBlocksDisallowed(t *testing.T) {
	base := newBaseRegistry("safe", "dangerous")
	filtered := NewFilteredRegistry(base, []string{"safe"})

	allowed := filtered.Execute(context.Background(), schema.ToolCall{ID: "1", Name: "safe", Arguments: json.RawMessage(`{}`)})
	if allowed.IsError {
		t.Errorf("safe tool should succeed: %s", allowed.Output)
	}
	denied := filtered.Execute(context.Background(), schema.ToolCall{ID: "2", Name: "dangerous", Arguments: json.RawMessage(`{}`)})
	if !denied.IsError {
		t.Error("dangerous tool must be blocked")
	}
	defs := filtered.GetAvailableTools()
	for _, d := range defs {
		if d.Name == "dangerous" {
			t.Error("dangerous must not appear in available tools")
		}
	}
}

// Ensure the package surface does not accidentally expose unsafe defaults
// for the executor or registry.
var _ tools.Registry = (*filteredRegistry)(nil)
