package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestSessionRunAndMessageLog(t *testing.T) {
	workDir := t.TempDir()
	manager := NewManagerWithHome(workDir, t.TempDir())

	sess, err := manager.Create(CreateOptions{
		Source:  SOURCECLI,
		WorkDir: workDir,
		UserID:  "u1",
		ChatID:  "c1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := os.Stat(sess.MessagesPath()); err != nil {
		t.Fatalf("messages log was not created: %v", err)
	}

	run, err := sess.StartRun("inspect bug")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(run.RootDir, "run.json")); err != nil {
		t.Fatalf("run metadata was not created: %v", err)
	}
	if err := run.Finish(); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}

	log := NewMessageLog(sess)
	if err := log.Append(run.ID, schema.Message{Role: schema.RoleUser, Content: "first"}); err != nil {
		t.Fatalf("Append(user) error = %v", err)
	}
	if err := log.Append(run.ID, schema.Message{Role: schema.RoleAssistant, Content: "second"}); err != nil {
		t.Fatalf("Append(assistant) error = %v", err)
	}

	messages, err := log.LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("LoadMessages() len = %d, want 2", len(messages))
	}
	if messages[0].Content != "first" || messages[1].Content != "second" {
		t.Fatalf("messages are not chronological: %#v", messages)
	}
}

func TestManagerStoresSessionsUnderHomeProjectDirectory(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	manager := NewManagerWithHome(workDir, homeDir)

	sess, err := manager.Create(CreateOptions{
		Source:  SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	projectKey := encodeProjectPath(cleanAbsPath(workDir))
	wantBase := filepath.Join(homeDir, ".foxharness", "projects", projectKey, "sessions")
	if got := filepath.Dir(sess.RootDir); got != wantBase {
		t.Fatalf("session parent dir = %q, want %q", got, wantBase)
	}
	if _, err := os.Stat(filepath.Join(sess.RootDir, "session.json")); err != nil {
		t.Fatalf("session metadata was not created under home project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".foxharness")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Create() wrote project-local .foxharness; stat err = %v", err)
	}
	if sess.WorkDir != cleanAbsPath(workDir) {
		t.Fatalf("session WorkDir = %q, want absolute cleaned workDir %q", sess.WorkDir, cleanAbsPath(workDir))
	}
}

func TestEncodeProjectPathMatchesClaudeStyleWithoutHash(t *testing.T) {
	got := encodeProjectPath("/Users/xiaoming/code/foxharness-go")
	want := "-Users-xiaoming-code-foxharness-go"
	if got != want {
		t.Fatalf("encodeProjectPath() = %q, want %q", got, want)
	}
}

func TestManagerOpenAndLatest(t *testing.T) {
	workDir := t.TempDir()
	manager := NewManagerWithHome(workDir, t.TempDir())

	first, err := manager.Create(CreateOptions{
		Source:  SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	time.Sleep(time.Millisecond)
	second, err := manager.Create(CreateOptions{
		Source:  SOURCEFeishu,
		WorkDir: workDir,
		UserID:  "u1",
		ChatID:  "c1",
	})
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	opened, err := manager.Open(first.ID)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if opened.ID != first.ID {
		t.Fatalf("Open() ID = %s, want %s", opened.ID, first.ID)
	}

	latest, err := manager.Latest(LookupOptions{
		Source: SOURCEFeishu,
		UserID: "u1",
		ChatID: "c1",
	})
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if latest.ID != second.ID {
		t.Fatalf("Latest() ID = %s, want %s", latest.ID, second.ID)
	}

	_, err = manager.Latest(LookupOptions{Source: SOURCESubagent})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Latest(missing) error = %v, want ErrNotFound", err)
	}
}

func TestManagerDoesNotReadLegacyProjectLocalSessions(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	manager := NewManagerWithHome(workDir, homeDir)

	legacyID := "legacy-session"
	legacyRoot := filepath.Join(workDir, ".foxharness", "sessions", legacyID)
	if err := os.MkdirAll(legacyRoot, 0755); err != nil {
		t.Fatalf("MkdirAll(legacyRoot) error = %v", err)
	}
	legacy := &Session{
		ID:        legacyID,
		Source:    SOURCECLI,
		WorkDir:   cleanAbsPath(workDir),
		RootDir:   legacyRoot,
		CreatedAt: time.Now(),
	}
	data, err := json.MarshalIndent(legacy, "", " ")
	if err != nil {
		t.Fatalf("MarshalIndent(legacy) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "session.json"), append(data, '\n'), 0644); err != nil {
		t.Fatalf("WriteFile(legacy session.json) error = %v", err)
	}

	if _, err := manager.Open(legacyID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open(legacy) error = %v, want ErrNotFound", err)
	}
	if _, err := manager.Latest(LookupOptions{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Latest() with only legacy sessions error = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".foxharness", "projects")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy lookup should not create home project dirs; stat err = %v", err)
	}
}

func TestCompactStateRoundTrip(t *testing.T) {
	workDir := t.TempDir()
	manager := NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(CreateOptions{
		Source:  SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	empty, err := LoadCompactState(sess)
	if err != nil {
		t.Fatalf("LoadCompactState(empty) error = %v", err)
	}
	if empty.CoveredUntilSeq != -1 {
		t.Fatalf("empty CoveredUntilSeq = %d, want -1", empty.CoveredUntilSeq)
	}

	want := &CompactState{
		Summary:         "summary",
		CoveredUntilSeq: 42,
	}
	if err := SaveCompactState(sess, want); err != nil {
		t.Fatalf("SaveCompactState() error = %v", err)
	}

	got, err := LoadCompactState(sess)
	if err != nil {
		t.Fatalf("LoadCompactState(saved) error = %v", err)
	}
	if got.Summary != want.Summary || got.CoveredUntilSeq != want.CoveredUntilSeq {
		t.Fatalf("state = %#v, want %#v", got, want)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatalf("UpdatedAt was not set")
	}
}
