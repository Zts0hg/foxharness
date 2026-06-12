package autodev

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	remote      string
}

// NewWorktreeManager creates a manager rooted at repoRoot. worktreeDir may
// be relative to repoRoot (the default is a sibling "../<repo>-worktrees");
// remote names the git remote consulted when classifying leftover worktrees.
func NewWorktreeManager(git GitRunner, repoRoot, worktreeDir, baseBranch, remote string) *WorktreeManager {
	return &WorktreeManager{git: git, repoRoot: repoRoot, worktreeDir: worktreeDir, baseBranch: baseBranch, remote: remote}
}

// maxWorktreeCandidates bounds the path/branch suffix search in Create so a
// misbehaving GitRunner can never spin it forever.
const maxWorktreeCandidates = 100

// Create provisions the worktree for item. An in-progress item resumes at
// the worktree its recorded branch is actually checked out in (the registry
// is the source of truth — the path may carry a collision suffix), falling
// back to reattaching the branch at its lockstep-derived path. New items
// walk path/branch candidate pairs in lockstep (<slug> + auto/<slug>, then
// <slug>-2 + auto/<slug>-2, ...): a leftover directory registered as this
// repository's worktree on the candidate branch and verifiably holding
// unpublished work — a run that crashed before the ledger recorded
// in-progress — is reused as a resume, while an unrelated directory, a
// published or undecidable leftover, or a stale branch (Remove deliberately
// retains branches after success, so a reset ledger replays into existing
// auto/* branches) advances to the next pair (Edge Cases).
func (m *WorktreeManager) Create(ctx context.Context, item LedgerItem) (Worktree, error) {
	root := m.resolvedWorktreeRoot()

	if item.Status == StatusInProgress && item.Branch != "" {
		return m.resume(ctx, root, item)
	}

	for i := 1; i <= maxWorktreeCandidates; i++ {
		name := item.Slug
		if i > 1 {
			name = fmt.Sprintf("%s-%d", item.Slug, i)
		}
		path := filepath.Join(root, name)
		branch := "auto/" + name

		if dirExists(path) {
			// Reuse only a half-provisioned worktree from a crashed run: it
			// must be registered on the candidate branch and verifiably hold
			// unpublished work. Anything else — including an undecidable
			// remote state — advances to the next pair.
			if m.registeredWorktreeOnBranch(ctx, path, branch) && m.reusableLeftover(ctx, path, branch) {
				return Worktree{Path: path, Branch: branch, Slug: item.Slug, Resumed: true}, nil
			}
			continue
		}

		out, err := m.git.Run(ctx, m.repoRoot, "worktree", "add", "-b", branch, path, m.baseBranch)
		if err == nil {
			return Worktree{Path: path, Branch: branch, Slug: item.Slug}, nil
		}
		// `worktree add -b` refuses an existing branch outright, so when the
		// branch turns out to exist the failure is the expected collision
		// with a prior run's retained branch — move to the next pair. Any
		// other failure (missing base branch, disk, registration conflicts)
		// is a genuine error.
		if m.branchExists(ctx, branch) {
			continue
		}
		return Worktree{}, fmt.Errorf("create worktree for %s: %w (%s)", item.Slug, err, out)
	}
	return Worktree{}, fmt.Errorf("create worktree for %s: no free path/branch pair within %d candidates under %s", item.Slug, maxWorktreeCandidates, root)
}

// resume returns the worktree for an item whose branch the ledger already
// recorded. The registry decides the path — Create may have chosen a
// collision-suffixed pair, so reconstructing it from the slug would miss the
// real worktree and git would refuse a second checkout of the branch. When
// the registered directory is gone (removed without `git worktree remove`)
// the stale record is pruned first; when the branch is checked out nowhere,
// it is reattached at its lockstep-derived path.
func (m *WorktreeManager) resume(ctx context.Context, root string, item LedgerItem) (Worktree, error) {
	path, registered := m.branchCheckoutPath(ctx, item.Branch)
	if registered {
		if dirExists(path) {
			return Worktree{Path: path, Branch: item.Branch, Slug: item.Slug, Resumed: true}, nil
		}
		// Best-effort: a failed prune simply leaves the reattach below to
		// report the underlying problem.
		_, _ = m.git.Run(ctx, m.repoRoot, "worktree", "prune")
	}

	path = filepath.Join(root, dirNameForBranch(item.Branch, item.Slug))
	if out, err := m.git.Run(ctx, m.repoRoot, "worktree", "add", path, item.Branch); err != nil {
		return Worktree{}, fmt.Errorf("reattach worktree for %s: %w (%s)", item.Slug, err, out)
	}
	return Worktree{Path: path, Branch: item.Branch, Slug: item.Slug, Resumed: true}, nil
}

// dirNameForBranch inverts the lockstep naming Create uses (auto/<name> ↔
// <name>). A ledger branch outside that scheme falls back to the slug.
func dirNameForBranch(branch, slug string) string {
	if strings.HasPrefix(branch, "auto/") {
		return strings.TrimPrefix(branch, "auto/")
	}
	return slug
}

