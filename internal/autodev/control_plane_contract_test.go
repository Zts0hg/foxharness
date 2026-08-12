package autodev

import (
	"context"
	"errors"
	"io"
	"testing"
)

type cancelledPreconditionGit struct{ calls int }

func (g *cancelledPreconditionGit) Run(ctx context.Context, _ string, _ ...string) (CommandResult, error) {
	g.calls++
	return CommandResult{}, ctx.Err()
}

func TestCPAUT008StartupCancellationIsNotARepositoryPreconditionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	git := &cancelledPreconditionGit{}
	orchestrator := New(Deps{
		Config: AutodevConfig{
			Concurrency: "serial",
			RemoteFlow: RemoteFlowConfig{
				CreateIssue: false,
				OpenPR:      false,
			},
		},
		RepoRoot: t.TempDir(),
		Git:      git,
		Reporter: NewTerminalReporter(io.Discard),
	})

	err := orchestrator.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	var precondition *PreconditionError
	if errors.As(err, &precondition) {
		t.Fatalf("Run classified cancellation as PreconditionError: %v", err)
	}
	if git.calls > 1 {
		t.Fatalf("git calls = %d, want at most the interrupted repository probe", git.calls)
	}
}
