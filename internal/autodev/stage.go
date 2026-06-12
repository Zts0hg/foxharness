package autodev

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GateChecker runs the completion gate inside a worktree. gate.go provides
// the production implementation; the implement stage's Verify depends on
// this seam so stage tests need no real go/gofmt processes.
type GateChecker interface {
	Check(ctx context.Context, workDir string, cfg GateConfig) (GateResult, error)
}

// Stage is one deterministic step of the pipeline: a seed prompt for the
// core Agent plus the Go-evaluated ground-truth Verify that gates
// advancement (REQ-007, REQ-029). The same shape serves SDD stages and
// remote publishing steps so a single RunStep loop drives both.
type Stage struct {
	// Name identifies the step in events, the ledger, and gaps.
	Name string
	// Command is the codexspec command materialized via CoreRunner.StagePrompt
	// (e.g. "codexspec:generate-spec"). Empty means Prompt seeds the step.
	Command string
	// Args builds the argument string passed to Command. May be nil.
	Args func(sc *StageContext) string
	// Append builds extra instructions appended to the materialized
	// Command body — used for inputs the command body does not consume as
	// arguments (e.g. the requirement Description for generate-spec) and
	// for hard step requirements. May be nil.
	Append func(sc *StageContext) string
	// Prompt builds a literal seed prompt for command-less steps.
	Prompt func(sc *StageContext) string
	// Prepare runs once before the step's first core run, e.g. to snapshot
	// pre-existing spec directories. May be nil.
	Prepare func(ctx context.Context, sc *StageContext) error
	// Skip reports that the step's outcome already exists so the step can
	// be skipped entirely (resume idempotency). May be nil.
	Skip func(ctx context.Context, sc *StageContext) bool
	// Verify is the read-only ground-truth predicate deciding advancement,
	// returning ok or a gap describing precisely what is still missing.
	Verify func(ctx context.Context, sc *StageContext) (ok bool, gap string)
}

// StageMachine drives one step at a time through the supervised loop:
// seed → core Run → Go Verify → engineer Review correction → retry, until
// the ground truth says the step completed (REQ-030). The LLM can never
// terminate the loop early; only Verify can.
type StageMachine struct {
	engineer EngineerAgent
	reporter Reporter
}

// NewStageMachine creates a StageMachine supervised by engineer and
// observed through reporter.
func NewStageMachine(engineer EngineerAgent, reporter Reporter) *StageMachine {
	return &StageMachine{engineer: engineer, reporter: reporter}
}

// RunStep executes st to ground-truth completion. The loop is unbounded by
// design (REQ-027: no abandonment budget) and exits only on Verify success,
// a hard runner error, or context cancellation.
func (m *StageMachine) RunStep(ctx context.Context, core CoreRunner, sc *StageContext, st Stage) error {
	sc.Stage = st.Name
	if m.reporter != nil {
		m.reporter.OnStageStart(ctx, sc.Slug, st.Name)
	}

	if st.Skip != nil && st.Skip(ctx, sc) {
		if m.reporter != nil {
			m.reporter.OnInfo(ctx, fmt.Sprintf("step %s already satisfied; skipping", st.Name))
		}
		return nil
	}
	if st.Prepare != nil {
		if err := st.Prepare(ctx, sc); err != nil {
			return fmt.Errorf("prepare step %s: %w", st.Name, err)
		}
	}

	msg, err := m.seedPrompt(ctx, core, sc, st)
	if err != nil {
		return err
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		res, err := core.Run(ctx, msg, m.reporter)
		if err != nil {
			return fmt.Errorf("core run for step %s: %w", st.Name, err)
		}

		ok, gap := st.Verify(ctx, sc)
		if m.reporter != nil {
			m.reporter.OnVerify(ctx, st.Name, ok, gap)
		}
		if ok {
			return nil
		}

		correction, err := m.engineer.Review(ctx, res, gap, *sc)
		if err != nil {
			return fmt.Errorf("engineer review for step %s: %w", st.Name, err)
		}
		// An engineer approval cannot advance a failing step (TC-025): the
		// ground truth wins, so a synthesized correction keeps the loop
		// converging on the gap.
		if strings.TrimSpace(correction) == "" {
			correction = fmt.Sprintf(
				"The step %q is not complete yet. Ground-truth verification reports: %s. Fix exactly that and continue.",
				st.Name, gap)
		}
		if m.reporter != nil {
			m.reporter.OnEngineerReview(ctx, st.Name, correction)
		}
		msg = correction
	}
}

