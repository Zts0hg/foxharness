package autodev

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

var coreLifecycleTimeout = 2 * time.Minute

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
	// Preflight performs an error-capable idempotency check before Skip.
	// It is used when ambiguous ground truth must fail rather than retry.
	Preflight func(ctx context.Context, sc *StageContext) (skip bool, err error)
	// Verify is the read-only ground-truth predicate deciding advancement,
	// returning ok or a gap describing precisely what is still missing.
	Verify func(ctx context.Context, sc *StageContext) (ok bool, gap string)
	// VerifyWithError is the error-capable form used by identity-sensitive
	// verification. When present it is authoritative over Verify.
	VerifyWithError func(ctx context.Context, sc *StageContext) (ok bool, gap string, err error)
}

// StageMachine drives one step at a time through the supervised loop:
// seed → core Run → Go Verify → engineer Review correction → retry, until
// the ground truth says the step completed (REQ-030). The LLM can never
// terminate the loop early; only Verify can.
type StageMachine struct {
	engineer EngineerAgent
	reporter Reporter
}

type coreRunnerReplacer interface {
	Replace(context.Context) error
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
	return m.runStep(ctx, core, sc, st, false)
}

// ResumeStep verifies a durably running stage before driving the core again.
// This closes the crash window between external completion and the ledger's
// verified transition without changing fresh-stage behavior.
func (m *StageMachine) ResumeStep(ctx context.Context, core CoreRunner, sc *StageContext, st Stage) error {
	return m.runStep(ctx, core, sc, st, true)
}

