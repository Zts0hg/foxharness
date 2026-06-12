// Package autodev implements the continuous-development driver that drains a
// backlog of requirements by running the CodexSpec SDD pipeline for each item
// inside an isolated git branch and worktree.
//
// The package is built around a two-plane architecture:
//
//   - The control plane is deterministic Go. It owns flow control only:
//     backlog parsing and selection, worktree lifecycle, driving the ordered
//     sequence of SDD and remote steps, read-only ground-truth verification
//     that each step truly completed, the durable state ledger, and cleanup.
//     The LLM can never cause a step to advance, be skipped, or end early.
//
//   - The execution plane is the LLM. The core Agent (the existing engine
//     loop, scoped to the item's worktree) performs all development work and
//     all repo mutations — implement, stage, commit, push, and issue/PR
//     creation — while an engineer Agent (an LLM in a senior-engineer
//     persona) supervises each run, answering its questions and steering
//     corrections when verification finds a gap.
//
// Dependency rule: internal/autodev depends only on abstractions declared in
// ports.go plus internal/engine, internal/tools, and internal/provider. It
// MUST NOT import internal/app or internal/tui; those packages provide the
// concrete adapters and entry points and import autodev (never the reverse),
// keeping the dependency graph cycle-free.
package autodev
