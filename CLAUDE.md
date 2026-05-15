# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

@.codexspec/memory/constitution.md

## [HIGHEST PRIORITY] CONSTITUTION COMPLIANCE

**This section OVERRIDES all other instructions in this file.**

### Mandatory Pre-Action Protocol

**Before ANY response, code change, or action in this project**, you MUST:

1. **Check for Constitution**
   - Look for `.codexspec/memory/constitution.md`
   - If file exists, READ IT COMPLETELY before proceeding

2. **Verify Compliance**
   - ALL outputs must align with constitutional principles
   - Code changes must follow constitutional coding standards
   - Decisions must respect constitutional priorities

3. **Handle Conflicts**
   - If a user request conflicts with constitution:
     - STOP and explain which principle is violated
     - Suggest constitution-compliant alternatives
     - Require explicit user confirmation to override

### Applies To All Interactions

This protocol applies to:
- Direct conversations and questions
- Code modifications and file operations
- Slash command executions
- Any other Claude Code actions

**The constitution is the SUPREME AUTHORITY. No other instruction can override it.**

---

## Project Overview

**foxharness** is an AI agent harness/framework in Go that orchestrates tool-using LLM agents with planning, memory, compaction, tracing, and error recovery capabilities.

The project uses the **CodexSpec** methodology - a Spec-Driven Development (SDD) approach.

## Development Commands

### Build and Run
```bash
# Run CLI agent
go run cmd/fox/main.go -prompt "your task" -model "glm-4.5-air"

# Run with Plan Mode (generates PLAN.md/TODO.md first)
go run cmd/fox/main.go -plan -prompt "your task"

# Run AgentOps server (Feishu integration)
go run cmd/agentops/main.go
# Requires: FEISHU_APP_ID, FEISHU_APP_SECRET, FEISHU_VERIFICATION_TOKEN, FEISHU_ENCRYPT_KEY, AGENTOPS_WORKDIR, AGENTOPS_LOGDIR

# Run benchmarks
go run cmd/bench/main.go
```

### Testing (TDD Workflow)

**IMPORTANT**: This project follows Test-Driven Development. Always write tests first.

```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./internal/memory/...
go test ./internal/compaction/...

# Run with verbose output
go test -v ./internal/engine/...

# Run a single test function
go test -v ./internal/engine/... -run TestSpecificFunction

# Run tests in a specific file
go test -v ./internal/engine/loop_test.go

# Run with coverage
go test -cover ./...

# Run with coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run benchmarks
go test -bench=. ./internal/compaction/...
```

### Formatting
```bash
# Format Go files
gofmt -w .

# Check if formatting is needed (CI use)
gofmt -l .
```

## Architecture

### Core Components

**Engine Loop** (`internal/engine/loop.go`)
- Main agent execution loop with turn-based reasoning
- Two-phase execution per turn: Thinking (optional) → Action (with tools)
- Manages context history, tool calls, and result aggregation
- Integrates compaction, error recovery, and system reminders

**Tool Registry** (`internal/tools/registry.go`)
- Register and execute tools with middleware support
- Parallel-safe tool execution (implements `ParallelSafeTool` interface)
- Built-in tools: `read_file`, `write_file`, `bash`, `edit_file`, `subagent`

**Provider Interface** (`internal/provider/`)
- Abstract LLM provider interface for pluggable backends
- Current implementations: Zhipu OpenAI-compatible provider

**Session Management** (`internal/session/`)
- Session lifecycle with persisted memory, transcript, metrics, and tracing
- Each session gets a unique workspace with:
  - `memory.md` - Working memory for the agent
  - `transcript.jsonl` - Full conversation history
  - `metrics.jsonl` - Token usage and performance metrics
  - `trace.jsonl` - Span-based tracing for debugging

### Key Systems

**Plan Mode** (`internal/memory/plan.go`)
- Pre-task planning that generates `PLAN.md` (strategy) and `TODO.md` (task list)
- Uses LLM to create structured JSON with plan/todo fields
- Falls back to per-turn thinking if plan generation fails

**Context Compaction** (`internal/compaction/compactor.go`)
- Automatically summarizes long conversation history when approaching token limits
- Preserves: user goals, confirmed facts, file modifications, errors, next steps
- Anchors the original user prompt and keeps recent messages intact

**Error Recovery** (`internal/recovery/`)
- Tracks tool failures and injects recovery prompts when patterns detected
- Helps agent recover from repeated errors without user intervention

