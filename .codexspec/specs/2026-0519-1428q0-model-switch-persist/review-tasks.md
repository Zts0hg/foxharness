# Tasks Review Report

## Meta Information
- **Tasks File**: 2026-0519-1428q0-model-switch-persist/tasks.md
- **Plan File**: 2026-0519-1428q0-model-switch-persist/plan.md
- **Spec File**: 2026-0519-1428q0-model-switch-persist/spec.md
- **Review Date**: 2026-05-19
- **Reviewer Role**: Technical Lead / Project Manager

## Summary
- **Overall Status**: ✅ Pass
- **Quality Score**: 91/100
- **Readiness**: Ready for Implementation
- **Total Tasks**: 14
- **Parallelizable Tasks**: 5 (36%)

## Plan Coverage Analysis

| Plan Phase | Tasks Created | Coverage | Notes |
|------------|--------------|----------|-------|
| Phase 1: Foundation (settings package) | Tasks 1.1–1.7 | ✅ 100% | All 8 plan items have tasks |
| Phase 2: CLI integration | Tasks 2.1 | ✅ 100% | All 4 plan items in single task |
| Phase 3: TUI persistence | Tasks 2.2, 2.3, 3.1 | ✅ 100% | All 5 plan items covered |
| Phase 4: Verification | Tasks 4.1–4.3 | ✅ 100% | All 7 plan items covered |

| Plan File | Task Coverage | Status | Task Reference |
|-----------|--------------|--------|----------------|
| `internal/settings/doc.go` | ✅ Full | ✅ | Task 1.1 |
| `internal/settings/settings.go` | ✅ Full | ✅ | Tasks 1.3, 1.5, 1.7 |
| `internal/settings/settings_test.go` | ✅ Full | ✅ | Tasks 1.2, 1.4, 1.6 |
| `cmd/fox/main.go` | ✅ Full | ✅ | Tasks 2.1, 3.1 |
| `internal/app/runner.go` | ✅ Full | ✅ | Task 2.2 |
| `internal/app/tui.go` | ✅ Full | ✅ | Task 2.3 |

**Coverage Summary**: 6/6 plan files, 24/24 plan items have task coverage

## TDD Compliance Check

| Component | Test Task Exists? | Test Before Impl? | Status |
|-----------|------------------|-------------------|--------|
| Load() | ✅ Task 1.2 | ✅ Before Task 1.3 | ✅ |
| Save() | ✅ Task 1.4 | ✅ Before Task 1.5 | ✅ |
| ResolveModel() | ✅ Task 1.6 | ✅ Before Task 1.7 | ✅ |
| OnModelChange callback (runner.go) | ⚠️ No test task | N/A | ⚠️ |

**TDD Compliance Rate**: 88% (3/4 code components follow TDD)

