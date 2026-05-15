# Feature: Comprehensive Code Documentation

## Overview

Add comprehensive, accurate documentation to all Go source files in the foxharness project. The project currently has minimal or missing documentation on exported packages, types, functions, and methods. This initiative ensures the codebase is self-documenting and follows Go documentation best practices as defined in the project constitution (v2.0.0).

## Goals

- Ensure every exported package has a complete doc comment
- Ensure every exported type, function, method, and constant has a complete doc comment
- Follow Go documentation standards (block comments, godoc-compatible)
- Document intent and design, not obvious mechanics
- Remove any existing teaching-style line comments
- Maintain alignment with the project constitution's documentation principles

## User Stories

### Story 1: Package Documentation

**As a** developer new to the foxharness project
**I want** each package to have clear documentation explaining its purpose
**So that** I can quickly understand what each module does and how to use it

**Acceptance Criteria:**
- [ ] Every package has a doc comment (either in doc.go or before package declaration)
- [ ] Package docs explain the package's purpose and key concepts
- [ ] Package docs follow godoc conventions (start with "Package {name}")

### Story 2: Exported Identifier Documentation

**As a** developer integrating with foxharness
**I want** every exported type, function, and method to be documented
**So that** I can understand the API without reading implementation code

**Acceptance Criteria:**
- [ ] All exported types have doc comments
- [ ] All exported functions have doc comments with parameter descriptions
- [ ] All exported methods have doc comments with receiver and parameter descriptions
- [ ] All exported constants and variables have doc comments

### Story 3: Remove Teaching Comments

**As a** developer maintaining the codebase
**I want** code to be self-documenting through clear names
**So that** teaching comments don't clutter the code and become outdated

**Acceptance Criteria:**
- [ ] No line-level teaching comments (e.g., "// increment counter")
- [ ] Complex algorithms have explanatory block comments
- [ ] Non-obvious design decisions are documented

## Functional Requirements

