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

func TestAssessSynchronousShellRejectsDetachedSchedulerForms(t *testing.T) {
	commands := []struct {
		command     string
		synchronous bool
	}{
		{"go test ./...", true},
		{"ls -la", true},
		{"VAR=value go test ./...", true},
		{"sleep 1 &", false},
		{"nohup sleep 1", false},
		{"at now -f script.sh", false},
		{`"at" now -f script.sh`, false},
		{`'at' now`, false},
		{"/usr/bin/at now", false},
		{"./at now", false},
		{"/bin/bash -c 'sleep 1 &'", false},
		{"$(at now) x", false},
		{`"open" -a Terminal`, false},
		{"systemd-run --collect sleep 1", false},
		{"launchctl submit -l x -- sleep 1", false},
		{"schtasks /create /tn demo /tr calc", false},
		{"crontab -", false},
	}
	for _, item := range commands {
		t.Run(item.command, func(t *testing.T) {
			synchronous, parsed := AssessSynchronousShell(item.command)
			if !parsed {
				t.Fatalf("AssessSynchronousShell(%q) did not parse", item.command)
			}
			if synchronous != item.synchronous {
				t.Fatalf("AssessSynchronousShell(%q) = %v, want %v", item.command, synchronous, item.synchronous)
			}
		})
	}
}
