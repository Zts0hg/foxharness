package automemory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/session"
)

func TestScopeForType(t *testing.T) {
	if ScopeForType(TypeUser) != ScopeUserGlobal {
		t.Fatalf("user type must map to user-global scope")
	}
	for _, typ := range []MemoryType{TypeProject, TypeFeedback, TypeReference} {
		if ScopeForType(typ) != ScopeProject {
			t.Fatalf("type %s must map to project scope", typ)
		}
	}
}

func TestDirsResolveTwoTiers(t *testing.T) {
	home := t.TempDir()
	workDir := "/Users/dev/code/widget"
	dirs := NewDirs(home, workDir)

	wantUser := filepath.Join(home, ".foxharness", "memory")
	if got := dirs.Dir(ScopeUserGlobal); got != wantUser {
		t.Fatalf("user-global dir = %q, want %q", got, wantUser)
	}

	key := session.EncodeProjectPath(workDir)
	wantProject := filepath.Join(home, ".foxharness", "projects", key, "memory")
	if got := dirs.Dir(ScopeProject); got != wantProject {
		t.Fatalf("project dir = %q, want %q", got, wantProject)
	}
	if got := dirs.DirForType(TypeFeedback); got != wantProject {
		t.Fatalf("feedback dir = %q, want %q", got, wantProject)
	}
}

func TestEnsureDirIsIdempotent(t *testing.T) {
	dirs := NewDirs(t.TempDir(), "/Users/dev/code/widget")
	for i := 0; i < 2; i++ {
		if err := dirs.EnsureDir(ScopeProject); err != nil {
			t.Fatalf("EnsureDir() iteration %d error = %v", i, err)
		}
	}
	if info, err := os.Stat(dirs.Dir(ScopeProject)); err != nil || !info.IsDir() {
		t.Fatalf("project dir was not created: err=%v", err)
	}
}

func TestFilePathRejectsTraversalSlugs(t *testing.T) {
	dirs := NewDirs(t.TempDir(), "/Users/dev/code/widget")
	bad := []string{"", "../escape", "a/b", "..", "foo/../bar", `back\slash`}
	for _, name := range bad {
		if _, err := dirs.FilePath(ScopeProject, name); err == nil {
			t.Fatalf("FilePath(%q) expected rejection", name)
		}
	}

	good, err := dirs.FilePath(ScopeProject, "user-role")
	if err != nil {
		t.Fatalf("FilePath(good) error = %v", err)
	}
	want := filepath.Join(dirs.Dir(ScopeProject), "user-role.md")
	if good != want {
		t.Fatalf("FilePath = %q, want %q", good, want)
	}
	// A name already ending in .md must not double the suffix.
	withExt, err := dirs.FilePath(ScopeProject, "user-role.md")
	if err != nil {
		t.Fatalf("FilePath(.md) error = %v", err)
	}
	if withExt != want {
		t.Fatalf("FilePath(.md) = %q, want %q", withExt, want)
	}
}