func (m *StageMachine) runStep(ctx context.Context, core CoreRunner, sc *StageContext, st Stage, verifyFirst bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sc.Stage = st.Name
	if m.reporter != nil {
		m.reporter.OnStageStart(ctx, sc.Slug, st.Name)
	}
	if st.Preflight != nil {
		skip, err := st.Preflight(ctx, sc)
		if err != nil {
			return fmt.Errorf("preflight step %s: %w", st.Name, err)
		}
		if skip {
			return nil
		}
	}

	if st.Skip != nil && st.Skip(ctx, sc) {
		if m.reporter != nil {
			m.reporter.OnInfo(ctx, fmt.Sprintf("step %s already satisfied; skipping", st.Name))
		}
		return nil
	}
	if verifyFirst && (st.Verify != nil || st.VerifyWithError != nil) {
		ok, gap, err := verifyStage(ctx, sc, st)
		if err != nil {
			return fmt.Errorf("verify step %s: %w", st.Name, err)
		}
		if m.reporter != nil {
			m.reporter.OnVerify(ctx, st.Name, ok, gap)
		}
		if ok {
			if m.reporter != nil {
				m.reporter.OnInfo(ctx, fmt.Sprintf("step %s already verified; skipping", st.Name))
			}
			return nil
		}
	}
	if st.Prepare != nil {
		attemptCtx, cancel := withDefaultTimeout(ctx, stageAttemptTimeout)
		err := st.Prepare(attemptCtx, sc)
		attemptErr := attemptCtx.Err()
		cancel()
		if err != nil {
			return fmt.Errorf("prepare step %s: %w", st.Name, err)
		}
		if attemptErr != nil {
			return attemptErr
		}
	}
	if st.Control != nil {
		attemptCtx, cancel := withDefaultTimeout(ctx, stageAttemptTimeout)
		err := st.Control(attemptCtx, sc)
		attemptErr := attemptCtx.Err()
		cancel()
		if err != nil {
			return fmt.Errorf("control step %s: %w", st.Name, err)
		}
		if attemptErr != nil {
			return attemptErr
		}
		if st.Verify != nil || st.VerifyWithError != nil {
			ok, gap, err := verifyStage(ctx, sc, st)
			if err != nil {
				return fmt.Errorf("verify control step %s: %w", st.Name, err)
			}
			if m.reporter != nil {
				m.reporter.OnVerify(ctx, st.Name, ok, gap)
			}
			if !ok {
				return fmt.Errorf("control step %s did not satisfy verification: %s", st.Name, gap)
			}
		}
		return nil
	}

	attemptCtx, cancel := withDefaultTimeout(ctx, stageAttemptTimeout)
	msg, err := m.seedPrompt(attemptCtx, core, sc, st)
	attemptErr := attemptCtx.Err()
	cancel()
	if err != nil {
		return err
	}
	if attemptErr != nil {
		return attemptErr
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		attempt := newCoreAttempt(sc, msg)
		if err := recordCoreAttempt(sc, runningCoreAttemptRecord(sc, attempt)); err != nil {
			return err
		}
		attemptCtx, cancel := withDefaultTimeout(ctx, stageAttemptTimeout)
		outcome := runCoreAttempt(attemptCtx, core, attempt, m.reporter)
		attemptErr := attemptCtx.Err()
		cancel()
		var contractErr error
		if outcome.Attempt != attempt {
			contractErr = fmt.Errorf("core run for step %s returned mismatched attempt correlation", st.Name)
			outcome.Attempt = attempt
			outcome.Status = CoreOutcomeFailed
			outcome.Cause = errors.Join(outcome.Cause, contractErr)
			outcome.RetryClass = CoreRetryNever
		}
		if err := outcome.Validate(); err != nil {
			return &CoreOutcomeError{Outcome: outcome, Err: fmt.Errorf("core run for step %s returned invalid outcome: %w", st.Name, err)}
		}
		if outcome.Lifecycle.RunStarted {
			if drainErr := drainCore(ctx, core); drainErr != nil {
				lifecycleErr := &CoreLifecycleError{Operation: "drain", Err: errors.Join(outcome.Cause, drainErr)}
				outcome.Status = CoreOutcomeFailed
				outcome.Cause = lifecycleErr
				outcome.RetryClass = CoreRetryNever
				if err := recordCoreAttempt(sc, terminalCoreAttemptRecord(sc, outcome)); err != nil {
					return &CoreOutcomeError{Outcome: outcome, Err: errors.Join(outcome.Cause, err)}
				}
				return &CoreOutcomeError{Outcome: outcome, Err: lifecycleErr}
			}
			outcome.Lifecycle.DrainCompleted = true
		}

		ok := false
		gap := ""
		var verifyErr error
		if outcome.Lifecycle.RunStarted {
			verifyCtx, verifyCancel := coreVerificationContext(ctx, outcome)
			ok, gap, verifyErr = verifyStage(verifyCtx, sc, st)
			if m.reporter != nil {
				m.reporter.OnVerify(verifyCtx, st.Name, ok, gap)
			}
			verifyCancel()
		} else {
			gap = "the core attempt did not start"
			if outcome.Cause != nil {
				gap += ": " + outcome.Cause.Error()
			}
		}
		if err := recordCoreAttempt(sc, terminalCoreAttemptRecord(sc, outcome)); err != nil {
			return &CoreOutcomeError{Outcome: outcome, Err: errors.Join(outcome.Cause, err)}
		}
		if verifyErr != nil {
			return &CoreOutcomeError{Outcome: outcome, Err: errors.Join(outcome.Cause, fmt.Errorf("verify step %s: %w", st.Name, verifyErr))}
		}
		if contractErr != nil {
			return &CoreOutcomeError{Outcome: outcome, Err: outcome.Cause}
		}
		if ok {
			if outcome.Status == CoreOutcomeCancelled {
				return &CoreOutcomeError{Outcome: outcome, Verified: true}
			}
			return nil
		}
		if outcome.Status == CoreOutcomeCancelled ||
			(outcome.Status != CoreOutcomeSucceeded && outcome.RetryClass == CoreRetryNever) {
			return &CoreOutcomeError{Outcome: outcome}
		}
		if attemptErr != nil && outcome.Status != CoreOutcomeCancelled {
			return &CoreOutcomeError{Outcome: outcome, Err: errors.Join(outcome.Cause, attemptErr)}
		}

		attemptCtx, cancel = withDefaultTimeout(ctx, stageAttemptTimeout)
		correction, err := m.engineer.Review(attemptCtx, outcome.reviewEvidence(), gap, *sc)
		attemptErr = attemptCtx.Err()
		cancel()
		if err != nil {
			return fmt.Errorf("engineer review for step %s: %w", st.Name, err)
		}
		if attemptErr != nil {
			return attemptErr
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
		if outcome.RetryClass == CoreRetryFreshRunner {
			replacer, ok := core.(coreRunnerReplacer)
			if !ok {
				return &CoreOutcomeError{Outcome: outcome, Err: errors.Join(outcome.Cause, errors.New("fresh core runner required but replacement is unavailable"))}
			}
			replaceCtx, replaceCancel := context.WithTimeout(context.WithoutCancel(ctx), coreLifecycleTimeout)
			err := replacer.Replace(replaceCtx)
			replaceCancel()
			if err != nil {
				return &CoreOutcomeError{Outcome: outcome, Err: errors.Join(outcome.Cause, fmt.Errorf("replace core runner: %w", err))}
			}
		}
		msg = correction
	}
}

func runCoreAttempt(ctx context.Context, core CoreRunner, attempt CoreAttempt, reporter Reporter) (outcome CoreOutcome) {
	defer func() {
		if value := recover(); value != nil {
			outcome = CoreOutcome{
				Attempt:    attempt,
				Status:     CoreOutcomeStartFailed,
				Cause:      &CorePanicError{Value: value},
				RetryClass: CoreRetryNever,
			}
		}
	}()
	return core.Run(ctx, attempt, reporter)
}

func coreVerificationContext(ctx context.Context, outcome CoreOutcome) (context.Context, context.CancelFunc) {
	if outcome.Status == CoreOutcomeCancelled || ctx.Err() != nil {
		return context.WithTimeout(context.WithoutCancel(ctx), stageAttemptTimeout)
	}
	return context.WithCancel(ctx)
}

func recordCoreAttempt(sc *StageContext, record CoreAttemptRecord) error {
	if sc.RecordCoreAttempt == nil {
		return nil
	}
	if err := sc.RecordCoreAttempt(record); err != nil {
		var commitErr *LedgerCommitError
		if errors.As(err, &commitErr) {
			return err
		}
		return &LedgerCommitError{Operation: "core-attempt-" + record.AttemptID + "-" + string(record.State), Err: err}
	}
	return nil
}

func drainCore(ctx context.Context, core CoreRunner) error {
	drainParent := ctx
	if ctx.Err() != nil {
		drainParent = context.WithoutCancel(ctx)
	}
	drainCtx, cancel := context.WithTimeout(drainParent, coreLifecycleTimeout)
	defer cancel()
	return core.Drain(drainCtx)
}

func verifyStage(ctx context.Context, sc *StageContext, st Stage) (bool, string, error) {
	if st.VerifyWithError != nil {
		return st.VerifyWithError(ctx, sc)
	}
	ok, gap := st.Verify(ctx, sc)
	return ok, gap, nil
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
				return path.Join(sc.FeatureDir, "requirements.md")
			},
			Verify: verifyReviewedArtifact("spec.md", "review-spec.md"),
		},
		{
			Name:    "spec-to-plan",
			Command: "codexspec:spec-to-plan",
			Args: func(sc *StageContext) string {
				return path.Join(sc.FeatureDir, "spec.md")
			},
			Verify: verifyReviewedArtifact("plan.md", "review-plan.md"),
		},
		{
			Name:    "plan-to-tasks",
			Command: "codexspec:plan-to-tasks",
			Args: func(sc *StageContext) string {
				return path.Join(sc.FeatureDir, "plan.md")
			},
			Verify: verifyReviewedArtifact("tasks.md", "review-tasks.md"),
		},
		{
			Name:    "implement-tasks",
			Command: "codexspec:implement-tasks",
			Args: func(sc *StageContext) string {
				return path.Join(sc.FeatureDir, "tasks.md")
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
			sc.FeatureDir = path.Join(specsRelDir, name)
		}
		workspace, err := openFeatureWorkspace(sc.WorkDir, sc.FeatureDir, true)
		if err != nil {
			return err
		}
		defer workspace.Close()
		document := requirementsDocument(sc, clock.Now())
		existing, err := workspace.readRegular("requirements.md")
		if err == nil {
			if requirementsDocumentMatches(existing, sc) {
				return nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect requirements artifact: %w", err)
		}
		return workspace.writeRegular("requirements.md", []byte(document), 0o644)
	}
}

