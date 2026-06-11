package autodev

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGit records GitRunner invocations and replays scripted outputs.
type fakeGit struct {
	calls   []string
	dirs    []string
	outputs map[string]string
	errs    map[string]error
}

func (f *fakeGit) Run(ctx context.Context, dir string, args ...string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	f.dirs = append(f.dirs, dir)
	if f.errs != nil {
		if err, ok := f.errs[key]; ok {
			return f.outputs[key], err
		}
	}
	return f.outputs[key], nil
}

func TestWorktreeCreateAddsBranchInSiblingDir(t *testing.T) {
	repoRoot := t.TempDir()
	git := &fakeGit{}
	mgr := NewWorktreeManager(git, repoRoot, "../wt", "main")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "engine-memory", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	wantPath := filepath.Join(filepath.Dir(repoRoot), "wt", "engine-memory")
	if wt.Path != wantPath {
		t.Errorf("Path = %q, want %q (sibling dir, REQ-005)", wt.Path, wantPath)
	}
	if wt.Branch != "auto/engine-memory" {
		t.Errorf("Branch = %q, want auto/engine-memory", wt.Branch)
	}
	if wt.Resumed {
		t.Error("Resumed = true for a fresh worktree, want false")
	}

	if len(git.calls) != 1 {
		t.Fatalf("git calls = %v, want exactly one worktree add", git.calls)
	}
	want := "worktree add -b auto/engine-memory " + wantPath + " main"
	if git.calls[0] != want {
		t.Errorf("git call = %q, want %q (TC-004)", git.calls[0], want)
	}
	if git.dirs[0] != repoRoot {
		t.Errorf("git dir = %q, want repo root %q", git.dirs[0], repoRoot)
	}
}

func TestWorktreeCreateResumesExistingInProgress(t *testing.T) {
	repoRoot := t.TempDir()
	wtPath := filepath.Join(filepath.Dir(repoRoot), "wt-resume", "engine-memory")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-resume", "main")

	wt, err := mgr.Create(context.Background(), LedgerItem{
		Slug:   "engine-memory",
		Status: StatusInProgress,
		Branch: "auto/engine-memory",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !wt.Resumed {
		t.Error("Resumed = false, want true for an existing in-progress worktree")
	}
	if wt.Path != wtPath {
		t.Errorf("Path = %q, want existing %q", wt.Path, wtPath)
	}
	if len(git.calls) != 0 {
		t.Errorf("git calls = %v, want none when resuming an existing worktree", git.calls)
	}
}

func TestWorktreeCreateReattachesExistingBranchWithoutDir(t *testing.T) {
	repoRoot := t.TempDir()
	git := &fakeGit{}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-reattach", "main")

	wt, err := mgr.Create(context.Background(), LedgerItem{
		Slug:   "engine-memory",
		Status: StatusInProgress,
		Branch: "auto/engine-memory",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	want := "worktree add " + wt.Path + " auto/engine-memory"
	if len(git.calls) != 1 || git.calls[0] != want {
		t.Errorf("git calls = %v, want [%q] (reuse the recorded branch)", git.calls, want)
	}
}

func TestWorktreeCreateSuffixesLeftoverPath(t *testing.T) {
	repoRoot := t.TempDir()
	base := filepath.Join(filepath.Dir(repoRoot), "wt-leftover")
	if err := os.MkdirAll(filepath.Join(base, "engine-memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-leftover", "main")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "engine-memory", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if wt.Path == filepath.Join(base, "engine-memory") {
		t.Error("Path reused a leftover directory, want a uniquely-suffixed path")
	}
	if !strings.HasPrefix(filepath.Base(wt.Path), "engine-memory-") {
		t.Errorf("Path = %q, want suffixed engine-memory-N", wt.Path)
	}
}

func TestWorktreeRemove(t *testing.T) {
	repoRoot := t.TempDir()
	git := &fakeGit{}
	mgr := NewWorktreeManager(git, repoRoot, "../wt", "main")

	wt := Worktree{Path: "/abs/wt/engine-memory", Branch: "auto/engine-memory"}
	if err := mgr.Remove(context.Background(), wt); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if len(git.calls) != 1 || git.calls[0] != "worktree remove --force /abs/wt/engine-memory" {
		t.Errorf("git calls = %v, want worktree remove (TC-014; the remote branch is untouched)", git.calls)
	}
	if git.dirs[0] != repoRoot {
		t.Errorf("git dir = %q, want repo root", git.dirs[0])
	}
}

func TestWorktreeCreatePropagatesGitError(t *testing.T) {
	repoRoot := t.TempDir()
	git := &fakeGit{errs: map[string]error{}, outputs: map[string]string{}}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-err", "main")
	wantPath := filepath.Join(filepath.Dir(repoRoot), "wt-err", "x")
	key := "worktree add -b auto/x " + wantPath + " main"
	git.errs[key] = errors.New("fatal: branch exists")

	if _, err := mgr.Create(context.Background(), LedgerItem{Slug: "x", Status: StatusPending}); err == nil {
		t.Fatal("Create returned nil error, want git failure propagated")
	}
}
