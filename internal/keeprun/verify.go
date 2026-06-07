package keeprun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// TaskContext carries the per-task facts a completion gate may need to inspect.
type TaskContext struct {
	// Slug is the task slug (and the keep-run-<slug> branch suffix).
	Slug string
	// WorktreeDir is the task's worktree, used as the working directory for git
	// and Go toolchain gates.
	WorktreeDir string
	// SpecDir is the .codexspec/specs/<slug>/ directory holding SDD artifacts.
	SpecDir string
	// BaseRef is the branch the worktree was rooted at (the repo default branch).
	BaseRef string
	// HeadCommitBefore is the worktree HEAD captured before the commit phase ran,
	// used by the commit-staged gate to confirm a new commit landed.
	HeadCommitBefore string
	// Config is the keep-run configuration cached for the task.
	Config Config
}

// CommandRunner runs name with args in dir and returns the combined output. The
// Verifier depends on it for git and Go-toolchain gates so tests can inject a
// fake and keep the gates fast and deterministic.
type CommandRunner func(ctx context.Context, dir, name string, args ...string) ([]byte, error)

// Verifier checks the deterministic per-phase completion gate that lets the
// orchestrator record a phase complete only when it provably produced its
// artifact (spec FR-013). Generative phases are judged by the filesystem; review
// phases by the injected verdict block (Decision 8) and, for review-code, by
// objective Go-toolchain gates; commit/implement phases by git state.
type Verifier struct {
	run CommandRunner
}

// VerifierOption configures a Verifier via the functional-options pattern.
type VerifierOption func(*Verifier)

// WithCommandRunner overrides the command runner; intended for tests.
func WithCommandRunner(r CommandRunner) VerifierOption {
	return func(v *Verifier) { v.run = r }
}

