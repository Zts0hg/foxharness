package hermetic

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

/* LocalGitRepository is a test-owned repository with isolated configuration. */
type LocalGitRepository struct {
	Dir     string
	homeDir string
}

/* NewLocalGitRepository initializes a repository strictly below root. */
func NewLocalGitRepository(ctx context.Context, root string) (*LocalGitRepository, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local Git root: %w", err)
	}
	repository := &LocalGitRepository{
		Dir:     filepath.Join(absRoot, "repository"),
		homeDir: filepath.Join(absRoot, "git-home"),
	}
	if err := os.MkdirAll(repository.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("create local Git repository: %w", err)
	}
	if err := os.MkdirAll(repository.homeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create local Git home: %w", err)
	}
	if _, err := repository.Git(ctx, "init", "--initial-branch=main"); err != nil {
		return nil, fmt.Errorf("initialize local Git repository: %w", err)
	}
	if _, err := repository.Git(ctx, "config", "user.name", "Hermetic Fixture"); err != nil {
		return nil, fmt.Errorf("configure local Git user name: %w", err)
	}
	if _, err := repository.Git(ctx, "config", "user.email", "fixture@example.invalid"); err != nil {
		return nil, fmt.Errorf("configure local Git user email: %w", err)
	}
	return repository, nil
}

/* Git runs the allowed local toolchain against the test-owned repository. */
func (r *LocalGitRepository) Git(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = r.Dir
	command.Env = []string{
		"HOME=" + r.homeDir,
		"TMPDIR=" + r.homeDir,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + filepath.Join(r.homeDir, "disabled-global-config"),
		"LC_ALL=C",
		"PATH=" + os.Getenv("PATH"),
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git %v: %w: %s", args, err, output)
	}
	return string(output), nil
}