func requirementsDocumentMatches(existing []byte, sc *StageContext) bool {
	title := strings.TrimSpace(sc.Item.Title)
	if title == "" {
		title = sc.Slug
	}
	if title == "" {
		title = "Autodev backlog item"
	}
	statement := authoritativeRequirement(title, sc.Item.Description)
	bytes, hash := sc.RequirementBytes, sc.RequirementHash
	if hash == "" {
		bytes, hash = requirementIdentity(statement)
	}
	doc := string(existing)
	return strings.Contains(doc, fmt.Sprintf("**Item ID**: `%s`", sc.ItemID)) &&
		strings.Contains(doc, fmt.Sprintf("**Requirement Bytes**: %d", bytes)) &&
		strings.Contains(doc, fmt.Sprintf("**Requirement SHA-256**: `%s`", hash)) &&
		strings.Contains(doc, "## Authoritative Requirement\n\n"+statement+"\n\n## Constraints")
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
	artifactPath := path.Join(sc.FeatureDir, artifact)
	workspace, err := openFeatureWorkspace(sc.WorkDir, sc.FeatureDir, false)
	if err != nil {
		return false, fmt.Sprintf("%s is unavailable: %v", artifactPath, err)
	}
	defer workspace.Close()
	size, err := workspace.regularSize(artifact)
	if err != nil {
		return false, fmt.Sprintf("%s is unavailable: %v", artifactPath, err)
	}
	if size == 0 {
		return false, fmt.Sprintf("%s exists but is empty", artifactPath)
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
		workspace, err := openFeatureWorkspace(sc.WorkDir, sc.FeatureDir, false)
		if err != nil {
			return false, err.Error()
		}
		defer workspace.Close()
		data, err := workspace.readRegular(review)
		if err != nil {
			return false, fmt.Sprintf("read review status: %v", err)
		}
		status, err := readReviewStatus(data, path.Join(sc.FeatureDir, review))
		if err != nil {
			return false, err.Error()
		}
		switch status {
		case "PASS", "PASS_WITH_WARNINGS":
			return true, ""
		default:
			return false, fmt.Sprintf("%s reports Overall Status %s", path.Join(sc.FeatureDir, review), status)
		}
	}
}

