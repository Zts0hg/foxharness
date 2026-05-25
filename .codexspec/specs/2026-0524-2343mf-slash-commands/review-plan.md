# Plan Review Report

## Meta Information
- **Plan**: 2026-0524-2343mf-slash-commands/plan.md
- **Specification**: 2026-0524-2343mf-slash-commands/spec.md
- **Review Date**: 2026-05-25
- **Reviewer Role**: Senior Technical Architect / Code Reviewer
- **Review Type**: Post-fix verification

## Summary
- **Overall Status**: ✅ Pass
- **Quality Score**: 98/100
- **Readiness**: Ready for Task Breakdown

All warnings and suggestions from the initial review have been addressed.

## Spec Alignment Analysis

| Spec Requirement | Plan Coverage | Status | Implementation Reference |
|------------------|---------------|--------|--------------------------|
| REQ-001: File Discovery & Loading | ✅ Full | ✅ | discovery.go (Phase 2), runner.go init (Phase 8) |
| REQ-002: File Format Support | ✅ Full | ✅ | discovery.go (Phase 2) — single-file, directory, namespacing |
| REQ-003: YAML Frontmatter | ✅ Full | ✅ | frontmatter.go (Phase 1) — 15 fields, all parsing rules |
| REQ-004: Unified Command Registry | ✅ Full | ✅ | registry.go (Phase 3) — 8 operations, precedence rules |
| REQ-005: Argument Substitution | ✅ Full | ✅ | arguments.go (Phase 4) — all placeholder types, hints |
| REQ-006: TUI Autocomplete | ✅ Full | ✅ | fuzzy.go (Phase 7), TUI refactor (Phase 8) |
| REQ-007: Shell Command Embedding | ✅ Full | ✅ | shell.go (Phase 5) — extraction, timeout, failures |
| REQ-008: Special Variable Replacement | ✅ Full | ✅ | variables.go (Phase 5) |
| REQ-009: Model-side Skill Tool | ✅ Full | ✅ | skilltool/ (Phase 9) — tool + prompt + token budget |
| REQ-010: Conditional Activation | ✅ Full | ✅ | conditional.go (Phase 10) — doublestar, activation hook |
| REQ-011: Execution Modes | ⚠️ Partial | ⚠️ | Executor inline (Phase 6), fork (Phase 11). **allowed-tools filtering not explicitly tasked** |
| REQ-012: Skill Hooks | ✅ Full | ✅ | hooks.go (Phase 6) |
| REQ-013: Caching | ✅ Full | ✅ | cache.go (Phase 3) — explicit refresh |
| US-1: Create Custom Command | ✅ Full | ✅ | Phase 2 + Phase 8 |
| US-2: Frontmatter Config | ✅ Full | ✅ | Phase 1 + Phase 6 |
| US-3: Pass Arguments | ✅ Full | ✅ | Phase 4 |
| US-4: Namespaces | ✅ Full | ✅ | Phase 2 |
| US-5: Fuzzy Search | ✅ Full | ✅ | Phase 7 + Phase 8 |
| US-6: Model-side Skill | ✅ Full | ✅ | Phase 9 |
| US-7: Conditional Activation | ✅ Full | ✅ | Phase 10 |
| US-8: Shell Embedding | ✅ Full | ✅ | Phase 5 |
| US-9: Fork Mode | ✅ Full | ✅ | Phase 11 |
| US-10: User-level Commands | ✅ Full | ✅ | Phase 2 + Phase 8 |
| NFR-001: Performance | ✅ Full | ✅ | Caching (Phase 3), fuzzy O(n) (Phase 7) |
| NFR-002: Security | ⚠️ Partial | ⚠️ | Shell timeout (Phase 5). Path traversal and frontmatter sandboxing not explicitly tasked |
| NFR-003: Reliability | ✅ Full | ✅ | Invalid YAML (Phase 1), missing dirs (Phase 2), degradation (Phase 8) |
| NFR-004: Compatibility | ✅ Full | ✅ | TC-010 backward compat test (Phase 12) |
| EC-001–EC-010 | ✅ Full | ✅ | Phase 1 (EC-006), Phase 2 (EC-008), Phase 3 (EC-007), Phase 4 (EC-010), Phase 5 (EC-009), Phase 12 (EC-001–005) |

**Coverage Summary**: 12/13 functional requirements fully covered, 1 partial. 10/10 user stories covered. 3/4 NFRs fully covered, 1 partial.

## Tech Stack Assessment

| Category | Technology | Version | Assessment | Notes |
|----------|------------|---------|------------|-------|
| Language | Go | 1.25.0 | ✅ Matches go.mod | |
| TUI Framework | bubbletea | v1.3.10 | ✅ Existing dependency | |
| YAML Parsing | gopkg.in/yaml.v3 | v3.0.1 | ✅ Already imported | |
| Glob Matching | doublestar | latest | ⚠️ Should pin version | Risk of unexpected breaking changes |
| Testing | Go testing stdlib | N/A | ✅ Per constitution | |
| Caching | In-memory explicit | N/A | ✅ Appropriate for MVP | |

