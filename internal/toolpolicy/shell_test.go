package toolpolicy

import "testing"

func TestAssessShellRejectsRipgrepHelperExecutionOptions(t *testing.T) {
	workspace := t.TempDir()
	tests := []struct {
		name    string
		command string
	}{
		{name: "hostname executable attached", command: "rg --hostname-bin=./evil needle ."},
		{name: "hostname executable separate", command: "rg --hostname-bin ./evil needle ."},
		{name: "archive helpers long", command: "rg --search-zip needle ."},
		{name: "archive helpers short", command: "rg -z needle ."},
		{name: "archive helpers short cluster", command: "rg -nz needle ."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readOnly, _, parsed := AssessShell(tt.command, workspace, workspace)
			if !parsed {
				t.Fatalf("AssessShell(%q) parsed = false, want true", tt.command)
			}
			if readOnly {
				t.Fatalf("AssessShell(%q) readOnly = true, want false", tt.command)
			}
		})
	}
}

func TestAssessShellAllowsExplicitlyDisabledRipgrepArchiveSearch(t *testing.T) {
	workspace := t.TempDir()
	readOnly, _, parsed := AssessShell("rg --no-search-zip needle .", workspace, workspace)
	if !parsed || !readOnly {
		t.Fatalf("AssessShell() = readOnly %t, parsed %t; want true, true", readOnly, parsed)
	}
}