- [REQ-001] Documentation MUST use block comment format (/* */ or // lines forming a block)
- [REQ-002] Package documentation MUST be placed in doc.go for multi-file packages or before the package declaration
- [REQ-003] Exported identifiers MUST have complete documentation
- [REQ-004] Documentation MUST describe what and why, not how (for obvious code)
- [REQ-005] Function documentation MUST include parameter and return value descriptions
- [REQ-006] Method documentation MUST describe the receiver's role
- [REQ-007] Non-obvious algorithms MUST have explanatory comments
- [REQ-008] All documentation MUST be godoc-compatible

## Non-Functional Requirements

- [NFR-001] Documentation MUST be written in clear, concise English
- [NFR-002] Code changes MUST NOT alter behavior (documentation-only changes)
- [NFR-003] All existing tests MUST continue to pass
- [NFR-004] Code MUST remain formatted with gofmt

## Affected Modules

The following modules and packages require documentation:

### Entry Points (cmd/)
- `cmd/agentops` - Feishu integration server
- `cmd/bench` - Benchmark runner
- `cmd/feishu` - Feishu webhook gateway
- `cmd/fox` - Main CLI agent

### Core Engine (internal/engine/)
- `loop.go` - Main agent execution loop
- `config.go` - Engine configuration

### Tools (internal/tools/)
- `registry.go` - Tool registration and execution
- `bash.go` - Bash command execution
- `read_file.go` - File reading tool
- `write_file.go` - File writing tool
- `edit_file.go` - File editing tool

### Provider (internal/provider/)
- `interface.go` - LLM provider abstraction
- `openai.go` - OpenAI-compatible provider

### Session Management (internal/session/)
- `session.go` - Session lifecycle
- `memory.go` - Session memory
- `transcript.go` - Conversation transcript

### Context Compaction (internal/compaction/)
- `compactor.go` - History summarization

### Memory & Planning (internal/memory/)
- `plan.go` - Plan mode for pre-task planning
- `store.go` - Memory storage

### Metrics (internal/metrics/)
- `recorder.go` - Performance metrics
- `summary.go` - Metric aggregation
- `token.go` - Token usage tracking

### Tracing (internal/tracing/)
- `tracer.go` - Span-based tracing
- `reader.go` - Trace reading

### Error Recovery (internal/recovery/)
- `error_tracker.go` - Tool failure tracking

### AgentOps (internal/agentops/)
- `runner.go` - Incident analysis runner
- `task.go` - Task handling
- `log_search.go` - Log search functionality
- `prompt.go` - Analysis prompts

### Feishu Integration (internal/feishu/)
- `gateway.go` - Webhook gateway
- `messenger.go` - Message handling
- `runner.go` - Feishu runner
- `task.go` - Feishu tasks

### Supporting Modules
- `internal/approval/` - Approval workflows
- `internal/app/` - CLI application
- `internal/middleware/` - Tool middleware
- `internal/reminder/` - System reminders
- `internal/schema/` - Message schemas
- `internal/subagent/` - Subagent management
- `internal/context/` - Prompt context
- `benchmarks/fixtures/counter_race/counter/` - Benchmark fixtures

## Acceptance Criteria (Test Cases)

- [TC-001] Running `go doc ./...` produces output for all packages
- [TC-002] Running `godoc` shows complete documentation for all exported symbols
- [TC-003] No line-level teaching comments exist (verified by code review)
- [TC-004] All exported identifiers have doc comments (verified by go/vet or golint)
- [TC-005] All tests pass: `go test ./...`
- [TC-006] Code is formatted: `gofmt -l .` returns no files
- [TC-007] Package doc comments exist for all packages

## Edge Cases

- **Interface-only packages**: Some packages may only contain interface definitions (e.g., `internal/provider/interface.go`). These need package documentation explaining the abstraction's purpose.
- **Test files**: Test files should have comments explaining complex test scenarios, but basic test helpers may not need documentation.
- **Internal packages**: Even though packages are `internal`, they still need documentation for project maintainers.
- **Benchmark fixtures**: Minimal documentation acceptable for simple fixtures.
- **Generated code**: Skip any auto-generated files.

## Output Examples

### Package Documentation Example
```go
// Package engine provides the core agent execution loop for foxharness.
//
// The engine orchestrates tool-using LLM agents through a turn-based reasoning
// process. Each turn consists of an optional Thinking phase followed by an
// Action phase where tools may be invoked.
//
// Key Components:
//   - Loop: Main execution loop managing turns and context
//   - Config: Engine configuration for providers, tools, and behavior
package engine
```

### Function Documentation Example
```go
// Run executes the agent loop with the given configuration.
//
// It processes the user prompt through a series of turns, each consisting
// of optional thinking followed by tool-enabled action execution. The loop
// continues until the agent signals completion or an error occurs.
//
// The ctx parameter provides cancellation support. The cfg parameter
// contains the engine configuration including provider, tools, and initial
// prompt. Returns the final response and any error encountered during
// execution.
func (l *Loop) Run(ctx context.Context, cfg *Config) (string, error) {
```

### Type Documentation Example
```go
// Loop represents the main agent execution loop.
//
// It manages the turn-based reasoning process, context history,
// tool execution, and result aggregation. The Loop integrates
// with compaction, error recovery, and system reminders.
type Loop struct {
    // provider is the LLM provider for generating responses
    provider Provider
    // tools is the registry of available tools
    tools *Registry
    // ...
}
```

## Out of Scope

- Refactoring code structure (only documentation changes)
- Adding new features or functionality
- Modifying test logic (tests must remain passing)
- Creating tutorial or guide documentation (focus is code documentation)
- Documentation for vendor/ or third-party dependencies
- README or user guide updates (only Go godoc comments)

## Constitution Compliance

This specification aligns with the project constitution (v2.0.0):

- **Principle 3 (Go Documentation Standards)**: Block-level comments only, no teaching comments
- **Principle 2 (Code Quality)**: Self-documenting through clear names
- **Principle 5 (Architecture)**: Public APIs stable and well-documented
