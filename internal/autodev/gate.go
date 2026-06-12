package autodev

import (
	"context"
	"strings"
)

// GateRunner executes the completion gate inside an item's worktree:
// go build ./..., go test ./..., and gofmt -l . (REQ-018). The test gate is
// mandatory — a config that disables it is overridden with a warning —
// while build and gofmt may be skipped (each skip warns prominently).
type GateRunner struct {
	exec     ExecRunner
	reporter Reporter
}

// NewGateRunner creates a GateRunner over exec, reporting warnings and
// results through reporter (which may be nil).
func NewGateRunner(exec ExecRunner, reporter Reporter) *GateRunner {
	return &GateRunner{exec: exec, reporter: reporter}
}

var _ GateChecker = (*GateRunner)(nil)

// Check runs the configured gates in workDir and aggregates the outcome.
// The returned error is reserved for infrastructure failures; a failing
// gate is reported through GateResult, not an error.
func (g *GateRunner) Check(ctx context.Context, workDir string, cfg GateConfig) (GateResult, error) {
	if !cfg.Test {
		cfg.Test = true
		g.warn(ctx, "WARNING: the test gate is mandatory and cannot be disabled; running go test anyway (REQ-018)")
	}

	result := GateResult{Passed: true}
	result.Steps = append(result.Steps, g.step(ctx, workDir, "build", cfg.Build, func() (string, bool) {
		out, err := g.exec.Run(ctx, workDir, "go", "build", "./...")
		return out, err == nil
	}))
	result.Steps = append(result.Steps, g.step(ctx, workDir, "test", cfg.Test, func() (string, bool) {
		out, err := g.exec.Run(ctx, workDir, "go", "test", "./...")
		return out, err == nil
	}))
	result.Steps = append(result.Steps, g.step(ctx, workDir, "gofmt", cfg.Gofmt, func() (string, bool) {
		out, err := g.exec.Run(ctx, workDir, "gofmt", "-l", ".")
		return out, err == nil && strings.TrimSpace(out) == ""
	}))

	for _, s := range result.Steps {
		if !s.Skipped && !s.Passed {
			result.Passed = false
		}
	}
	return result, nil
}

func (g *GateRunner) step(ctx context.Context, workDir, name string, enabled bool, run func() (string, bool)) GateStep {
	if !enabled {
		g.warn(ctx, "WARNING: the "+name+" gate is disabled by configuration; skipping it weakens the completion gate")
		return GateStep{Name: name, Skipped: true}
	}
	out, passed := run()
	return GateStep{Name: name, Passed: passed, Output: out}
}

func (g *GateRunner) warn(ctx context.Context, msg string) {
	if g.reporter != nil {
		g.reporter.OnInfo(ctx, msg)
	}
}
