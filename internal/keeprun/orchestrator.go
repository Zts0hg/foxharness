package keeprun

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// worktreeManager is the subset of *Manager the orchestrator depends on. It is
// an interface so orchestrator tests can inject a fake and avoid real git.
type worktreeManager interface {
	DefaultBranch(ctx context.Context) (string, error)
	ListBranches(ctx context.Context) ([]string, error)
	Create(ctx context.Context, slug, baseRef string) (string, error)
	Remove(ctx context.Context, worktreeDir string) error
	HeadCommit(ctx context.Context, dir string) (string, error)
	ResolveSpecDir(ctx context.Context, worktreeDir string) (string, error)
}

// phaseVerifier is the subset of *Verifier the orchestrator depends on.
type phaseVerifier interface {
	VerifyPhase(ctx context.Context, phase Phase, tc TaskContext, out PhaseOutcome) error
}

// ProgressKind classifies a ProgressEvent.
type ProgressKind int

const (
	// EventTaskStart marks the start (or resume) of a backlog task.
	EventTaskStart ProgressKind = iota
	// EventPhaseStart marks the start of an SDD phase.
	EventPhaseStart
	// EventPhaseComplete marks a phase recorded complete after its gate passed.
	EventPhaseComplete
	// EventPhaseRetry marks a failed phase attempt that will be retried.
	EventPhaseRetry
	// EventReviewFix marks a fix run triggered because a review was not clean.
	EventReviewFix
	// EventTaskComplete marks a task marked done and its worktree cleaned up.
	EventTaskComplete
	// EventExit marks the orchestrator exiting because no pending task remains.
	EventExit
)

// ProgressEvent is a structured progress update the orchestrator emits (FR-012).
type ProgressEvent struct {
	Kind    ProgressKind
	Task    string
	Slug    string
	Phase   int
	Total   int
	Command string
	Attempt int
	Message string
}

// ProgressSink receives progress events for display by the TUI.
type ProgressSink interface {
	Event(ev ProgressEvent)
}

type nopSink struct{}

func (nopSink) Event(ProgressEvent) {}

// BackoffPolicy controls the wait between phase retries. The wait grows
// exponentially from Base and is capped at Max, but there is no cap on the
// number of retries (spec FR-007): a persistently failing phase keeps retrying
// at the Max interval rather than being abandoned.
type BackoffPolicy struct {
	Base time.Duration
	Max  time.Duration
}

func (b BackoffPolicy) wait(attempt int) time.Duration {
	base, max := b.Base, b.Max
	if base <= 0 {
		base = time.Second
	}
	if max <= 0 {
		max = 5 * time.Minute
	}
	d := base
	for i := 0; i < attempt; i++ {
		d *= 2
		if d <= 0 || d >= max {
			return max
		}
	}
	if d > max {
		return max
	}
	return d
}

// sleepFunc waits for d unless ctx is canceled first.
type sleepFunc func(ctx context.Context, d time.Duration) error

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Orchestrator drives the keep-run pipeline deterministically: it selects the
// next pending task, manages the task's worktree, sequences the SDD phases
// (reaching the LLM only through the injected PhaseRunner), gates each phase with
// the verifier, persists resume state, updates BACKLOG.md, and exits when no
// pending task remains (spec FR-013, NFR-006).
type Orchestrator struct {
	repoDir string
	runner  PhaseRunner
	wt      worktreeManager
	verify  phaseVerifier
	sink    ProgressSink
	backoff BackoffPolicy
	sleep   sleepFunc
	now     func() time.Time
}

// Option configures an Orchestrator.
type Option func(*Orchestrator)

// WithWorktrees overrides the worktree manager (used in tests).
func WithWorktrees(w worktreeManager) Option { return func(o *Orchestrator) { o.wt = w } }

// WithVerifier overrides the phase verifier (used in tests).
func WithVerifier(v phaseVerifier) Option { return func(o *Orchestrator) { o.verify = v } }

// WithProgressSink sets the progress sink.
func WithProgressSink(s ProgressSink) Option { return func(o *Orchestrator) { o.sink = s } }

// WithBackoff overrides the retry backoff policy.
func WithBackoff(b BackoffPolicy) Option { return func(o *Orchestrator) { o.backoff = b } }

// WithSleeper overrides the sleep function (used in tests for a fake clock).
func WithSleeper(fn func(ctx context.Context, d time.Duration) error) Option {
	return func(o *Orchestrator) { o.sleep = fn }
}