func (m *StageMachine) seedPrompt(ctx context.Context, core CoreRunner, sc *StageContext, st Stage) (string, error) {
	var prompt string
	switch {
	case st.Command != "":
		args := ""
		if st.Args != nil {
			args = st.Args(sc)
		}
		materialized, err := core.StagePrompt(ctx, st.Command, args)
		if err != nil {
			return "", fmt.Errorf("materialize %s: %w", st.Command, err)
		}
		prompt = materialized
	case st.Prompt != nil:
		prompt = st.Prompt(sc)
	default:
		return "", fmt.Errorf("step %s has neither Command nor Prompt", st.Name)
	}
	if st.Append != nil {
		if extra := strings.TrimSpace(st.Append(sc)); extra != "" {
			prompt += "\n\n" + extra
		}
	}
	return prompt, nil
}

// PipelineDeps carries the verification dependencies LeanPipeline closures
// capture: the completion gate and the read-only git queries used by the
// implement stage's Verify.
type PipelineDeps struct {
	Gate     GateChecker
	Git      GitRunner
	Gates    GateConfig
	Reporter Reporter
}

// specsRelDir is the directory generate-spec creates feature dirs under,
// relative to the worktree root.
const specsRelDir = ".codexspec/specs"

// LeanPipeline returns the v1 SDD stages in fixed order:
// generate-spec → spec-to-plan → plan-to-tasks → implement-tasks (REQ-009).
// The backlog item's Description seeds generate-spec as the already
// clarified requirement (REQ-010), and the spec directory produced by
// generate-spec is bound to the StageContext and threaded through the later
// stages (REQ-011).
func LeanPipeline(deps PipelineDeps) []Stage {
	return []Stage{
		{
			Name:    "generate-spec",
			Command: "codexspec:generate-spec",
			Args: func(sc *StageContext) string {
				return sc.Item.Description
			},
			// The generate-spec command body does not consume $ARGUMENTS,
			// so the already-clarified requirement is appended explicitly
			// (REQ-010; specify/clarify are intentionally skipped).
			Append: func(sc *StageContext) string {
				return "## Requirement (already clarified — generate the spec directly from it)\n\n" +
					sc.Item.Description +
					"\n\nDo not wait for further clarification; this requirement is final. " +
					"Create the spec directory and spec.md under " + specsRelDir + "/ now."
			},
			Prepare: snapshotSpecDirs,
			Verify:  verifyGenerateSpec,
		},
		{
			Name:    "spec-to-plan",
			Command: "codexspec:spec-to-plan",
			Args: func(sc *StageContext) string {
				return filepath.Join(sc.SpecDir, "spec.md")
			},
			Verify: verifySpecArtifact("plan.md"),
		},
		{
			Name:    "plan-to-tasks",
			Command: "codexspec:plan-to-tasks",
			Args: func(sc *StageContext) string {
				return filepath.Join(sc.SpecDir, "spec.md") + " " + filepath.Join(sc.SpecDir, "plan.md")
			},
			Verify: verifySpecArtifact("tasks.md"),
		},
		{
			Name:    "implement-tasks",
			Command: "codexspec:implement-tasks",
			Args: func(sc *StageContext) string {
				return filepath.Join(sc.SpecDir, "tasks.md")
			},
			Verify: verifyImplement(deps),
		},
	}
}

// snapshotSpecDirs records the spec directories existing before
// generate-spec runs so the new one is detectable by diff (REQ-011).
func snapshotSpecDirs(ctx context.Context, sc *StageContext) error {
	sc.PreexistingSpecDirs = map[string]bool{}
	entries, err := os.ReadDir(filepath.Join(sc.WorkDir, specsRelDir))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			sc.PreexistingSpecDirs[e.Name()] = true
		}
	}
	return nil
}

