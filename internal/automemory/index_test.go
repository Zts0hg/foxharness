package automemory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func saveN(t *testing.T, store *Store, scope Scope, n int, descLen int) {
	t.Helper()
	typ := TypeUser
	if scope == ScopeProject {
		typ = TypeReference
	}
	for i := 0; i < n; i++ {
		desc := strings.Repeat("x", descLen)
		if desc == "" {
			desc = "d"
		}
		mem := Memory{
			Name:        fmt.Sprintf("mem-%04d", i),
			Description: desc,
			Type:        typ,
			Body:        "body",
		}
		if err := store.Save(mem); err != nil {
			t.Fatalf("Save(%d) error = %v", i, err)
		}
	}
}

func countEntries(index string) (entries int, hasNotice bool) {
	for _, line := range strings.Split(index, "\n") {
		switch {
		case strings.HasPrefix(line, "- ["):
			entries++
		case strings.HasPrefix(line, "- …"):
			hasNotice = true
		}
	}
	return entries, hasNotice
}

func TestBuildIndexEntryFormatAndLineLength(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(Memory{Name: "user-role", Description: "Staff engineer, terse answers.", Type: TypeUser, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	index, err := store.BuildIndex(ScopeUserGlobal)
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	want := "- [user-role](user-role.md) — Staff engineer, terse answers."
	if strings.TrimSpace(index) != want {
		t.Fatalf("index = %q, want %q", index, want)
	}
}

func TestBuildIndexCanonicalizesNameWithMarkdownSuffix(t *testing.T) {
	store := newTestStore(t)
	if err := store.dirs.EnsureDir(ScopeProject); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: project-note.md\ndescription: Project convention.\ntype: reference\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(store.dirs.Dir(ScopeProject), "project-note.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	index, err := store.BuildIndex(ScopeProject)
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	if strings.Contains(index, "project-note.md.md") {
		t.Fatalf("index rendered a doubled markdown suffix:\n%s", index)
	}
	want := "- [project-note](project-note.md) — Project convention."
	if strings.TrimSpace(index) != want {
		t.Fatalf("index = %q, want %q", index, want)
	}
}

func TestBuildIndexEnforcesLineLength(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(Memory{Name: "longone", Description: strings.Repeat("y", 400), Type: TypeUser, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	index, err := store.BuildIndex(ScopeUserGlobal)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(index), "\n") {
		if n := len([]rune(line)); n >= 150 {
			t.Fatalf("index line has %d chars, must be < 150: %q", n, line)
		}
	}
}

func TestBuildIndexTruncatesByLineCount(t *testing.T) {
	store := newTestStore(t)
	saveN(t, store, ScopeUserGlobal, 250, 0)
	index, err := store.BuildIndex(ScopeUserGlobal)
	if err != nil {
		t.Fatal(err)
	}
	entries, hasNotice := countEntries(index)
	if entries > maxIndexLines {
		t.Fatalf("entries = %d, want <= %d", entries, maxIndexLines)
	}
	if !hasNotice {
		t.Fatalf("expected a truncation notice when over the line cap")
	}
}

func TestBuildIndexEnforcesByteCap(t *testing.T) {
	store := newTestStore(t)
	saveN(t, store, ScopeUserGlobal, 250, 110)
	index, err := store.BuildIndex(ScopeUserGlobal)
	if err != nil {
		t.Fatal(err)
	}
	entries, hasNotice := countEntries(index)
	if !hasNotice {
		t.Fatalf("expected a truncation notice when over the byte cap")
	}
	if entries >= 250 {
		t.Fatalf("byte cap should have dropped entries; got %d", entries)
	}
	if len(index) > maxIndexBytes+512 {
		t.Fatalf("index is %d bytes, expected near the %d cap", len(index), maxIndexBytes)
	}
}

func TestBuildIndexReflectsCurrentFilesNoDrift(t *testing.T) {
	store := newTestStore(t)
	saveN(t, store, ScopeProject, 3, 0)
	// Forget is an empty-content write: the emptied file becomes non-loadable and
	// must drop out of the regenerated index (no drift).
	memPath, err := store.dirs.FilePath(ScopeProject, "mem-0001")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memPath, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	index, err := store.BuildIndex(ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(index, "mem-0001") {
		t.Fatalf("removed memory still in index:\n%s", index)
	}
	entries, _ := countEntries(index)
	if entries != 2 {
		t.Fatalf("entries = %d, want 2 after removal", entries)
	}
}

func TestBuildIndexEmptyScope(t *testing.T) {
	store := newTestStore(t)
	index, err := store.BuildIndex(ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(index) != "" {
		t.Fatalf("empty scope index = %q, want empty", index)
	}
}