// NewOrchestrator creates an orchestrator rooted at repoDir. runner is the only
// path to the LLM; the remaining collaborators default to real implementations
// and can be overridden for testing.
func NewOrchestrator(repoDir string, runner PhaseRunner, opts ...Option) *Orchestrator {
	o := &Orchestrator{
		repoDir: repoDir,
		runner:  runner,
		wt:      NewManager(repoDir),
		verify:  NewVerifier(),
		sink:    nopSink{},
		backoff: BackoffPolicy{Base: time.Second, Max: 5 * time.Minute},
		sleep:   sleepCtx,
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func backlogPath(repoDir string) string { return filepath.Join(repoDir, "BACKLOG.md") }

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Run processes pending tasks until none remain or ctx is canceled. It re-reads
// BACKLOG.md after each task so newly added or changed tasks are picked up.
func (o *Orchestrator) Run(ctx context.Context) error {
	cfg, _ := LoadConfig(o.repoDir)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		content, err := os.ReadFile(backlogPath(o.repoDir))
		if err != nil {
			return fmt.Errorf("read BACKLOG.md: %w", err)
		}
		tasks, err := ParseBacklog(string(content))
		if err != nil {
			return fmt.Errorf("parse BACKLOG.md: %w", err)
		}
		task, ok := firstPending(tasks)
		if !ok {
			msg := "All tasks completed"
			if len(tasks) == 0 {
				msg = "No tasks found"
			}
			o.sink.Event(ProgressEvent{Kind: EventExit, Message: msg})
			return nil
		}
		if err := o.processTask(ctx, task, cfg); err != nil {
			return err
		}
	}
}

func firstPending(tasks []Task) (Task, bool) {
	for _, t := range tasks {
		if strings.EqualFold(strings.TrimSpace(t.Status), "pending") {
			return t, true
		}
	}
	return Task{}, false
}

// processTask runs one backlog task through the pipeline: resume-or-create its
// worktree, execute the remaining phases with gates, mark it done, and clean up.
func (o *Orchestrator) processTask(ctx context.Context, task Task, cfg Config) error {
	slug := GenerateSlug(task.Title)
	worktreeDir := filepath.Join(o.repoDir, ".claude", "worktrees", slug)

	var state State
	nextPhase := 1
	if fileExists(filepath.Join(worktreeDir, stateFileName)) {
		s, err := ReadState(worktreeDir)
		if err != nil {
			return fmt.Errorf("read state for %q: %w", slug, err)
		}
		state = s
		nextPhase = state.NextPhase()
		if state.TaskSlug != "" {
			slug = state.TaskSlug
		}
		if state.WorktreePath != "" {
			worktreeDir = state.WorktreePath
		}
	} else {
		branches, err := o.wt.ListBranches(ctx)
		if err != nil {
			return fmt.Errorf("list branches: %w", err)
		}
		// Strip the "keep-run-" prefix from each branch name so that
		// DeduplicateSlug compares slugs in the same namespace.
		taken := make([]string, 0, len(branches))
		for _, b := range branches {
			taken = append(taken, strings.TrimPrefix(b, worktreeBranchPrefix))
		}
		slug = DeduplicateSlug(slug, taken)
		baseRef, err := o.wt.DefaultBranch(ctx)
		if err != nil {
			return fmt.Errorf("resolve default branch: %w", err)
		}
		worktreeDir, err = o.wt.Create(ctx, slug, baseRef)
		if err != nil {
			return fmt.Errorf("create worktree for %q: %w", slug, err)
		}
		state = State{
			TaskSlug:      slug,
			WorktreePath:  worktreeDir,
			RemoteEnabled: cfg.RemoteEnabled,
			LastPhaseAt:   o.now().UTC().Format(time.RFC3339),
		}
		if err := WriteState(worktreeDir, state); err != nil {
			return err
		}
	}

	// The codexspec commands own the feature-directory name (a timestamp+random
	// prefix), so keep-run detects it rather than constructing it. On a fresh task
	// it does not exist yet (SpecDir stays empty until generate-spec creates it);
	// on resume the resolve below recovers it from the worktree.
	tc := TaskContext{Slug: slug, WorktreeDir: worktreeDir, Config: cfg}
	o.resolveSpecDir(ctx, &tc)

	o.sink.Event(ProgressEvent{Kind: EventTaskStart, Task: task.Title, Slug: slug, Phase: nextPhase, Total: len(PipelinePhases())})

	phases := PipelinePhases()
	for i := nextPhase - 1; i < len(phases); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		phase := phases[i]
		phaseNum := i + 1
		if phase.Remote && !cfg.RemoteEnabled {
			break
		}
		if phase.Command == "codexspec:commit-staged" {
			if head, err := o.wt.HeadCommit(ctx, worktreeDir); err == nil {
				tc.HeadCommitBefore = head
			}
		}
		o.sink.Event(ProgressEvent{Kind: EventPhaseStart, Task: task.Title, Slug: slug, Phase: phaseNum, Total: len(phases), Command: phase.Command})

		if err := o.runPhase(ctx, phase, &tc); err != nil {
			return err
		}

		state.CompletedPhases = append(state.CompletedPhases, phaseNum)
		state.LastPhaseAt = o.now().UTC().Format(time.RFC3339)
		if err := WriteState(worktreeDir, state); err != nil {
			return err
		}
		o.sink.Event(ProgressEvent{Kind: EventPhaseComplete, Slug: slug, Phase: phaseNum, Total: len(phases), Command: phase.Command})
	}

	content, err := os.ReadFile(backlogPath(o.repoDir))
	if err != nil {
		return fmt.Errorf("re-read BACKLOG.md: %w", err)
	}
	updated := UpdateStatus(string(content), task.HeadingLine, "done")
	if err := os.WriteFile(backlogPath(o.repoDir), []byte(updated), 0o644); err != nil {
		return fmt.Errorf("update BACKLOG.md: %w", err)
	}

	if err := o.wt.Remove(ctx, worktreeDir); err != nil {
		o.sink.Event(ProgressEvent{Kind: EventTaskComplete, Task: task.Title, Slug: slug, Message: "worktree cleanup failed: " + err.Error()})
		return nil
	}
	o.sink.Event(ProgressEvent{Kind: EventTaskComplete, Task: task.Title, Slug: slug})
	return nil
}

