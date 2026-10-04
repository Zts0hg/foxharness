package childruntime

import (
	"context"
	"github.com/Zts0hg/foxharness/internal/mcpintegration"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/testsupport/mcpfixture"
	"strings"
	"testing"
)

func TestChildBorrowsParentBrowserAndHydratesLocalImages(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	f := mcpfixture.Start(t, work)
	h, err := mcpintegration.New(context.Background(), home, work, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close(context.Background())
	model := &mcpfixture.Model{}
	r := New(Config{Provider: model, HomeDir: home, WorkDir: work, ParentProfile: CLIExec, Services: h})
	for range 2 {
		_, err := r.Run(context.Background(), subagent.Request{ParentSessionID: "parent", ParentRunID: "run", DelegationID: "call", Task: "capture", Depth: 1})
		if err != nil {
			t.Fatal(err)
		}
	}
	images, _ := model.Counts()
	if images != 2 || f.Calls.Load() != 2 || f.Active() != 1 {
		t.Fatalf("images=%d calls=%d active=%d", images, f.Calls.Load(), f.Active())
	}
}
func TestMCPChildReadOnlyAndExplicitEmptyCannotExecuteExternalTools(t *testing.T) {
	for _, readOnly := range []bool{true, false} {
		home, work := t.TempDir(), t.TempDir()
		f := mcpfixture.Start(t, work)
		h, err := mcpintegration.New(context.Background(), home, work, nil)
		if err != nil {
			t.Fatal(err)
		}
		model := &captureProvider{}
		r := New(Config{Provider: model, HomeDir: home, WorkDir: work, ParentProfile: CLIExec, Services: h})
		var allowed []string
		if !readOnly {
			allowed = []string{}
		}
		_, err = r.Run(context.Background(), subagent.Request{ParentSessionID: "parent", ParentRunID: "run", Task: "inspect", ReadOnly: readOnly, AllowedTools: allowed, Depth: 1})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range model.tools {
			if strings.HasPrefix(d.Name, "mcp_") || d.Name == "delegate_task" {
				t.Fatalf("child privilege widened: %s", d.Name)
			}
		}
		if f.Calls.Load() != 0 {
			t.Fatal("restricted child dispatched")
		}
		h.Close(context.Background())
	}
}
