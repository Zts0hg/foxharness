package autodev

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WorktreeManager owns the only repository writes the control plane
// performs: creating each item's isolated branch + worktree in the sibling
// worktree directory and removing the worktree after a successful PR
// (REQ-005, REQ-006).
type WorktreeManager struct {
	git         GitRunner
	repoRoot    string
	worktreeDir string
	baseBranch  string
}

// NewWorktreeManager creates a manager rooted at repoRoot. worktreeDir may
// be relative to repoRoot (the default is a sibling "../<repo>-worktrees").
func NewWorktreeManager(git GitRunner, repoRoot, worktreeDir, baseBranch string) *WorktreeManager {
	return &WorktreeManager{git: git, repoRoot: repoRoot, worktreeDir: worktreeDir, baseBranch: baseBranch}
}

// Create provisions the worktree for item. An in-progress item with an
// existing worktree directory is resumed as-is; an in-progress item whose
// directory is gone gets its recorded branch checked out into a fresh
// worktree; otherwise a new branch auto/<slug> is created from the base
// branch. A leftover directory from an unrelated prior run is left intact
// and a uniquely-suffixed path is used instead (Edge Cases).
func (m *WorktreeManager) Create(ctx context.Context, item LedgerItem) (Worktree, error) {
	root := m.resolvedWorktreeRoot()
	path := filepath.Join(root, item.Slug)

	if item.Status == StatusInProgress && item.Branch != "" {
		if dirExists(path) {
			return Worktree{Path: path, Branch: item.Branch, Slug: item.Slug, Resumed: true}, nil
		}
		if out, err := m.git.Run(ctx, m.repoRoot, "worktree", "add", path, item.Branch); err != nil {
			return Worktree{}, fmt.Errorf("reattach worktree for %s: %w (%s)", item.Slug, err, out)
		}
		return Worktree{Path: path, Branch: item.Branch, Slug: item.Slug, Resumed: true}, nil
	}

	for i := 2; dirExists(path); i++ {
		path = filepath.Join(root, fmt.Sprintf("%s-%d", item.Slug, i))
	}

	branch := "auto/" + item.Slug
	if out, err := m.git.Run(ctx, m.repoRoot, "worktree", "add", "-b", branch, path, m.baseBranch); err != nil {
		return Worktree{}, fmt.Errorf("create worktree for %s: %w (%s)", item.Slug, err, out)
	}
	return Worktree{Path: path, Branch: branch, Slug: item.Slug}, nil
}

// Remove deletes the local worktree; the branch (local and remote) is
// deliberately retained so the pushed PR stays reviewable (REQ-006).
func (m *WorktreeManager) Remove(ctx context.Context, wt Worktree) error {
	if out, err := m.git.Run(ctx, m.repoRoot, "worktree", "remove", "--force", wt.Path); err != nil {
		return fmt.Errorf("remove worktree %s: %w (%s)", wt.Path, err, out)
	}
	return nil
}

func (m *WorktreeManager) resolvedWorktreeRoot() string {
	if filepath.IsAbs(m.worktreeDir) {
		return m.worktreeDir
	}
	return filepath.Clean(filepath.Join(m.repoRoot, m.worktreeDir))
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
