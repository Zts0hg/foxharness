//go:build darwin

package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/processtree"
)

type darwinReadOnlyBashRunner struct {
	sandboxPath string
}

func newPlatformReadOnlyBashRunner() readOnlyBashRunner {
	const sandboxPath = "/usr/bin/sandbox-exec"
	if info, err := os.Stat(sandboxPath); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return unavailableReadOnlyBashRunner{}
	}
	return darwinReadOnlyBashRunner{sandboxPath: sandboxPath}
}

func (r darwinReadOnlyBashRunner) Run(ctx context.Context, request readOnlyBashRequest) BashCommandResult {
	if request.Timeout <= 0 {
		request.Timeout = defaultBashTimeout
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()

	cmd, err := newDarwinReadOnlyBashCommand(timeoutCtx, r.sandboxPath, request)
	if err != nil {
		return BashCommandResult{Err: fmt.Errorf("%w: %v", ErrReadOnlyBashSandboxUnavailable, err)}
	}
	output := newBoundedOutput(MaxBashOutputBytes)
	cmd.Stdout = output
	cmd.Stderr = output
	tree, err := processtree.Start(cmd)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || sandboxInfrastructureFailure(output.String()) {
			return BashCommandResult{Output: output.String(), Truncated: output.Truncated(), Err: fmt.Errorf("%w: %v: %s", ErrReadOnlyBashSandboxUnavailable, err, strings.TrimSpace(output.String()))}
		}
		return BashCommandResult{Output: output.String(), Truncated: output.Truncated(), Err: err}
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case err = <-wait:
	case <-timeoutCtx.Done():
		cleanupErr := tree.Signal(true)
		select {
		case <-wait:
			err = errors.Join(timeoutCtx.Err(), cleanupErr)
		case <-time.After(bashReapTimeout):
			err = errors.Join(timeoutCtx.Err(), cleanupErr, fmt.Errorf("read-only Bash process tree was not reaped within %s", bashReapTimeout))
		}
	}
	err = errors.Join(err, tree.Close(bashReapTimeout))
	result := BashCommandResult{Output: output.String(), Truncated: output.Truncated(), Err: err}
	if timeoutCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		return result
	}
	if err != nil && errors.Is(err, os.ErrNotExist) {
		result.Err = fmt.Errorf("%w: %v: %s", ErrReadOnlyBashSandboxUnavailable, err, strings.TrimSpace(result.Output))
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	return result
}

// sandboxInfrastructureFailure reports whether the captured output is the
// sandbox runner's own diagnostic. It is only sound before the sandboxed
// command has run: at that point no command output can be present, so a
// runner diagnostic is the only possible content. After the command has run
// the content is indistinguishable from command output that merely echoes
// the diagnostic prefix, and the classification is not attempted.
func sandboxInfrastructureFailure(output string) bool {
	firstLine := output
	if index := strings.IndexAny(output, "\r\n"); index >= 0 {
		firstLine = output[:index]
	}
	return strings.HasPrefix(strings.TrimSpace(firstLine), "sandbox-exec:")
}

func newDarwinReadOnlyBashCommand(ctx context.Context, sandboxPath string, request readOnlyBashRequest) (*exec.Cmd, error) {
	profile, err := darwinReadOnlySandboxProfile(request)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(sandboxPath) == "" {
		return nil, ErrReadOnlyBashSandboxUnavailable
	}
	cmd := exec.CommandContext(ctx, sandboxPath, "-p", profile, "/bin/bash", "--noprofile", "--norc", "-c", request.Command)
	cmd.Dir = request.WorkDir
	cmd.Env = readOnlyBashEnvironment()
	return cmd, nil
}

func darwinReadOnlySandboxProfile(request readOnlyBashRequest) (string, error) {
	roots := []string{"/System", "/usr", "/bin"}
	workDir, err := canonicalReadableRoot(request.WorkDir)
	if err != nil {
		return "", err
	}
	roots = append(roots, workDir)
	for _, root := range request.ReadableRoots {
		resolved, err := canonicalReadableRoot(root)
		if err != nil {
			return "", err
		}
		roots = append(roots, resolved)
	}
	var profile strings.Builder
	profile.WriteString("(version 1)\n")
	profile.WriteString("(deny default)\n")
	profile.WriteString("(deny network*)\n")
	profile.WriteString("(deny file-write*)\n")
	profile.WriteString("(allow process*)\n")
	profile.WriteString("(allow sysctl-read)\n")
	profile.WriteString("(allow mach-lookup (global-name \"com.apple.system.opendirectoryd.libinfo\"))\n")
	profile.WriteString("(allow file-read*\n")
	profile.WriteString("  (literal \"/dev/null\")\n")
	profile.WriteString("  (literal \"/dev/urandom\")\n")
	for _, ancestor := range readableRootAncestors(roots) {
		fmt.Fprintf(&profile, "  (literal \"%s\")\n", escapeSandboxProfileString(ancestor))
	}
	for _, root := range roots {
		fmt.Fprintf(&profile, "  (subpath \"%s\")\n", escapeSandboxProfileString(root))
	}
	profile.WriteString(")\n")
	return profile.String(), nil
}

func readableRootAncestors(roots []string) []string {
	seen := make(map[string]struct{})
	for _, root := range roots {
		for parent := filepath.Dir(root); ; parent = filepath.Dir(parent) {
			seen[parent] = struct{}{}
			if parent == string(filepath.Separator) {
				break
			}
		}
	}
	ancestors := make([]string, 0, len(seen))
	for ancestor := range seen {
		ancestors = append(ancestors, ancestor)
	}
	sort.Strings(ancestors)
	return ancestors
}

func canonicalReadableRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("readable root %q is not a directory", root)
	}
	return filepath.Clean(resolved), nil
}

func escapeSandboxProfileString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "\"", "\\\"")
}
