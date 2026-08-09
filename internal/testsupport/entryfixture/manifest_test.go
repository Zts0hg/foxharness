package entryfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
