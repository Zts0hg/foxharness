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
- `$codexspec:spec-to-design` to produce `design.md`.
- `$codexspec:spec-to-plan` to produce `plan.md`.
- `$codexspec:plan-to-tasks` to produce `tasks.md`.
- `$codexspec:implement-tasks` to implement approved tasks.
- `$codexspec:distill` to capture reusable cross-feature knowledge into `.codexspec/profile/`.
- `$codexspec:evolve` to contribute vetted profile knowledge back upstream via a reviewed PR.

Before making workflow decisions, read `.codexspec/memory/constitution.md`.
<!-- CODEXSPEC END -->

<!-- CODEXSPEC PROFILE START -->
## CodexSpec Project Profile

**Project constraints (highest priority — read these FIRST):** before any non-trivial work you MUST read every record under `.codexspec/profile/constraints/` — the project's hard prohibitions (严禁 / 仅允许). Honor them before anything else.

**Actively recall relevant knowledge before non-trivial work.** Do not wait to stumble on it: scan the record headings and their `scope/when` (and, for strategies, `trigger`) lines across the categories below, and read every record whose condition matches the task you are about to start (each directory holds one record per file):

- `.codexspec/profile/conventions/` — cross-feature conventions / steering; read before adopting a pattern, structure, or naming choice.
- `.codexspec/profile/pitfalls/` — known traps and their workarounds; read before implementing or debugging in an area that may have bitten before.
- `.codexspec/profile/decisions/` — past cross-feature / architectural decisions; read before deciding in the same area, to reuse prior rationale rather than re-litigate it.
- `.codexspec/profile/strategies/` — metacognitive trigger→action rules (and `scope: self` notes on the agent's own recurring slips); read before choosing an approach or when a fix is not converging.
- `.codexspec/profile/runbooks/` — ordered multi-step procedures with failure recovery; read before carrying out a known multi-step task.

Read the full record — each carries a `status` of `candidate` or `vetted`; weight `candidate` items with appropriate caution. A directory may be empty until `/codexspec:distill` has captured knowledge.

**Capture knowledge as you go.** When this session produces reusable cross-feature knowledge — even in plain chat or a non-SDD fix — run `/codexspec:distill` near that moment rather than only at wrap-up. It is non-blocking and early-exits when there is nothing new.
<!-- CODEXSPEC PROFILE END -->