// runPhase executes one phase to a verified-complete state. Review phases iterate
// run -> fix -> re-run until clean; other phases retry until their artifact gate
// passes. Both respect ctx cancellation and the no-cap backoff (FR-007).
func (o *Orchestrator) runPhase(ctx context.Context, phase Phase, tc *TaskContext) error {
	if phase.Review {
		return o.runReviewPhase(ctx, phase, tc)
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		out, err := o.runner.RunPhase(ctx, o.requestFor(phase, *tc, ""))
		if err == nil {
			o.resolveSpecDir(ctx, tc)
			if verr := o.verify.VerifyPhase(ctx, phase, *tc, out); verr == nil {
				return nil
			} else {
				err = verr
			}
		}
		o.sink.Event(ProgressEvent{Kind: EventPhaseRetry, Slug: tc.Slug, Command: phase.Command, Attempt: attempt, Message: err.Error()})
		if serr := o.sleep(ctx, o.backoff.wait(attempt)); serr != nil {
			return serr
		}
	}
}

// resolveSpecDir updates tc.SpecDir to the codexspec feature directory detected
// in the worktree when one exists. Detection is best-effort: a lookup error or a
// not-yet-created directory leaves the previous value intact, so a transient
// failure degrades to the normal gate-and-retry path rather than aborting the
// task. Once set, the resolved directory flows to later phases (as their prompt's
// read/write target) and to the verifier's artifact gates.
func (o *Orchestrator) resolveSpecDir(ctx context.Context, tc *TaskContext) {
	if dir, err := o.wt.ResolveSpecDir(ctx, tc.WorktreeDir); err == nil && dir != "" {
		tc.SpecDir = dir
	}
}

// runReviewPhase runs the review-and-fix loop. It runs the review (dispatched by
// the adapter per review_mode), checks the verdict gate, and on "not clean" runs
// a fix (a plain engine run guided by review_fix_prompt) before re-reviewing.
func (o *Orchestrator) runReviewPhase(ctx context.Context, phase Phase, tc *TaskContext) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		out, err := o.runWithRetry(ctx, o.requestFor(phase, *tc, ReviewVerdictInstruction))
		if err != nil {
			return err
		}
		o.resolveSpecDir(ctx, tc)
		if o.verify.VerifyPhase(ctx, phase, *tc, out) == nil {
			return nil
		}
		o.sink.Event(ProgressEvent{Kind: EventReviewFix, Slug: tc.Slug, Command: phase.Command})
		if _, err := o.runWithRetry(ctx, o.fixRequestFor(phase, *tc)); err != nil {
			return err
		}
	}
}

// runWithRetry retries RunPhase on transient error (no gate check) until it
// returns without error or ctx is canceled. There is no retry cap (FR-007).
func (o *Orchestrator) runWithRetry(ctx context.Context, req PhaseRequest) (PhaseOutcome, error) {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return PhaseOutcome{}, err
		}
		out, err := o.runner.RunPhase(ctx, req)
		if err == nil {
			return out, nil
		}
		o.sink.Event(ProgressEvent{Kind: EventPhaseRetry, Slug: req.Phase.Command, Command: req.Phase.Command, Attempt: attempt, Message: err.Error()})
		if serr := o.sleep(ctx, o.backoff.wait(attempt)); serr != nil {
			return PhaseOutcome{}, serr
		}
	}
}

func (o *Orchestrator) requestFor(phase Phase, tc TaskContext, instruction string) PhaseRequest {
	return PhaseRequest{
		Phase:       phase,
		WorktreeDir: tc.WorktreeDir,
		SpecDir:     tc.SpecDir,
		Config:      tc.Config,
		Instruction: instruction,
	}
}

// fixRequestFor builds the request for a review fix: the same command run as a
// plain (non-review) engine run guided by review_fix_prompt, so the adapter does
// not re-dispatch it through the review (subagent) path.
func (o *Orchestrator) fixRequestFor(phase Phase, tc TaskContext) PhaseRequest {
	plain := phase
	plain.Review = false
	return PhaseRequest{
		Phase:       plain,
		WorktreeDir: tc.WorktreeDir,
		SpecDir:     tc.SpecDir,
		Config:      tc.Config,
		Instruction: tc.Config.ReviewFixPrompt,
	}
}
