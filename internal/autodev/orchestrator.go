package autodev

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// PreconditionError marks a startup validation failure (not a git repo, gh
// missing or unauthenticated). Entry points map it to exit code 2.
type PreconditionError struct {
	Reason string
}

// Error implements the error interface.
func (e *PreconditionError) Error() string { return "autodev precondition failed: " + e.Reason }

// Deps wires the orchestrator's injectable dependencies. All external
// boundaries — the core Agent, the engineer LLM, git, gh, and the gate
// commands — arrive as interfaces so the orchestrator is deterministic
// under test (NFR-002).
type Deps struct {
	// Config is the resolved autodev configuration.
	Config AutodevConfig
	// RepoRoot is the main repository root.
	RepoRoot string
	// CoreFactory creates one CoreRunner per item, scoped to its worktree.
	CoreFactory CoreRunnerFactory
	// Engineer is the simulated engineer supervising every core run.
	Engineer EngineerAgent
	// Git serves worktree infrastructure and read-only verification.
	Git GitRunner
	// Exec serves the completion gate and read-only gh queries.
	Exec ExecRunner
	// Reporter observes the full interaction.
	Reporter Reporter
	// Clock stamps ledger mutations; nil defaults to SystemClock.
	Clock Clock
	// BuildPipeline overrides the SDD pipeline construction; nil defaults
	// to RequirementsFirstPipeline. Primarily a test seam.
	BuildPipeline func(PipelineDeps) []Stage
}

// Orchestrator is the deterministic control plane: it validates
// preconditions, seeds the ledger from the backlog, and drains pending
// items strictly serially — worktree → SDD stages → completion gate →
// remote publishing → ledger → cleanup (REQ-003, REQ-004, REQ-027).
type Orchestrator struct {
	deps      Deps
	machine   *StageMachine
	worktrees *WorktreeManager
	publisher *RemotePublisher
	pipeline  []Stage
}

// New wires an Orchestrator from deps.
func New(deps Deps) *Orchestrator {
	if deps.Clock == nil {
		deps.Clock = SystemClock{}
	}
	machine := NewStageMachine(deps.Engineer, deps.Reporter)
	gate := NewGateRunner(deps.Exec, deps.Reporter)
	pipelineDeps := PipelineDeps{
		Gate:     gate,
		Git:      deps.Git,
		Gates:    deps.Config.Gates,
		Reporter: deps.Reporter,
		Clock:    deps.Clock,
	}
	build := deps.BuildPipeline
	if build == nil {
		build = RequirementsFirstPipeline
	}
	return &Orchestrator{
		deps:      deps,
		machine:   machine,
		worktrees: NewWorktreeManager(deps.Git, deps.RepoRoot, deps.Config.WorktreeDir, deps.Config.BaseBranch, deps.Config.Remote),
		publisher: NewRemotePublisher(machine, deps.Git, deps.Exec, deps.Reporter, deps.Config),
		pipeline:  build(pipelineDeps),
	}
}

// Run drains the backlog: it validates preconditions up front (Edge
// Cases), seeds the ledger, then processes items one at a time until no
// in-progress or pending item remains.
func (o *Orchestrator) Run(ctx context.Context) error {
	if err := validateConcurrency(o.deps.Config.Concurrency); err != nil {
		return err
	}
	led, err := LoadLedger(filepath.Join(o.deps.RepoRoot, ".foxharness", "autodev-state.json"), o.deps.Clock)
	if err != nil {
		return err
	}
	if err := o.checkPreconditions(ctx); err != nil {
		return err
	}
	for _, w := range o.deps.Config.Warnings {
		o.deps.Reporter.OnInfo(ctx, w)
	}

	backlogPath := o.deps.Config.BacklogFile
	if !filepath.IsAbs(backlogPath) {
		backlogPath = filepath.Join(o.deps.RepoRoot, backlogPath)
	}
	items, err := Parse(backlogPath)
	if err != nil {
		return err
	}
	reconcileErr := led.Seed(items)
	if err := led.Save(); err != nil {
		return &LedgerCommitError{Operation: "seed", Err: err}
	}
	if reconcileErr != nil {
		return reconcileErr
	}

	total := len(led.InProgress()) + len(led.Pending())
	index := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, ok := o.nextItem(led)
		if !ok {
			break
		}
		index++
		if err := o.processItem(ctx, index, total, item, led); err != nil {
			return err
		}
	}
	o.deps.Reporter.OnInfo(ctx, "backlog drained")
	return nil
}

