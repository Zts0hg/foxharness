package keeprun

import "context"

// PhaseRunner executes a single SDD phase by driving the LLM engine to run the
// corresponding /codexspec:* command to completion. It is the only seam through
// which the orchestrator reaches the LLM, which lets the orchestrator be
// unit-tested with a fake implementation and no real model (spec NFR-006).
type PhaseRunner interface {
	// RunPhase runs the phase described by req and returns its outcome. The
	// implementation honors req.AllowedTools (notably excluding merge
	// operations, FR-010) and req.Config (review mode and guidance prompts).
	RunPhase(ctx context.Context, req PhaseRequest) (PhaseOutcome, error)
}

// PhaseRequest fully describes one phase invocation.
type PhaseRequest struct {
	// Phase is the SDD phase to run, including its codexspec command.
	Phase Phase
	// WorktreeDir is the task's worktree; the phase runs with it as the working
	// directory so artifacts land in the right place.
	WorktreeDir string
	// SpecDir is the .codexspec/specs/<slug>/ directory that holds the task's
	// SDD artifacts (spec.md, plan.md, tasks.md, review reports).
	SpecDir string
	// Config carries the keep-run configuration (review mode, clarify and
	// review-fix prompts) that guides non-interactive decisions (FR-004).
	Config Config
	// Instruction is optional extra guidance injected into the run, such as the
	// verdict-block contract for a review phase or a fix prompt on a review retry.
	Instruction string
	// AllowedTools restricts the tool set for the run. It must exclude
	// merge-capable operations so the merge prohibition holds by construction
	// (FR-010).
	AllowedTools []string
}

// PhaseOutcome carries what the orchestrator and verifier need to judge a phase.
type PhaseOutcome struct {
	// Output is the raw engine output for the phase. A review phase embeds its
	// machine-readable verdict block here for ReviewClean to parse.
	Output string
}

// ReviewVerdictInstruction is injected (via PhaseRequest.Instruction) into every
// review phase so the run ends with the machine-readable verdict block the
// verifier parses deterministically (Decision 8). This constant is the single
// source of the verdict contract; the /codexspec:review-* command files are not
// modified. Its format matches the pattern ReviewClean expects.
const ReviewVerdictInstruction = "When the review is complete, append to your output one final " +
	"machine-readable verdict line, exactly in this form and not inside a code fence:\n" +
	`<!-- keep-run-verdict: {"status":"pass","critical":0,"high":0} -->` + "\n" +
	`Set "status" to "pass" only when zero issues, warnings, and suggestions remain; ` +
	`otherwise use "needs_work" or "fail" and report the count of critical and high findings.`
