package subagent

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

type finalReportProvider struct{}

func (p *finalReportProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "subagent report"},
	}, nil
}

func TestManagerRunDoesNotWriteStdout(t *testing.T) {
	manager := NewManager(&finalReportProvider{}, t.TempDir())

	var result *Result
	stdout := captureStdout(t, func() {
		var err error
		result, err = manager.Run(context.Background(), Request{
			ParentSessionID: "parent-session",
			Task:            "inspect code",
			ReadOnly:        true,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	if stdout != "" {
		t.Fatalf("Run() wrote stdout %q, want empty", stdout)
	}
	if result == nil || result.Report != "subagent report" {
		t.Fatalf("Run() result = %#v, want subagent report", result)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = previous
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("reader Close() error = %v", err)
	}

	return string(out)
}

// TestManagerBuildComposerInjectsPersistentMemory verifies delegated subagent
// tasks receive the cross-session persistent memory index (the P2-3 regression:
// after legacy MEMORY.md injection was removed, subagents had no durable
// memory).
func TestManagerBuildComposerInjectsPersistentMemory(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	store := automemory.NewStore(home, workDir)
	if err := store.Save(automemory.Memory{
		Name:        "user-role",
		Description: "Staff engineer, terse answers.",
		Type:        automemory.TypeUser,
		Body:        "The user is a staff engineer.",
	}); err != nil {
		t.Fatal(err)
	}

	mgr := &Manager{workDir: workDir, homeDir: home}
	sess := &session.Session{ID: "sub", RootDir: t.TempDir(), WorkDir: workDir}
	prompt, err := mgr.buildComposer(sess).Compose("explore the codebase")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Persistent Memory", "user-role.md"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("subagent composer missing %q:\n%s", want, prompt)
		}
	}
}

func TestManagerBuildComposerUsesReadOnlyPersistentMemoryGuidance(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	store := automemory.NewStore(home, workDir)
	if err := store.Save(automemory.Memory{
		Name:        "user-role",
		Description: "Staff engineer, terse answers.",
		Type:        automemory.TypeUser,
		Body:        "The user is a staff engineer.",
	}); err != nil {
		t.Fatal(err)
	}

	mgr := &Manager{workDir: workDir, homeDir: home}
	sess := &session.Session{ID: "sub", RootDir: t.TempDir(), WorkDir: workDir}
	prompt, err := mgr.buildComposer(sess).Compose("explore the codebase")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"read-only", "read_file", "user-role.md"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("subagent composer missing read-only guidance %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"Create or update a memory",
		"write_file/edit_file",
		"Forget a memory",
		"empty content",
		"Save only what is surprising",
		"Dedup first",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("subagent composer must not include write guidance %q:\n%s", forbidden, prompt)
		}
	}
}

func TestManagerBuildComposerOmitsWorkingMemoryWriteGuidanceWhenReadOnly(t *testing.T) {
	workDir := t.TempDir()
	mgr := &Manager{workDir: workDir, homeDir: t.TempDir()}
	sess := &session.Session{ID: "sub", RootDir: t.TempDir(), WorkDir: workDir}

	prompt, err := mgr.buildComposer(sess).Compose("explore the codebase")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"Keep it current as you work",
		"write_file and edit_file tools",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("read-only subagent composer must not include working-memory write guidance %q:\n%s", forbidden, prompt)
		}
	}
	if !strings.Contains(prompt, "working_memory.md") {
		t.Fatalf("read-only subagent should still see working memory contents:\n%s", prompt)
	}
}
