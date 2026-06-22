package automemory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(t.TempDir(), "/Users/dev/code/widget")
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	store := newTestStore(t)
	mem := Memory{
		Name:        "user-role",
		Description: "Staff engineer, terse answers.",
		Type:        TypeUser,
		Body:        "The user is a staff engineer.",
	}
	if err := store.Save(mem); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load(ScopeUserGlobal)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("Load() len = %d, want 1", len(loaded))
	}
	got := loaded[0]
	if got.Name != mem.Name || got.Description != mem.Description || got.Type != mem.Type {
		t.Fatalf("loaded frontmatter mismatch: %+v", got)
	}
	if strings.TrimSpace(got.Body) != mem.Body {
		t.Fatalf("loaded body = %q, want %q", got.Body, mem.Body)
	}
}

func TestStoreSaveValidatesBeforeWriting(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(Memory{Name: "", Description: "d", Type: TypeUser}); err == nil {
		t.Fatalf("Save() expected validation error for empty name")
	}
}

func TestStoreSaveIsAtomicNoTempLeftovers(t *testing.T) {
	store := newTestStore(t)
	mem := Memory{Name: "n", Description: "d", Type: TypeProject, Body: "rule\n\n**Why:** w\n**How to apply:** h"}
	if err := store.Save(mem); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	// Overwrite to exercise the rename-over-existing path.
	mem.Description = "updated"
	if err := store.Save(mem); err != nil {
		t.Fatalf("Save() overwrite error = %v", err)
	}

	entries, err := os.ReadDir(store.dirs.Dir(ScopeProject))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") || strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
	loaded, _ := store.Load(ScopeProject)
	if len(loaded) != 1 || loaded[0].Description != "updated" {
		t.Fatalf("overwrite did not take effect: %+v", loaded)
	}
}

