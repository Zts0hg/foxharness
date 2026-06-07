package keeprun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// gitOut runs a git command and returns its trimmed standard output.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// initRepo creates an isolated git repository with one commit on a deterministic
// "main" branch so that "git worktree add" has a valid HEAD to branch from.
func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "README.md")
	runGitTest(t, repo, "commit", "-m", "init")
	runGitTest(t, repo, "branch", "-M", "main")
	return repo
}

func TestManagerHeadCommit(t *testing.T) {
	repo := initRepo(t)
	m := NewManager(repo)
	want := gitOut(t, repo, "rev-parse", "HEAD")
	got, err := m.HeadCommit(context.Background(), repo)
	if err != nil {
		t.Fatalf("HeadCommit error: %v", err)
	}
	if got != want {
		t.Errorf("HeadCommit = %q, want %q", got, want)
	}
}

func TestNewManagerWithOptions(t *testing.T) {
	m := NewManager("/some/repo", WithTimeout(5*time.Second))
	if m.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", m.timeout)
	}
	if m.repoDir != "/some/repo" {
		t.Errorf("repoDir = %q, want /some/repo", m.repoDir)
	}
}

func TestDefaultBranch(t *testing.T) {
	repo := initRepo(t)
	m := NewManager(repo)
	got, err := m.DefaultBranch(context.Background())
	if err != nil {
		t.Fatalf("DefaultBranch error: %v", err)
	}
	if got != "main" {
		t.Errorf("DefaultBranch = %q, want main", got)
	}
}

func TestManagerCreate(t *testing.T) {
	repo := initRepo(t)
	m := NewManager(repo)
	ctx := context.Background()

	dir, err := m.Create(ctx, "add-dark-mode", "main")
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	if !filepath.IsAbs(dir) {
		t.Errorf("Create returned non-absolute path %q", dir)
	}
	if filepath.Base(dir) != "add-dark-mode" {
		t.Errorf("worktree base = %q, want add-dark-mode", filepath.Base(dir))
	}
	wantUnder := filepath.Join(repo, ".claude", "worktrees", "add-dark-mode")
	if absWant, _ := filepath.Abs(wantUnder); absWant != dir {
		t.Errorf("worktree dir = %q, want %q", dir, absWant)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("worktree dir not created: stat err=%v", err)
	}

	branches, err := m.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches error: %v", err)
	}
	if !slices.Contains(branches, "keep-run-add-dark-mode") {
		t.Errorf("branches = %v, want to contain keep-run-add-dark-mode", branches)
	}
}

// TestManagerCreateUsesBaseRef verifies that Create branches from the explicit
// baseRef rather than from the caller's current HEAD.
func TestManagerCreateUsesBaseRef(t *testing.T) {
	repo := initRepo(t)
	m := NewManager(repo)
	ctx := context.Background()

	mainSHA := gitOut(t, repo, "rev-parse", "main")

	// Move HEAD onto a different branch with its own commit.
	runGitTest(t, repo, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "feature.txt")
	runGitTest(t, repo, "commit", "-m", "feature work")
	featureSHA := gitOut(t, repo, "rev-parse", "HEAD")
	if featureSHA == mainSHA {
		t.Fatal("precondition failed: feature commit should differ from main")
	}

	dir, err := m.Create(ctx, "task", "main")
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if got := gitOut(t, dir, "rev-parse", "HEAD"); got != mainSHA {
		t.Errorf("worktree HEAD = %q, want main %q (not feature %q)", got, mainSHA, featureSHA)
	}
}

func TestManagerCreateCollisionErrors(t *testing.T) {
	repo := initRepo(t)
	m := NewManager(repo)
	ctx := context.Background()

	if _, err := m.Create(ctx, "dup", "main"); err != nil {
		t.Fatalf("first Create error: %v", err)
	}
	if _, err := m.Create(ctx, "dup", "main"); err == nil {
		t.Fatal("second Create with same slug: expected error, got nil")
	}
}

func TestManagerRemove(t *testing.T) {
	repo := initRepo(t)
	m := NewManager(repo)
	ctx := context.Background()

	dir, err := m.Create(ctx, "to-remove", "main")
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	// A state file is an untracked file in the worktree; Remove must still
	// succeed (it uses --force).
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := m.Remove(ctx, dir); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after Remove: stat err=%v", err)
	}

	if err := m.Remove(ctx, dir); err == nil {
		t.Error("Remove on already-removed worktree: expected error, got nil")
	}
}

func TestManagerRemovePreservesBranch(t *testing.T) {
	repo := initRepo(t)
	m := NewManager(repo)
	ctx := context.Background()

	dir, err := m.Create(ctx, "keep-branch", "main")
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if err := m.Remove(ctx, dir); err != nil {
		t.Fatalf("Remove error: %v", err)
	}

	// The feature branch must survive worktree removal so it can serve as the
	// task artifact in local-only mode (FR-006).
	branches, err := m.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches error: %v", err)
	}
	if !slices.Contains(branches, "keep-run-keep-branch") {
		t.Errorf("branch removed with worktree; branches = %v", branches)
	}
}

func TestManagerListBranchesOutsideRepoErrors(t *testing.T) {
	// A temp dir that is not a git repository makes git fail, exercising the
	// runGit error branch.
	m := NewManager(t.TempDir())
	if _, err := m.ListBranches(context.Background()); err == nil {
		t.Fatal("ListBranches outside a git repo: expected error, got nil")
	}
}

