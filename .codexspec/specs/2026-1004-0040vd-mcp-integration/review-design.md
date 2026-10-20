# Design Review Report

## Summary

- **Overall Status**: PASS
- **Compatibility Score**: 100/100
- **Authority Mode**: Requirements-first
- **Readiness**: Ready for Planning
- **Review Rounds**: 1 initial review; 0 automatic fix rounds.
- **Evidence**: Confirmed requirements and specification; constitution; actual profile resolution and child narrowing, session create/open/close, tool registry and permission decoration, result and provider encoding, process-tree ownership; inspected pinned SDK v1.8.0 transport, client, and OAuth APIs.
- **Limit**: Document review only; no implementation or runtime acceptance is claimed.

## Requirement Coverage

| Requirement | Design Reference | Result |
|---|---|---|
| REQ-001 | C03, C05, DD01 | Covered |
| REQ-002 | C03, DD01–02 | Covered |
| REQ-003 | C01, DD03, configuration contract | Covered |
| REQ-004 | C01, DD03 | Covered |
| REQ-005 | C01, C08, management contract | Covered |
| REQ-006 | C04, DD03–04 | Covered |
| REQ-007 | C02–03, DD07 | Covered |
| REQ-008 | C02, C08, DD07 | Covered |
| REQ-009 | C02, C08, DD07 | Covered |
| REQ-010 | C04–05, C09, DD05 | Covered |
| REQ-011 | C04–05, DD05 | Covered |
| REQ-012 | C04–05, DD04–05 | Covered |
| REQ-013 | C04, DD04 | Covered |
| REQ-014 | C03–04, C08, DD08 | Covered |
| REQ-015 | C01, C03–04, C08 | Covered |
| REQ-016 | C03–04, DD02, DD04, DD07 | Covered |
| REQ-017 | C05–06, DD06, DD08 | Covered |
| REQ-018 | C03, C05–06 | Covered |
| REQ-019 | C06–07, DD06 | Covered |
| REQ-020 | C06–07, DD06 | Covered |
| REQ-021 | C06–07, DD06 | Covered |
| REQ-022 | C05–06, DD08 | Covered |
| REQ-023 | C09, DD09 | Covered |
| REQ-024 | C03, C09, DD01, DD09 | Covered |
| NFR-001 | Graph, C04, C07, C09 | Covered |
| NFR-002 | C04, C06–07, C09, DD05 | Covered |
| NFR-003 | C04, DD08 | Covered |
| NFR-004 | C02, DD07 | Covered |
| NFR-005 | C09, DD09 | Covered |
| NFR-006 | C06–07, DD06 | Covered |

## Verified Defects

### Critical

None.

### Warnings

None.

### Minor

None.

## Review Reasoning

- New package paths are explicitly proposed; existing paths and SDK symbols were inspected. The three SDK transports and OAuth token-source hooks exist at the pinned version.
- A consumer-owned optional runtime service preserves engine/package boundaries. The design explicitly covers both new and reopened live sessions, immutable external capability extension, child borrowing, and owner cleanup.
- Read-only child exclusion for unknown-effect remote operations preserves CON-001; writable authorized children still support the confirmed page handoff. No annotation is promoted into authority.
- Whole-definition override, delayed configuration activation, localized failures, serial calls, explicit reconnect, uncertain-call no-replay, and non-image fallback match confirmed decisions.
- Rich content crosses the entire text-oriented pipeline. OpenAI's supplemental image message is placed after a complete tool-result group; Anthropic uses correlated result blocks. Old persisted records keep their existing representation.
- OPEN-001 through OPEN-005 are resolved as delegated implementation choices; the standalone Resources/Prompts/server exclusions remain intact.

## Risk Advisories

- Prove subprocess tree cleanup and OAuth refresh under cancellation with short injected deadlines; SDK defaults alone do not establish Fox's lifecycle guarantee.
- Test message copying, projection, compaction, and persistence with image parts; provider request tests alone do not establish end-to-end durability.
- The SDK's OAuth client surface is experimental. Pinning and wrapper tests are necessary evidence; the design does not assume every remote provider implements identical registration behavior.

## Design Opportunities

None required for planning.

## Score Derivation

- Critical root causes: 0
- Warning root causes: 0
- Minor root causes: 0
- Formula: no verified defects = 100. Advisories are non-scoring.
