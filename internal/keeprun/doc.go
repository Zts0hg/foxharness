// Package keeprun provides the data types, parsers, and algorithms that back
// the /keep-run autonomous Spec-Driven Development (SDD) pipeline runner.
//
// The /keep-run command processes a prioritized backlog of development tasks,
// driving each one through the full SDD lifecycle in an isolated git worktree
// and exiting only when every task is done. The runtime driver is the
// .claude/commands/codexspec/keep-run.md prompt command; this package supplies
// the testable building blocks that define the expected formats and algorithms:
//
//   - backlog parsing and status updates for BACKLOG.md (see backlog.go)
//   - phase-level resume state tracking via .keep-run-state.json (see backlog.go)
//   - keep-run.config.json loading with sensible defaults (see config.go)
//   - deterministic, filesystem-safe slug generation from task titles (see slug.go)
//   - the ordered definition of the twelve SDD pipeline phases (see phase.go)
//   - git worktree lifecycle management for task isolation (see worktree.go)
package keeprun