// nextItem resumes in-progress work before starting new pending items
// (REQ-022), both in priority order.
func (o *Orchestrator) nextItem(led *Ledger) (LedgerItem, bool) {
	if ip := led.InProgress(); len(ip) > 0 {
		return ip[0], true
	}
	if pd := led.Pending(); len(pd) > 0 {
		return pd[0], true
	}
	return LedgerItem{}, false
}

// processItem runs one item end to end: worktree, per-item core runner with
// the engineer asker installed, SDD stages from the recorded resume point,
// remote publishing, ledger completion, and worktree cleanup.
func (o *Orchestrator) processItem(ctx context.Context, index, total int, item LedgerItem, led *Ledger) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	o.deps.Reporter.OnItemStart(ctx, index, total, item)

	wt, err := o.worktrees.Create(ctx, item)
	if err != nil {
		return err
	}
	o.deps.Reporter.OnWorktree(ctx, wt)

	record := func(operation string, mut func(*LedgerItem)) error {
		if err := led.Commit(item.Slug, mut); err != nil {
			return &LedgerCommitError{Operation: operation, Err: err}
		}
		return nil
	}
	if err := record("item-in-progress", func(it *LedgerItem) {
		it.Status = StatusInProgress
		it.Branch = wt.Branch
	}); err != nil {
		return err
	}

	sc := &StageContext{
		Item:             Item{SourceID: item.SourceID, Type: "", Title: item.Title, Priority: item.Priority, Description: item.Description},
		ItemID:           item.ItemID,
		RequirementBytes: item.RequirementBytes,
		RequirementHash:  item.RequirementHash,
		Slug:             item.Slug,
		WorkDir:          wt.Path,
		RepoRoot:         o.deps.RepoRoot,
		Branch:           wt.Branch,
		BaseBranch:       o.deps.Config.BaseBranch,
		Remote:           o.deps.Config.Remote,
		FeatureDir:       item.FeatureDir,
	}
	bindCoreAttemptRecorder(sc, &item, record)
	if err := preflightFeatureWorkspace(sc); err != nil {
		return fmt.Errorf("validate feature workspace for %s: %w", item.Slug, err)
	}
	core := newItemCoreRunner(ctx, o.deps.CoreFactory, wt.Path, o.deps.Config.Model)
	core.SetUserAsker(NewEngineerAsker(o.deps.Engineer, o.deps.Reporter, sc))
	coreCloseAttempted := false
	closeCore := func() error {
		if coreCloseAttempted {
			return nil
		}
		coreCloseAttempted = true
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), coreLifecycleTimeout)
		defer cancel()
		if err := core.Close(closeCtx); err != nil {
			return &CoreLifecycleError{Operation: "close", Err: err}
		}
		return nil
	}
	defer func() {
		if err := closeCore(); err != nil {
			retErr = errors.Join(retErr, err)
		}
	}()
	start := o.resumeIndex(item)
	for i := start; i < len(o.pipeline); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		st := o.pipeline[i]
		stage, ok := pipelineStage(st.Name)
		if !ok {
			return fmt.Errorf("pipeline stage %q is not part of the durable stage vocabulary", st.Name)
		}
		resuming := i == start && item.Stage == stage && item.StageState == StageStateRunning
		if !resuming {
			if err := record("stage-"+st.Name+"-intent", func(it *LedgerItem) {
				it.Stage = stage
				it.StageState = StageStateRunning
			}); err != nil {
				return err
			}
		}
		var runErr error
		if resuming {
			runErr = o.machine.ResumeStep(ctx, core, sc, st)
		} else {
			runErr = o.machine.RunStep(ctx, core, sc, st)
		}
		verifiedTerminal := false
		if runErr != nil {
			var outcomeErr *CoreOutcomeError
			verifiedTerminal = errors.As(runErr, &outcomeErr) && outcomeErr.Verified
		}
		if runErr != nil && !verifiedTerminal {
			return runErr
		}
		if err := record("stage-"+st.Name+"-verified", func(it *LedgerItem) {
			it.Stage = stage
			it.StageState = StageStateVerified
			if sc.FeatureDir != "" {
				it.FeatureDir = sc.FeatureDir
			}
		}); err != nil {
			return err
		}
		if verifiedTerminal {
			return runErr
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := core.ensure(); err != nil {
		return err
	}
	current, _ := led.Get(item.Slug)
	if !isPublishingStage(current.Stage) {
		if err := record("publish-intent", func(it *LedgerItem) {
			it.Stage = StagePublish
			it.StageState = StageStateRunning
		}); err != nil {
			return err
		}
		current, _ = led.Get(item.Slug)
	}
	current.Description = item.Description
	result, err := o.publisher.Publish(ctx, core, wt, current, record)
	if err != nil {
		return err
	}
	if err := closeCore(); err != nil {
		return err
	}

	if err := record("item-done", func(it *LedgerItem) {
		it.Status = StatusDone
		it.Stage = StageDone
		it.StageState = StageStateVerified
		it.Issue = result.Issue
		it.PR = result.PR
	}); err != nil {
		return err
	}
	done, _ := led.Get(item.Slug)
	o.deps.Reporter.OnItemDone(ctx, done)

	if err := o.worktrees.Remove(ctx, wt); err != nil {
		// The PR is already pushed and recorded; a leftover worktree is an
		// inspection aid, not a failure (REQ-006).
		o.deps.Reporter.OnInfo(ctx, "WARNING: failed to remove worktree: "+err.Error())
	}
	return nil
}

