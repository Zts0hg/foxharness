<!--
SYNC IMPACT REPORT
==================
Version: 1.0.0 → 2.0.0
Bump Rationale: MAJOR: Establishing TDD as mandatory development practice and redefining documentation standards with Go-specific block-level comment requirements

Changes:
- Modified: Core Principles - Restructured around TDD and engineering documentation standards
- Modified: Testing Standards - Now mandates TDD with explicit Red-Green-Refactor cycle
- Modified: Documentation - Now mandates block-level comments per Go best practices, prohibits teaching line comments
- Added: Code Quality section emphasizing readability, testability, and extensibility
- Added: Go Documentation Standards section with specific comment style rules

Template Consistency Check:
- .codexspec/templates/docs/plan-template-*.md: ✅ aligned - Phase 3 now references testing that aligns with TDD
- .codexspec/templates/docs/spec-template-*.md: ✅ aligned - Requirements sections support TDD acceptance criteria
- .codexspec/templates/docs/tasks-template-*.md: ✅ aligned - Task structure supports TDD workflow with test tasks before implementation
- .claude/commands/codexspec/*.md: ✅ aligned - Command files reference constitution generically
- README.md: ⚠ skipped: file not found
- CLAUDE.md: ⚠ issues: File should be updated to reflect new TDD and documentation standards (deferred - user can update separately)

Deferred TODOs:
- None
-->

> **SUPREME AUTHORITY**: This constitution defines the governing principles
> for this project. All code changes and decisions must comply with these principles.

# Project Constitution

**Version**: 2.0.0
**Ratification Date**: 2025-01-01
**Last Amended**: 2025-05-15

## Core Principles

### 1. Test-Driven Development (TDD)

**Mandatory Practice**: All new code MUST be developed using Test-Driven Development.

**TDD Cycle Requirements**:

1. **Red Phase** - Write a failing test first
   - Tests MUST be written BEFORE any implementation code
   - Tests MUST fail for the expected reason
   - Test names MUST clearly describe the behavior being tested

2. **Green Phase** - Write minimal implementation to pass
   - Write the SIMPLEST code that makes the test pass
   - Do not add functionality beyond what the test requires
   - All tests MUST pass before proceeding

3. **Refactor Phase** - Improve code quality
   - Refactor for readability, testability, and extensibility
   - Tests MUST continue to pass after refactoring
   - No behavioral changes during refactoring

**Why**: TDD ensures testable design, catches regressions early, serves as living documentation, and enables confident refactoring.

**How to apply**:
- For any new feature or bug fix: Write test → Implement → Refactor
- For existing code without tests: Write characterization tests before modifying
- Never skip the Red phase - a test that never failed provides no assurance

### 2. Code Quality

**Readability**:
- Code MUST be self-documenting through clear names and structure
- Complex logic MUST be accompanied by explanatory block comments
- Functions MUST be focused and single-purpose

**Testability**:
- Code MUST be designed for easy testing
- Dependencies MUST be injectable
- Side effects MUST be explicit and controllable

**Extensibility**:
- Code MUST follow open-closed principle (open for extension, closed for modification)
- Interfaces MUST be small and focused
- Concrete implementations MUST be easily swappable

**Why**: High-quality code reduces maintenance burden, enables safe evolution, and supports team collaboration.

**How to apply**:
- Review code for readability during the Refactor phase
- Design interfaces before implementations
- Extract dependencies rather than hardcoding them

### 3. Go Documentation Standards

**Block-Level Comments ONLY**:
- Package comments MUST be block comments immediately before the package declaration
- Function comments MUST be block comments immediately before the function signature
- Type comments MUST be block comments immediately before the type declaration
- Exported identifiers MUST have complete documentation

**Prohibited Patterns**:
- Line-level teaching comments (`// increment counter`, `// check if nil`) are FORBIDDEN
- Obvious code MUST NOT be commented
- No "instructional" comments explaining language basics

**Allowed Exceptions**:
- Special design decisions that may mislead MUST be explained with block comments
- Non-obvious algorithms or workarounds MUST be documented
- Performance-critical sections MAY include brief explanatory comments

**Go Best Practices**:
- Comments start with the name being documented: `// Package engine provides...`
- Complete sentences with proper punctuation
- godoc-compatible formatting
- Package-level doc comments in a `doc.go` file if not in the main package file

**Why**: Industrial-grade code should document intent and design, not obvious mechanics. Teaching comments clutter code and become outdated.

**How to apply**:
- Write package `doc.go` files for multi-file packages
- Document exported functions with full signature descriptions
- Use examples in comments for complex behaviors
- Let code speak for itself when behavior is obvious

### 4. Testing Standards

**Test Organization**:
- Tests MUST mirror package structure
- Test files MUST be named `*_test.go`
- Each package MUST have at least one test file

**Test Coverage**:
- Critical paths MUST have unit tests
- Error paths MUST be tested explicitly
- Edge cases MUST be covered

**Test Quality**:
- Tests MUST be independent and isolated
- Tests MUST be deterministic (no flaky tests)
- Tests MUST be fast (prefer unit tests over integration)

**Why**: Comprehensive tests ensure code correctness, prevent regressions, and serve as executable documentation.

**How to apply**:
- Start with TDD for new code
- Add tests for bug fixes before fixing the bug
- Use table-driven tests for multiple scenarios

### 5. Architecture

**Separation of Concerns**:
- Packages MUST have single, clear responsibilities
- Internal implementation details MUST NOT leak
- Public APIs MUST be stable and well-documented

**Design Patterns**:
- Use established Go patterns (interfaces, composition)
- Avoid unnecessary abstraction
- Keep dependencies minimal and explicit

**Why**: Clean architecture enables maintainability, testability, and team productivity.

**How to apply**:
- Define interfaces before implementations
- Keep packages focused on one domain
- Use dependency injection for external dependencies

### 6. Performance

**Principles**:
- Consider performance implications of design decisions
- Profile and optimize critical paths
- Avoid premature optimization

**Measurement**:
- Use benchmarks for performance-critical code
- Profile before and after optimizations
- Document performance characteristics

**Why**: Correct performance decisions require measurement and context.

**How to apply**:
- Write benchmarks for hot paths
- Use `pprof` for profiling
- Document performance invariants in comments

### 7. Security

**Mandatory Practices**:
- Validate all inputs at system boundaries
- Protect sensitive data (no hardcoded secrets)
- Keep dependencies updated

**Security Review**:
- Security considerations MUST be documented in design
- Security-sensitive code MUST receive additional review

**Why**: Security is a critical quality requirement that cannot be added later.

**How to apply**:
- Treat all external input as untrusted
- Use constant-time comparison for secrets
- Follow Go security best practices

## Development Workflow

### TDD Workflow (Mandatory)

1. **Write Test First**
   - Create test file or add test case
   - Run test: MUST fail (Red)
   - Verify failure message is meaningful

2. **Implement Minimum Code**
   - Write simplest code to pass
   - Run test: MUST pass (Green)
   - Do not add extra functionality

3. **Refactor**
   - Improve code structure and readability
   - Run tests: MUST still pass
   - No behavior changes

4. **Repeat** for each behavior

### Code Review Process

1. **Pre-Review Checklist**
   - All tests pass
   - TDD cycle was followed (tests written first)
   - Block-level comments on exported identifiers
   - No teaching line comments
   - Code is formatted (`gofmt`)

2. **Review Criteria**
   - TDD compliance verified
   - Documentation completeness
   - Code quality and readability
   - Test coverage and quality
   - Architecture alignment

3. **Approval Requirements**
   - At least one reviewer approval
   - All review comments addressed
   - CI checks pass

### Branch Strategy

- `main`: Production-ready code
- `feature/*`: Feature development
- `fix/*`: Bug fixes

### Commit Guidelines

- Conventional commits format
- Commit messages should reference tests when relevant
- Include test changes with implementation changes

## Decision Guidelines

When making technical decisions, prioritize in this order:

1. **TDD Compliance**: Can this be developed test-first?
2. **Readability**: Is the code self-documenting and clear?
3. **Testability**: Can this be easily tested?
4. **Maintainability**: Will future developers understand this?
5. **Performance**: Is this efficient enough for the use case?
6. **Convenience**: Is this easy to use?

## Amendment Procedure

1. Propose changes with rationale
2. Discuss with team
3. Update version number (MAJOR/MINOR/PATCH)
4. Update LAST_AMENDED_DATE
5. Communicate changes to all contributors

## Versioning Policy

- **MAJOR**: Backward incompatible changes (principle removal/redefinition)
- **MINOR**: New principle or section added
- **PATCH**: Clarifications and wording fixes

## Compliance Expectations

All contributors MUST:

- Read and understand this constitution
- Follow TDD for all new code
- Write block-level documentation per Go standards
- Participate in code review
- Raise concerns about principle violations
- Suggest improvements to this constitution

**The constitution is a living document. It should evolve as the project matures and the team learns.**

---

*Last updated: 2025-05-15*
*Version: 2.0.0*