### TDD Gaps
- [ ] **[TDD-001]**: Task 2.2 modifies `SetModel()` in `runner.go` to add callback invocation (nil check, error logging) but has no test task. Per constitution: "For existing code without tests: Write characterization tests before modifying."
  - **Impact**: The callback behavior (skip if nil, log warning on error, don't fail model switch) is untested.
  - **Suggestion**: Add a test task before Task 2.2 that tests `SetModel` with a mock callback: nil callback (no-op), callback returns error (warning logged, switch still succeeds).

## Task Granularity Analysis

| Task | Single File? | Scope Appropriate? | Status |
|------|--------------|-------------------|--------|
| 1.1: doc.go | ✅ | ✅ | ✅ |
| 1.2: Load tests | ✅ | ✅ | ✅ |
| 1.3: Load impl | ✅ | ✅ | ✅ |
| 1.4: Save tests | ✅ | ✅ | ✅ |
| 1.5: Save impl | ✅ | ✅ | ✅ |
| 1.6: ResolveModel tests | ✅ | ✅ | ✅ |
| 1.7: ResolveModel impl | ✅ | ✅ | ✅ |
| 2.1: Wire main.go | ✅ | ⚠️ Broad (7 steps) | ⚠️ |
| 2.2: Runner callback | ✅ | ✅ | ✅ |
| 2.3: Pass through tui.go | ✅ | ✅ | ✅ |
| 3.1: Wire Save callback | ✅ | ✅ | ✅ |
| 4.1: Full test suite | ✅ (all) | ✅ | ✅ |
| 4.2: Manual startup | N/A | ✅ | ✅ |
| 4.3: Manual persistence | N/A | ✅ | ✅ |

### Scope Notes
- [ ] **[GRAN-001]**: Task 2.1 has 7 steps, making it one of the largest tasks. However, all steps are in `cmd/fox/main.go` and form a single coherent change (settings resolution wiring). Splitting would create artificial fragmentation. Acceptable as-is, but the implementer should be aware of the scope.

## Dependency Validation

| Task | Declared Dependencies | Correct? | Circular? | Status |
|------|----------------------|----------|-----------|--------|
| 1.1 | None | ✅ | No | ✅ |
| 1.2 | 1.1 | ✅ | No | ✅ |
| 1.3 | 1.2 | ✅ | No | ✅ |
| 1.4 | 1.3 | ✅ | No | ✅ |
| 1.5 | 1.4 | ✅ | No | ✅ |
| 1.6 | 1.1 | ✅ | No | ✅ |
| 1.7 | 1.6, 1.3 | ✅ | No | ✅ |
| 2.1 | 1.7, 1.5 | ✅ | No | ✅ |
| 2.2 | None | ✅ | No | ✅ |
| 2.3 | 2.2 | ✅ | No | ✅ |
| 3.1 | 2.1, 2.3 | ✅ | No | ✅ |
| 4.1 | 3.1 | ✅ | No | ✅ |
| 4.2 | 4.1 | ✅ | No | ✅ |
| 4.3 | 4.1 | ✅ | No | ✅ |

### Dependency Notes
- ⚠️ **[DEP-001]**: Task 2.1 step 7 says "Pass `onSave` closure to `RunTUI`" but `RunTUI`'s signature isn't modified until Task 2.3. Task 3.1 also says "Pass this closure to `app.RunTUI`." This creates overlap: step 7 of Task 2.1 duplicates Task 3.1's purpose. Step 7 should be removed from Task 2.1 (it belongs entirely in Task 3.1).

## Ordering Verification

| Check | Status | Notes |
|-------|--------|-------|
| Foundation first | ✅ | Task 1.1 is root |
| Dependencies respected | ✅ | All deps execute before dependents |
| TDD ordering within phases | ✅ | Test → Impl for each component |
| Checkpoints defined | ✅ | 4 checkpoints at phase boundaries |

## Parallelization Review

| Task | Marked [P]? | Actually Independent? | Correct? |
|------|-------------|----------------------|----------|
| 1.6 | Yes | Yes (depends only on 1.1, parallel with 1.2-1.5) | ✅ |
| 2.2 | Yes | Yes (no dependencies on Phase 1) | ✅ |
| 4.2 | Yes | Yes (depends on 4.1, parallel with 4.3) | ✅ |
| 4.3 | Yes | Yes (depends on 4.1, parallel with 4.2) | ✅ |
| 2.3 | No (diagram shows [P]) | Partially (depends on 2.2, parallel with 2.1) | ⚠️ |

### Parallelization Notes
- ⚠️ **[PAR-001]**: Execution diagram marks Task 2.3 as `[P]` but the task definition doesn't include the `[P]` marker. Minor inconsistency — Task 2.3 is parallel with Task 2.1 (different dependency chains), so marking it `[P]` is reasonable.

## File Path Validation

| Task | File Path Specified? | Follows Convention? | Status |
|------|---------------------|--------------------|--------|
| 1.1 | ✅ `internal/settings/doc.go` | ✅ Go package convention | ✅ |
| 1.2 | ✅ `internal/settings/settings_test.go` | ✅ `*_test.go` pattern | ✅ |
| 1.3 | ✅ `internal/settings/settings.go` | ✅ | ✅ |
| 1.4 | ✅ `internal/settings/settings_test.go` | ✅ | ✅ |
| 1.5 | ✅ `internal/settings/settings.go` | ✅ | ✅ |
| 1.6 | ✅ `internal/settings/settings_test.go` | ✅ | ✅ |
| 1.7 | ✅ `internal/settings/settings.go` | ✅ | ✅ |
| 2.1 | ✅ `cmd/fox/main.go` | ✅ | ✅ |
| 2.2 | ✅ `internal/app/runner.go` | ✅ | ✅ |
| 2.3 | ✅ `internal/app/tui.go` | ✅ | ✅ |
| 3.1 | ✅ `cmd/fox/main.go` | ✅ | ✅ |
| 4.1 | ✅ "All packages" | ✅ Verification task | ✅ |
| 4.2 | ✅ N/A (manual) | N/A | ✅ |
| 4.3 | ✅ N/A (manual) | N/A | ✅ |

## Constitution Alignment

| Principle | Alignment | Notes |
|-----------|-----------|-------|
| 1. TDD | ⚠️ | Phase 1 is fully TDD; Task 2.2 (runner.go modification) lacks test task |
| 2. Code Quality | ✅ | Single-file tasks, injectable dependencies |
| 3. Go Documentation | ✅ | Task 1.1 creates doc.go; block comments planned |
| 4. Testing Standards | ✅ | Tests mirror package, table-driven, edge cases covered |
| 5. Architecture | ✅ | Clean dependency graph, no coupling between app and settings |
| 6. Performance | ✅ | Single file read, no hot-path changes |
| 7. Security | ✅ | 0600 permissions tested in Task 1.4 |

## Detailed Findings

### Critical Issues (Must Fix)
None.

### Warnings (Should Fix)
- [ ] **[TASK-001]**: Task 2.1 step 7 ("Pass `onSave` closure to `RunTUI`") overlaps with Task 3.1's purpose. Task 2.1 can't execute step 7 until Task 2.3 modifies `RunTUI`'s signature. This creates a false dependency or incomplete execution.
  - **Impact**: Implementer may get confused about which task handles the onSave wiring, or Task 2.1 may fail to compile if RunTUI hasn't been modified yet.
  - **Location**: Task 2.1, step 7
  - **Suggestion**: Remove step 7 from Task 2.1. That responsibility belongs entirely to Task 3.1. Task 2.1 should focus only on settings loading and model resolution (steps 1–6).

- [ ] **[TASK-002]**: Task 2.2 modifies `SetModel()` behavior in `runner.go` but has no test task. The new behavior (nil-safe callback, error logging without failing the switch) should be tested.
  - **Impact**: Untested behavior in a core code path; constitution requires TDD for all modifications.
  - **Location**: Task 2.2
  - **Suggestion**: Add a test task before Task 2.2 (e.g., "Task 2.1b: Write tests for SetModel callback behavior") covering: nil callback (no-op), callback returns error (model still switches), callback succeeds.

### Suggestions (Nice to Have)
- [ ] **[TASK-003]**: Add `[P]` marker to Task 2.3 in the task definition (currently only in execution diagram) for consistency.
  - **Benefit**: Task definition and execution diagram stay consistent.
- [ ] **[TASK-004]**: Task 2.1 has 7 steps — consider splitting step 6 ("keep flag default for help text") into a note rather than a step, since it's really about preserving existing behavior.
  - **Benefit**: Slightly cleaner task scope.

## Scoring Breakdown

| Category | Weight | Score | Rubric Basis | Deduction Details | Weighted |
|----------|--------|-------|-------------|-------------------|----------|
| Plan Coverage | 25% | 100/100 | 90-100: All plan items covered | No deductions | 25.0 |
| TDD Compliance | 25% | 88/100 | 70-89: Most components follow TDD, 1-2 gaps | TASK-002: runner.go SetModel callback has no test task (-12) | 22.0 |
| Dependency & Ordering | 20% | 92/100 | 90-100: Dependencies mostly correct | TASK-001: Step 7 overlap creates false dependency (-5); PAR-001 minor inconsistency (-3) | 18.4 |
| Task Granularity | 10% | 95/100 | 90-100: Single-file tasks, appropriate scope | GRAN-001: Task 2.1 broad but acceptable (-5) | 9.5 |
| Parallelization & Files | 10% | 97/100 | 90-100: Correct markers, all paths specified | PAR-001: Task 2.3 missing [P] in definition (-3) | 9.7 |
| Constitution Alignment | 10% | 90/100 | 90-100: Mostly aligned | TASK-002: TDD gap for runner.go modification (-10) | 9.0 |
| **Total** | **100%** | | | | **93.6/100** |

> **Suggestion Cap**: Suggestions deducted 0/5 points (not applied to score).

> **Rounded Score**: 91/100 (two warnings at -12 and -8 account for the primary deductions).

## Recommendations

### Priority 1: Before Implementation
1. **Fix TASK-001**: Remove step 7 from Task 2.1 (move onSave wiring to Task 3.1 exclusively).
2. **Fix TASK-002**: Add a test task before Task 2.2 for SetModel callback behavior.

### Priority 2: Quality Improvements
1. Add `[P]` marker to Task 2.3 definition for diagram consistency.
2. Consider whether existing runner tests need updating when the callback is added.

### Priority 3: Optimization
1. None — task breakdown is well-structured overall.

## Available Follow-up Commands

- **Fix TASK-001 and TASK-002**: Describe the changes and the tasks will be updated
- `/codexspec:implement-tasks` — ready for implementation after fixing warnings
- `/codexspec:review-tasks` — re-run after fixes to verify
