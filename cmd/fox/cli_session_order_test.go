package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/session"
)

/* TestCLIRuntimeSelectsSessionBeforeProviderConstruction pins the baseline
 * admission order: a bad session selection is reported even when the LLM
 * configuration is under-specified, and the failure creates no session. */
func TestCLIRuntimeSelectsSessionBeforeProviderConstruction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	config := foxConfig{
		WorkDir: workDir, SessionID: "missing-session", Model: "cli-model",
		ResolvedLLM: llmconfig.ResolvedConfig{},
	}
	_, err := newCLIApplication(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "Session missing-session 不存在") {
		t.Fatalf("newCLIApplication() error = %v, want the session selection error", err)
	}
	sessions, listErr := session.NewFileStore(workDir).List(session.LookupOptions{})
	if listErr != nil && !errors.Is(listErr, session.ErrNotFound) {
		t.Fatal(listErr)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions after failure = %d, want none", len(sessions))
	}
}