func TestStoreLoadSkipsMalformedFrontmatter(t *testing.T) {
	store := newTestStore(t)
	if err := store.dirs.EnsureDir(ScopeProject); err != nil {
		t.Fatalf("EnsureDir() error = %v", err)
	}
	dir := store.dirs.Dir(ScopeProject)
	if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte("not a memory at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := Memory{Name: "good", Description: "d", Type: TypeReference, Body: "x"}
	if err := store.Save(good); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load(ScopeProject)
	if err != nil {
		t.Fatalf("Load() must not error on malformed files: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Name != "good" {
		t.Fatalf("Load() should skip malformed and keep good; got %+v", loaded)
	}
}

func TestStoreLoadIgnoresIndexFile(t *testing.T) {
	store := newTestStore(t)
	_ = store.dirs.EnsureDir(ScopeUserGlobal)
	if err := os.WriteFile(store.dirs.IndexPath(ScopeUserGlobal), []byte("# Memory Index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ScopeUserGlobal)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("Load() must ignore %s; got %+v", indexFileName, loaded)
	}
}

func TestStoreLoadEmptyScopeReturnsNothing(t *testing.T) {
	store := newTestStore(t)
	loaded, err := store.Load(ScopeProject)
	if err != nil {
		t.Fatalf("Load() on empty/absent scope error = %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("Load() = %+v, want empty", loaded)
	}
}

// TestStoreLoadSkipsFileWithUnsafeName simulates an inline write_file that
// bypassed Store.Save and wrote a memory whose frontmatter name would escape
// the memory directory (e.g. "../escape"). Such a memory must be rejected at
// load time so it is never rendered into the injected index as a traversal
// link.
func TestStoreLoadSkipsFileWithUnsafeName(t *testing.T) {
	store := newTestStore(t)
	if err := store.dirs.EnsureDir(ScopeProject); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: ../escape\ndescription: evil link\ntype: reference\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(store.dirs.Dir(ScopeProject), "evil.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(ScopeProject)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("unsafe-name memory must be skipped, got %+v", loaded)
	}
	idx, err := store.BuildIndex(ScopeProject)
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	if strings.Contains(idx, "escape") || strings.Contains(idx, "..") {
		t.Fatalf("index must not render an unsafe name:\n%s", idx)
	}
}

func TestStoreLoadSkipsFileWithNonSlugName(t *testing.T) {
	store := newTestStore(t)
	if err := store.dirs.EnsureDir(ScopeProject); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: bad](link\ndescription: prompt injection\ntype: reference\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(store.dirs.Dir(ScopeProject), "bad](link.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(ScopeProject)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("non-slug memory must be skipped, got %+v", loaded)
	}
	idx, err := store.BuildIndex(ScopeProject)
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	if strings.Contains(idx, "bad](") {
		t.Fatalf("index must not render a non-slug name:\n%s", idx)
	}
	if manifest := store.Manifest(); strings.Contains(manifest, "bad](") {
		t.Fatalf("manifest must not render a non-slug name:\n%s", manifest)
	}
}

// TestStoreLoadSkipsTypeInWrongScope ensures a memory whose type implies a
// different scope than the directory it was written to is skipped, so a
// project-typed file dropped into the user-global dir (e.g. by a confused
// extraction pass) is not injected for every project.
func TestStoreLoadSkipsTypeInWrongScope(t *testing.T) {
	store := newTestStore(t)
	if err := store.dirs.EnsureDir(ScopeUserGlobal); err != nil {
		t.Fatal(err)
	}
	// type: project belongs in the project scope, but it was written into the
	// user-global directory directly.
	raw := "---\nname: leaked-build\ndescription: project fact in global dir\ntype: project\n---\n\nrule\n\n**Why:** w\n**How to apply:** h\n"
	if err := os.WriteFile(filepath.Join(store.dirs.Dir(ScopeUserGlobal), "leaked-build.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(ScopeUserGlobal)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("a project-typed memory in the user-global dir must be skipped, got %+v", loaded)
	}
}

// TestStoreLoadSkipsNameFilenameMismatch ensures a memory whose frontmatter name
// does not match its filename is skipped, so the index never advertises a link
// to a non-existent file (foo.md with name: bar must not index [bar](bar.md)).
func TestStoreLoadSkipsNameFilenameMismatch(t *testing.T) {
	store := newTestStore(t)
	if err := store.dirs.EnsureDir(ScopeProject); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: bar\ndescription: mismatched\ntype: reference\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(store.dirs.Dir(ScopeProject), "foo.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(ScopeProject)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("a name/filename mismatch must be skipped, got %+v", loaded)
	}
}

func TestStoreRejectsNestedMemoryAsLoadable(t *testing.T) {
	store := newTestStore(t)
	if err := store.dirs.EnsureDir(ScopeProject); err != nil {
		t.Fatal(err)
	}
	nestedPath := filepath.Join(store.dirs.Dir(ScopeProject), "topic", "nested.md")
	if err := os.MkdirAll(filepath.Dir(nestedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: nested\ndescription: d\ntype: reference\n---\n\nbody\n"
	if err := os.WriteFile(nestedPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	if store.IsLoadableMemoryAt(nestedPath) {
		t.Fatalf("nested memory file must not be considered loadable because Load/BuildIndex scan only direct children")
	}
	loaded, err := store.Load(ScopeProject)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("nested memory file must not be loaded, got %+v", loaded)
	}
}

// TestStoreIsLoadableMemoryAt verifies the predicate the tracker uses to decide
// whether a successful memory-dir write actually produced a valid, indexable
// memory (P2-2).
func TestStoreIsLoadableMemoryAt(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(Memory{Name: "user-role", Description: "d", Type: TypeUser, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	valid, err := store.dirs.FilePath(ScopeUserGlobal, "user-role")
	if err != nil {
		t.Fatal(err)
	}
	if !store.IsLoadableMemoryAt(valid) {
		t.Fatalf("a valid user memory must be loadable")
	}

	dir := store.dirs.Dir(ScopeUserGlobal)
	bad := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(bad, []byte("no frontmatter at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if store.IsLoadableMemoryAt(bad) {
		t.Fatalf("a malformed file must not be loadable")
	}

	idx := store.dirs.IndexPath(ScopeUserGlobal)
	if err := os.WriteFile(idx, []byte("# index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if store.IsLoadableMemoryAt(idx) {
		t.Fatalf("the index file must not be loadable as a memory")
	}

	if store.IsLoadableMemoryAt(filepath.Join(t.TempDir(), "elsewhere.md")) {
		t.Fatalf("a path outside the memory dirs must not be loadable")
	}
}
