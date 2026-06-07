package tui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
)

// mergeGuard must satisfy the Middleware interface.
var _ middleware.Middleware = mergeGuard{}

func bashCall(t *testing.T, command string) schema.ToolCall {
	t.Helper()
	b, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		t.Fatal(err)
	}
	return schema.ToolCall{Name: "bash", Arguments: json.RawMessage(b)}
}

func TestMergeGuardBeforeExecute(t *testing.T) {
	var g mergeGuard
	cases := []struct {
		command string
		deny    bool
	}{
		{"git merge main", true},
		{"cd /w && git merge origin/main", true},
		{"gh pr merge 12 --squash", true},
		{"glab mr merge 7", true},
		{"git commit -m 'note about a merge'", false},
		{"go test ./...", false},
		{"git push -u origin keep-run-x", false},
	}
	for _, c := range cases {
		d, err := g.BeforeExecute(context.Background(), bashCall(t, c.command))
		if err != nil {
			t.Fatalf("BeforeExecute(%q) error: %v", c.command, err)
		}
		denied := d.Type == middleware.DecisionDeny
		if denied != c.deny {
			t.Errorf("BeforeExecute(%q): denied=%v, want %v", c.command, denied, c.deny)
		}
	}
}

func TestMergeGuardIgnoresNonBash(t *testing.T) {
	var g mergeGuard
	call := schema.ToolCall{Name: "write_file", Arguments: json.RawMessage(`{"path":"x","content":"git merge main"}`)}
	d, err := g.BeforeExecute(context.Background(), call)
	if err != nil || d.Type != middleware.DecisionAllow {
		t.Errorf("non-bash call: got (%v, %v), want Allow", d.Type, err)
	}
}
