package keeprun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// worktreeBranchPrefix is prepended to a task slug to form the branch name.
	worktreeBranchPrefix = "keep-run-"
	// defaultGitTimeout bounds each git invocation so a hung command cannot stall
	// the pipeline indefinitely.
	defaultGitTimeout = 2 * time.Minute
)

// Manager handles git worktree lifecycle operations for task isolation. Each
// task is developed in its own worktree under .claude/worktrees/<slug> on a
// dedicated keep-run-<slug> branch (spec FR-005).
type Manager struct {
	repoDir string
	timeout time.Duration
}

// ManagerOption configures a Manager via the functional-options pattern.
type ManagerOption func(*Manager)

// WithTimeout overrides the per-command git timeout.
func WithTimeout(d time.Duration) ManagerOption {
	return func(m *Manager) { m.timeout = d }
}

// NewManager creates a worktree manager rooted at repoDir.
func NewManager(repoDir string, opts ...ManagerOption) *Manager {
	m := &Manager{repoDir: repoDir, timeout: defaultGitTimeout}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// worktreesRoot returns the directory that holds all keep-run worktrees.
func worktreesRoot(repoDir string) string {
	return filepath.Join(repoDir, ".claude", "worktrees")
}

// Create creates a new worktree at .claude/worktrees/<slug> with a fresh branch
// keep-run-<slug> rooted at baseRef. baseRef is typically the repository's
// default branch (see DefaultBranch) so that worktrees stay isolated from the
// caller's current checkout; when baseRef is empty, the branch is rooted at the
// current HEAD. It returns the absolute path to the worktree directory.
//
// Create fails if the branch already exists; callers requiring uniqueness should
// resolve collisions with ListBranches and DeduplicateSlug before calling.
func (m *Manager) Create(ctx context.Context, slug, baseRef string) (string, error) {
	if err := os.MkdirAll(worktreesRoot(m.repoDir), 0o755); err != nil {
		return "", fmt.Errorf("create worktrees root: %w", err)
	}

	branch := worktreeBranchPrefix + slug
	dir := filepath.Join(worktreesRoot(m.repoDir), slug)

	args := []string{"worktree", "add", "-b", branch, dir}
	if baseRef != "" {
		args = append(args, baseRef)
	}
	if _, err := m.runGit(ctx, args...); err != nil {
		return "", fmt.Errorf("create worktree for %q: %w", slug, err)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir, nil
	}
	return abs, nil
}

// DefaultBranch resolves the repository's default branch so callers can pass a
// stable baseRef to Create regardless of the current checkout. It prefers the
// remote's advertised default (origin/HEAD), then a local "main" or "master",
// and finally falls back to the current branch.
func (m *Manager) DefaultBranch(ctx context.Context) (string, error) {
	if out, err := m.runGit(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if ref := strings.TrimSpace(string(out)); ref != "" {
			return strings.TrimPrefix(ref, "origin/"), nil
		}
	}
	for _, name := range []string{"main", "master"} {
		if _, err := m.runGit(ctx, "show-ref", "--verify", "--quiet", "refs/heads/"+name); err == nil {
			return name, nil
		}
	}
	out, err := m.runGit(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve default branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Remove deletes the worktree at worktreeDir. It uses --force so that untracked
// artifacts such as the state file do not block removal. The associated branch
// is intentionally preserved so it can serve as the task artifact (FR-006).
func (m *Manager) Remove(ctx context.Context, worktreeDir string) error {
	if _, err := m.runGit(ctx, "worktree", "remove", "--force", worktreeDir); err != nil {
		return fmt.Errorf("remove worktree %q: %w", worktreeDir, err)
	}
	return nil
}

// ListBranches returns all branch names matching "keep-run-*".
func (m *Manager) ListBranches(ctx context.Context) ([]string, error) {
	out, err := m.runGit(ctx, "branch", "--list", worktreeBranchPrefix+"*", "--format", "%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("list keep-run branches: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			branches = append(branches, name)
		}
	}
	return branches, nil
}

// HeadCommit returns the HEAD commit SHA of the worktree (or repository) at dir.
// The orchestrator captures it before the commit phase so the commit-staged gate
// can confirm a new commit landed.
func (m *Manager) HeadCommit(ctx context.Context, dir string) (string, error) {
	rctx := ctx
	if m.timeout > 0 {
		var cancel context.CancelFunc
		rctx, cancel = context.WithTimeout(ctx, m.timeout)
		defer cancel()
	}
	out, err := exec.CommandContext(rctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("head commit in %q: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// runGit executes a git command in the manager's repository using an argument
// array (never a shell string) to prevent injection, applying the configured
// timeout. Combined stdout+stderr is returned to aid error diagnosis.
func (m *Manager) runGit(ctx context.Context, args ...string) ([]byte, error) {
	if m.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.timeout)
		defer cancel()
	}

	full := append([]string{"-C", m.repoDir}, args...)
	out, err := exec.CommandContext(ctx, "git", full...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// IsWorktree detects whether dir is a git worktree. A worktree contains a .git
// file (not a directory) with content "gitdir: <path>". This is used to detect
// when /keep-run is being run from within a worktree, which can lead to nested
// worktree structures and path resolution issues.
func IsWorktree(dir string) bool {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return false
	}
	// In a worktree, .git is a file pointing to the main repo's .git directory.
	// In the main repo, .git is a directory.
	return !info.IsDir()
}

// ResolveMainRepo resolves the main repository path from a worktree directory.
// It reads the .git file which contains "gitdir: <path>" and resolves
// it to the main repository's root directory. This allows /keep-run to create
// worktrees from the main repo even when invoked from within a worktree.
func ResolveMainRepo(worktreeDir string) (string, error) {
	gitFile := filepath.Join(worktreeDir, ".git")
	content, err := os.ReadFile(gitFile)
	if err != nil {
		return "", fmt.Errorf("read .git file: %w", err)
	}

	// Content format: "gitdir: <path>\n"
	// The path can be absolute (e.g., "/path/to/repo/.git/worktrees/<name>")
	// or relative (e.g., "../.git/worktrees/<name>")
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gitdir:") {
			gitDir := strings.TrimPrefix(line, "gitdir:")
			gitDir = strings.TrimSpace(gitDir)

			var absGitDir string
			if filepath.IsAbs(gitDir) {
				// Path is already absolute
				absGitDir = gitDir
			} else {
				// Relative path: resolve from worktree directory
				absGitDir = filepath.Join(worktreeDir, gitDir)
			}

			// The worktrees directory's parent is the main repo's .git
			worktreesDir := filepath.Dir(absGitDir)
			if filepath.Base(worktreesDir) == "worktrees" {
				// /path/to/repo/.git/worktrees/<name> -> /path/to/repo/.git -> /path/to/repo
				mainGitDir := filepath.Dir(worktreesDir)
				mainRepoDir := filepath.Dir(mainGitDir)
				return mainRepoDir, nil
			}
		}
	}
	return "", fmt.Errorf("invalid .git file format")
}

// DetectRepoEnvironment detects whether the current directory is a worktree
// and returns the appropriate repository path. If the current directory is a
// worktree, it returns the main repository path; otherwise it returns the
// current directory. The boolean return indicates whether a worktree was detected.
func DetectRepoEnvironment(currentDir string) (mainRepo string, isWorktree bool, err error) {
	absDir, err := filepath.Abs(currentDir)
	if err != nil {
		return "", false, fmt.Errorf("resolve absolute path: %w", err)
	}

	if IsWorktree(absDir) {
		main, err := ResolveMainRepo(absDir)
		if err != nil {
			return "", true, fmt.Errorf("resolve main repo from worktree: %w", err)
		}
		return main, true, nil
	}
	return absDir, false, nil
}
