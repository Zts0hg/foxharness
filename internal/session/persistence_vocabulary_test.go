package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPersistenceIdentifiersRetainStringEncoding(t *testing.T) {
	value := struct {
		Session ID    `json:"session_id"`
		Run     RunID `json:"run_id"`
	}{
		Session: ID("session-baseline-0001"),
		Run:     RunID("run-baseline-0002"),
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(encoded), `{"session_id":"session-baseline-0001","run_id":"run-baseline-0002"}`; got != want {
		t.Fatalf("Marshal() = %s, want %s", got, want)
	}
}

func TestFileStoreOwnsStoredSessionAndRunPersistence(t *testing.T) {
	workDir := t.TempDir()
	store := NewFileStoreWithHome(workDir, t.TempDir())

	storedSession, err := store.Create(CreateOptions{
		Source:  SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	var _ *StoredSession = storedSession

	storedRun, err := store.StartRun(storedSession, "inspect persistence")
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	var _ *StoredRun = storedRun
	if err := store.FinishRun(storedRun); err != nil {
		t.Fatalf("FinishRun() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(storedRun.RootDir, "run.json"))
	if err != nil {
		t.Fatalf("ReadFile(run.json) error = %v", err)
	}
	var reloaded StoredRun
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatalf("Unmarshal(run.json) error = %v", err)
	}
	if reloaded.ID != storedRun.ID || reloaded.SessionID != storedSession.ID || reloaded.EndedAt == nil {
		t.Fatalf("reloaded run = %#v, want finished run %#v", reloaded, storedRun)
	}
}

func TestTranscriptLogRetainsTranscriptEventEncoding(t *testing.T) {
	storedSession := &StoredSession{RootDir: t.TempDir()}
	log := NewTranscriptLog(storedSession)
	if err := log.AppendRun(RunID("run-baseline-0002"), "assistant_message", map[string]any{"text": "done"}); err != nil {
		t.Fatalf("AppendRun() error = %v", err)
	}

	data, err := os.ReadFile(storedSession.TranscriptPath())
	if err != nil {
		t.Fatalf("ReadFile(transcript) error = %v", err)
	}
	if strings.Count(string(data), "\n") != 1 {
		t.Fatalf("transcript bytes = %q, want one JSONL record", data)
	}
	var event TranscriptEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("Unmarshal(transcript event) error = %v", err)
	}
	if event.RunID != RunID("run-baseline-0002") || event.Type != "assistant_message" {
		t.Fatalf("event = %#v", event)
	}
}

func TestStoredVocabularyReencodesImmutableMetadataExactly(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "characterization", "v1", "sessions", "context-lifecycle")
	tests := []struct {
		path  string
		value any
	}{
		{path: filepath.Join(root, "session.json"), value: &StoredSession{}},
		{path: filepath.Join(root, "runs", "run-baseline-0001", "run.json"), value: &StoredRun{}},
	}
	for _, test := range tests {
		original, err := os.ReadFile(test.path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", test.path, err)
		}
		if err := json.Unmarshal(original, test.value); err != nil {
			t.Fatalf("Unmarshal(%q) error = %v", test.path, err)
		}
		reencoded, err := json.MarshalIndent(test.value, "", " ")
		if err != nil {
			t.Fatalf("MarshalIndent(%q) error = %v", test.path, err)
		}
		reencoded = append(reencoded, '\n')
		if !bytes.Equal(reencoded, original) {
			t.Fatalf("reencoded %q = %q, want immutable bytes %q", test.path, reencoded, original)
		}
	}

	assertJSONLReencodesExactly[MessageRecord](t, filepath.Join(root, "messages.jsonl"))

	event := TranscriptEvent{
		Time:  time.Date(2026, 8, 10, 8, 3, 0, 0, time.UTC),
		RunID: RunID("run-baseline-0001"),
		Type:  "tool_call",
		Payload: struct {
			ID        string         `json:"id"`
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}{ID: "call-baseline-0001", Name: "read_file", Arguments: map[string]any{"path": "README.md"}},
	}
	encodedEvent, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal(TranscriptEvent) error = %v", err)
	}
	wantEvent := `{"time":"2026-08-10T08:03:00Z","run_id":"run-baseline-0001","type":"tool_call","payload":{"id":"call-baseline-0001","name":"read_file","arguments":{"path":"README.md"}}}`
	if string(encodedEvent) != wantEvent {
		t.Fatalf("Marshal(TranscriptEvent) = %s, want %s", encodedEvent, wantEvent)
	}
}

func assertJSONLReencodesExactly[T any](t *testing.T, path string) {
	t.Helper()
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	lines := bytes.Split(bytes.TrimSuffix(original, []byte{'\n'}), []byte{'\n'})
	for index, line := range lines {
		var value T
		if err := json.Unmarshal(line, &value); err != nil {
			t.Fatalf("Unmarshal(%q line %d) error = %v", path, index+1, err)
		}
		reencoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("Marshal(%q line %d) error = %v", path, index+1, err)
		}
		if !bytes.Equal(reencoded, line) {
			t.Fatalf("reencoded %q line %d = %q, want %q", path, index+1, reencoded, line)
		}
	}
}
