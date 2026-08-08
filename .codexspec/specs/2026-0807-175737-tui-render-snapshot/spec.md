# Feature Specification: TUI Render Snapshot

<!--
Language: Generated in the document language configured in .codexspec/config.yml (en).
Compiled from requirements.md. Do not introduce product decisions absent from confirmed entries.
-->

**Feature Branch**: `2026-0807-175737-tui-render-snapshot`
**Created**: 2026-08-07
**Status**: Draft
**Input**: Confirmed requirements record `.codexspec/specs/2026-0807-175737-tui-render-snapshot/requirements.md`

## Context

The `fox` TUI is built on Bubble Tea + Lipgloss, where the whole interface is a pure
function `Model.View() string` that returns the ANSI frame about to be painted. Today an
AI coding agent editing TUI rendering code cannot see the result: existing tests only assert
`strings.Contains` over `stripANSI(View())`, which cannot reveal layout, colors, or styling
as they actually render. This feature gives the agent a runnable way to obtain a faithful,
self-contained HTML rendering of the current code's TUI, so it can autonomously inspect the effect after a change.

## Goals

- Let an AI coding agent obtain, on demand, a faithful rendering (self-contained HTML) of the latest code's TUI for a chosen scene or a real session.
- Reproduce the real rendering faithfully (truecolor, SGR styles, box-drawing, and every glyph including CJK), not an approximation or tofu.
- Keep frame content deterministic so repeated runs are comparable.
- Require no real terminal, no test-runner container, and no external render binary.

## Non-Goals

