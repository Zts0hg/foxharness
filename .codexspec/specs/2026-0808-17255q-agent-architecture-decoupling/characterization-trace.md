# Characterization Baseline Trace

**Feature**: `2026-0808-17255q-agent-architecture-decoupling`
**Baseline Status**: Not frozen
**Baseline Commit**: TBD
**Authoritative Scenarios**: `requirements.md`

## Status Rules

- `pending`: The mandatory characterization scenario has not yet produced complete passing evidence.
- `pending-verification`: The residual-risk proof has not yet classified the item as a defect or verified current behavior.
- `blocked-defect`: A proof established a defect and baseline freeze is blocked pending separately confirmed correction semantics, Red evidence, an independent Green defect commit, and affected scenario reruns.
- `verified-current`: A `DV-*` proof did not establish a defect and records current behavior without authorizing a change.
- `corrected`: A proven defect completed the separately approved correction workflow and all affected scenarios pass against the corrected implementation.
- `pass`: The scenario has an executable hermetic test, authoritative fixture or expected result, recorded command, passing result, and source commit.

No row may become `pass` because of a document-only claim, coverage percentage, skipped test, missing external configuration, or an environment exception. `B00` requires every scenario row to be `pass`, every `DV-*` row to be `verified-current` or `corrected`, and the baseline commit and evidence fields to be complete.

## Trace Table

