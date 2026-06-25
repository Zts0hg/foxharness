# AGENTS.md

## Commands

- Run all tests with `go test ./...`.
- Format Go files with `gofmt -w`.

## Rules

- Do not edit files under `vendor/`.
- Prefer edit_file over write_file when changing existing code.

<!-- CODEXSPEC START -->
## CodexSpec

This project uses CodexSpec for requirements-first spec-driven development.

Use these Codex skills when working on CodexSpec workflows:

- `$codexspec:constitution` to create or update project principles.
- `$codexspec:specify` to capture confirmed requirements.
- `$codexspec:generate-spec` to produce `spec.md`.
- `$codexspec:spec-to-plan` to produce `plan.md`.
- `$codexspec:plan-to-tasks` to produce `tasks.md`.
- `$codexspec:implement-tasks` to implement approved tasks.

Before making workflow decisions, read `.codexspec/memory/constitution.md`.
<!-- CODEXSPEC END -->
