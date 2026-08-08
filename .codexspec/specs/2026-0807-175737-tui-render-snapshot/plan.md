# Design Document: TUI Render Snapshot

<!--
Language: document language from .codexspec/config.yml (en).
Constrained technical design derived from spec.md; preserves confirmed intent.
-->

## Context

The `fox` TUI (`internal/tui`) is a Bubble Tea program whose entire frame is the pure function
`Model.View() string`. `NewModel(ctx, runner, cfg)` returns a `Model` defaulting to `width: 96,
height: 28` and `now: time.Now`; size is later adjusted by `tea.WindowSizeMsg`
(`m.width = max(msg.Width, minWidth)`, `m.height = max(msg.Height, 1)`). Transcript items are
`entry` values in `m.entries`; overlays (`askForm`, `planForm`, `permissionForm`, `approvalForm`,
`effortForm`, `rewindSelector`) and the animation state (`spinnerFrame int`, clock `now`) are
**unexported** package fields. `cmd/fox/main.go` dispatches subcommands off `os.Args[1:]`
(`exec`, `config`). The `Runner` interface has 16 methods; tests build models with an unexported
`fakeRunner` (`internal/tui/model_test.go`). (Historical: the original plan used `charmbracelet/freeze`;
it was dropped during implementation — see the Design amendment under Goals.)

## Goals / Non-Goals

**Goals:**

- Provide a `fox render` subcommand that renders a chosen built-in TUI scene — or a real session's transcript — to a self-contained HTML file (REQ-001, REQ-008, DEC-005, DEC-008).
- Reproduce the real render faithfully in a browser, including CJK and symbol glyphs; deterministic frame content (NFR-001, NFR-003).
- Require no real terminal and no external render binary; keep `go build`/`go test` on the Go toolchain only (NFR-002).

**Non-Goals:**

- Live-session frame dumping (OUT-001), CI pixel/byte gate (OUT-002), pixel-identical reproduction of a specific terminal (OUT-003), changing the TUI's real glyphs.

> **Design amendment (2026-08-08)**: The original plan rendered PNG via `charmbracelet/freeze`.
> During implementation freeze's `resvg-go` proved unable to render CJK or embed custom fonts
> (tofu even with a complete font; blank frame when a family is named; hangs on some fonts), and
> VHS could not be installed. The mechanism was changed to **ANSI → self-contained HTML → browser**
> (DEC-007/008/009). PD-003 below is rewritten accordingly; PD-001/002/004/005/006 are unchanged.

## Decisions

### Decision PD-001: Render core lives inside package `internal/tui` (non-test file)

**Context**: Scene states require unexported internals — `now`, `spinnerFrame`, `entries`, and the
overlay form constructors. A `cmd/fox` subcommand (production code) must call this, so it cannot
live in `_test.go`.
**Decision**: Add a new non-test file `internal/tui/snapshot.go` exporting a minimal API:
`SceneNames() []string` and `RenderSceneANSI(name string, width, height int) (string, error)`.
The exported core returns the ANSI frame only — it does not shell out.
**Rationale**: Keeps freeze/exec out of the pure, unit-testable core (NFR-002) and honors CON-003
(reuse `NewModel`, harness inside the tui package). Covers REQ-003, NFR-001, CON-003.

### Decision PD-002: Deterministic frame via fixed size + injected clock + fixed spinner

**Context**: `View()` output must be reproducible (CON-001), but `now: time.Now` and `spinnerFrame`
are time/animation dependent.
**Decision**: In `RenderSceneANSI`, after building the scene's `Model`: (a) drive one
`tea.WindowSizeMsg{Width, Height}` through `Update` to set layout, (b) override `m.now` with a fixed
`time.Time`, (c) set `m.spinnerFrame` to a fixed value, then call `View()`.
**Rationale**: All three knobs are in-package and already exist; no source change to the TUI runtime.
Covers NFR-001, CON-001.

### Decision PD-003: Render to a self-contained HTML document; the browser is the renderer

