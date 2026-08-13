package entryfixture

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestManifestVerifyDetectsFixtureTampering(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "sessions", "session.json"), []byte("baseline"))
	writeTestManifest(t, root, Manifest{
		Version:        "v1",
		BaselineStatus: BaselineStatusFrozen,
		BaselineCommit: "abc123",
		Fixtures: []Fixture{{
			Path:         "sessions/session.json",
			SHA256:       testHash([]byte("baseline")),
			SourceCommit: "abc123",
			Source:       "CLIExec",
			Semantics:    "stored CLI session metadata",
		}},
	})

	manifest, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := manifest.Verify(root); err != nil {
		t.Fatalf("Verify() baseline error = %v", err)
	}

	writeTestFile(t, filepath.Join(root, "sessions", "session.json"), []byte("changed"))
	if err := manifest.Verify(root); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("Verify() tampered error = %v, want sha256 mismatch", err)
	}
}

func TestManifestRejectsIncompleteFrozenAuthority(t *testing.T) {
	root := t.TempDir()
	writeTestManifest(t, root, Manifest{
		Version:        "v1",
		BaselineStatus: BaselineStatusFrozen,
	})

	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "baseline_commit") {
		t.Fatalf("Load() error = %v, want missing baseline_commit", err)
	}
}

func TestManifestRejectsEmptyFrozenAuthority(t *testing.T) {
	root := t.TempDir()
	writeTestManifest(t, root, Manifest{
		Version:        "v1",
		BaselineStatus: BaselineStatusFrozen,
		BaselineCommit: "abc123",
	})

	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "at least one fixture") {
		t.Fatalf("Load() error = %v, want empty frozen authority rejection", err)
	}
}

func TestManifestRejectsMixedFrozenSourceCommits(t *testing.T) {
	root := t.TempDir()
	writeTestManifest(t, root, Manifest{
		Version:        "v1",
		BaselineStatus: BaselineStatusFrozen,
		BaselineCommit: "abc123",
		Fixtures: []Fixture{{
			Path:         "session.json",
			SHA256:       testHash([]byte("baseline")),
			SourceCommit: "def456",
			Source:       "CLIExec",
			Semantics:    "stored session metadata",
		}},
	})

	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "baseline_commit") {
		t.Fatalf("Load() error = %v, want mixed source commit rejection", err)
	}
}

func TestManifestRejectsUnsortedFixturePaths(t *testing.T) {
	root := t.TempDir()
	writeTestManifest(t, root, Manifest{
		Version:        "v1",
		BaselineStatus: BaselineStatusFrozen,
		BaselineCommit: "abc123",
		Fixtures: []Fixture{
			fixtureForTest("z.txt", "abc123", []byte("z")),
			fixtureForTest("a.txt", "abc123", []byte("a")),
		},
	})

	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "sorted") {
		t.Fatalf("Load() error = %v, want unsorted path rejection", err)
	}
}

func TestManifestVerifyRejectsUnlistedFixtureFile(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "listed.txt"), []byte("listed"))
	writeTestFile(t, filepath.Join(root, "unlisted.txt"), []byte("unlisted"))
	writeTestManifest(t, root, Manifest{
		Version:        "v1",
		BaselineStatus: BaselineStatusFrozen,
		BaselineCommit: "abc123",
		Fixtures: []Fixture{
			fixtureForTest("listed.txt", "abc123", []byte("listed")),
		},
	})

	manifest, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := manifest.Verify(root); err == nil || !strings.Contains(err.Error(), "unlisted fixture") {
		t.Fatalf("Verify() error = %v, want unlisted fixture rejection", err)
	}
}

func TestCharacterizationV1AuthorityIsFrozenVerifiableAndCopyable(t *testing.T) {
	const correctedSourceCommit = "ee649228970ed08cbf567df4f6ca576560323585"
	root := filepath.Join("..", "..", "..", "testdata", "characterization", "v1")

	manifest, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if manifest.BaselineStatus != BaselineStatusFrozen {
		t.Fatalf("baseline_status = %q, want %q", manifest.BaselineStatus, BaselineStatusFrozen)
	}
	if manifest.BaselineCommit != correctedSourceCommit {
		t.Fatalf("baseline_commit = %q, want %q", manifest.BaselineCommit, correctedSourceCommit)
	}
	if err := manifest.Verify(root); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	destinationRoot := t.TempDir()
	for _, fixture := range manifest.Fixtures {
		copied, err := CopyFixture(root, fixture.Path, destinationRoot)
		if err != nil {
			t.Fatalf("CopyFixture(%q) error = %v", fixture.Path, err)
		}
		original, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(fixture.Path)))
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", fixture.Path, err)
		}
		copyData, err := os.ReadFile(copied)
		if err != nil {
			t.Fatalf("ReadFile(copy %q) error = %v", fixture.Path, err)
		}
		if string(copyData) != string(original) {
			t.Fatalf("copied fixture %q differs from immutable source", fixture.Path)
		}
	}
}

