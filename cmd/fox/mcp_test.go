package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestMCPManagementRunsWithoutLLMConfiguration(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	var out bytes.Buffer
	if err := runMCP(context.Background(), home, work, []string{"add", "browser", "--scope", "project", "--json", `{"transport":"stdio","command":"fixture","enabled":false}`}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runMCP(context.Background(), home, work, []string{"status"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "disabled") || !strings.Contains(out.String(), "probe") {
		t.Fatal(out.String())
	}
	if err := runMCP(context.Background(), home, work, []string{"reconnect", "browser"}, &out); err == nil {
		t.Fatal("cross-process reconnect accepted")
	}
}