// NewVerifier returns a Verifier that shells out to git and the Go toolchain by
// default.
func NewVerifier(opts ...VerifierOption) *Verifier {
	v := &Verifier{run: execRunner}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

func execRunner(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// verdictPattern extracts the JSON payload of a keep-run verdict block.
var verdictPattern = regexp.MustCompile(`(?s)<!--\s*keep-run-verdict:\s*(\{.*?\})\s*-->`)

// prClosesPattern matches an issue reference such as "Closes #12" or "#12".
var prClosesPattern = regexp.MustCompile(`#\d+`)

// urlPattern matches an http(s) URL, used as evidence a PR was created.
var urlPattern = regexp.MustCompile(`https?://\S+`)

// reviewVerdict is the machine-readable result a review phase emits (Decision 8).
type reviewVerdict struct {
	Status   string `json:"status"`
	Critical int    `json:"critical"`
	High     int    `json:"high"`
}

// ReviewClean reports whether a review phase's output contains a verdict block
// with status "pass". A missing or malformed block counts as not clean so the
// orchestrator keeps iterating (fail-safe; Decision 8).
func ReviewClean(out PhaseOutcome) bool {
	m := verdictPattern.FindStringSubmatch(out.Output)
	if m == nil {
		return false
	}
	var v reviewVerdict
	if err := json.Unmarshal([]byte(m[1]), &v); err != nil {
		return false
	}
	return v.Status == "pass"
}

// VerifyPhase returns nil when phase provably produced its artifact, or an error
// describing the unmet gate. The gate per phase matches the plan's verification
// matrix.
func (v *Verifier) VerifyPhase(ctx context.Context, phase Phase, tc TaskContext, out PhaseOutcome) error {
	switch phase.Command {
	case "codexspec:specify", "codexspec:clarify":
		if strings.TrimSpace(out.Output) == "" {
			return fmt.Errorf("%s: empty output", phase.Command)
		}
		return nil
	case "codexspec:generate-spec":
		return requireNonEmptyFile(filepath.Join(tc.SpecDir, "spec.md"))
	case "codexspec:spec-to-plan":
		return requireNonEmptyFile(filepath.Join(tc.SpecDir, "plan.md"))
	case "codexspec:plan-to-tasks":
		return requireNonEmptyFile(filepath.Join(tc.SpecDir, "tasks.md"))
	case "codexspec:implement-tasks":
		return v.verifyImplement(ctx, tc)
	case "codexspec:review-spec", "codexspec:review-plan", "codexspec:review-tasks":
		if !ReviewClean(out) {
			return fmt.Errorf("%s: review not clean", phase.Command)
		}
		return nil
	case "codexspec:review-code":
		if !ReviewClean(out) {
			return fmt.Errorf("%s: review not clean", phase.Command)
		}
		return v.verifyObjectiveGates(ctx, tc.WorktreeDir)
	case "codexspec:commit-staged":
		return v.verifyCommitted(ctx, tc)
	case "codexspec:pr":
		return verifyPR(out)
	default:
		return fmt.Errorf("no completion gate defined for %s", phase.Command)
	}
}

// requireNonEmptyFile confirms path exists and holds non-whitespace content.
func requireNonEmptyFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("artifact %s: %w", filepath.Base(path), err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return fmt.Errorf("artifact %s is empty", filepath.Base(path))
	}
	return nil
}

// verifyImplement gates the implement-tasks phase: the project tests must pass
// and the working tree must show changes.
func (v *Verifier) verifyImplement(ctx context.Context, tc TaskContext) error {
	if out, err := v.run(ctx, tc.WorktreeDir, "go", "test", "./..."); err != nil {
		return fmt.Errorf("implement-tasks: tests failing: %w: %s", err, strings.TrimSpace(string(out)))
	}
	out, err := v.run(ctx, tc.WorktreeDir, "git", "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("implement-tasks: git status: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("implement-tasks: no working-tree changes produced")
	}
	return nil
}

// verifyObjectiveGates runs the review-code objective gates (build, vet, test,
// gofmt) in dir. gofmt must report no unformatted files.
func (v *Verifier) verifyObjectiveGates(ctx context.Context, dir string) error {
	for _, step := range [][]string{
		{"go", "build", "./..."},
		{"go", "vet", "./..."},
		{"go", "test", "./..."},
	} {
		if out, err := v.run(ctx, dir, step[0], step[1:]...); err != nil {
			return fmt.Errorf("review-code gate %q failed: %w: %s", strings.Join(step, " "), err, strings.TrimSpace(string(out)))
		}
	}
	out, err := v.run(ctx, dir, "gofmt", "-l", ".")
	if err != nil {
		return fmt.Errorf("review-code gofmt: %w", err)
	}
	if files := strings.TrimSpace(string(out)); files != "" {
		return fmt.Errorf("review-code: unformatted files: %s", files)
	}
	return nil
}

// verifyCommitted gates the commit-staged phase: HEAD must have advanced since
// the phase started and the working tree must be clean.
func (v *Verifier) verifyCommitted(ctx context.Context, tc TaskContext) error {
	head, err := v.run(ctx, tc.WorktreeDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("commit-staged: rev-parse: %w", err)
	}
	if strings.TrimSpace(string(head)) == tc.HeadCommitBefore {
		return fmt.Errorf("commit-staged: HEAD did not advance")
	}
	st, err := v.run(ctx, tc.WorktreeDir, "git", "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("commit-staged: git status: %w", err)
	}
	if strings.TrimSpace(string(st)) != "" {
		return fmt.Errorf("commit-staged: working tree not clean")
	}
	return nil
}

// verifyPR gates the pr phase. A real push/PR cannot be re-derived here, so the
// gate inspects the phase output the orchestrator captured for a PR URL and an
// issue reference (Closes #N).
func verifyPR(out PhaseOutcome) error {
	if !urlPattern.MatchString(out.Output) {
		return fmt.Errorf("pr: no PR URL in output")
	}
	if !prClosesPattern.MatchString(out.Output) {
		return fmt.Errorf("pr: output does not reference an issue (#N)")
	}
	return nil
}