func TestCharacterizationV1PersistedSessionIsReadableAfterCopy(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "characterization", "v1")
	manifest, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	destinationRoot := t.TempDir()
	for _, fixture := range manifest.Fixtures {
		if _, err := CopyFixture(root, fixture.Path, destinationRoot); err != nil {
			t.Fatalf("CopyFixture(%q) error = %v", fixture.Path, err)
		}
	}

	sessionRoot := filepath.Join(destinationRoot, "sessions", "context-lifecycle")
	var persistedSession session.StoredSession
	decodeJSONFile(t, filepath.Join(sessionRoot, "session.json"), &persistedSession)
	if persistedSession.ID != "session-baseline-0001" || persistedSession.Source != session.SOURCECLI {
		t.Fatalf("persisted session identity = %#v", persistedSession)
	}
	persistedSession.RootDir = sessionRoot

	records, err := session.NewMessageLog(&persistedSession).LoadRecords()
	if err != nil {
		t.Fatalf("LoadRecords() error = %v", err)
	}
	if len(records) != 7 || records[5].RunID != "run-baseline-0002" || records[6].Message.Content != "The next step is characterization coverage." {
		t.Fatalf("continuation records = %#v", records)
	}
	compact, err := session.LoadCompactState(&persistedSession)
	if err != nil {
		t.Fatalf("LoadCompactState() error = %v", err)
	}
	if compact.CoveredUntilSeq != 4 || compact.Summary == "" {
		t.Fatalf("compact state = %#v", compact)
	}

	cp, ok := checkpoint.New(checkpoint.Config{SessionDir: sessionRoot}).(*checkpoint.FileCheckpointer)
	if !ok {
		t.Fatal("checkpoint.New() did not return *FileCheckpointer")
	}
	if err := cp.RestoreStateFromLog(); err != nil {
		t.Fatalf("RestoreStateFromLog() error = %v", err)
	}
	state := cp.State()
	if len(state.Snapshots) != 2 || state.Snapshots[1].TrackedFileBackups["WORKSPACE/main.go"].Version != 2 {
		t.Fatalf("checkpoint state = %#v", state)
	}

	store := memory.NewSessionStore(t.TempDir(), sessionRoot)
	history := memory.NewStateHistory(store)
	if err := history.RestoreBeforeMessage(5); err != nil {
		t.Fatalf("RestoreBeforeMessage(5) error = %v", err)
	}
	plan, err := os.ReadFile(store.PlanPath())
	if err != nil {
		t.Fatalf("ReadFile(restored PLAN.md) error = %v", err)
	}
	if string(plan) != "# PLAN\n\n## Goal\n\nInspect the runtime boundary.\n" {
		t.Fatalf("restored PLAN.md = %q", plan)
	}
	if err := history.RestoreBeforeMessage(0); err != nil {
		t.Fatalf("RestoreBeforeMessage(0) error = %v", err)
	}
	if _, err := os.Stat(store.PlanPath()); !os.IsNotExist(err) {
		t.Fatalf("PLAN.md after absent-state restore: stat error = %v", err)
	}

	var firstRun session.StoredRun
	decodeJSONFile(t, filepath.Join(sessionRoot, "runs", "run-baseline-0001", "run.json"), &firstRun)
	if firstRun.SessionID != persistedSession.ID || firstRun.EndedAt == nil {
		t.Fatalf("first run = %#v", firstRun)
	}
	transcript := decodeTranscript(t, filepath.Join(sessionRoot, "transcript.jsonl"))
	if len(transcript) != 6 || transcript[0].Type != "user_prompt" || transcript[5].RunID != "run-baseline-0002" {
		t.Fatalf("transcript = %#v", transcript)
	}

	if err := manifest.Verify(root); err != nil {
		t.Fatalf("Verify(original after copied-state mutation) error = %v", err)
	}
}

func TestCopyFixtureCreatesIndependentFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "outputs", "cli.txt")
	writeTestFile(t, source, []byte("original"))
	destinationRoot := t.TempDir()

	copied, err := CopyFixture(root, "outputs/cli.txt", destinationRoot)
	if err != nil {
		t.Fatalf("CopyFixture() error = %v", err)
	}
	writeTestFile(t, copied, []byte("mutated copy"))
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(original) error = %v", err)
	}
	if string(original) != "original" {
		t.Fatalf("original = %q, want unchanged", original)
	}
}

func TestCopyFixtureRejectsTraversal(t *testing.T) {
	if _, err := CopyFixture(t.TempDir(), "../outside", t.TempDir()); err == nil {
		t.Fatal("CopyFixture() error = nil, want traversal rejection")
	}
}

func TestSequenceClockAndIDsAreDeterministic(t *testing.T) {
	start := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	clock := NewSequenceClock(start, 2*time.Second)
	if got := clock.Now(); !got.Equal(start) {
		t.Fatalf("first Now() = %v, want %v", got, start)
	}
	if got := clock.Now(); !got.Equal(start.Add(2 * time.Second)) {
		t.Fatalf("second Now() = %v, want %v", got, start.Add(2*time.Second))
	}

	ids := NewIDSequence("run", 7)
	if got := ids.Next(); got != "run-0007" {
		t.Fatalf("first Next() = %q, want run-0007", got)
	}
	if got := ids.Next(); got != "run-0008" {
		t.Fatalf("second Next() = %q, want run-0008", got)
	}
}

func writeTestManifest(t *testing.T, root string, manifest Manifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	writeTestFile(t, filepath.Join(root, ManifestFilename), data)
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func testHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fixtureForTest(path, commit string, data []byte) Fixture {
	return Fixture{
		Path:         path,
		SHA256:       testHash(data),
		SourceCommit: commit,
		Source:       "CLIExec",
		Semantics:    "test fixture",
	}
}

func decodeJSONFile(t *testing.T, path string, destination any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}
}

func decodeTranscript(t *testing.T, path string) []session.TranscriptEvent {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	defer file.Close()

	var events []session.TranscriptEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event session.TranscriptEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("Unmarshal transcript event error = %v", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan transcript error = %v", err)
	}
	return events
}