**Context**: DEC-007/008/009 replace freeze. Faithful rendering of CJK/symbol glyphs needs a real
font-rendering engine with fallback; the browser provides it with no native binary.
**Decision**: Add `internal/tui/snapshot_html.go` with `RenderSceneHTML(name, w, h)`:
`RenderSceneANSI` → `RecolorForImage` (16-color codex ANSI → fixed Monokai Pro truecolor + pinned
default fg) → `frameToHTML` (parse the SGR subset lipgloss emits into `<span>`s, wrap in a `<pre>`
with the `#2d2a2e` background and the Sarasa Mono SC font embedded as a `data:font/woff2` URI). The
font is vendored at `internal/tui/assets/SarasaMonoSC.woff2` and pulled in with `go:embed`. `cmd/fox`
just writes the HTML string to `-out`.
**Rationale**: Pure-Go, self-contained (CON-004), deterministic; the browser renders every glyph
(CON-004, DEC-009). No `os/exec`, no `LookPath`, no external tool. Covers REQ-004, REQ-005, REQ-007,
NFR-002, NFR-003.
**Alternatives rejected**: freeze (can't do CJK/custom fonts — verified); VHS (install blocked);
system-fallback CJK font (not monospace → alignment drift).

### Decision PD-004: Expose as a `fox render` subcommand mirroring `exec`

**Context**: DEC-005 = a `fox` subcommand.
**Decision**: Add a `render` branch in `cmd/fox/main.go` with flags `-scene`, `-out`, `-width`,
`-height`, and `-list` (enumerate scenes). Defaults: a representative scene, a default output path,
and a fixed default size.
**Rationale**: Follows the existing `exec`/`config` dispatch pattern (reuse before new abstraction).
Covers REQ-001, REQ-002, DEC-005.

### Decision PD-005: Scenes are an ordered in-package registry seeded with the required set

**Context**: DEC-006 requires an extensible fixture catalog with a defined seed set.
**Decision**: An ordered `[]scene{name, build func() Model}` registry in `snapshot.go`, seeded with:
`transcript`, `sidebar`, `permission-form`, `approval-form`, `plan-form`, `ask-form`,
`effort-form`, `selector`. Adding a scene = one registry entry. Unknown name → error listing
available names.
**Rationale**: Smallest extensible structure; drives the unknown-scene error behavior (spec Story 1
/ Scenario 3). Covers REQ-003, DEC-006.

### Decision PD-006: A minimal in-package static `Runner` for fixtures

**Context**: Scene builders call `NewModel`, which needs a `Runner`; the existing `fakeRunner` is
test-only and not compiled into the binary.
**Decision**: Add a small non-test `snapshotRunner` in `snapshot.go` implementing the 16-method
`Runner` interface with static values (mirroring `fakeRunner`'s shape).
**Rationale**: Localized (~40 lines of stubs), avoids a broad refactor of test code.
**Alternative considered**: Extract the existing `fakeRunner` into shared non-test support and reuse
it in both tests and snapshots — rejected for now as a larger blast radius; may be revisited if
duplication grows. Covers REQ-003, CON-003.

## Architecture

```
cmd/fox/main.go                 internal/tui/snapshot.go + snapshot_html.go
  render subcommand               SceneNames() []string
    ├─ parse -scene/-out/-w/-h    RenderSceneANSI(name,w,h) → codex 16-color ANSI
    │        /-list /-session       (scene registry: name → build() Model → View())
    ├─ tui.RenderSceneHTML(...)   RecolorForImage(ansi) → Monokai truecolor
    │        (or from a session)  frameToHTML(ansi): SGR → <span>s in a <pre>,
    │                               + #2d2a2e bg + Sarasa Mono SC (go:embed woff2, data:)
    └─ os.WriteFile(-out, html)   → self-contained HTML; open in a browser to view
```

<!-- Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-007, REQ-008, NFR-001, NFR-002, NFR-003 -->

## Components

- **`internal/tui/snapshot.go`**: scene registry, `snapshotRunner`, `SceneNames`, `RenderSceneANSI`,
  and `RecolorForImage` (16-color → Monokai truecolor). Pure, no exec. Covers REQ-003, REQ-007, NFR-001, CON-001, CON-003.
- **`internal/tui/snapshot_html.go`**: `RenderSceneHTML`, `frameToHTML`, the SGR→HTML span converter,
  and the `go:embed` of the Sarasa Mono SC WOFF2. Covers REQ-004, REQ-005, REQ-007, NFR-003, CON-004.
- **`internal/tui/assets/SarasaMonoSC.woff2`** (+ `README.md`): the embedded font (SIL OFL 1.1).
- **`internal/tui/snapshot_test.go` / `snapshot_html_test.go`**: determinism, styling-preserved,
  recolor, scene markers, HTML self-containment, SGR→CSS, escaping. Covers NFR-001, NFR-003, REQ-003.
- **`cmd/fox/main.go`** (edit): `render` subcommand — flag parsing, `RenderSceneHTML`, write HTML,
  `-list`. Covers REQ-001, REQ-002, REQ-004, DEC-005.
- **Docs** (README / usage text): `fox render` HTML examples + embedded-font note. Supports CON-004.

## Implementation Phases

1. **Render core (TDD)** — `snapshotRunner`, scene registry, `SceneNames`, `RenderSceneANSI` with
   fixed size/clock/spinner. Tests first: repeat-call byte-equality (NFR-001), unknown-scene error.
   Covers REQ-003, NFR-001, CON-001/003.
2. **Seed scenes** — the eight seed states; per-scene marker test over `stripANSI(View())`. Covers REQ-003, DEC-006.
3. **HTML export** — `RecolorForImage` (Monokai) + `frameToHTML` + embedded Sarasa WOFF2; tests for
   truecolor mapping, HTML self-containment, SGR→CSS, escaping. Covers REQ-004, REQ-005, REQ-007, NFR-003, CON-004.
4. **`fox render` subcommand** — flags, `tui.RenderSceneHTML`, write HTML, `-list`. Covers REQ-001, REQ-002, DEC-005.
5. **Real-session replay** — load a session's `transcript.jsonl` (by id/path) into a Model and render
   the same HTML. Covers REQ-008, DEC-010.
6. **Docs** — `fox render` HTML usage + embedded-font note. Supports CON-004.

## Verification Strategy

- **Unit (go test, Go toolchain only)**: `SceneNames` returns the seed set; `RenderSceneANSI` is
  byte-identical across two calls at the same size (NFR-001); per-scene marker present; unknown scene
  → error listing names; `RecolorForImage` maps 16-color → Monokai truecolor with no bare 16-color
  codes; `RenderSceneHTML` is self-contained (`@font-face` + `data:font/woff2;base64,`, no external
  refs) and free of raw ANSI; SGR→CSS mapping and HTML escaping. Confirms NFR-002 (no external binary).
- **Manual/agent (browser)**: run `fox render -scene=transcript -out=x.html`, open in a browser (or
  screenshot via Playwright), confirm colors, layout, and glyphs — including CJK and `⬢` — render
  faithfully (SC-001, SC-005, NFR-003).
- **Constitution**: core developed test-first; block-level doc comments on exported identifiers.

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| Self-contained HTML embeds a ~5.3 MB WOFF2 → ~7 MB per file | Larger artifacts | Acceptable for occasional on-demand renders; WOFF2 already halves the TTF; could subset later |
| Embedded font is not the user's exact terminal font | Not pixel-identical to their iTerm2 | Accepted (OUT-003); a complete monospace CJK font renders faithfully and reproducibly |
| Browser needed to view/screenshot | Consumer-side dependency | Not a build/run dependency of `fox render`; any browser or Playwright works |
| Overlay scenes depend on unexported form constructors | A form refactor may break a scene builder | Scenes are localized in one file; a failing scene test flags it immediately |
| `snapshotRunner` duplicates `fakeRunner` shape | Minor duplication | Accept for now; PD-006 alternative (extract shared) if it grows |

## Implementation Notes

- Do not modify TUI runtime behavior or glyphs; snapshot code only reads/constructs state.
- `RenderSceneANSI` returns the raw codex frame; `RecolorForImage` + `frameToHTML` are the export layer.
- The font is a build-time embedded asset (`go:embed`), not a runtime dependency.

## Plan-Level Decisions Summary

| ID | Decision | Covers |
|----|----------|--------|
| PD-001 | Render core in `internal/tui/snapshot.go` (non-test) | REQ-003, NFR-001, CON-003 |
| PD-002 | Deterministic via fixed size + injected clock + fixed spinner | NFR-001, CON-001 |
| PD-003 | ANSI → self-contained HTML (embedded Monokai + Sarasa font); browser renders | REQ-004, REQ-005, REQ-007, NFR-002, NFR-003, CON-004 |
| PD-004 | `fox render` subcommand mirroring `exec` | REQ-001, REQ-002, DEC-005 |
| PD-005 | Ordered in-package scene registry + seed set | REQ-003, DEC-006 |
| PD-006 | Minimal in-package `snapshotRunner` | REQ-003, CON-003 |

## Requirements Coverage

| Spec Requirement | Plan Coverage |
|------------------|---------------|
| REQ-001 | PD-004; Phase 4; Architecture |
| REQ-002 | PD-004 (flags, `-list`); Phase 4 |
| REQ-003 | PD-001, PD-005, PD-006; Phases 1–2 |
| REQ-004 | PD-003; Phase 3 |
| REQ-005 | PD-003 (self-contained, embedded font); Phase 3; Verification (self-containment test) |
| REQ-006 | Non-Goals + Verification (no gate); OUT-002 |
| REQ-007 | PD-003 (`RecolorForImage` + embedded Sarasa); Phase 3 |
| REQ-008 | Phase 5 (real-session replay) |
| NFR-001 | PD-002; Phase 1; Verification (byte-equality) |
| NFR-002 | PD-001, PD-003; Verification (no external binary) |
| NFR-003 | PD-003 (browser + embedded font); manual browser verification |
