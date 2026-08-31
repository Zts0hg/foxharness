package runtime

import (
	"context"
	"strings"
	"testing"
)

/* TestRestrictedRunLogsSlashRestriction verifies that admitting a restricted
 * run announces the applied allowed-tools restriction, and unrestricted runs
 * stay silent. */
func TestRestrictedRunLogsSlashRestriction(t *testing.T) {
	logs := captureDiagnosticLog(t)
	harness, err := NewRuntimeHarness(newLifecycleStore(), successfulHarnessDependencies(nil))
	if err != nil {
		t.Fatal(err)
	}
	agentSession, err := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	turns := 1
	if _, err := agentSession.Run(context.Background(), RunSpec{
		Prompt: "inspect", Model: "model-a", ProviderProtocol: "messages",
		MaxTurns: &turns, AllowedTools: []string{"read_file"},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "[slash] restricting next run to allowed tools: [read_file]") {
		t.Fatalf("restricted run did not announce the slash restriction:\n%s", logs.String())
	}

	before := logs.Len()
	if _, err := agentSession.Run(context.Background(), RunSpec{
		Prompt: "free", Model: "model-a", ProviderProtocol: "messages", MaxTurns: &turns,
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String()[before:], "[slash] restricting next run") {
		t.Fatalf("unrestricted run announced a slash restriction:\n%s", logs.String()[before:])
	}
}

/* TestChildRunAdmissionDoesNotLogSlashRestriction verifies that the baseline
 * slash-restriction admission line stays a main-run announcement: child runs
 * always carry a non-nil allowed-tools snapshot and must stay silent. */
func TestChildRunAdmissionDoesNotLogSlashRestriction(t *testing.T) {
	logs := captureDiagnosticLog(t)
	harness, err := NewRuntimeHarness(newLifecycleStore(), successfulHarnessDependencies(nil))
	if err != nil {
		t.Fatal(err)
	}
	child, err := harness.CreateSession(context.Background(), ChildRun, SessionOptions{WorkDir: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	turns := 1
	if _, err := child.Run(context.Background(), RunSpec{
		Prompt: "delegate", Model: "model-a", ProviderProtocol: "messages",
		MaxTurns: &turns, AllowedTools: []string{"bash", "read_file"},
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "[slash] restricting next run") {
		t.Fatalf("child run announced a slash restriction:\n%s", logs.String())
	}
}
