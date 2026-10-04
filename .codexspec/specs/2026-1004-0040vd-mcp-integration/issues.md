# Implementation Issues

## Workspace architecture scans include nested historical worktrees
- **Task**: T001 / baseline verification
- **Error**: The initial `go test ./...` failed only in `internal/architecturetest`; recursive scanners included `.worktrees/` and `.claude/worktrees/` containing unrelated legacy source.
- **Attempted**: Ran the unchanged HEAD in a temporary archive without nested worktrees. `go test ./...` passed. Subsequent full checks copy all tracked and nonignored feature files into that same temporary verification directory.
- **Status**: Workaround Found. Original nested worktrees remain untouched; no architecture assertion is weakened. The original checkout's scan interference remains an environment limitation.