func TestManagerListBranches(t *testing.T) {
	repo := initRepo(t)
	m := NewManager(repo)
	ctx := context.Background()

	branches, err := m.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches error: %v", err)
	}
	if len(branches) != 0 {
		t.Errorf("expected no keep-run branches initially, got %v", branches)
	}

	if _, err := m.Create(ctx, "alpha", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, "beta", "main"); err != nil {
		t.Fatal(err)
	}

	branches, err = m.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches error: %v", err)
	}
	for _, want := range []string{"keep-run-alpha", "keep-run-beta"} {
		if !slices.Contains(branches, want) {
			t.Errorf("branches = %v, want to contain %q", branches, want)
		}
	}
}

func TestIsWorktree(t *testing.T) {
	repo := initRepo(t)

	// Main repo is not a worktree
	if IsWorktree(repo) {
		t.Errorf("main repo incorrectly identified as worktree: %s", repo)
	}

	// Create a worktree
	ctx := context.Background()
	m := NewManager(repo)
	worktreeDir, err := m.Create(ctx, "test-worktree", "main")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Remove(ctx, worktreeDir); err != nil {
			t.Logf("cleanup worktree failed: %v", err)
		}
	}()

	// Worktree should be detected
	if !IsWorktree(worktreeDir) {
		t.Errorf("worktree not detected: %s", worktreeDir)
	}
}

func TestResolveMainRepo(t *testing.T) {
	repo := initRepo(t)
	// Normalize repo path to handle macOS /tmp -> /private/tmp symlink
	absRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Test with main repo (should return same path, not a worktree)
	mainRepo, isWorktree, err := DetectRepoEnvironment(repo)
	if err != nil {
		t.Fatalf("DetectRepoEnvironment on main repo failed: %v", err)
	}
	if isWorktree {
		t.Error("main repo incorrectly detected as worktree")
	}
	// Normalize mainRepo for comparison
	absMainRepo, err := filepath.EvalSymlinks(mainRepo)
	if err != nil {
		t.Fatal(err)
	}
	if absMainRepo != absRepo {
		t.Errorf("main repo path mismatch: got %s, want %s", absMainRepo, absRepo)
	}

	// Create a worktree
	m := NewManager(repo)
	worktreeDir, err := m.Create(ctx, "test-worktree", "main")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Remove(ctx, worktreeDir); err != nil {
			t.Logf("cleanup worktree failed: %v", err)
		}
	}()

	// Test ResolveMainRepo from worktree
	resolvedMain, err := ResolveMainRepo(worktreeDir)
	if err != nil {
		t.Fatalf("ResolveMainRepo failed: %v", err)
	}
	// Normalize resolved path for comparison
	absResolved, err := filepath.EvalSymlinks(resolvedMain)
	if err != nil {
		t.Fatal(err)
	}
	if absResolved != absRepo {
		t.Errorf("ResolveMainRepo returned %s, want %s", absResolved, absRepo)
	}

	// Test DetectRepoEnvironment from worktree
	detectedMain, detectedIsWorktree, err := DetectRepoEnvironment(worktreeDir)
	if err != nil {
		t.Fatalf("DetectRepoEnvironment from worktree failed: %v", err)
	}
	if !detectedIsWorktree {
		t.Error("worktree not detected by DetectRepoEnvironment")
	}
	absDetected, err := filepath.EvalSymlinks(detectedMain)
	if err != nil {
		t.Fatal(err)
	}
	if absDetected != absRepo {
		t.Errorf("DetectRepoEnvironment returned %s, want %s", absDetected, absRepo)
	}
}

func TestDetectRepoEnvironment(t *testing.T) {
	repo := initRepo(t)
	// Normalize repo path to handle macOS /tmp -> /private/tmp symlink
	absRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Test from main repo
	mainRepo, isWorktree, err := DetectRepoEnvironment(repo)
	if err != nil {
		t.Fatalf("DetectRepoEnvironment from main repo failed: %v", err)
	}
	if isWorktree {
		t.Error("main repo incorrectly detected as worktree")
	}
	// Normalize for comparison
	absMain, err := filepath.EvalSymlinks(mainRepo)
	if err != nil {
		t.Fatal(err)
	}
	if absMain != absRepo {
		t.Errorf("main repo path: got %s, want %s", absMain, absRepo)
	}

	// Create a worktree
	m := NewManager(repo)
	worktreeDir, err := m.Create(ctx, "test-worktree", "main")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Remove(ctx, worktreeDir); err != nil {
			t.Logf("cleanup worktree failed: %v", err)
		}
	}()

	// Test from worktree
	detectedRepo, detectedIsWorktree, err := DetectRepoEnvironment(worktreeDir)
	if err != nil {
		t.Fatalf("DetectRepoEnvironment from worktree failed: %v", err)
	}
	if !detectedIsWorktree {
		t.Error("worktree not detected")
	}
	// Normalize for comparison
	absDetected, err := filepath.EvalSymlinks(detectedRepo)
	if err != nil {
		t.Fatal(err)
	}
	if absDetected != absRepo {
		t.Errorf("detected main repo: got %s, want %s", absDetected, absRepo)
	}
}