// resumeIndex maps a running SDD stage to itself and a verified SDD stage
// to its successor. Post-pipeline stages skip the SDD pipeline entirely;
// invalid stages have already failed closed while loading the ledger.
func (o *Orchestrator) resumeIndex(item LedgerItem) int {
	if item.Stage == StageNone {
		return 0
	}
	for i, st := range o.pipeline {
		if PipelineStage(st.Name) == item.Stage {
			if item.StageState == StageStateVerified {
				return i + 1
			}
			return i
		}
	}
	return len(o.pipeline)
}

func isPublishingStage(stage PipelineStage) bool {
	switch stage {
	case StagePublish, StageStageChanges, StageCommitStaged, StagePush, StageIssue, StagePR:
		return true
	default:
		return false
	}
}

// checkPreconditions validates the genuine startup requirements: the repo
// root is a git work tree and gh is installed and authenticated. These are
// the only failure paths handled outside the engineer↔core loop (REQ-027).
func (o *Orchestrator) checkPreconditions(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result, runErr := o.deps.Git.Run(ctx, o.deps.RepoRoot, "rev-parse", "--is-inside-work-tree")
	if err := ctx.Err(); err != nil {
		return err
	}
	out, err := strictCommandStdout(result, runErr)
	if err != nil || strings.TrimSpace(out) != "true" {
		return &PreconditionError{Reason: fmt.Sprintf("%s is not a git repository (git rev-parse: %s; %v)", o.deps.RepoRoot, strings.TrimSpace(result.Output()), err)}
	}
	if o.deps.Config.RemoteFlow.CreateIssue || o.deps.Config.RemoteFlow.OpenPR {
		result, runErr := o.deps.Exec.Run(ctx, o.deps.RepoRoot, "gh", "auth", "status")
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := strictCommandStdout(result, runErr); err != nil {
			return &PreconditionError{Reason: fmt.Sprintf("gh is not installed or not authenticated (gh auth status: %s; %v)", strings.TrimSpace(result.Output()), err)}
		}
	}
	return nil
}