// verifyGenerateSpec passes when a non-empty spec.md exists in the bound
// spec directory, binding the directory first when a new one appeared
// under .codexspec/specs/ (REQ-011, REQ-012).
func verifyGenerateSpec(ctx context.Context, sc *StageContext) (bool, string) {
	if sc.SpecDir != "" {
		return nonEmptyFile(sc, "spec.md")
	}

	entries, err := os.ReadDir(filepath.Join(sc.WorkDir, specsRelDir))
	if err != nil {
		return false, fmt.Sprintf("no spec directory was created under %s", specsRelDir)
	}
	for _, e := range entries {
		if !e.IsDir() || sc.PreexistingSpecDirs[e.Name()] {
			continue
		}
		candidate := filepath.Join(specsRelDir, e.Name())
		if info, err := os.Stat(filepath.Join(sc.WorkDir, candidate, "spec.md")); err == nil && info.Size() > 0 {
			sc.SpecDir = candidate
			return true, ""
		}
	}
	return false, fmt.Sprintf("no new directory containing a non-empty spec.md exists under %s", specsRelDir)
}

// verifySpecArtifact passes when the named artifact exists non-empty in
// the bound spec directory.
func verifySpecArtifact(artifact string) func(ctx context.Context, sc *StageContext) (bool, string) {
	return func(ctx context.Context, sc *StageContext) (bool, string) {
		if sc.SpecDir == "" {
			return false, "no spec directory is bound for this item"
		}
		return nonEmptyFile(sc, artifact)
	}
}

func nonEmptyFile(sc *StageContext, artifact string) (bool, string) {
	path := filepath.Join(sc.SpecDir, artifact)
	info, err := os.Stat(filepath.Join(sc.WorkDir, path))
	if err != nil {
		return false, fmt.Sprintf("%s does not exist", path)
	}
	if info.Size() == 0 {
		return false, fmt.Sprintf("%s exists but is empty", path)
	}
	return true, ""
}

// verifyImplement passes when the completion gate is green AND the worktree
// holds real changes — a non-empty diff against the base branch or a dirty
// working tree (REQ-012, REQ-018, REQ-029; TC-010, TC-026).
func verifyImplement(deps PipelineDeps) func(ctx context.Context, sc *StageContext) (bool, string) {
	return func(ctx context.Context, sc *StageContext) (bool, string) {
		result, err := deps.Gate.Check(ctx, sc.WorkDir, deps.Gates)
		if err != nil {
			return false, fmt.Sprintf("completion gate could not run: %v", err)
		}
		if deps.Reporter != nil {
			deps.Reporter.OnGate(ctx, result)
		}
		if !result.Passed {
			return false, gateGap(result)
		}

		dirty, dirtyErr := deps.Git.Run(ctx, sc.WorkDir, "status", "--porcelain")
		if dirtyErr == nil && strings.TrimSpace(dirty) != "" {
			return true, ""
		}
		diff, diffErr := deps.Git.Run(ctx, sc.WorkDir, "diff", sc.BaseBranch+"...HEAD", "--name-only")
		if diffErr == nil && strings.TrimSpace(diff) != "" {
			return true, ""
		}
		// A failing git query is its own gap: claiming "no changes" when
		// git itself broke would steer the engineer at a phantom problem.
		if dirtyErr != nil || diffErr != nil {
			return false, fmt.Sprintf("cannot inspect the worktree diff (git status: %v; git diff: %v)", dirtyErr, diffErr)
		}
		return false, "the worktree contains no changes (empty diff); implement-tasks must produce real code changes"
	}
}

func gateGap(result GateResult) string {
	var failed []string
	for _, s := range result.Steps {
		if !s.Passed && !s.Skipped {
			detail := s.Name
			if out := strings.TrimSpace(s.Output); out != "" {
				detail += ": " + oneLine(out, 400)
			}
			failed = append(failed, detail)
		}
	}
	if len(failed) == 0 {
		return "the completion gate failed"
	}
	return "the completion gate failed — " + strings.Join(failed, "; ")
}
