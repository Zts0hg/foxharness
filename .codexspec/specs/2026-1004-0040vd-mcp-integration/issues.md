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