// worktreeEntry is one record parsed from `git worktree list --porcelain`.
type worktreeEntry struct {
	Path   string
	Branch string
}

// linkedWorktrees returns the linked worktrees registered to the main
// repository with their checked-out branches (short names). Two entries are
// dropped outright because no autodev operation may ever select them
// (worktree isolation, REQ-005): the primary checkout — always listed first
// by git — and the invoking checkout (repoRoot may itself be a linked
// worktree autodev was started from, in which case it is not the first
// entry). It returns nil when the query fails; callers treat that
// conservatively (no reuse, no resume-by-registry).
func (m *WorktreeManager) linkedWorktrees(ctx context.Context) []worktreeEntry {
	out, err := m.git.Run(ctx, m.repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil
	}
	var entries []worktreeEntry
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			entries = append(entries, worktreeEntry{Path: strings.TrimPrefix(line, "worktree ")})
		case strings.HasPrefix(line, "branch refs/heads/") && len(entries) > 0:
			entries[len(entries)-1].Branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	var linked []worktreeEntry
	for i, e := range entries {
		if i == 0 || samePath(e.Path, m.repoRoot) {
			continue
		}
		linked = append(linked, e)
	}
	return linked
}

// branchCheckoutPath reports where branch is checked out among this
// repository's linked worktrees. Only checkouts inside the managed worktree
// root qualify: a worktree the user added elsewhere may legitimately have
// the branch checked out, and the core agent must never be pointed at it
// (worktree isolation, REQ-005).
func (m *WorktreeManager) branchCheckoutPath(ctx context.Context, branch string) (string, bool) {
	root := m.resolvedWorktreeRoot()
	for _, e := range m.linkedWorktrees(ctx) {
		if e.Branch == branch && pathWithinRoot(root, e.Path) {
			return e.Path, true
		}
	}
	return "", false
}

// reusableLeftover reports whether the leftover worktree at path holds
// unpublished work and is therefore safe to resume the pipeline in.
// Publication evidence outranks the dirty state: a HEAD matching the remote
// branch tip marks a completed run whose Remove failed — work is committed
// before it is pushed, so everything committed there is published and any
// dirt is post-publication debris (resuming would push new commits onto the
// PR branch). Otherwise a missing or different tip means the leftover holds
// local-only commits or none at all — the crashed-run signature. When the
// remote cannot be queried, only a dirty tree proves unpublished work;
// skipping a clean one costs one suffixed duplicate, and an unreachable
// remote dooms the pipeline's own push anyway.
func (m *WorktreeManager) reusableLeftover(ctx context.Context, path, branch string) bool {
	status, err := m.git.Run(ctx, path, "status", "--porcelain")
	if err != nil {
		return false
	}
	dirty := strings.TrimSpace(status) != ""
	head, err := m.git.Run(ctx, path, "rev-parse", "HEAD")
	if err != nil {
		return false
	}
	remote, err := m.git.Run(ctx, m.repoRoot, "ls-remote", "--heads", m.remote, branch)
	if err != nil {
		return dirty
	}
	tip := strings.Fields(remote)
	published := len(tip) > 0 && tip[0] == strings.TrimSpace(head)
	return !published
}

// registeredWorktreeOnBranch reports whether path is registered as a linked
// worktree of the main repository with branch checked out. Asking the
// registry — rather than the directory itself — guarantees an unrelated
// repository that happens to use the same branch name is never reused. The
// registered location must itself lie inside the managed root, sharing
// branchCheckoutPath's containment invariant: a symlink at the candidate
// path would otherwise smuggle in an external checkout that samePath happily
// resolves to.
func (m *WorktreeManager) registeredWorktreeOnBranch(ctx context.Context, path, branch string) bool {
	root := m.resolvedWorktreeRoot()
	for _, e := range m.linkedWorktrees(ctx) {
		if e.Branch == branch && samePath(e.Path, path) && pathWithinRoot(root, e.Path) {
			return true
		}
	}
	return false
}

// samePath compares two paths with symlinks resolved (git prints canonical
// paths, e.g. /private/var vs /var on macOS), falling back to a lexical
// comparison when either path cannot be resolved.
func samePath(a, b string) bool {
	return canonicalPath(a) == canonicalPath(b)
}

// pathWithinRoot reports whether path is strictly inside root, with both
// sides canonicalized first.
func pathWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(canonicalPath(root), canonicalPath(path))
	if err != nil || rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// canonicalPath resolves symlinks when possible and cleans the path
// otherwise (e.g. for paths that do not exist yet).
func canonicalPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// branchExists reports whether the local branch exists in the main
// repository. rev-parse --verify exits non-zero for a missing ref, which the
// GitRunner surfaces as an error.
func (m *WorktreeManager) branchExists(ctx context.Context, branch string) bool {
	_, err := m.git.Run(ctx, m.repoRoot, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
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
