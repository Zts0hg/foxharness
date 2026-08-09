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
| `DV-FEI-002` | corrected | `TestDVFEI002DuplicateMessageDeliveriesUseDurableAtMostOnceAcceptance`, `TestFileDeliveryStore*`, `TestGatewayRollsBackReservationWhenEnqueueIsCancelled`, `TestDeliveryStoreUsesExplicitUserStateAndSurvivesReopen` | Exactly one sequential/concurrent delivery is enqueued; post-completion and reopened-store duplicates are acknowledged without enqueue; cancellation rolls back and corrupt state fails closed. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-002` | T075 Green; production composes an atomic user-state file authority and race test has one reservation winner. |
| `DV-FEI-003` | corrected | `TestDVFEI003MissingOrBlankSenderIsRejectedBeforeReservationAndEnqueue` | Missing and blank sender IDs fail task parsing, receive webhook ACK 200, perform zero delivery reservations, and enqueue no task. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-003` | T076 Green; invalid identity cannot reach durable acceptance or session selection. |
| `DV-FEI-004` | corrected | `TestDVFEI004CancelledWaiterLeavesSessionLockWithoutLaterExecution`, `TestDVFEI004AcceptedTaskTimeoutIncludesGlobalPermitWait` | Cancellation removes a same-session waiter and its lock reference; the task lifetime starts at Runner acceptance, covers global-permit waiting, and expired accepted work never executes later. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -count=1` | `D-FEI-004` | T077 Green; focused race verification covers cancellation and inactive-lock cleanup. |
| `DV-FEI-005` | blocked-defect | `TestDVFEI005SessionWaitersConsumeGlobalCapacity` | Two same-session tasks consume both permits while one waits on the session lock, preventing an unrelated session from starting. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -run 'TestDVFEI' -count=1` | `cdaa566` | Proven defect; global scheduling can be blocked by one session and FIFO is not guaranteed. |
| `DV-FEI-006` | blocked-defect | `TestDVFEI006RunnerReturnsWithoutDrainingAcceptedTask`, `TestDVFEI006ProductionEntryHasNoCoordinatedShutdown` | Runner returns on cancellation/channel close while accepted work is active; main uses background context and has no signal, intake stop, gateway shutdown, task-channel close, or drain protocol. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -run 'TestDVFEI' -count=1` | `cdaa566` | Proven defect; shutdown is not coordinated. |
| `DV-FEI-007` | blocked-defect | `TestDVFEI007DuplicateApprovalCanBlockAndResolveTwice` | A duplicate approval blocks on the full result channel, then succeeds after the first result drains; a later response fails only after cleanup. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -run 'TestDVFEI' -count=1` | `cdaa566` | Proven defect; terminal resolution is blocking and not exactly once. |
| `DV-FEI-008` | blocked-defect | `TestDVFEI008CompactorFallsBackInsteadOfUsingProviderModel` | Feishu leaves `CompactionConfig.Model` empty, producing the 128K fallback rather than the selected provider model's 200K window. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -run 'TestDVFEI' -count=1` | `cdaa566` | Proven defect; run and compactor model snapshots diverge. |
| `DV-FEI-009` | blocked-defect | `TestDVFEI009PanicRecoveryEmitsNoTerminalReply` | Panic recovery logs and releases capacity but sends zero correlated terminal messages. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -run 'TestDVFEI' -count=1` | `cdaa566` | Proven defect; accepted task can disappear without a terminal reply. |
| `DV-FEI-010` | blocked-defect | `TestDVFEI010DeliveryFailureIsOnlyLoggedAndNotReturned` | SDK delivery errors are detectable, but reporter reduces them to logs and runner ignores at least six receipt/session/final/failure send errors. | `env GOCACHE=/tmp/fox-go-build-cache go test ./internal/feishu ./cmd/feishu -run 'TestDVFEI' -count=1` | `cdaa566` | Proven defect; controlling adapter receives no terminal delivery failure. |
| `DV-AOP-001` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AOP-002` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AOP-003` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AOP-004` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AOP-005` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-AOP-006` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-BEN-001` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-BEN-002` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-BEN-003` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-BEN-004` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-BEN-005` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-BEN-006` | pending-verification | TBD | TBD | TBD | TBD | |
| `DV-BEN-007` | pending-verification | TBD | TBD | TBD | TBD | |
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
