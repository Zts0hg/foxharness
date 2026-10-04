# Implementation Issues

## Workspace architecture scans include nested historical worktrees
- **Task**: T001 / baseline verification
- **Error**: The initial `go test ./...` failed only in `internal/architecturetest`; recursive scanners included `.worktrees/` and `.claude/worktrees/` containing unrelated legacy source.
- **Attempted**: Ran the unchanged HEAD in a temporary archive without nested worktrees. `go test ./...` passed. Subsequent full checks copy all tracked and nonignored feature files into that same temporary verification directory.
- **Status**: Workaround Found. Original nested worktrees remain untouched; no architecture assertion is weakened. The original checkout's scan interference remains an environment limitation.


## Configured model did not pass screenshot analysis
- **Task**: T013-S05 / SC-007 / T015
- **Error**: The existing default Fox `zhipu / glm-5.2` profile accepted the Claude-compatible image request but answered blue for the controlled page's orange badge.
- **Attempted**: Repaired the live test's unsupported auth setting, ran the real Playwright integration with the local default profile, independently inspected the orange screenshot, and verified exact screenshot bytes in both provider request fixtures. Asked the user for a configured image-capable model; no credentials are requested in chat.
- **Status**: Resolved on 2026-10-04 using user-selected `glm-5.3-flash` with the existing Zhipu Claude-compatible endpoint and credentials. The model correctly answered orange and the full Playwright acceptance passed. T015 remains pending independent final review.

## Feishu browser continuity across ordinary messages
- **Task**: T012-S03 / T015; review follow-up `FU-CG-002` from target `sha256:e5ed84c41038c5191eef5f95702ed38c39b39001809442244e8f9ae475351479`.
- **Error**: Each Feishu message can reuse the persisted session ID but opens a new live AgentSession; task drain closes its MCP connection. Browser state and the MCP configuration snapshot therefore do not survive ordinary consecutive messages.
- **Attempted**: Verified the factory/runner lifecycle against REQ-006/013, DEC-005/006 and the design's distinction between persisted records and live sessions. Requested an explicit choice between keeping browser state until a new session/service shutdown, and retaining task-local connections with a clear notice. The independent text-resource defect has been repaired and all deterministic checks pass.
- **Status**: Resolved by the user's explicit option 1 on 2026-10-04, recorded as confirmed DEC-010. Feishu now retains one live AgentSession per chat/user and closes it on explicit replacement/service shutdown. Cross-message identity/configuration, separate approvals/artifacts, cancellation/failure, user isolation, replacement and cleanup propagation tests pass. Fresh complete-feature review and T015 completion remain pending.
