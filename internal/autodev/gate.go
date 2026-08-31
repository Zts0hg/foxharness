package autodev

import (
	"context"
	"strings"
)

// GateRunner executes the completion gate inside an item's worktree.
type GateRunner struct {
	exec     ExecRunner
	reporter Reporter
}

func NewGateRunner(exec ExecRunner, reporter Reporter) *GateRunner {
	return &GateRunner{exec: exec, reporter: reporter}
}

var _ GateChecker = (*GateRunner)(nil)

func (g *GateRunner) Check(ctx context.Context, workDir string, cfg GateConfig) (GateResult, error) {
	if !cfg.Test {
		cfg.Test = true
		g.warn(ctx, "WARNING: the test gate is mandatory and cannot be disabled; running go test anyway (REQ-018)")
	}

	type gateCommand struct {
		name    string
		enabled bool
		program string
		args    []string
		accept  func(CommandResult, error) bool
	}
	commands := []gateCommand{
		{name: "build", enabled: cfg.Build, program: "go", args: []string{"build", "./..."}, accept: func(_ CommandResult, err error) bool { return err == nil }},
		{name: "test", enabled: cfg.Test, program: "go", args: []string{"test", "./..."}, accept: func(_ CommandResult, err error) bool { return err == nil }},
		{name: "gofmt", enabled: cfg.Gofmt, program: "gofmt", args: []string{"-l", "."}, accept: func(r CommandResult, err error) bool {
			return err == nil && strings.TrimSpace(r.Stdout) == ""
		}},
	}

	result := GateResult{Passed: true}
	for _, command := range commands {
		if err := ctx.Err(); err != nil {
			result.Passed = false
			return result, err
		}
		if !command.enabled {
			g.warn(ctx, "WARNING: the "+command.name+" gate is disabled by configuration; skipping it weakens the completion gate")
			result.Steps = append(result.Steps, GateStep{Name: command.name, Skipped: true})
			continue
		}
		stepCtx, cancel := withDefaultTimeout(ctx, gateTimeout)
		commandResult, runErr := g.exec.Run(stepCtx, workDir, command.program, command.args...)
		cancel()
		output := commandResult.Output()
		truncated := commandResult.OverflowError() != nil
		if truncated {
			output = strings.TrimSpace(output) + "\n[output truncated: " + commandResult.OverflowError().Error() + "]"
		}
		result.Steps = append(result.Steps, GateStep{
			Name:            command.name,
			Passed:          command.accept(commandResult, runErr),
			Output:          output,
			OutputTruncated: truncated,
		})
		if ctx.Err() != nil {
			result.Passed = false
			return result, ctx.Err()
		}
	}
	for _, step := range result.Steps {
		if !step.Skipped && !step.Passed {
			result.Passed = false
		}
	}
	return result, nil
}

func (g *GateRunner) warn(ctx context.Context, msg string) {
	if g.reporter != nil {
		g.reporter.OnInfo(ctx, msg)
	}
}
