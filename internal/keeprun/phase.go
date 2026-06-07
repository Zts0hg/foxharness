package keeprun

// Phase represents a single step in the SDD pipeline driven by /keep-run.
type Phase struct {
	// Name is a human-readable description of the phase.
	Name string
	// Command is the codexspec slash command to invoke, e.g. "codexspec:specify".
	Command string
	// Review marks a review phase that iterates until all issues, warnings, and
	// suggestions are resolved.
	Review bool
	// Remote marks a phase that performs remote operations (push, issue, PR) and
	// therefore runs only when remote_enabled is true.
	Remote bool
}

// PipelinePhases returns the twelve SDD pipeline phases in the exact order
// defined by spec FR-003. The review phases (review-spec, review-plan,
// review-tasks, review-code) iterate until clean, and only the final pr phase
// requires remote access.
func PipelinePhases() []Phase {
	return []Phase{
		{Name: "Requirement clarification", Command: "codexspec:specify"},
		{Name: "Further clarification", Command: "codexspec:clarify"},
		{Name: "Generate spec.md", Command: "codexspec:generate-spec"},
		{Name: "Review spec", Command: "codexspec:review-spec", Review: true},
		{Name: "Generate plan.md", Command: "codexspec:spec-to-plan"},
		{Name: "Review plan", Command: "codexspec:review-plan", Review: true},
		{Name: "Generate task breakdown", Command: "codexspec:plan-to-tasks"},
		{Name: "Review tasks", Command: "codexspec:review-tasks", Review: true},
		{Name: "TDD implementation", Command: "codexspec:implement-tasks"},
		{Name: "Code review", Command: "codexspec:review-code", Review: true},
		{Name: "Commit", Command: "codexspec:commit-staged"},
		{Name: "Push and create PR", Command: "codexspec:pr", Remote: true},
	}
}