| Scenario ID | Status | Executable Test | Fixture or Expected Outcome | Command | Source Commit | Notes / Evidence |
|---|---|---|---|---|---|---|
| `RT-001` | pending | TBD | TBD | TBD | TBD | |
| `RT-002` | pending | TBD | TBD | TBD | TBD | |
| `RT-003` | pending | TBD | TBD | TBD | TBD | |
| `RT-004` | pending | TBD | TBD | TBD | TBD | |
| `RT-005` | pending | TBD | TBD | TBD | TBD | |
| `RT-006` | pending | TBD | TBD | TBD | TBD | |
| `RT-007` | pending | TBD | TBD | TBD | TBD | |
| `ST-001` | pending | TBD | TBD | TBD | TBD | |
| `ST-002` | pending | TBD | TBD | TBD | TBD | |
| `ST-003` | pending | TBD | TBD | TBD | TBD | |
| `ST-004` | pending | TBD | TBD | TBD | TBD | |
| `ST-005` | pending | TBD | TBD | TBD | TBD | |
| `ST-006` | pending | TBD | TBD | TBD | TBD | |
| `TL-001` | pending | TBD | TBD | TBD | TBD | |
| `TL-002` | pending | TBD | TBD | TBD | TBD | |
| `TL-003` | pending | TBD | TBD | TBD | TBD | |
| `TL-004` | pending | TBD | TBD | TBD | TBD | |
| `TL-005` | pending | TBD | TBD | TBD | TBD | |
| `TL-006` | pending | TBD | TBD | TBD | TBD | |
| `TL-007` | pending | TBD | TBD | TBD | TBD | |
| `TL-008` | pending | TBD | TBD | TBD | TBD | |
| `CX-001` | pending | TBD | TBD | TBD | TBD | |
| `CX-002` | pending | TBD | TBD | TBD | TBD | |
| `CX-003` | pending | TBD | TBD | TBD | TBD | |
| `CX-004` | pending | TBD | TBD | TBD | TBD | |
| `CX-005` | pending | TBD | TBD | TBD | TBD | |
| `CX-006` | pending | TBD | TBD | TBD | TBD | |
| `CX-007` | pending | TBD | TBD | TBD | TBD | |
| `CX-008` | pending | TBD | TBD | TBD | TBD | |
| `PL-001` | pending | TBD | TBD | TBD | TBD | |
| `PL-002` | pending | TBD | TBD | TBD | TBD | |
| `PL-003` | pending | TBD | TBD | TBD | TBD | |
| `PL-004` | pending | TBD | TBD | TBD | TBD | |
| `PL-005` | pending | TBD | TBD | TBD | TBD | |
| `RS-001` | pending | TBD | TBD | TBD | TBD | |
| `RS-002` | pending | TBD | TBD | TBD | TBD | |
| `RS-003` | pending | TBD | TBD | TBD | TBD | |
| `RS-004` | pending | TBD | TBD | TBD | TBD | |
| `RS-005` | pending | TBD | TBD | TBD | TBD | |
| `RS-006` | pending | TBD | TBD | TBD | TBD | |
| `RS-007` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-001` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-002` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-003` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-004` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-005` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-006` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-007` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-008` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-009` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-010` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-011` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-012` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-013` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-014` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-015` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-016` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-017` | pending | TBD | TBD | TBD | TBD | |
| `PF-TUI-018` | pending | TBD | TBD | TBD | TBD | |
| `UI-TUI-001` | pending | TBD | TBD | TBD | TBD | |
| `UI-TUI-002` | pending | TBD | TBD | TBD | TBD | |
| `UI-TUI-003` | pending | TBD | TBD | TBD | TBD | |
| `UI-TUI-004` | pending | TBD | TBD | TBD | TBD | |
| `UI-TUI-005` | pending | TBD | TBD | TBD | TBD | |
| `UI-TUI-006` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-001` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-002` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-003` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-004` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-005` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-006` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-007` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-008` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-009` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-010` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-011` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-012` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-013` | pending | TBD | TBD | TBD | TBD | |
| `PF-CLI-014` | pending | TBD | TBD | TBD | TBD | |
| `UI-CLI-001` | pending | TBD | TBD | TBD | TBD | |
| `UI-CLI-002` | pending | TBD | TBD | TBD | TBD | |
| `UI-CLI-003` | pending | TBD | TBD | TBD | TBD | |
| `UI-CLI-004` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-001` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-002` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-003` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-004` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-005` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-006` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-007` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-008` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-009` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-010` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-011` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-012` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-013` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-014` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-015` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-016` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-017` | pending | TBD | TBD | TBD | TBD | |
| `PF-FEI-018` | pending | TBD | TBD | TBD | TBD | |
| `UI-FEI-001` | pending | TBD | TBD | TBD | TBD | |
| `UI-FEI-002` | pending | TBD | TBD | TBD | TBD | |
| `UI-FEI-003` | pending | TBD | TBD | TBD | TBD | |
| `UI-FEI-004` | pending | TBD | TBD | TBD | TBD | |
| `UI-FEI-005` | pending | TBD | TBD | TBD | TBD | |
| `UI-FEI-006` | pending | TBD | TBD | TBD | TBD | |
| `UI-FEI-007` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-001` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-002` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-003` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-004` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-005` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-006` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-007` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-008` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-009` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-010` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-011` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-012` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-013` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-014` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-015` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-016` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-017` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-018` | pending | TBD | TBD | TBD | TBD | |
| `PF-AOP-019` | pending | TBD | TBD | TBD | TBD | |
| `UI-AOP-001` | pending | TBD | TBD | TBD | TBD | |
| `UI-AOP-002` | pending | TBD | TBD | TBD | TBD | |
| `UI-AOP-003` | pending | TBD | TBD | TBD | TBD | |
| `UI-AOP-004` | pending | TBD | TBD | TBD | TBD | |
| `UI-AOP-005` | pending | TBD | TBD | TBD | TBD | |
| `UI-AOP-006` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-001` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-002` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-003` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-004` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-005` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-006` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-007` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-008` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-009` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-010` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-011` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-012` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-013` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-014` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-015` | pending | TBD | TBD | TBD | TBD | |
| `PF-BEN-016` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-001` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-002` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-003` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-004` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-005` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-006` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-007` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-008` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-009` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-010` | pending | TBD | TBD | TBD | TBD | |
| `EV-BEN-011` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-001` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-002` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-003` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-004` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-005` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-006` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-007` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-008` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-009` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-010` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-011` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-012` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-013` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-014` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-015` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-016` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-017` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-018` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-019` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-020` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-021` | pending | TBD | TBD | TBD | TBD | |
| `PF-CHD-022` | pending | TBD | TBD | TBD | TBD | |
| `IA-CHD-001` | pending | TBD | TBD | TBD | TBD | |
| `IA-CHD-002` | pending | TBD | TBD | TBD | TBD | |
| `IA-CHD-003` | pending | TBD | TBD | TBD | TBD | |
| `IA-CHD-004` | pending | TBD | TBD | TBD | TBD | |
| `IA-CHD-005` | pending | TBD | TBD | TBD | TBD | |
| `IA-CHD-006` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-001` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-002` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-003` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-004` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-005` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-006` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-007` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-008` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-009` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-010` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-011` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-012` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-013` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-014` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-015` | pending | TBD | TBD | TBD | TBD | |
| `PF-AUT-016` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-001` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-002` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-003` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-004` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-005` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-006` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-007` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-008` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-009` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-010` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-011` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-012` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-013` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-014` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-015` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-016` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-017` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-018` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-019` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-020` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-021` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-022` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-023` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-024` | pending | TBD | TBD | TBD | TBD | |
| `CP-AUT-025` | pending | TBD | TBD | TBD | TBD | |
| `UI-AUT-001` | pending | TBD | TBD | TBD | TBD | |
| `UI-AUT-002` | pending | TBD | TBD | TBD | TBD | |
| `UI-AUT-003` | pending | TBD | TBD | TBD | TBD | |
| `UI-AUT-004` | pending | TBD | TBD | TBD | TBD | |
| `UI-AUT-005` | pending | TBD | TBD | TBD | TBD | |
| `UI-AUT-006` | pending | TBD | TBD | TBD | TBD | |
| `DV-FEI-001` | corrected | `TestDVFEI001ApprovalCallbackIsAuthenticatedBoundedAndReachable` | Bearer-authenticated bounded strict JSON callback maps unauthorized, malformed, oversized, success, unknown, and wrong-method requests to deterministic outcomes. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu ./internal/approval -count=1` | `D-FEI-001` | T074 Green; shared approval Store resolution is externally reachable without exposing pending IDs before authentication. |
| `DV-FEI-002` | corrected | `TestDVFEI002DuplicateMessageDeliveriesUseDurableAtMostOnceAcceptance`, `TestFileDeliveryStore*`, `TestGatewayRollsBackReservationWhenEnqueueIsUnavailable`, `TestDeliveryStoreUsesExplicitUserStateAndSurvivesReopen` | Exactly one sequential/concurrent delivery is enqueued; post-completion and reopened-store duplicates are acknowledged without enqueue; unavailable enqueue rolls back without blocking and corrupt state fails closed. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-002` | T075 Green plus `28386a0`; production composes an atomic user-state file authority, fails fast when the local queue is unavailable, and race testing has one reservation winner. |
| `DV-FEI-003` | corrected | `TestDVFEI003MissingOrBlankSenderIsRejectedBeforeReservationAndEnqueue` | Missing and blank sender IDs fail task parsing, receive webhook ACK 200, perform zero delivery reservations, and enqueue no task. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-003` | T076 Green; invalid identity cannot reach durable acceptance or session selection. |
| `DV-FEI-004` | corrected | `TestDVFEI004CancelledWaiterLeavesSessionLockWithoutLaterExecution`, `TestDVFEI004AcceptedTaskTimeoutIncludesGlobalPermitWait` | Cancellation removes a same-session waiter and its lock reference; the task lifetime starts at Runner acceptance, covers global-permit waiting, and expired accepted work never executes later. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-004` | T077 Green; focused race verification covers cancellation and inactive-lock cleanup. |
| `DV-FEI-005` | corrected | `TestDVFEI005PerSessionFIFOLeavesCapacityForOtherSessions` | Only each session head is globally runnable: same-session successors remain FIFO without consuming capacity, unrelated sessions use available capacity, and head completion enables only that session's next task. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-005` | T078 Green; focused race verification preserves FIFO and the global concurrency limit. |
| `DV-FEI-006` | corrected | `TestDVFEI006RunnerDrainsAcceptedTasksOnChannelClose`, `TestDVFEI006RunnerCancelsQueuedAndInflightTasksBeforeReturning`, `TestDVFEI006ProductionEntryCoordinatesShutdown` | Ordinary input closure drains accepted FIFO work; process cancellation cancels queued and active contexts and waits for active termination; production shutdown orders HTTP listener stop, task-channel close, Runner cancellation, and bounded completion waiting. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-006` | T079 Green; signal composition uses a single explicit shutdown deadline and never closes tasks before listener termination. |
| `DV-FEI-007` | corrected | `TestDVFEI007ApprovalResolutionIsNonBlockingAndExactlyOnce`, `TestDVFEI007CancelledApprovalCannotResolveLater`, `TestDVFEI007ApprovalCallbackMapsConflictAndNotFound`, `TestStoreTimeoutRemovesPendingRequest`, `TestStoreConcurrentResolveHasExactlyOneWinner` | The first resolution atomically wins; pending duplicates return conflict without blocking; waiter cleanup makes late/expired/cancelled IDs not found; HTTP maps these states to 409 and 404. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/approval ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-007` | T080 Green; 32-way race verification has one winner and 31 immediate conflicts. |
| `DV-FEI-008` | corrected | `TestDVFEI008SelectedModelSnapshotConfiguresEngineAndCompactor` | Runner reads provider model metadata once per task, applies the identical snapshot to engine and compactor configs, and resolves the known model's registered context window instead of fallback. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu ./internal/engine ./internal/compaction -count=1` | `D-FEI-008` | T081 Green; rotating metadata proves there is one frozen read and no engine/compactor divergence. |
| `DV-FEI-009` | corrected | `TestDVFEI009PanicRecoveryEmitsOneOutcomeAndBoundedTerminalReply` | Panic recovery emits exactly one failed outcome correlated by task/chat, attempts exactly one terminal reply with a fresh deadline-bearing context, completes cleanup, and permits the next queued task to run. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-009` | T082 Green; focused race verification preserves one outcome, one delivery attempt, and scheduler progress. |
| `DV-FEI-010` | corrected | `TestDVFEI010DeliveryFailuresAreTypedAndObservedByStage`, `TestDVFEI010MessengerBoundsTextBeforeTransport`, `TestDVFEI010ProductionEntryComposesDeliveryFailureObserver` | Receipt, session, lifecycle, final, ordinary-failure, panic-failure, and cancellation transport errors emit typed task/chat/stage/cause records; all text is bounded before transport; production explicitly logs through the injected observer. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-010` | T083 Green; no Runner direct-send error remains hidden and failed transport is observable rather than represented as successful delivery. |
| `DV-AOP-001` | corrected | `TestDVAOP001AgentOpsUsesGatewayDurableAcceptanceAuthority`, `TestDVAOP001GatewayRejectsMissingAndEmptyMessageIDsBeforeAgentOps`, `TestFileDeliveryStoreConcurrentReservationHasOneWinner`, `TestGatewayRollsBackReservationWhenEnqueueIsUnavailable` | AgentOps composes the shared user-state file authority at the Gateway, rejects invalid IDs before acceptance, acknowledges sequential/concurrent/restart duplicates without a second enqueue, and releases live failed enqueue reservations. No process-local task-acceptance Deduper remains. | `env GOCACHE=/tmp/fox-go-build-cache go test ./cmd/agentops ./internal/agentops ./internal/feishu -count=1` | `D-AOP-001` | T084 Green; focused race verification preserves one reservation winner and restart-persistent at-most-once enqueue behavior. |
| `DV-AOP-002` | corrected | `TestDVAOP002ProductionEntryCoordinatesShutdownAndTwoChannelDrain`, `TestDVAOP002RunnerWaitsForAcceptedWork`, `TestDVAOP002AcceptedPermitWaiterReachesCancellationHandling` | AgentOps stops and joins the HTTP producer before closing the Feishu task channel, cancels task contexts, lets the bridge drain and close the AgentOps channel, and waits for every Runner worker under the same bounded process deadline. Ordinary channel closure drains without cancellation; accepted permit waiters enter cancellation handling instead of disappearing. | `env GOCACHE=/tmp/fox-go-build-cache go test ./cmd/agentops ./internal/agentops ./internal/feishu -count=1` | `D-AOP-002` | T085 Green; focused race verification covers signal shutdown, two-channel drain, accepted-worker completion, and bounded concurrency. |
| `DV-AOP-003` | corrected | `TestDVAOP003EveryExecutionPathEmitsOneTypedOutcome`, `TestDVAOP003PanicReleasesCapacityBeforeTerminalTransition` | One Runner-owned transition emits exactly one correlated `completed`, `failed`, or `cancelled` outcome for success, ordinary failure, timeout, parent cancellation, and panic. Reasons remain typed, run and scheduling resources are released first, and non-success terminal delivery uses one fresh deadline-bearing context. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/agentops ./cmd/agentops -count=1` | `D-AOP-003` | T086 Green; focused race verification includes a blocked panic observer while the successor obtains the sole scheduling permit. |
| `DV-AOP-004` | corrected | `TestDVAOP004OneProviderSnapshotConfiguresEngineCompactorAndChild` | AgentOps reads provider protocol/model once per task into a delegating snapshot wrapper and supplies that identical provider identity to engine configuration, compaction, automemory hooks, and the child manager. Child metadata reads cannot re-read or diverge from the parent model. | `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./internal/subagent ./internal/agentops ./internal/engine ./internal/compaction ./cmd/agentops -count=1 -timeout=60s` | `D-AOP-004` | T087 Green; the registered selected model resolves the non-default compaction window and a rotating metadata source is read once despite child access. |
| `DV-AOP-005` | corrected | `TestDVAOP005DeliveryFailuresAreTypedBoundedAndNonRecursive`, `TestDVAOP005TerminalReasonsMapToTypedDeliveryStages`, `TestDVAOP005ProductionEntryComposesDeliveryFailureObserver` | Every Runner send passes one boundary that truncates before fake or real transport and reports typed task/chat/stage/cause failures with observer panic isolation. Session failure does not abort execution; final failure permits one failure delivery; failed failure delivery is observed without recursion. Panic, timeout, cancellation, ordinary failure, and final stages remain distinct. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/agentops ./cmd/agentops ./internal/feishu -count=1 -timeout=60s` | `D-AOP-005` | T088 Green; production explicitly installs the logging observer and focused race verification preserves independent task and delivery outcomes. |
| `DV-AOP-006` | corrected | `TestDVAOP006LogSearchRejectsEscapeAndNonRegularTargets`, `TestDVAOP006LogSearchResourceBoundsRemainEffective`, `TestLogSearchHonorsCanceledContextBeforeScanning`, `TestLogSearchStopsAfterLimitWithoutDrainingReader` | `log_search` opens the service-relative target through `os.Root`, validates the opened handle is a regular file, and rejects traversal, either path separator, outside-root symlinks, and directories. Valid files retain ordered case-insensitive matching, early limit stop, cancellation, 200 matches, and the one-MiB scanner bound. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/agentops -run 'TestDVAOP006|TestLogSearch|TestRunnerBuildRegistryAllowsReadOnlyLogSearch' -count=1 -timeout=30s` | `D-AOP-006` | T089 Green; rooted open prevents check/open races and focused race verification passes. |
| `DV-BEN-001` | corrected | `TestDVBEN001CaseDeadlineCoversFactoryAndStopsUnstartedValidations` | A case-owned context defaults to 600 seconds and reaches fixture copy, harness construction, runtime, and validation. Parent cancellation and timeout stop new validation execution while emitting one ordered typed record per remaining entry; `cancelled` and `timed_out` remain distinct. Terminal cleanup deliberately does not reuse the expired case context and is owned by `DV-BEN-004`. | `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -count=1 -timeout=60s` | `D-BEN-001` | T090 Green; focused race verification passes and the command-level two-minute deadline remains bounded by its parent case context. |
| `DV-BEN-002` | corrected | `TestDVBEN002AcceptedRepeatAlwaysHasTypedStatusAndSeparateEvidence`, `TestDVBEN002ResultStatusControlsProcessExit` | Every accepted repeat returns one `completed`, `failed`, `cancelled`, `timed_out`, or `infrastructure_failed` result while runtime, evaluation, and infrastructure evidence remain separate. The command retains partial results, writes available summary/JSON, and returns 0 only for all-success, 1 for execution/evaluation terminal failure, or 2 for infrastructure/input/report failure. | `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -count=1 -timeout=60s` | `D-BEN-002` | T091 Green; focused race and vet pass. |
| `DV-BEN-003` | corrected | `TestDVBEN003RuntimeFidelityDerivesFromResolvedSpecification`, `TestDVBEN003CompositionUsesResolvedRuntimeSpec`, `TestBuildHarnessReportsRuntimeFidelity` | One immutable benchmark-only runtime specification contains provider/model, turn budget, tool surface, and prompt, memory, compaction, permission, observation, and interaction policies. Engine, compactor, machine-readable fidelity, and human-visible invariant/difference claims derive from that same snapshot; composition contains no independent fidelity literals. | `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -count=1 -timeout=60s` | `D-BEN-003` | T092 Green; focused race and vet pass without introducing a general runtime profile layer. |
| `DV-BEN-004` | corrected | `TestDVBEN004FixtureAndValidationStayWithinOwnedRoots`, `TestDVBEN004PartialFixtureCopyFailureRemovesWorkspace`, `TestDVBEN004FailedWorkspaceCleanupUsesFreshBoundedContext`, `TestDVBEN004CleanupFailureBecomesInfrastructureEvidence`, `TestDVBEN004CleanupTimeoutBecomesInfrastructureEvidence`, `TestDVBEN004PanicStillCleansWorkspace`, `TestRunCaseIncludesRuntimeFidelityMetadata` | Fixture copy accepts directories and regular files only, rejects source symlinks and unsupported types without mutating the fixture, and creates destination entries through an owned root. `file_contains` rejects absolute paths, traversal, symlinks, and non-regular targets while accepting rooted nested files. Successful workspaces remain available; every observed failure, cancellation, timeout, panic, and partial copy removes its workspace under a fresh bounded context, while cleanup failure or timeout becomes typed infrastructure evidence. | `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -count=1 -timeout=60s` | `D-BEN-004` | T093 Green; focused race, architecture tests, and package vet pass. Cleanup has a 30-second default independent of an expired case context. |
| `DV-BEN-005` | corrected | `TestDVBEN005CaseLoadingRejectsInvalidStructuralDomains`, `TestDVBEN005RelativeFixtureResolvesFromCaseDirectory`, `TestDVBEN005RepeatMustBeStrictlyPositiveAndOverflowSafe` | YAML rejects unknown and duplicate top-level or validation fields. Trimmed required case fields, turn and timeout domains, known validation types, exact required non-vacuous fields, explicit nulls, and irrelevant-field presence are validated in deterministic structural order before fixture or runtime work. Relative fixtures resolve from the case-file directory. Repeat zero, negative, and integer overflow return input exit 2 without a report. | `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -count=1 -timeout=60s` | `D-BEN-005` | T094 Green; focused race, architecture tests, and package vet pass. The checked-in counter-race case now uses its case-directory-relative fixture path. |
| `DV-BEN-006` | corrected | `TestDVBEN006CommandOutputIsIndependentlyBounded`, `TestDVBEN006CommandFailurePreservesSeparateOutput`, `TestDVBEN006CancellationKillsIgnoringDescendantsAndReaps`, `TestDVBEN006ValidatorTimeoutIsDistinctAndOrdered`, `TestDVBEN006ActiveCancellationSynthesizesRemainingResults` | A dedicated validator executor retains stdout and stderr independently up to exactly one MiB and emits typed per-stream overflow evidence. Every command runs in a process-group or platform-equivalent tree boundary. Overflow, parent cancellation, case deadline, and validator timeout initiate bounded TERM-to-KILL termination and wait for reaping; non-zero exit keeps separate bounded streams. Validation order and cardinality remain stable, and post-cancellation entries are synthetic and unexecuted. | `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./internal/benchmark ./cmd/bench -count=1 -timeout=60s` | `D-BEN-006` | T095 Green; focused race, architecture tests, package vet, and Windows cross-compilation pass. |
| `DV-BEN-007` | corrected | `TestDVBEN007ResultCarriesStableProvenanceAndRuntimeState`, `TestDVBEN007DefinitionAndFixtureIdentitiesAreRootIndependentAndSensitive`, `TestDVBEN007RuntimeFailureRetainsAgentRunAndCause`, `TestDVBEN007RuntimeContextCausesMapTypedStatus`, `TestDVBEN007SetupAndEvaluationTerminalStatesRemainSeparate`, `TestDVBEN007CorrectedSchemaMatchesNormalizedGolden`, `TestDVBEN007RepeatOrchestrationUsesOneBasedIdentity`, `TestWriteJSONRejectsMissingOrUnsupportedSchemaVersion` | Schema v1 records one-based repeat, actual Agent run, normalized case-definition and materialized fixture SHA-256 identities, aggregate and runtime terminal states/causes, provider/model, effective case and command deadlines, and the complete resolved runtime-fidelity snapshot. Fixture identity ignores source root and normalized permission differences but changes with content; case identity changes with semantic input. Setup, runtime, evaluation, cancellation, and timeout remain distinct. Production retains volatile values while the checked-in success/failure golden normalizes only duration, workspace, session, run, and timestamps. | `env HOME=/tmp/fox-test-home GOMODCACHE=/Users/xiaoming/go/pkg/mod GOCACHE=/tmp/fox-go-build-cache go test ./... -count=1 -timeout=180s` | `D-BEN-007` | T096 Green; corrected writer rejects nil or non-v1 results. Focused race, architecture tests, package vet, Windows cross-compilation, and the complete repository gate pass. T042 correction stop cleared. |
| `DV-CHD-001` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-CHD-002` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-CHD-003` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-CHD-004` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-CHD-005` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-CHD-006` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AUT-001` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AUT-002` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AUT-003` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AUT-004` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AUT-005` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AUT-006` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AUT-007` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AUT-008` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AUT-009` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AUT-010` | pending-verification | TBD | TBD | TBD | TBD | |

## Baseline Freeze Checklist

- [ ] Every identifier exactly matches the confirmed catalog and no unconfirmed identifier is present.
- [ ] Every scenario row names at least one executable hermetic test.
- [ ] Every scenario row names its authoritative immutable fixture or expected outcome.
- [ ] Every scenario row records the exact passing command and result.
- [ ] Every fixture records source commit, profile or entry source, semantics, and integrity hash in `testdata/characterization/v1/manifest.json`.
- [ ] Every `DV-*` row is `verified-current` or `corrected`; no verification is pending.
- [ ] Every proven defect has separately confirmed semantics, behavior-sensitive Red evidence, an independent Green defect commit, and affected scenario reruns.
- [ ] The exact initial architecture-violation allowlist is recorded and rejects additions or broadening.
- [ ] `go test ./...` passes offline without credentials, ambient user state, missing-configuration skips, or external services.
- [ ] The corrected source commit is recorded as `B00`.
