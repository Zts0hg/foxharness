package tui

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const gitBranchTimeout = 500 * time.Millisecond

func (m *Model) refreshRuntimeInfo() {
	m.sessionID = m.runner.SessionID()
	m.modelName = m.runner.Model()
	m.project = projectFolderName(m.runner.WorkDir())
	m.gitBranch = gitBranchForWorkDir(m.runner.WorkDir())
	m.contextUsage = normalizeContextUsage(m.runner.ContextUsage())
}

func projectFolderName(workDir string) string {
	if strings.TrimSpace(workDir) == "" {
		return "."
	}
	clean := filepath.Clean(workDir)
	base := filepath.Base(clean)
	if base == "." || base == string(filepath.Separator) {
		return clean
	}
	return base
}

func gitBranchForWorkDir(workDir string) string {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return "-"
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitBranchTimeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, "git", "-C", workDir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "-"
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "-"
	}
	return branch
}

func normalizeContextUsage(usage string) string {
	usage = strings.TrimSpace(usage)
	if usage == "" {
		return "unknown"
	}
	return usage
}
