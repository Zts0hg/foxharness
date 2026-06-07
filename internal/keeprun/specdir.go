package keeprun

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// featureDirPattern matches the codexspec feature-directory naming convention: a
// timestamp prefix plus two random characters, e.g. "2026-0604-1430k7-add-dark".
// The /codexspec:generate-spec command owns this name, so keep-run discovers the
// directory rather than constructing it.
var featureDirPattern = regexp.MustCompile(`^\d{4}-\d{4}-\d{4}[a-z0-9]{2}-`)

// selectSpecDir picks the task's SDD feature directory from the worktree-relative
// paths of untracked files (the output of `git ls-files --others`). Among paths
// under .codexspec/specs/, it returns the newest directory whose name matches the
// feature-id convention; because the timestamp prefix sorts chronologically, the
// lexicographically greatest name is the most recent. Inherited specs committed
// on the base branch are tracked and never appear in the input, so they are
// excluded by construction. It returns "" when no feature directory is present.
func selectSpecDir(untracked []string) string {
	best := ""
	for _, raw := range untracked {
		parts := strings.Split(strings.TrimSpace(filepath.ToSlash(raw)), "/")
		for i := 0; i+2 < len(parts); i++ {
			if parts[i] == ".codexspec" && parts[i+1] == "specs" {
				if dir := parts[i+2]; featureDirPattern.MatchString(dir) && dir > best {
					best = dir
				}
				break
			}
		}
	}
	return best
}

// ResolveSpecDir discovers the absolute path of the SDD feature directory the
// codexspec commands created for the task in worktreeDir. Because those commands
// own the directory name (a timestamp+random prefix), keep-run cannot construct
// it and must detect it. Until the commit phase the artifacts are untracked, so
// the task's feature directory is the newest untracked .codexspec/specs/<dir>
// matching the feature-id convention; directories inherited (and committed) on
// the base branch are tracked and therefore excluded. It returns "" (and no
// error) when the directory does not exist yet, so callers can treat detection
// as best-effort and fall back to gate-and-retry.
func (m *Manager) ResolveSpecDir(ctx context.Context, worktreeDir string) (string, error) {
	rctx := ctx
	if m.timeout > 0 {
		var cancel context.CancelFunc
		rctx, cancel = context.WithTimeout(ctx, m.timeout)
		defer cancel()
	}

	out, err := exec.CommandContext(rctx, "git", "-C", worktreeDir,
		"ls-files", "--others", "--exclude-standard", "--", ".codexspec/specs").Output()
	if err != nil {
		return "", fmt.Errorf("resolve spec dir in %q: %w", worktreeDir, err)
	}

	dir := selectSpecDir(strings.Split(string(out), "\n"))
	if dir == "" {
		return "", nil
	}
	return filepath.Join(worktreeDir, ".codexspec", "specs", dir), nil
}
