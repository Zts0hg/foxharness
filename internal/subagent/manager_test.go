package subagent

import (
	"context"
	"encoding/json"
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

// loopingToolCallProvider is a fake provider whose every Generate response
// requests a read_file tool call, forcing the engine to keep looping. It is
// used to drive a subagent past its turn budget so the exhaustion path can be
// exercised deterministically and quickly.
type loopingToolCallProvider struct {
	calls int
}

func (p *loopingToolCallProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	return &provider.GenerateResponse{
		Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "call_exhaustion",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"nonexistent-exhaustion-probe"}`),
			}},
		},
	}, nil
}

func TestDefaultMaxTurnsIs200(t *testing.T) {
	if DefaultMaxTurns != 200 {
		t.Fatalf("DefaultMaxTurns = %d, want 200", DefaultMaxTurns)
	}
}

func TestNewManagerDefaultsMaxTurnsTo200(t *testing.T) {
	m := NewManager(&finalReportProvider{}, t.TempDir())
	if m.maxTurns != DefaultMaxTurns {
		t.Fatalf("maxTurns = %d, want %d (DefaultMaxTurns)", m.maxTurns, DefaultMaxTurns)
	}
}

func TestWithMaxTurnsOverridesDefault(t *testing.T) {
	m := NewManager(&finalReportProvider{}, t.TempDir()).WithMaxTurns(3)
	if m.maxTurns != 3 {
		t.Fatalf("maxTurns = %d, want 3", m.maxTurns)
	}
}

// TestRunHonorsInjectedMaxTurnsAndPreservesExhaustion verifies that an injected
// turn budget actually governs the engine (REQ-004) and that the exhaustion
// behavior is preserved when the budget is reached (REQ-005): Run returns an
// error whose message carries the injected limit, not the old default of 8.
func TestRunHonorsInjectedMaxTurnsAndPreservesExhaustion(t *testing.T) {
	mgr := NewManager(&loopingToolCallProvider{}, t.TempDir()).WithMaxTurns(1)

	result, err := mgr.Run(context.Background(), Request{
		ParentSessionID: "parent-session",
		Task:            "loop until stopped",
		ReadOnly:        true,
	})
	if err == nil {
		t.Fatalf("Run() error = nil, want exhaustion error for injected MaxTurns=1; result=%#v", result)
	}
	if !strings.Contains(err.Error(), "超过最大 Turn 数限制: 1") {
		t.Fatalf("Run() error = %q, want error containing %q", err.Error(), "超过最大 Turn 数限制: 1")
	}
}