var reviewStatusRE = regexp.MustCompile(`(?im)\*\*Overall Status\*\*\s*:\s*([A-Z_]+)`)

func readReviewStatus(data []byte, artifactPath string) (string, error) {
	m := reviewStatusRE.FindSubmatch(data)
	if len(m) != 2 {
		return "", fmt.Errorf("%s has no parseable Overall Status", artifactPath)
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
	statement := authoritativeRequirement(title, sc.Item.Description)
	requirementBytes, requirementHash := sc.RequirementBytes, sc.RequirementHash
	if requirementHash == "" {
		requirementBytes, requirementHash = requirementIdentity(statement)
	}
	featureName := path.Base(sc.FeatureDir)
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
	fmt.Fprintf(&b, "**Item ID**: `%s`\n", sc.ItemID)
	fmt.Fprintf(&b, "**Requirement Bytes**: %d\n", requirementBytes)
	fmt.Fprintf(&b, "**Requirement SHA-256**: `%s`\n", requirementHash)
	b.WriteString("**Status**: Confirmed\n")
	fmt.Fprintf(&b, "**Last Confirmed**: %s\n\n", confirmedAt)
	b.WriteString("## Authority Rules\n\n")
	b.WriteString("- Only entries with `Status: confirmed` are binding downstream inputs.\n")
	b.WriteString("- The backlog item title and description are the confirmation source for this unattended autodev run.\n")
	b.WriteString("- AI inferences must not be promoted to confirmed requirements without a later user-confirmed backlog update.\n\n")
	b.WriteString("## Needs\n\n")
	fmt.Fprintf(&b, "### NEED-001: %s\n\n", title)
	b.WriteString("- **Status**: confirmed\n")
	b.WriteString("- **Statement**: See the complete authoritative requirement below.\n")
	b.WriteString("- **Rationale**: This behavior is required by the autodev backlog item.\n")
	fmt.Fprintf(&b, "- **User Evidence**: \"%s\"\n", oneLine(title+": "+statement, 500))
	fmt.Fprintf(&b, "- **Confirmed At**: %s\n\n", confirmedAt)
	b.WriteString("## Authoritative Requirement\n\n")
	b.WriteString(statement)
	if !strings.HasSuffix(statement, "\n") {
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
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
	fmt.Fprintf(&b, "- **Summary Presented**: %s\n", oneLine(statement, 500))
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

		dirtyResult, runErr := deps.Git.Run(ctx, sc.WorkDir, "status", "--porcelain")
		dirty, dirtyErr := strictCommandStdout(dirtyResult, runErr)
		if dirtyErr == nil && strings.TrimSpace(dirty) != "" {
			return true, ""
		}
		diffResult, runErr := deps.Git.Run(ctx, sc.WorkDir, "diff", sc.BaseBranch+"...HEAD", "--name-only")
		diff, diffErr := strictCommandStdout(diffResult, runErr)
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
	artifactPath := path.Join(sc.FeatureDir, "tasks.md")
	workspace, err := openFeatureWorkspace(sc.WorkDir, sc.FeatureDir, false)
	if err != nil {
		return false, fmt.Sprintf("%s is unavailable: %v", artifactPath, err)
	}
	defer workspace.Close()
	data, err := workspace.readRegular("tasks.md")
	if err != nil {
		return false, fmt.Sprintf("%s is unavailable: %v", artifactPath, err)
	}
	matches := taskCheckboxRE.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return false, fmt.Sprintf("%s contains no markdown task checkboxes", artifactPath)
	}
	var unchecked int
	for _, m := range matches {
		if len(m) == 2 && string(m[1]) == " " {
			unchecked++
		}
	}
	if unchecked > 0 {
		return false, fmt.Sprintf("%s has %d unchecked task checkbox(es)", artifactPath, unchecked)
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
