# Specification Review Report

## Summary
- **Overall Status**: PASS
- **Compatibility Score**: 100/100
- **Authority Mode**: Requirements-first
- **Readiness**: Ready for Planning
- **Auto-fix rounds**: 1 (MIN-001 resolved; final re-review clean)

## Traceability

| Confirmed Entry | Spec Reference | Result |
|-----------------|----------------|--------|
| NEED-001 | REQ-001, REQ-002, REQ-008, REQ-010, US1, US2 | Covered |
| NEED-002 | REQ-003, US1, Expected Error Behavior, SC-001 | Covered |
| NEED-003 | REQ-004, NFR-002, US4, Edge Cases, SC-004 | Covered |
| NEED-004 | REQ-006, REQ-010, REQ-012, US2, SC-002 | Covered |
| NEED-005 | REQ-007, NFR-002, US4, Edge Cases, SC-005 | Covered |
| CON-001 | REQ-011, Constraints, Non-Goals | Covered |
| CON-002 | REQ-012, NFR-003, Constraints | Covered |
| DEC-001 | REQ-004, REQ-005, NFR-001, Edge Cases | Covered |
| DEC-002 | REQ-008, REQ-009, Constraints, Dependencies, SC-006 | Covered |
| DEC-003 | REQ-001, REQ-002, Constraints | Covered |
| DEC-004 | REQ-002, Out of Scope (OUT-001, OUT-002) | Covered |
| DEC-005 | REQ-006, REQ-012, NFR-003 | Covered |
| OUT-001 | Out of Scope | Covered |
| OUT-002 | Out of Scope | Covered |
| OUT-003 | Out of Scope, Non-Goals | Covered |

All `REQ`/`NFR` items carry at least one valid `Sources:` reference. OPEN-001 is resolved by DEC-005 and is not promoted to a confirmed requirement. Assumptions A-1/A-2/A-3 are labeled and do not expand scope.

## Verified Defects

### Critical
None.

### Warnings
None.

### Minor

#### MIN-001: Non-interactive stdin behavior is unspecified

- **Status**: Resolved (auto-fixed, round 1)
- **Evidence**: OUT-002 defers a non-interactive / scriptable configuration mode to a later version. The wizard is interactive by construction (REQ-001, REQ-010 collect fields through prompts).
- **Location**: Edge Cases; REQ-001.
- **Mismatch**: The specification did not state what happens when `fox config` is run without an interactive terminal (for example, piped stdin in CI). The wizard could block waiting for input or behave in an undefined way.
- **Impact**: A user or CI pipeline invoking `fox config` without a TTY may observe a hang or unclear failure, with no guidance that interactive mode is required.
- **Remediation applied**: Added an Edge Case stating that, when stdin is not an interactive terminal, the wizard exits with a clear message that interactive mode is required, because a non-interactive configuration mode is out of scope (OUT-002). Directly determined by OUT-002; introduces no new product decision.

## Risk Advisories

### RA-001: Plaintext secret at rest when inline storage is chosen

- **Applicability condition**: Only when a user opts into inline `api_key` storage (DEC-001 / REQ-005).
- **Risk**: `~/.foxharness/settings.json` then contains a plaintext API key. Any process or backup with read access to the home directory can read it.
- **Relationship to confirmed goal**: This is the user-confirmed trade-off in DEC-001, not a defect. A future hardening step could recommend restrictive file permissions (for example `0600`) on `settings.json` when an inline key is present, or document the plaintext risk in the wizard's warning text. No action is required for v1 planning.

## Design Opportunities

### DO-001: Reuse the existing validation path inside the wizard

- **Applicability condition**: When implementing REQ-004 (preflight), REQ-010 (field collection), and the connectivity probe (REQ-007).
- **Benefit**: Routing wizard input through the existing `internal/llmconfig` resolution and validation keeps a single source of truth, so the field errors a user sees in the wizard match the errors `fox` reports at startup.
- **Relationship to confirmed goal**: Supports NEED-003 and the "fail clearly" intent inherited from the provider feature. Optional; does not affect status.

## Score Derivation
- Critical root causes: 0
- Warning root causes: 0
- Minor root causes: 0 (MIN-001 auto-fixed in round 1)
- Formula: no defects → 100
