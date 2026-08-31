package tools

import (
	"context"
	"errors"
	"path/filepath"
	"time"
)

// ErrReadOnlyBashSandboxUnavailable indicates that the execution-layer
// containment boundary could not be established. Callers must fail closed.
var ErrReadOnlyBashSandboxUnavailable = errors.New("read-only sandbox unavailable")

type readOnlyBashRequest struct {
	WorkDir       string
	ReadableRoots []string
	Command       string
	Timeout       time.Duration
}

type readOnlyBashRunner interface {
	Run(context.Context, readOnlyBashRequest) BashCommandResult
}

type unavailableReadOnlyBashRunner struct{}

func (unavailableReadOnlyBashRunner) Run(context.Context, readOnlyBashRequest) BashCommandResult {
	return BashCommandResult{Err: ErrReadOnlyBashSandboxUnavailable}
}

func newReadOnlyBashToolWithRunner(workDir string, runner readOnlyBashRunner) *BashTool {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		absWorkDir = filepath.Clean(workDir)
	}
	return &BashTool{
		workDir:        filepath.Clean(absWorkDir),
		readOnly:       true,
		readOnlyRunner: runner,
	}
}

func readOnlyBashEnvironment() []string {
	return []string{
		"PATH=/usr/bin:/bin",
		"HOME=/nonexistent",
		"TMPDIR=/nonexistent",
		"LANG=C",
		"LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	}
}
