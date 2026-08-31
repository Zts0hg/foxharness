package tui

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/collaboration"
)

const gitBranchTimeout = 500 * time.Millisecond

func (m *Model) refreshRuntimeInfo() {
	state := m.runner.State()
	m.applyRuntimeState(state)
}

func (m *Model) applyRuntimeState(state app.InteractiveSessionState) {
	m.sessionID = state.Session.ID
	m.modelName = state.Model
	m.project = projectFolderName(state.WorkDir)
	m.gitBranch = gitBranchForWorkDir(state.WorkDir)
	m.contextUsage = normalizeContextUsage(state.ContextUsage)
	m.collaborationMode = collaboration.Normalize(collaboration.Mode(state.CollaborationMode))
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