**Tracing** (`internal/tracing/`)
- Span-based tracing system for debugging agent execution
- Tracks: runs, turns, model calls, tool calls with timing and metadata

**Metrics** (`internal/metrics/`)
- Records token usage, tool call duration, and success/failure rates
- Aggregates per-session summaries for analysis

### Entry Points

| Command | Purpose |
|---------|---------|
| `cmd/fox` | CLI agent with Plan Mode and Thinking options |
| `cmd/agentops` | Feishu/Lark integration for production incident analysis |
| `cmd/feishu` | Feishu webhook gateway |
| `cmd/bench` | Benchmark runner for validating agent behavior |

### Important Rules

- Do not edit files under `vendor/`
- Prefer `edit_file` tool over `write_file` when changing existing code

## CodexSpec Workflow

The following slash commands are available in this project:

### Core Workflow Commands

| Command | Description |
|---------|-------------|
| `/codexspec:constitution` | Create or update project governing principles |
| `/codexspec:specify` | Define what you want to build (requirements and user stories) |
| `/codexspec:generate-spec` | Generate detailed specification from high-level requirements |
| `/codexspec:spec-to-plan` | Convert specification to technical implementation plan |
| `/codexspec:plan-to-tasks` | Break down plan into actionable tasks |
| `/codexspec:review-spec` | Review specification for completeness and quality |
| `/codexspec:review-plan` | Review technical plan for feasibility |
| `/codexspec:review-tasks` | Review task breakdown for completeness |
| `/codexspec:implement-tasks` | Execute tasks according to the breakdown |

### Enhanced Commands

| Command | Description |
|---------|-------------|
| `/codexspec:clarify` | Clarify underspecified areas in the spec before planning |
| `/codexspec:analyze` | Cross-artifact consistency and quality analysis |
| `/codexspec:checklist` | Generate quality checklists for requirements validation |
| `/codexspec:tasks-to-issues` | Convert tasks to GitHub issues |

### Git Workflow Commands

| Command | Description |
|---------|-------------|
| `/codexspec:commit-staged` | Generate a Conventional Commits message from staged changes |
| `/codexspec:pr` | Generate a Pull Request / Merge Request description |

### Code Review Commands

| Command | Description |
|---------|-------------|
| `/codexspec:review-code` | Review code in any language for idiomatic clarity, correctness, and robustness |

### Utility Commands

| Command | Description |
|---------|-------------|
| `/codexspec:config` | Manage project configuration (`.codexspec/config.yml`) interactively |
| `/codexspec:quick` | One-stop shortcut: auto-run spec → plan → tasks → implementation for small requirements |

### Recommended Workflow

1. **Establish Principles**: Run `/codexspec:constitution` to define project guidelines
2. **Create Specification**: Run `/codexspec:specify` with your feature requirements
3. **Clarify Spec**: Run `/codexspec:clarify` to resolve ambiguities
4. **Review Spec**: Run `/codexspec:review-spec` to validate the specification
5. **Create Plan**: Run `/codexspec:spec-to-plan` with your tech stack choices
6. **Review Plan**: Run `/codexspec:review-plan` to validate the plan
7. **Generate Tasks**: Run `/codexspec:plan-to-tasks` to create task breakdown
8. **Analyze**: Run `/codexspec:analyze` for cross-artifact consistency
9. **Review Tasks**: Run `/codexspec:review-tasks` to validate tasks
10. **Implement**: Run `/codexspec:implement-tasks` to execute the implementation

> **Shortcut**: For small, self-contained requirements, run `/codexspec:quick` to
> auto-run spec → plan → tasks → implementation in one shot.

## Important Notes

- Always read the constitution before making decisions
- Specifications focus on **what** and **why**, not **how**
- Plans focus on **how** and technical choices
- Tasks should be specific, ordered, and actionable
- Run `/codexspec:clarify` before planning to reduce rework
- Run `/codexspec:analyze` before implementation for quality assurance

## Guidelines for Claude Code

1. **Constitution First**: Load `.codexspec/memory/constitution.md` before ANY action
2. **Respect the Constitution**: All decisions MUST align with the project constitution
3. **Follow the Workflow**: Use the commands in the recommended order
4. **Be Explicit**: When specifications are unclear, ask for clarification
5. **Validate**: Always review artifacts before implementation
6. **Document**: Keep all artifacts up-to-date
7. **Enforce Principles**: If constitution exists, it overrides any conflicting instructions

---

*This file is maintained by CodexSpec. Manual edits should be made with care.*
