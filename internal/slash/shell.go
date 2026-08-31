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

const maxEmbeddedShellOutputBytes = 1 << 20

// shellEmbedRe matches the !`command` syntax. The non-greedy capture
// stops at the first backtick so multiple embeddings on one line are
// parsed independently.
var shellEmbedRe = regexp.MustCompile("!`([^`]+)`")

// EmbeddedShellCommands returns shell embeddings in execution order without
// running them.
func EmbeddedShellCommands(content string) []string {
	matches := shellEmbedRe.FindAllStringSubmatch(content, -1)
	commands := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			commands = append(commands, match[1])
		}
	}
	return commands
}

// ExecuteEmbeddedShell replaces every !`command` occurrence in content
// with the trimmed stdout produced by running the command via `sh -c`.
// Failures (non-zero exit, timeout, missing binary, cancellation of
// parent ctx) are reported inline as `[ERROR: ...]` markers so the
// command body remains a valid prompt.
//
// parent provides cancellation: if the caller's context is canceled
// (e.g. the user pressed Ctrl+C while the TUI prepare stage was
// running), in-flight embeddings are killed via the child process
// context derived from parent. timeout caps each individual embedded
// command; 0 selects DefaultShellTimeout. workDir, when non-empty,
// scopes the shell's current working directory.
func ExecuteEmbeddedShell(parent context.Context, content, workDir string, timeout time.Duration) (string, error) {
	if parent == nil {
		parent = context.Background()
	}
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
		stdout, runErr := runShellOnce(parent, sub[1], workDir, timeout)
		if runErr != nil {
			return formatShellError(sub[1], runErr)
		}
		return strings.TrimRight(stdout, "\n")
	})
	return out, nil
}

func runShellOnce(parent context.Context, command, workDir string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	configureEmbeddedShellCommand(cmd)
	if workDir != "" {
		cmd.Dir = workDir
	}
	output := &boundedShellOutput{}
	cmd.Stdout = output
	err := cmd.Run()
	switch {
	case errors.Is(parent.Err(), context.Canceled):
		return "", fmt.Errorf("canceled")
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", fmt.Errorf("timeout after %s", timeout)
	}
	if output.overflow {
		return "", fmt.Errorf("stdout exceeded %d bytes", maxEmbeddedShellOutputBytes)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("exit code %d", exitErr.ExitCode())
		}
		return "", err
	}
	return string(output.data), nil
}

type boundedShellOutput struct {
	data     []byte
	overflow bool
}

func (o *boundedShellOutput) Write(p []byte) (int, error) {
	remaining := maxEmbeddedShellOutputBytes - len(o.data)
	if remaining > 0 {
		keep := len(p)
		if keep > remaining {
			keep = remaining
		}
		o.data = append(o.data, p[:keep]...)
	}
	if len(p) > remaining {
		o.overflow = true
	}
	return len(p), nil
}

func formatShellError(command string, err error) string {
	return fmt.Sprintf("[ERROR: command failed: %s: %v]", strings.TrimSpace(command), err)
}
