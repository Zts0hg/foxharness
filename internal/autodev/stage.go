package autodev

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
	// Control runs a deterministic, non-LLM control-plane step. When set,
	// RunStep executes Control and verifies once instead of seeding the
	// core Agent.
	Control func(ctx context.Context, sc *StageContext) error
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
	if st.Control != nil {
		if err := st.Control(ctx, sc); err != nil {
			return fmt.Errorf("control step %s: %w", st.Name, err)
		}
		if st.Verify != nil {
			ok, gap := st.Verify(ctx, sc)
			if m.reporter != nil {
				m.reporter.OnVerify(ctx, st.Name, ok, gap)
			}
			if !ok {
				return fmt.Errorf("control step %s did not satisfy verification: %s", st.Name, gap)
			}
		}
		return nil
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

// PipelineDeps carries the fixed requirements-first pipeline dependencies:
// the completion gate, read-only git queries, and clock used to create
// CodexSpec feature workspace IDs.
type PipelineDeps struct {
	Gate     GateChecker
	Git      GitRunner
	Gates    GateConfig
	Reporter Reporter
	Clock    Clock
}

// specsRelDir is the CodexSpec feature workspace root, relative to the
// worktree root.
const specsRelDir = ".codexspec/specs"

// RequirementsFirstPipeline returns the fixed CodexSpec requirements-first
// SDD stages. The workflow is intentionally not user-configurable: the
// control plane materializes confirmed requirements from the backlog, then
// drives CodexSpec with explicit artifact paths and gates each generated
// artifact on its paired review report.
func RequirementsFirstPipeline(deps PipelineDeps) []Stage {
	if deps.Clock == nil {
		deps.Clock = SystemClock{}
	}
	return []Stage{
		{
			Name:    "materialize-requirements",
			Control: materializeRequirements(deps.Clock),
			Verify:  verifySpecArtifact("requirements.md"),
		},
		{
			Name:    "generate-spec",
			Command: "codexspec:generate-spec",
			Args: func(sc *StageContext) string {
				return filepath.Join(sc.FeatureDir, "requirements.md")
			},
			Verify: verifyReviewedArtifact("spec.md", "review-spec.md"),
		},
		{
			Name:    "spec-to-plan",
			Command: "codexspec:spec-to-plan",
			Args: func(sc *StageContext) string {
				return filepath.Join(sc.FeatureDir, "spec.md")
			},
			Verify: verifyReviewedArtifact("plan.md", "review-plan.md"),
		},
		{
			Name:    "plan-to-tasks",
			Command: "codexspec:plan-to-tasks",
			Args: func(sc *StageContext) string {
				return filepath.Join(sc.FeatureDir, "plan.md")
			},
			Verify: verifyReviewedArtifact("tasks.md", "review-tasks.md"),
		},
		{
			Name:    "implement-tasks",
			Command: "codexspec:implement-tasks",
			Args: func(sc *StageContext) string {
				return filepath.Join(sc.FeatureDir, "tasks.md")
			},
			Verify: verifyImplement(deps),
		},
	}
}

func materializeRequirements(clock Clock) func(ctx context.Context, sc *StageContext) error {
	return func(ctx context.Context, sc *StageContext) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if sc.FeatureDir == "" {
			name, err := newFeatureDirName(clock, sc.Slug)
			if err != nil {
				return err
			}
			sc.FeatureDir = filepath.Join(specsRelDir, name)
		}
		reqPath := filepath.Join(sc.WorkDir, sc.FeatureDir, "requirements.md")
		if info, err := os.Stat(reqPath); err == nil && info.Size() > 0 {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(reqPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(reqPath, []byte(requirementsDocument(sc, clock.Now())), 0o644)
	}
}

// verifySpecArtifact passes when the named artifact exists non-empty in
// the bound feature directory.
func verifySpecArtifact(artifact string) func(ctx context.Context, sc *StageContext) (bool, string) {
	return func(ctx context.Context, sc *StageContext) (bool, string) {
		if sc.FeatureDir == "" {
			return false, "no feature directory is bound for this item"
		}
		return nonEmptyFile(sc, artifact)
	}
}

func nonEmptyFile(sc *StageContext, artifact string) (bool, string) {
	path := filepath.Join(sc.FeatureDir, artifact)
	info, err := os.Stat(filepath.Join(sc.WorkDir, path))
	if err != nil {
		return false, fmt.Sprintf("%s does not exist", path)
	}
	if info.Size() == 0 {
		return false, fmt.Sprintf("%s exists but is empty", path)
	}
	return true, ""
}

func verifyReviewedArtifact(artifact, review string) func(ctx context.Context, sc *StageContext) (bool, string) {
	return func(ctx context.Context, sc *StageContext) (bool, string) {
		if ok, gap := verifySpecArtifact(artifact)(ctx, sc); !ok {
			return false, gap
		}
		if ok, gap := verifySpecArtifact(review)(ctx, sc); !ok {
			return false, gap
		}
		status, err := readReviewStatus(filepath.Join(sc.WorkDir, sc.FeatureDir, review))
		if err != nil {
			return false, err.Error()
		}
		switch status {
		case "PASS", "PASS_WITH_WARNINGS":
			return true, ""
		default:
			return false, fmt.Sprintf("%s reports Overall Status %s", filepath.Join(sc.FeatureDir, review), status)
		}
	}
}

var reviewStatusRE = regexp.MustCompile(`(?im)\*\*Overall Status\*\*\s*:\s*([A-Z_]+)`)

func readReviewStatus(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read review status: %w", err)
	}
	m := reviewStatusRE.FindSubmatch(data)
	if len(m) != 2 {
		return "", fmt.Errorf("%s has no parseable Overall Status", path)
	}
	return string(m[1]), nil
}

func newFeatureDirName(clock Clock, slug string) (string, error) {
	if strings.TrimSpace(slug) == "" {
		slug = "item"
	}
	suffix, err := randomSuffix(2)
	if err != nil {
		return "", err
	}
	return clock.Now().UTC().Format("2006-0102-1504") + suffix + "-" + slug, nil
}

const randomAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomSuffix(n int) (string, error) {
	var b strings.Builder
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(randomAlphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(randomAlphabet[idx.Int64()])
	}
	return b.String(), nil
}

func requirementsDocument(sc *StageContext, now time.Time) string {
	confirmedAt := now.UTC().Format(time.RFC3339)
	title := strings.TrimSpace(sc.Item.Title)
	if title == "" {
		title = sc.Slug
	}
	if title == "" {
		title = "Autodev backlog item"
	}
	statement := strings.TrimSpace(sc.Item.Description)
	if statement == "" {
		statement = title
	}
	statementLine := oneLine(statement, 4000)
	featureName := filepath.Base(sc.FeatureDir)
	featureID := featureName
	if len(featureID) >= len("2006-0102-1504ab") {
		featureID = featureID[:len("2006-0102-1504ab")]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Confirmed Requirements: %s\n\n", title)
	b.WriteString("<!--\n")
	b.WriteString("This file is generated by fox autodev from a backlog item. The backlog item is treated as the confirmed user input for unattended development.\n")
	b.WriteString("-->\n\n")
	fmt.Fprintf(&b, "**Feature ID**: `%s`\n", featureID)
	b.WriteString("**Status**: Confirmed\n")
	fmt.Fprintf(&b, "**Last Confirmed**: %s\n\n", confirmedAt)
	b.WriteString("## Authority Rules\n\n")
	b.WriteString("- Only entries with `Status: confirmed` are binding downstream inputs.\n")
	b.WriteString("- The backlog item title and description are the confirmation source for this unattended autodev run.\n")
	b.WriteString("- AI inferences must not be promoted to confirmed requirements without a later user-confirmed backlog update.\n\n")
	b.WriteString("## Needs\n\n")
	fmt.Fprintf(&b, "### NEED-001: %s\n\n", title)
	b.WriteString("- **Status**: confirmed\n")
	fmt.Fprintf(&b, "- **Statement**: %s\n", statementLine)
	b.WriteString("- **Rationale**: This behavior is required by the autodev backlog item.\n")
	fmt.Fprintf(&b, "- **User Evidence**: \"%s\"\n", oneLine(title+": "+statementLine, 500))
	fmt.Fprintf(&b, "- **Confirmed At**: %s\n\n", confirmedAt)
	b.WriteString("## Constraints\n\n")
	b.WriteString("No confirmed constraints were supplied by the backlog item.\n\n")
	b.WriteString("## Decisions\n\n")
	b.WriteString("No confirmed trade-off decisions were supplied by the backlog item.\n\n")
	b.WriteString("## Out of Scope\n\n")
	b.WriteString("No confirmed exclusions were supplied by the backlog item.\n\n")
	b.WriteString("## Open Questions\n\n")
	b.WriteString("No blocking open questions were supplied by the backlog item.\n\n")
	b.WriteString("## Superseded Entries\n\n")
	b.WriteString("No superseded entries.\n\n")
	b.WriteString("## Confirmation Log\n\n")
	fmt.Fprintf(&b, "### Session %s\n\n", confirmedAt)
	fmt.Fprintf(&b, "- **Summary Presented**: %s\n", oneLine(statementLine, 500))
	b.WriteString("- **User Confirmation**: The backlog item is treated as confirmed input for unattended autodev.\n")
	b.WriteString("- **Entries Confirmed**: NEED-001\n")
	return b.String()
}

// verifyImplement passes when the completion gate is green AND the worktree
// holds real changes — a non-empty diff against the base branch or a dirty
// working tree (REQ-012, REQ-018, REQ-029; TC-010, TC-026).
func verifyImplement(deps PipelineDeps) func(ctx context.Context, sc *StageContext) (bool, string) {
	return func(ctx context.Context, sc *StageContext) (bool, string) {
		if ok, gap := verifyTasksComplete(sc); !ok {
			return false, gap
		}

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

var taskCheckboxRE = regexp.MustCompile(`(?m)^\s*[-*]\s+\[([ xX])\]`)

func verifyTasksComplete(sc *StageContext) (bool, string) {
	if sc.FeatureDir == "" {
		return false, "no feature directory is bound for this item"
	}
	path := filepath.Join(sc.WorkDir, sc.FeatureDir, "tasks.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("%s does not exist", filepath.Join(sc.FeatureDir, "tasks.md"))
	}
	matches := taskCheckboxRE.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return false, fmt.Sprintf("%s contains no markdown task checkboxes", filepath.Join(sc.FeatureDir, "tasks.md"))
	}
	var unchecked int
	for _, m := range matches {
		if len(m) == 2 && string(m[1]) == " " {
			unchecked++
		}
	}
	if unchecked > 0 {
		return false, fmt.Sprintf("%s has %d unchecked task checkbox(es)", filepath.Join(sc.FeatureDir, "tasks.md"), unchecked)
	}
	return true, ""
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
