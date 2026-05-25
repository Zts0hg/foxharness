package slash

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// DefaultShellTimeout is the default upper bound on shell embedding
// execution. Callers may override per-call via ExecuteEmbeddedShell.
const DefaultShellTimeout = 30 * time.Second

// shellEmbedRe matches the !`command` syntax. The non-greedy capture
// stops at the first backtick so multiple embeddings on one line are
// parsed independently.
var shellEmbedRe = regexp.MustCompile("!`([^`]+)`")

// ExecuteEmbeddedShell replaces every !`command` occurrence in content
// with the trimmed stdout produced by running the command via `sh -c`.
// Failures (non-zero exit, timeout, missing binary) are reported inline
// as `[ERROR: ...]` markers so the command body remains a valid prompt.
//
// timeout caps each individual embedded command; 0 selects
// DefaultShellTimeout. workDir, when non-empty, scopes the shell's
// current working directory so commands cannot escape the project.
func ExecuteEmbeddedShell(content, workDir string, timeout time.Duration) (string, error) {
	if !strings.Contains(content, "!`") {
		return content, nil
	}
	if timeout <= 0 {
		timeout = DefaultShellTimeout
	}
	out := shellEmbedRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := shellEmbedRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		stdout, runErr := runShellOnce(sub[1], workDir, timeout)
		if runErr != nil {
			return formatShellError(sub[1], runErr)
		}
		return strings.TrimRight(stdout, "\n")
	})
	return out, nil
}

func runShellOnce(command, workDir string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if workDir != "" {
		cmd.Dir = workDir
	}
	output, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("timeout after %s", timeout)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("exit code %d", exitErr.ExitCode())
		}
		return "", err
	}
	return string(output), nil
}

func formatShellError(command string, err error) string {
	return fmt.Sprintf("[ERROR: command failed: %s: %v]", strings.TrimSpace(command), err)
}