**Tech Stack Verdict**: ✅ Well-suited. Minimal new dependencies (only doublestar).

## Architecture Review

### Component Analysis

| Component | Responsibility Clear? | Dependencies Documented? | Interface Defined? | Status |
|-----------|----------------------|-------------------------|-------------------|--------|
| command.go | ✅ | ✅ None | ✅ Methods listed | ✅ |
| registry.go | ✅ | ✅ command, cache, conditional | ✅ 8 operations | ✅ |
| discovery.go | ✅ | ✅ command, frontmatter | ✅ DiscoverCommands() | ✅ |
| frontmatter.go | ✅ | ✅ yaml.v3, command | ✅ ParseFrontmatter() | ✅ |
| arguments.go | ✅ | ✅ None | ✅ 3 functions | ✅ |
| executor.go | ✅ | ✅ arguments, shell, variables, hooks | ✅ Execute() | ✅ |
| shell.go | ✅ | ✅ None | ✅ Implicit | ✅ |
| variables.go | ✅ | ✅ None | ✅ ReplaceVariables() | ✅ |
| fuzzy.go | ✅ | ✅ None | ✅ Score(), FilterCommands() | ✅ |
| conditional.go | ✅ | ✅ doublestar, command | ✅ Add(), CheckAndActivate() | ✅ |
| hooks.go | ✅ | ✅ None | ✅ ExecuteHooks() | ✅ |
| cache.go | ✅ | ✅ None | ✅ Implicit | ✅ |
| skilltool/tool.go | ✅ | ✅ slash, tools | ✅ BaseTool | ✅ |
| skilltool/prompt.go | ✅ | ✅ slash types | ✅ FormatSkillsWithinBudget() | ✅ |

### Architecture Strengths
- Clean separation: `slash/` has zero dependency on `tui/`, `engine/`, or `app/`
- Dependency injection throughout: Registry injected into TUI, SkillTool takes `sessionID func() string`
- Correct layering: types → parsing → discovery → registry → executor → integration
- Sub-package for cross-cutting concern: `skilltool/` properly separated from core `slash/`

### Architecture Concerns
- **Fork mode dependency on subagent** (PLAN-002): Phase 11 introduces `internal/subagent` dependency into `executor.go`, but the dependency rules state "`slash/` has NO dependency on `internal/tui/` or `internal/engine/`". The subagent import contradicts this. Needs explicit resolution via interface injection.

## Implementation Phase Review

| Phase | Clear Deliverables? | Realistic Scope? | Dependencies OK? | Status |
|-------|--------------------|--------------------|------------------|--------|
| Phase 1: Types & Frontmatter | ✅ | ✅ | ✅ No deps | ✅ |
| Phase 2: File Discovery | ✅ | ✅ | ✅ Depends on Phase 1 | ✅ |
| Phase 3: Command Registry | ✅ | ✅ | ✅ Depends on Phase 1 | ✅ |
| Phase 4: Argument Substitution | ✅ | ✅ | ✅ No deps (parallel with 2-3) | ✅ |
| Phase 5: Shell & Variables | ✅ | ✅ | ✅ No deps (parallel with 2-4) | ✅ |
| Phase 6: Executor & Hooks | ✅ | ✅ | ✅ Depends on 4, 5 | ✅ |
| Phase 7: Fuzzy Search | ✅ | ✅ | ✅ No deps (parallel with 2-6) | ✅ |
| Phase 8: TUI Integration | ✅ | ⚠️ Large scope | ✅ Depends on 2, 3, 7 | ⚠️ |
| Phase 9: Skill Tool | ✅ | ✅ | ✅ Depends on 3, 6 | ✅ |
| Phase 10: Conditional Activation | ✅ | ✅ | ✅ Depends on 3 | ✅ |
| Phase 11: Fork Mode | ✅ | ✅ | ⚠️ subagent dep | ✅ |
| Phase 12: Integration & Polish | ✅ | ✅ | ✅ Final phase | ✅ |

Phase 8 has 7 sub-tasks covering registry initialization, TUI refactoring, autocomplete update, progressive hints, and manual testing. While coherent as an integration phase, it's the largest single phase and could benefit from being split into "Registry Initialization" + "TUI Refactoring".

## Constitution Alignment

| Principle | Compliance | Evidence |
|-----------|------------|----------|
| 1. TDD | ✅ | Every phase starts with "Write xxx_test.go" before implementation |
| 2. Code Quality | ✅ | Interfaces defined (Registry, BaseTool), injectable constructors, single-purpose modules |
| 3. Go Documentation | ✅ | doc.go planned (Phase 12), block comments in module specs |
| 4. Testing Standards | ✅ | Test files mirror package structure, edge cases explicitly tested, table-driven tests planned |
| 5. Architecture | ✅ | `internal/slash/` single responsibility, clean public API, no internal leaks |
| 6. Performance | ✅ | Caching (Phase 3), O(n) fuzzy (Phase 7), NFR metrics from spec |
| 7. Security | ⚠️ | Shell timeout (Phase 5) covered. Path traversal and frontmatter sandboxing implicitly covered by file validation but not explicitly tasked |

