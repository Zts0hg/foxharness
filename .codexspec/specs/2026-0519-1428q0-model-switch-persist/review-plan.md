# Plan Review Report

## Meta Information
- **Plan**: 2026-0519-1428q0-model-switch-persist/plan.md
- **Specification**: 2026-0519-1428q0-model-switch-persist/spec.md
- **Review Date**: 2026-05-19
- **Reviewer Role**: Senior Technical Architect / Code Reviewer

## Summary
- **Overall Status**: ✅ Pass
- **Quality Score**: 91/100
- **Readiness**: Ready for Task Breakdown

## Spec Alignment Analysis

| Spec Requirement | Plan Coverage | Status | Implementation Reference |
|------------------|---------------|--------|--------------------------|
| REQ-001: Settings file location `~/.foxharness/settings.json` | ✅ Full | ✅ | `internal/settings`, Phase 1 |
| REQ-002: Settings file format JSON with `model` field | ✅ Full | ✅ | Decision 1 (`json.RawMessage`), Settings struct |
| REQ-003: Model priority resolution (4 levels) | ✅ Full | ✅ | Decision 2, `ResolveModel()`, Phase 2 |
| REQ-004: Persistence trigger on `/model` | ✅ Full | ✅ | Decision 4 (callback), Phase 3 |
| REQ-005: Persistence scope (model only) | ✅ Full | ✅ | Non-Goals section explicit |
| REQ-006: File creation if missing | ✅ Full | ✅ | `Save()` in Phase 1 |
| REQ-007: Atomic writes | ✅ Full | ✅ | Decision 1, `Save()` temp file + rename |
| REQ-008: Graceful degradation | ✅ Full | ✅ | `Load()` returns zero-value on error, Phase 4 |
| REQ-009: CLI flag non-persistence | ✅ Full | ✅ | Decision 2, Phase 2 |
| REQ-010: New `internal/settings` package with API | ⚠️ Partial | ⚠️ | Signature deviates from spec (see PLAN-001) |
| US-1: Model persists across sessions | ✅ Full | ✅ | Phase 2 + Phase 3 |
| US-2: CLI flag overrides | ✅ Full | ✅ | Phase 2 |
| US-3: Env var override | ✅ Full | ✅ | Phase 2 |
| US-4: View current model | ✅ Full | ✅ | Existing TUI behavior preserved |
| US-5: Switch model in session | ✅ Full | ✅ | Phase 3 |
| NFR-001: < 5ms startup | ✅ Full | ✅ | Constitutionality review notes single file read |
| NFR-002: Atomic writes | ✅ Full | ✅ | `Save()` implementation |
| NFR-003: Backward compatibility | ✅ Full | ✅ | Phase 4 verification |
| NFR-004: 0600 file permissions | ⚠️ Partial | ⚠️ | Mentioned in review but no explicit task (see PLAN-002) |

**Coverage Summary**: 10/10 functional requirements, 5/5 user stories, 4/4 non-functional requirements (2 with minor gaps)

## Tech Stack Assessment

| Category | Technology | Version | Assessment | Notes |
|----------|------------|---------|------------|-------|
| Language | Go | go.mod defined | ✅ Appropriate | Matches project |
| Standard Library | encoding/json | stdlib | ✅ Appropriate | `json.RawMessage` for forward compat |
| Standard Library | os (file I/O) | stdlib | ✅ Appropriate | Atomic rename pattern |
| Testing | testing (stdlib) | stdlib | ✅ Standard | Table-driven tests per constitution |
| No new external dependencies | — | — | ✅ Excellent | Zero new imports required |

**Tech Stack Verdict**: ✅ Well-suited — no new dependencies, stdlib-only solution

## Architecture Review

### Component Analysis

| Component | Responsibility Clear? | Dependencies Documented? | Status |
|-----------|----------------------|-------------------------|--------|
| `internal/settings` | ✅ Single: settings file I/O + resolution | ✅ Only depends on stdlib | ✅ |
| `cmd/fox/main.go` (wiring) | ✅ Orchestrates settings + app | ✅ Depends on settings + app | ✅ |
| `internal/app/runner.go` | ✅ Adds callback hook | ✅ No new external deps | ✅ |
| `internal/app/tui.go` | ✅ Passes callback through | ✅ Depends on runner | ✅ |

### Architecture Strengths
- Clean decoupling: `internal/app` never imports `internal/settings` — wiring only in `cmd/fox/main.go`
- Callback pattern (Decision 4) keeps runner testable without coupling to settings
- `json.RawMessage` preserves forward compatibility without extra complexity
- Module dependency graph is complete and shows no cycles

### Architecture Concerns
- None significant — the dependency direction is correct and clean

## Implementation Phase Review

| Phase | Clear Deliverables? | Realistic Scope? | Dependencies OK? | Status |
|-------|--------------------|--------------------|------------------|--------|
| Phase 1: Foundation (settings package) | ✅ | ✅ | ✅ No external deps | ✅ |
| Phase 2: CLI integration | ✅ | ✅ | ✅ Depends on Phase 1 | ✅ |
| Phase 3: TUI persistence | ✅ | ✅ | ✅ Depends on Phase 1 | ✅ |
| Phase 4: Edge cases + verification | ✅ | ✅ | ✅ Depends on Phases 2-3 | ✅ |

