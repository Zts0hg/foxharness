# Tasks: TUI Render Snapshot

<!--
Language: document language from .codexspec/config.yml (en).
Amended 2026-08-08: mechanism changed from freeze/PNG to self-contained HTML + embedded font
(see requirements DEC-007/008/009, plan PD-003). Groups 1-2 and 6-7 were unaffected; groups 3-4
were reworked for HTML; group 5 (real-session replay) is the outstanding follow-up.
Constitution: new Go code is developed test-first.
-->

## 1. Render Core (TDD) — `internal/tui`

- [x] 1.1 (Red) Add `internal/tui/snapshot_test.go` with failing unit tests for the pure core: `SceneNames()` returns a non-empty ordered set; `RenderSceneANSI(name,w,h)` on an unknown name errors listing available scenes; the same scene rendered twice at the same size is byte-identical; output non-empty. — Covers: REQ-003, NFR-001, NFR-002; Plan: Phase 1
- [x] 1.2 (Green) Add `internal/tui/snapshot.go`: static `snapshotRunner` (16-method `Runner`); ordered scene registry seeded with `transcript`; `SceneNames()`; `RenderSceneANSI` (build model → `WindowSizeMsg` via `Update` → fixed `now` + `spinnerFrame` → `View()`), forcing truecolor via `lipgloss.SetColorProfile`. Block-level doc comments on exported identifiers. — Covers: REQ-003, NFR-001; Plan: Phase 1, PD-001/PD-002/PD-005/PD-006 — deps: 1.1

## 2. Seed Scenes — `internal/tui`

- [x] 2.1 Extend the registry with the remaining seed scenes via in-package constructors: `sidebar`, `permission-form`, `approval-form`, `plan-form`, `ask-form`, `effort-form`, `selector`; `transcript` includes tool-call + user/assistant entries. Per-scene marker test over `stripANSI(View())`. — Covers: REQ-003, DEC-006; Plan: Phase 2, PD-005 — deps: 1.2

## 3. HTML Export (TDD) — `internal/tui`

- [x] 3.1 Add `RecolorForImage` (16-color codex ANSI → fixed Monokai Pro truecolor + pinned default fg) in `snapshot.go`, with a test that palette codes become truecolor and no bare 16-color codes remain. — Covers: REQ-007; Plan: Phase 3, PD-003 — deps: 1.2
- [x] 3.2 Vendor `internal/tui/assets/SarasaMonoSC.woff2` (SIL OFL 1.1) + `assets/README.md`; add `internal/tui/snapshot_html.go` with `RenderSceneHTML` and `frameToHTML` (SGR→`<span>` conversion, `#2d2a2e` bg, font embedded via `go:embed` as a `data:font/woff2` URI). Tests: HTML is self-contained (`@font-face` + `base64,`, no external refs), free of raw ANSI, SGR→CSS mapping, HTML escaping. — Covers: REQ-004, REQ-005, REQ-007, NFR-003, CON-004; Plan: Phase 3, PD-003 — deps: 3.1

## 4. `fox render` Subcommand — `cmd/fox`

- [x] 4.1 Implement the `render` branch in `cmd/fox/main.go` (mirroring `exec`/`config` dispatch): parse `-scene`, `-out`, `-width`, `-height`, `-list`; call `tui.RenderSceneHTML`; write the HTML to `-out` (default `<scene>.html`); `-list` prints scene names. Tests: `-list` output, unknown-scene error (no file written), and a self-contained-HTML-written assertion. — Covers: REQ-001, REQ-002, REQ-004, REQ-005, DEC-005; Plan: Phase 4, PD-004 — deps: 3.2

## 5. Real-Session Replay — `internal/tui` + `cmd/fox`

- [x] 5.1 Add `RenderSessionANSI` / `RenderSessionHTML(records, w, h)` in `internal/tui` (refactor the render core into `renderModelANSI`; build a Model from `[]session.MessageRecord` via the existing `entriesFromMessageHistory`). Test: records render into the same self-contained HTML (incl. CJK). — Covers: REQ-008; Plan: Phase 5, DEC-010
- [x] 5.2 Add `fox render -session <id|dir|messages.jsonl>` (+ `-workdir/-C` to resolve an id): `loadSessionRecords` resolves a directory / `messages.jsonl` path / session id (via `session.Manager.Open`), loads records, renders via the HTML core; missing/unreadable source → non-zero error, no file written. Tests: replay writes HTML with the transcript content; missing source errors and writes nothing. — Covers: REQ-008; Plan: Phase 5, DEC-010 — deps: 5.1

## 6. Documentation

- [x] 6.1 Update the CLI usage text / README for `fox render` HTML output (scenes, flags, examples) and note the embedded Sarasa Mono SC font (OFL). Direct implementation. — Covers: CON-004, REQ-006; Plan: Phase 6 — deps: 4.1

## 7. Verification Checkpoint

- [x] 7.1 Run `gofmt -l .`, `go vet`, `go build ./...`, `go test ./...` — all green on the Go toolchain alone (NFR-002). Rendered `transcript` (and a CJK-injected variant) to HTML and screenshotted via Playwright to confirm colors, layout, CJK, and `⬢` render faithfully (SC-001, SC-005, NFR-003). — Covers: NFR-002, NFR-003; Plan: Verification Strategy — deps: 2.1, 4.1, 6.1
- [x] 7.2 Re-ran the checkpoint after task 5: full suite green; rendered a **real on-disk session** via `fox render -session <dir>` and screenshotted it — actual transcript content (markdown, lists, callouts, code) renders faithfully. — Covers: REQ-008, NFR-003 — deps: 5.2

## Dependencies

```
1.1 -> 1.2 -> 2.1 --------------------------\
          \-> 3.1 -> 3.2 -> 4.1 -> 6.1 ------> 7.1
                              \-> 5.1 -> 5.2 -> 7.2
```

## Coverage Table

| Plan / Requirement | Task(s) |
|--------------------|---------|
| PD-001 (render core) | 1.2 |
| PD-002 (fixed size + clock + spinner) | 1.1, 1.2 |
| PD-003 (ANSI → self-contained HTML + embedded font) | 3.1, 3.2 |
| PD-004 (`fox render` subcommand) | 4.1 |
| PD-005 (scene registry + seed set) | 1.2, 2.1 |
| PD-006 (`snapshotRunner`) | 1.2 |
| REQ-001 | 4.1 |
| REQ-002 | 4.1 |
| REQ-003 | 1.1, 1.2, 2.1 |
| REQ-004 | 3.2, 4.1 |
| REQ-005 | 3.2 |
| REQ-006 | 6.1; (no gate — verified by 7.1) |
| REQ-007 | 3.1, 3.2 |
| REQ-008 | 5.1, 5.2 |
| NFR-001 | 1.1, 1.2 |
| NFR-002 | 1.1, 3.2, 7.1 |
| NFR-003 | 3.2, 7.1 |

## Notes

- Mechanism amended from freeze/PNG to self-contained HTML + embedded Sarasa Mono SC (DEC-007/008/009); no product glyphs changed.
- No pixel regression gate (REQ-006 / OUT-002); Go tests assert HTML structure/content, and 7.1 does a manual browser visual check.
- Task group 5 (real-session replay, REQ-008/DEC-010) is the outstanding work.
