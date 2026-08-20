package autodev

import (
	"context"
	"fmt"
)

// itemCoreRunner owns the replaceable CoreRunner generation for exactly one
// Autodev item. The orchestrator owns this composition concern; adapters only
// adapt one concrete runtime generation.
type itemCoreRunner struct {
	ownerCtx context.Context
	factory  CoreRunnerFactory
	workDir  string
	model    string
	asker    QuestionAsker
	current  CoreRunner
}

func newItemCoreRunner(ctx context.Context, factory CoreRunnerFactory, workDir, model string) *itemCoreRunner {
	return &itemCoreRunner{ownerCtx: ctx, factory: factory, workDir: workDir, model: model}
}

func (r *itemCoreRunner) ensure() error {
	if r.current != nil {
		return nil
	}
	if r.factory == nil {
		return fmt.Errorf("core runner factory is unavailable")
	}
	created, err := r.factory.New(r.ownerCtx, r.workDir, r.model)
	if err != nil {
		return err
	}
	created.SetUserAsker(r.asker)
	r.current = created
	return nil
}

func (r *itemCoreRunner) Run(ctx context.Context, attempt CoreAttempt, reporter CoreReporter) CoreOutcome {
	if err := r.ensure(); err != nil {
		status, retryClass := ClassifyCoreError(ctx, err, false)
		return CoreOutcome{Attempt: attempt, Status: status, Cause: err, RetryClass: retryClass}
	}
	return r.current.Run(ctx, attempt, reporter)
}

func (r *itemCoreRunner) Drain(ctx context.Context) error {
	if r.current == nil {
		return nil
	}
	return r.current.Drain(ctx)
}

func (r *itemCoreRunner) Close(ctx context.Context) error {
	if r.current == nil {
		return nil
	}
	current := r.current
	r.current = nil
	return current.Close(ctx)
}

func (r *itemCoreRunner) Replace(ctx context.Context) error {
	if err := r.Close(ctx); err != nil {
		return err
	}
	return r.ensure()
}

func (r *itemCoreRunner) SetUserAsker(asker QuestionAsker) {
	r.asker = asker
	if r.current != nil {
		r.current.SetUserAsker(asker)
	}
}

func (r *itemCoreRunner) SetModel(model string) error {
	r.model = model
	if r.current == nil {
		return nil
	}
	return r.current.SetModel(model)
}

func (r *itemCoreRunner) WorkDir() string { return r.workDir }

func (r *itemCoreRunner) StagePrompt(ctx context.Context, command, args string) (string, error) {
	if err := r.ensure(); err != nil {
		return "", err
	}
	return r.current.StagePrompt(ctx, command, args)
}