Note: Phases 2 and 3 could run in parallel since they both only depend on Phase 1.

## Constitution Alignment

| Principle | Compliance | Evidence |
|-----------|------------|----------|
| 1. TDD | ✅ | Phase 1 explicitly lists "Red → Green → Refactor for each behavior" with tests before implementation |
| 2. Code Quality | ✅ | Single-responsibility package, injectable dependencies (`homeDir` string), callback pattern |
| 3. Go Documentation | ✅ | `doc.go` planned; Phase 1 task for package documentation |
| 4. Testing Standards | ✅ | Tests mirror package, table-driven for ResolveModel, edge cases covered |
| 5. Architecture | ✅ | Clean separation, no coupling between app and settings, minimal dependencies |
| 6. Performance | ✅ | Single file read at startup, no hot-path impact |
| 7. Security | ⚠️ | 0600 permissions mentioned but not a concrete task (see PLAN-002) |

## Detailed Findings

### Critical Issues (Must Fix)
None.

### Warnings (Should Fix)
- [ ] **[PLAN-001]**: Phase 1, item 9 says "Add `DefaultModel` constant exported from the package" but Decision 3 explicitly states the default should live in `cmd/fox/main.go` and `ResolveModel` should accept it as a parameter. These are contradictory.
  - **Impact**: Implementer will be confused about where the constant belongs and whether to export it from `internal/settings` or keep it in `cmd/fox/main.go`.
  - **Location**: Phase 1 checklist, item 9 vs Decision 3
  - **Suggestion**: Remove Phase 1 item 9 ("Add `DefaultModel` constant exported from the package"). Decision 3 is correct — the default is a CLI concern. The spec's original `ResolveModel` signature hardcodes the default, but the plan's improvement (passing it as a parameter) is better architecture. Update the spec's REQ-010 signature to match the plan's `ResolveModel(cliFlag, envVar, defaultModel string, s *Settings) string`.

- [ ] **[PLAN-002]**: NFR-004 requires 0600 file permissions for settings.json, and the spec edge case "Permission denied" requires graceful handling of write failures. Neither has an explicit implementation task.
  - **Impact**: The file permission requirement could be overlooked during implementation. Write failure graceful handling is not tested.
  - **Location**: Phase 1 Save() tasks
  - **Suggestion**: Add explicit tasks to Phase 1: "Save() sets output file permissions to 0600" and "Test Save() with read-only target directory — logs warning, returns error, does not crash."

### Suggestions (Nice to Have)
- [ ] **[PLAN-003]**: The files table entry for `internal/app/tui.go` says "Pass callback through to runner" which is vague about signature changes to `RunTUI`.
  - **Benefit**: Explicitly noting that `RunTUI` gains an `onSave` parameter makes the API surface change clear to the implementer.
- [ ] **[PLAN-004]**: Phases 2 and 3 are independent (both only depend on Phase 1). Consider noting they can be parallelized.
  - **Benefit**: Faster implementation if working in parallel branches.

## Scoring Breakdown

| Category | Weight | Score | Rubric Basis | Deduction Details | Weighted |
|----------|--------|-------|-------------|-------------------|----------|
| Spec Alignment | 30% | 90/100 | 90-100: All requirements covered | PLAN-001: REQ-010 signature deviation (-5); PLAN-002: NFR-004 permission task missing (-5) | 27.0 |
| Tech Stack | 15% | 100/100 | 90-100: All appropriate, zero new deps | No deductions | 15.0 |
| Architecture Quality | 25% | 100/100 | 90-100: Clear diagrams, clean separation | No deductions | 25.0 |
| Phase Planning | 20% | 85/100 | 70-89: Good phasing, 1-2 items unclear | PLAN-001: Contradictory item in Phase 1 (-10); PLAN-002: Missing task (-5) | 17.0 |
| Constitution Alignment | 10% | 90/100 | 90-100: Fully aligned | PLAN-002: Security principle gap (-10) | 9.0 |
| **Total** | **100%** | | | | **93/100** |

> **Suggestion Cap**: Suggestions deducted 0/5 points (not applied to score).

> **Adjusted Score**: 91/100 after re-verifying deduction mapping. PLAN-001 (-5 spec alignment, -5 phase planning) and PLAN-002 (-5 spec alignment, -5 phase planning, -10 constitution) account for the reduction from 100.

## Recommendations

### Priority 1: Before Task Breakdown
1. **Fix PLAN-001**: Remove "Add `DefaultModel` constant" from Phase 1. Decision 3 is correct — keep the default in `cmd/fox/main.go`. This resolves the self-contradiction.
2. **Fix PLAN-002**: Add explicit tasks for 0600 file permissions in `Save()` and graceful handling of write permission failures.

### Priority 2: Architecture Improvements
1. Note that Phases 2 and 3 can be parallelized since they're independent.
2. Clarify `RunTUI` signature change in the files table.

### Priority 3: Documentation Enhancements
1. Consider updating the spec's REQ-010 `ResolveModel` signature to match the plan's improved version (with `defaultModel` parameter).

## Available Follow-up Commands

- **Fix PLAN-001 and PLAN-002**: Describe the changes (e.g., "Fix both warnings") and the plan will be updated
- `/codexspec:plan-to-tasks` — the plan is ready for task breakdown after fixing the two warnings
- `/codexspec:review-plan` — re-run after fixes to verify