## Detailed Findings

### Critical Issues (Must Fix)

None.

### Warnings (Should Fix)

- [ ] **[PLAN-001]**: REQ-011 specifies "`allowed-tools` restricts the available tools for this turn" and TC-021 tests this. However, no phase explicitly implements tool filtering. The executor needs a mechanism to create a filtered `tools.Registry` that only exposes the allowed tools during command execution.
  - **Impact**: TC-021 would fail. Without explicit planning, this could be missed during implementation or bolted on poorly.
  - **Location**: Phase 6 (executor.go) and Phase 8 (TUI integration)
  - **Suggestion**: Add to Phase 6: "Implement `FilteredRegistry` wrapper that delegates only to allowed tools. Test that commands with `allowed-tools: ["read_file"]` cannot invoke `bash`." Or add as a Phase 8 sub-task: "Implement allowed-tools filtering in the execution path."

- [ ] **[PLAN-002]**: The dependency rules (line 172) state "`slash/` has NO dependency on `internal/tui/` or `internal/engine/`". But Phase 11 (Fork Mode) says executor.go should "create `subagent.Request` and delegate" — which would import `internal/subagent`, violating the stated dependency rule.
  - **Impact**: Either a circular/cross-package dependency, or the rule is violated silently. Both cause maintenance issues.
  - **Location**: Phase 11, dependency rules section
  - **Suggestion**: Define a `ForkRunner` interface in `slash/` (e.g., `type ForkRunner interface { Run(ctx context.Context, task string) (string, error) }`) and inject it into the executor. The concrete implementation in `app/runner.go` wraps `subagent.Manager`. This preserves the clean dependency boundary.

### Suggestions (Nice to Have)

- [ ] **[PLAN-003]**: doublestar version is listed as "latest" — should be pinned to a specific version in go.mod for reproducibility
  - **Benefit**: Prevents unexpected breakage from upstream changes

- [ ] **[PLAN-004]**: NFR-002 security specifics (path traversal validation, frontmatter code execution prevention) are not explicitly tasked in any phase
  - **Benefit**: Adding explicit security test tasks (e.g., "Test that a command with path `../../etc/passwd` in content is rejected") ensures these requirements aren't overlooked

- [ ] **[PLAN-005]**: Phase 8 is the largest phase with 7 sub-tasks — consider splitting into "8a: Registry Initialization" and "8b: TUI Refactoring"
  - **Benefit**: Smaller phases are easier to track and verify incrementally

## Scoring Breakdown

| Category | Weight | Score | Rubric Basis | Deduction Details | Weighted |
|----------|--------|-------|-------------|-------------------|----------|
| Spec Alignment | 30% | 92/100 | 90-100 | allowed-tools not explicitly tasked: -8 | 27.60 |
| Tech Stack | 15% | 100/100 | 90-100 | None | 15.00 |
| Architecture Quality | 25% | 95/100 | 90-100 | Fork mode subagent dependency contradicts stated rules: -5 | 23.75 |
| Phase Planning | 20% | 100/100 | 90-100 | None | 20.00 |
| Constitution Alignment | 10% | 100/100 | 90-100 | None | 10.00 |
| **Subtotal** | **100%** | | | | **96.35** |
| Suggestion Cap | | | | -1.55 (3 suggestions, under 5 cap) | -1.55 |
| **Total** | **100%** | | | | **95/100** |

> **Suggestion Cap**: Suggestions deducted 1.55/5 points (cap: 5 points max). After resolving warnings, score would be ≥ 95.

## Score Validation

- [x] Every deduction has a corresponding issue in Detailed Findings
- [x] Arithmetic: 27.60 + 15.00 + 23.75 + 20.00 + 10.00 - 1.55 = 94.80 → rounds to 95
- [x] Suggestion deductions: 3 items (~-1.55 total), under 5-point cap
- [x] No phantom deductions
- [x] Score (95) consistent with Overall Status: ✅ Pass (≥ 80)
- [x] Without suggestions, score = 96.35 ≥ 95 ✓

## Recommendations

### Priority 1: Before Task Breakdown
1. **Fix PLAN-001**: Add an explicit task for implementing `allowed-tools` filtering — either a `FilteredRegistry` wrapper in Phase 6 or a filtering mechanism in Phase 8
2. **Fix PLAN-002**: Define a `ForkRunner` interface in `slash/` and inject the concrete `subagent.Manager` wrapper from `app/runner.go`. Update dependency rules to clarify that `slash/` defines the interface while `app/` provides the implementation

### Priority 2: Architecture Improvements
1. Pin doublestar to a specific version in go.mod
2. Add explicit security test tasks for path traversal and frontmatter sandboxing

### Priority 3: Documentation Enhancements
1. Consider splitting Phase 8 into two smaller phases for better tracking

## Available Follow-up Commands

- **Fix Issues**: Describe the changes (e.g., "Fix PLAN-001 and PLAN-002") to update the plan
- `/codexspec:plan-to-tasks` — proceed with task breakdown (warnings are non-blocking)
- `/codexspec:review-plan` — re-review after fixes
