package slash

import (
	"context"
	"log"
	"os/exec"
	"time"
)

// DefaultHookTimeout caps a single hook's runtime when the caller passes
// timeout <= 0. Hooks are intended to be short, side-effecting commands;
// any genuine long-running work belongs in the command body, not a hook.
const DefaultHookTimeout = 30 * time.Second

// ExecuteHooks runs the `before` hook from h, if defined. Failures are
// logged but never returned — hooks must not block command execution.
func ExecuteHooks(ctx context.Context, h *FrontmatterHooks, workDir string, timeout time.Duration) error {
	if h == nil || h.Before == "" {
		return nil
	}
	runHook(ctx, "before", h.Before, workDir, timeout)
	return nil
}

// ExecuteAfterHook runs the `after` hook from h, if defined. Failures are
// logged but never returned.
func ExecuteAfterHook(ctx context.Context, h *FrontmatterHooks, workDir string, timeout time.Duration) error {
	if h == nil || h.After == "" {
		return nil
	}
	runHook(ctx, "after", h.After, workDir, timeout)
	return nil
}

func runHook(ctx context.Context, phase, command, workDir string, timeout time.Duration) {
	if timeout <= 0 {
		timeout = DefaultHookTimeout
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(hookCtx, "sh", "-c", command)
	if workDir != "" {
		cmd.Dir = workDir
	}
	if err := cmd.Run(); err != nil {
		log.Printf("[slash] %s hook failed (non-blocking): %v", phase, err)
	}
}