See [Out of Scope](#out-of-scope). In short: no runtime dumping of live sessions, no CI pixel/byte
regression gate, and no cross-platform pixel-identical guarantee.

## User Scenarios & Testing

### User Story 1 - Visually verify a TUI change (Priority: P1)

After editing TUI rendering code, the agent runs one command to render a representative scene to a
self-contained HTML file and then opens it in a browser (or screenshots it) to confirm the change looks right.

**Why this priority**: This is the core need (NEED-001); without it the agent cannot verify TUI rendering at all.

**Independent Test**: Run the render subcommand for the `transcript` scene against the current code, then
open the produced HTML in a browser and confirm it reflects the intended layout/colors/glyphs — delivered with no other stories present.

**Acceptance Scenarios**:

1. **Given** the current code, **When** the agent runs the render subcommand for a valid scene with an output path, **Then** a self-contained HTML file is written at that path showing that scene's `View()` output with its real colors, glyphs (including CJK), and layout when opened in a browser.
2. **Given** the same scene and terminal size, **When** the command is run twice, **Then** the underlying rendered frame content is identical both times (deterministic).
3. **Given** an unknown scene name, **When** the command runs, **Then** it exits non-zero with a message listing the available scene names, and writes no output file.

---

### User Story 2 - Add a scene for the view being changed (Priority: P2)

When the agent changes a view that is not yet covered, it adds a new fixture scene so that view can be rendered.

**Why this priority**: Extensibility keeps the tool useful across arbitrary TUI changes (DEC-006), but the seed set already covers the common cases.

**Independent Test**: Add one fixture entry for a new scene, run the command for it, and confirm an HTML file is produced — without modifying the render/CLI plumbing.

**Acceptance Scenarios**:

1. **Given** a new fixture scene added to the catalog, **When** the agent lists or renders scenes, **Then** the new scene is selectable and renders without changes to the command wiring.

---

### User Story 3 - Render a real session for inspection (Priority: P3)

The agent (or user) renders an actual session's `transcript.jsonl` into the same self-contained HTML to inspect how real content looks.

**Why this priority**: Fixtures verify code changes; replaying a real session verifies how the current code renders actual conversation content (DEC-010). Additive to the fixtures.

**Independent Test**: Point the command at a session id or a `transcript.jsonl` path and confirm a self-contained HTML rendering of that session's transcript is produced.

**Acceptance Scenarios**:

1. **Given** a valid session id or transcript path, **When** the render command runs, **Then** it writes a self-contained HTML rendering the session's messages (user/assistant/tool) with real colors and glyphs.
2. **Given** a missing or unreadable session/transcript, **When** the command runs, **Then** it exits non-zero with a clear error and writes no output file.

### Edge Cases

- Unknown scene name → non-zero exit listing available scenes; no file written (Story 1, Scenario 3).
- Missing/unreadable session or transcript → clear error, non-zero exit, no file written (Story 3).
- Output path unwritable (bad directory/permissions) → clear error, non-zero exit, no partial-success message.
- Terminal size below the TUI minimum (`minWidth`/`minHeight`) → render still produces the same degraded frame the TUI itself would show at that size (no special-casing beyond what `View()` already does).

## Requirements

### Functional Requirements

- **REQ-001**: The `fox` binary MUST provide a `render` subcommand whose job is to render a selected TUI scene's `Model.View()` output to a self-contained HTML file on disk.
  - Sources: NEED-001, DEC-001, DEC-005, DEC-008
- **REQ-002**: The render subcommand MUST accept a scene selector, an output path, and terminal width/height, and MUST be able to report the set of available scene names.
  - Sources: NEED-001, DEC-005, CON-001
- **REQ-003**: Scenes MUST be built-in fixtures constructed via the existing `NewModel(ctx, runner, cfg)` plus a fake runner, forming an easily-extensible catalog. The seed catalog MUST include: (1) the main transcript with tool calls plus user/assistant messages, (2) the sidebar / runtime info, and (3) the overlay forms — permission, approval, plan, ask, effort, and selector.
  - Sources: DEC-006, CON-003
- **REQ-004**: The subcommand MUST convert the frame's ANSI to a self-contained HTML document (pure Go) that a browser renders faithfully. It MUST NOT depend on an external render binary (freeze/VHS/ttyd/ffmpeg).
  - Sources: DEC-007, DEC-008, CON-004
- **REQ-005**: The HTML MUST be self-contained: it MUST embed the color palette and the font (as a `data:` URI), with no external stylesheet, font, or asset reference, so any browser renders it identically offline.
  - Sources: DEC-008, DEC-009, CON-004
- **REQ-006**: Snapshots MUST be generated on demand for visual inspection only. The feature MUST NOT introduce any byte/hash/pixel equality assertion or CI gate over rendered images. Committing produced artifacts for review is permitted but not required.
  - Sources: DEC-004
- **REQ-007**: The render MUST bake a fixed, terminal-independent appearance: the codex theme's 16-color ANSI MUST be recolored to the Monokai Pro dark palette (absolute truecolor) over a `#2d2a2e` background, and the embedded font MUST be Sarasa Mono SC so CJK, box-drawing, and symbol glyphs render with correct monospace metrics.
  - Sources: DEC-009, CON-004
- **REQ-008**: The subcommand MUST also render a real session's transcript (selected by session id or an explicit `transcript.jsonl` path) into the same self-contained HTML, reusing the render/HTML core.
  - Sources: DEC-010

### Non-Functional Requirements

- **NFR-001**: Rendered frame content (the ANSI from `View()`) MUST be deterministic for a given scene and terminal size — achieved by fixing the terminal dimensions, injecting a fixed clock (`Model.now`), and fixing the spinner/animation frame (`Model.spinnerFrame`).
  - Sources: CON-001
- **NFR-002**: The feature MUST NOT add a build-time or run-time dependency on any external render binary; `go build ./...` and `go test ./...` MUST pass with nothing beyond the Go toolchain, and producing HTML MUST need no external tool.
  - Sources: CON-004
- **NFR-003**: The rendered artifact MUST faithfully preserve the real rendering — truecolor, SGR styles, box-drawing, and every glyph including CJK — rather than an approximation or tofu. Fidelity is delivered by the browser rendering the embedded-font HTML.
  - Sources: NEED-001, DEC-007, DEC-009

### Key Entities

- **Scene**: A named, deterministic fixture describing a TUI state to render (e.g., `transcript`, `sidebar`, `permission-form`). Maps a name to a builder that returns a `Model` at that state.
- **Rendered frame**: The ANSI string returned by `Model.View()` for a scene (or a replayed session) at a fixed size and clock, recolored to the Monokai palette for export.
- **Snapshot document**: The self-contained HTML produced from the frame, embedding the Monokai palette and the Sarasa Mono SC font; a browser renders it, and it may be screenshotted to PNG on demand.

## Success Criteria

### Measurable Outcomes

- **SC-001**: After editing TUI rendering code, the agent obtains a self-contained HTML rendering the new code with a single command invocation and no real terminal.
- **SC-002**: Re-running the same scene at the same size yields byte-identical rendered frame content.
- **SC-003**: Adding a new renderable scene requires only adding one fixture entry to the catalog (no changes to CLI/render plumbing).
- **SC-004**: The command needs nothing beyond the Go toolchain to produce HTML, and `go build ./...` / `go test ./...` stay green with no external render binary.
- **SC-005**: The HTML renders CJK, box-drawing, and symbol glyphs (e.g. `⬢`, `✦`) correctly in a browser — no tofu.

## Out of Scope

- **OUT-001**: Runtime per-frame dumping of a *live interactive* session to disk. Reason: non-deterministic and unnecessary. (Rendering a *persisted* transcript on demand is in scope — REQ-008.)
- **OUT-002**: A strict pixel/byte/hash regression gate in CI. Reason: the goal is visual inspection; Go tests assert HTML structure/content deterministically instead.
- **OUT-003**: Pixel-identical reproduction of a specific terminal (e.g. the user's iTerm2). Reason: a terminal-adaptive theme through the OS font-fallback chain has no single offline appearance; a fixed self-contained appearance is rendered instead.
- **Changing the TUI's real glyphs** to suit the renderer. Reason: the snapshot must reflect real styling, not drive it (user principle).

## Open Questions

> Open items remain questions. They MUST NOT be treated as confirmed requirements.

- None. (OPEN-001 resolved — freeze was dropped; see DEC-007.)

## Assumptions

- The subcommand name is `render` and its flags (scene selector or session/transcript source, output path, width/height) follow the existing `fox exec` subcommand's CLI style. This is a presentation assumption; exact flag names are settled during planning.
- When invoked without an explicit output path, a sensible default path is used. This is a convenience assumption only and introduces no new product decision.

## Dependencies

- Existing `internal/tui` `NewModel` constructor and the fake-runner test pattern for building scene states, plus `session.MessageRecord` loading for transcript replay (REQ-008).
- `charmbracelet/x/ansi` (already a project dependency) for handling `View()` output.
- Embedded font: **Sarasa Mono SC** (WOFF2, SIL OFL 1.1), vendored under `internal/tui/assets/` and embedded via `go:embed` — a build-time asset, not a runtime dependency.
- A browser (or a headless screenshot tool such as Playwright) to view/screenshot the HTML — consumer-side, not a build/run dependency of `fox render`.

## Requirements Traceability

| Confirmed Requirement | Spec Coverage | Notes |
|-----------------------|---------------|-------|
| NEED-001 | REQ-001, NFR-003; SC-001; Story 1 | Core goal: agent obtains a faithful render of the current TUI |
| CON-001 | REQ-002, NFR-001; SC-002; Story 1/Scenario 2 | Deterministic frame content |
| CON-003 | REQ-003; SC-003; Story 2 | Reuse NewModel + fake runner; harness in tui support code |
| CON-004 | REQ-004, REQ-005, REQ-007, NFR-002 | Self-contained; no external render binary |
| DEC-001 | REQ-001 | Mechanism is a runnable command, not a test |
| DEC-004 | REQ-006; OUT-002 | On-demand inspection; no regression gate |
| DEC-005 | REQ-001, REQ-002; Assumptions | Delivered as a `fox` subcommand |
| DEC-006 | REQ-003; Story 2 | Built-in extensible fixture scenes + seed set |
| DEC-007 | REQ-004, NFR-002, NFR-003 | ANSI → self-contained HTML → browser (replaces freeze) |
| DEC-008 | REQ-001, REQ-005, REQ-006 | Self-contained HTML artifact (replaces PNG) |
| DEC-009 | REQ-007, NFR-003 | Fixed Monokai palette + embedded Sarasa Mono SC |
| DEC-010 | REQ-008 | Real-session transcript replay (follow-up) |
| OUT-001 | Out of Scope | Live per-frame dumping excluded |
| OUT-002 | Out of Scope; REQ-006 | No pixel/byte CI gate |
| OUT-003 | Out of Scope | No pixel-identical reproduction of a specific terminal |
| CON-002 (superseded) | — | Freeze dropped; see CON-004 |
| DEC-002/DEC-003 (superseded) | — | See DEC-008 / DEC-007 |
| OPEN-001 (resolved) | — | Moot after freeze dropped |
