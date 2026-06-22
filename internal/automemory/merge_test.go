package automemory

import (
	"strings"
	"testing"
)

func seedTwoTier(t *testing.T, store *Store) {
	t.Helper()
	if err := store.Save(Memory{Name: "user-role", Description: "Staff engineer.", Type: TypeUser, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Memory{Name: "proj-build", Description: "Build with make.", Type: TypeProject, Body: "rule\n\n**Why:** w\n**How to apply:** h"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Memory{Name: "ref-dash", Description: "Dashboard URL.", Type: TypeReference, Body: "https://x"}); err != nil {
		t.Fatal(err)
	}
}

func TestMergedIndexStringMergesBothTiers(t *testing.T) {
	store := newTestStore(t)
	seedTwoTier(t, store)

	merged := store.MergedIndexString()
	for _, want := range []string{
		"user-role.md",
		"proj-build.md",
		"ref-dash.md",
	} {
		if !strings.Contains(merged, want) {
			t.Fatalf("merged index missing %q:\n%s", want, merged)
		}
	}
}

func TestMergedIndexStringEmptyWhenNoMemories(t *testing.T) {
	store := newTestStore(t)
	if got := strings.TrimSpace(store.MergedIndexString()); got != "" {
		t.Fatalf("merged index = %q, want empty", got)
	}
}

func TestManifestListsExistingMemoriesWithTypes(t *testing.T) {
	store := newTestStore(t)
	seedTwoTier(t, store)

	manifest := store.Manifest()
	for _, want := range []string{
		"[user] user-role.md: Staff engineer.",
		"[project] proj-build.md: Build with make.",
		"[reference] ref-dash.md: Dashboard URL.",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
}

func TestManifestEmptyWhenNoMemories(t *testing.T) {
	store := newTestStore(t)
	if got := strings.TrimSpace(store.Manifest()); got != "" {
		t.Fatalf("manifest = %q, want empty", got)
	}
}
