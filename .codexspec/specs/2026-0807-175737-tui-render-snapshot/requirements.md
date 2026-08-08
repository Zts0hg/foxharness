# Confirmed Requirements: tui-render-snapshot

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0807-175737`
**Status**: Amended during implementation
**Last Confirmed**: 2026-08-08

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: Let an AI coding agent obtain a PNG of the latest code's TUI rendering

- **Status**: confirmed
- **Statement**: Provide a runnable way to render the current code's TUI output to a PNG image on disk, so an AI coding agent can autonomously fetch and visually inspect the actual rendered effect after making TUI-related changes.
- **Rationale**: TUI rendering changes are otherwise unverifiable by the agent; existing tests only assert on `strings.Contains` over `stripANSI(View())`, which cannot reveal layout, colors, or styling as they actually render.
- **User Evidence**: "我只是希望能够有一种方案可以实现让 AI Coding Agent 获取最新代码渲染的 TUI 效果图"
- **Confirmed At**: 2026-08-07

## Constraints

### CON-001: Snapshot frame content must be deterministic

- **Status**: confirmed
- **Statement**: The rendered frame (the ANSI produced by `Model.View()`) must be reproducible: fixed terminal width/height, an injected fixed clock, and a fixed spinner/animation frame, so re-running the command yields the same content regardless of wall-clock time or animation phase.
- **User Evidence**: Verified feasible in code — `Model` exposes an injectable `now func() time.Time` and a `spinnerFrame int` field; `runningElapsed()` reads `m.nowTime()`.

### CON-002: freeze is an external dependency and must degrade gracefully when absent

- **Status**: superseded
- **Replaced By**: CON-004
- **Statement**: (Original) Rendering to PNG shells out to `charmbracelet/freeze`, an external binary; when absent, fail with an install hint. Superseded: freeze was dropped entirely (see DEC-007), so there is no external render binary to degrade around.
- **User Evidence**: `which freeze` → "freeze not found" during discovery.

### CON-004: The rendered artifact must be self-contained (no external render binary or asset)

- **Status**: confirmed
- **Statement**: `fox render` produces a self-contained HTML file with the palette and font embedded. It requires no external render binary (no freeze/VHS/ttyd/ffmpeg) and no external asset fetch at view time; `go build ./...` / `go test ./...` stay independent of any such tool.
- **Rationale**: freeze's renderer (resvg-go) proved unable to render CJK or custom fonts (verified: tofu even when given a font containing the glyphs), and VHS's toolchain could not be installed. A browser is the reliable, universal renderer; embedding the font makes the output portable and reproducible.
- **User Evidence**: "找一个能够显示全部内容的单一字体用于freeze" then, after freeze was proven incapable, "如果换成vhs可以解决也可以"; VHS install blocked → HTML approach confirmed with option "A" (embed the font).

### CON-003: Reuse existing model construction patterns

- **Status**: confirmed
- **Statement**: Scene states are built with the existing `NewModel(ctx, runner, cfg)` constructor plus a fake runner (mirroring current test helpers). The rendering harness lives in the `internal/tui` package's support/render code and does not require a real terminal.
- **User Evidence**: Existing tests construct models this way (`internal/tui/model_test.go`).

## Decisions

### DEC-001: Mechanism is a runnable snapshot/render command, not a test

- **Status**: confirmed
- **Decision**: Deliver a directly-runnable command whose sole job is: build a chosen TUI scene's `Model` → obtain `View()` ANSI → render to PNG via freeze. The AI agent runs the command and reads the resulting PNG.
- **Alternatives Rejected**: Golden snapshot tests driven under `go test` (see DEC-000) — rejected because the core need does not require a test-runner container, regression assertions, or golden comparison.
- **Reason**: The user's objective is simply "get the latest code's rendered TUI image", which needs only the render core exposed as a command.
- **User Evidence**: "为什么会涉及到 go test ? 我只是希望能够有一种方案可以实现让 AI Coding Agent 获取最新代码渲染的 TUI 效果图"

### DEC-002: Output format is PNG

- **Status**: superseded
- **Replaced By**: DEC-008
- **Decision**: (Original) The persisted rendering is a PNG image produced by freeze. Superseded: freeze cannot render CJK/custom fonts, so the persisted artifact became a self-contained HTML file (a browser renders it, and it can be screenshotted to PNG on demand).
- **User Evidence**: User selected "渲染成 PNG 图片"; "我希望能够准确复现直接 TUI 渲染的样式".

### DEC-003: Render PNG via charmbracelet/freeze (external CLI)

- **Status**: superseded
- **Replaced By**: DEC-007
- **Decision**: (Original) Convert the frame's ANSI to PNG using `charmbracelet/freeze`.
- **Why superseded**: During implementation, freeze's `resvg-go` engine proved unable to render CJK or embed custom fonts — it produced tofu boxes for `⬢`, `✦`, and Chinese even when handed a font (Sarasa Mono SC) that contains those glyphs, and rendered a blank frame when a font family was named. It also hung on some system fonts (Monaco). freeze does no font fallback, so it cannot reproduce the real terminal, whose glyphs come from the OS font-fallback chain. VHS (the fallback candidate) could not be installed (ffmpeg/ttyd toolchain download failures).
- **User Evidence**: verified empirically during the 2026-08-08 session; user directed "如果换成vhs可以解决也可以" and then approved the HTML approach.

### DEC-004: On-demand generation for inspection; no strict regression gate

- **Status**: confirmed
- **Decision**: PNGs are generated on demand for the agent to read and eyeball. There is no byte/pixel equality assertion. PNGs MAY optionally be committed for human/AI review.
- **Alternatives Rejected**: Hard PNG byte/hash gate; a parallel ANSI-stripped text golden as a CI gate — both rejected as unnecessary for the stated goal.
- **Reason**: freeze output is not byte-stable across environments, and the goal is visual inspection, not automated regression gating.
- **User Evidence**: User selected "按需生成供我看图".

### DEC-005: Delivery surface is a `fox` subcommand

- **Status**: confirmed
- **Decision**: Expose the command as a `fox` subcommand, e.g. `go run ./cmd/fox render --scene=<name> --out=<path>` (alongside the existing `exec` subcommand).
- **Alternatives Rejected**: A separate standalone tool (e.g. `cmd/foxsnap`) — rejected in favor of reusing the existing binary for discoverability.
- **Reason**: Reuses the existing entrypoint; better discoverability.
- **User Evidence**: User confirmed recommendation "A + 内置固定场景".

### DEC-006: Scenes are built-in fixed fixtures, extensible

- **Status**: confirmed
- **Decision**: The states to render are built-in fixture scenes constructed in code, forming an easily-extensible catalog. Seed scenes: (1) main transcript with tool calls plus user/assistant messages, (2) sidebar / runtime info, (3) the overlay forms — permission, approval, plan, ask, effort, and selector.
- **Alternatives Rejected**: Rendering from a real `session/<id>/transcript.jsonl` was initially deferred; it has since been confirmed as a follow-up (see DEC-010), additive to the fixtures.
- **Reason**: Fixtures are the simplest, deterministic path and directly serve "run after a change to see the current code's effect".
- **User Evidence**: User confirmed recommendation "A + 内置固定场景".

### DEC-007: Render via ANSI → self-contained HTML → browser (replaces freeze)

- **Status**: confirmed
- **Decision**: `fox render` converts the frame's ANSI to a self-contained HTML document (pure Go); a browser renders it faithfully. The browser is the universal, correct renderer — it handles CJK, box-drawing, symbol glyphs, and font fallback, none of which freeze could do.
- **Alternatives Rejected**: freeze (DEC-003) — proven incapable of CJK/custom fonts; VHS — install blocked and still not iTerm2-exact; a single comprehensive font fed to freeze — freeze still tofu'd it.
- **Reason**: Faithful rendering of the real (CJK-heavy) content requires a real font-rendering engine with fallback; the browser provides it with no fragile native binary.
- **User Evidence**: user approved the HTML direction after freeze and VHS were both ruled out.

### DEC-008: Output artifact is a self-contained HTML file (replaces PNG)

- **Status**: confirmed
- **Decision**: The persisted artifact is `<scene>.html`, self-contained (embedded palette + font, no external references). It is inspected by opening in a browser or by screenshotting the browser (e.g. via Playwright) to a PNG on demand.
- **Reason**: HTML defers rendering to the browser (DEC-007); self-containment keeps it portable and reproducible independent of the host's terminal font.
- **User Evidence**: user selected option "A" (embed the font into a self-contained artifact).

### DEC-009: Bake a fixed dark appearance — Monokai Pro palette + embedded Sarasa Mono SC

- **Status**: confirmed
- **Decision**: Recolor the codex theme's 16-color ANSI to the fixed Monokai Pro dark palette (absolute truecolor), use `#2d2a2e` as the background, and embed the Sarasa Mono SC font (WOFF2, SIL OFL 1.1). This gives a deterministic, terminal-independent appearance in which CJK is monospace-aligned and every glyph renders.
- **Alternatives Rejected**: (a) match the user's iTerm2 palette/font at render time — rejected as unreliable (their terminal uses runtime color presets + OS font fallback that cannot be read offline); (b) rely on the browser's system fallback font for CJK — rejected because system CJK fonts are not monospace, drifting alignment.
- **Reason**: The default codex theme is terminal-adaptive (ANSI indices + terminal default bg/fg), which has no single offline appearance; a fixed absolute palette + a complete monospace CJK font makes exports faithful to the code's intent and reproducible everywhere.
- **User Evidence**: "把这套 Monokai Pro 深色调色板 + #2d2a2e 背景作为 fox render 的默认外观固定下来"; option "A".

### DEC-010: Add real-session replay rendering as a follow-up

- **Status**: confirmed
- **Decision**: In addition to the built-in fixtures, `fox render` will support rendering a real session's `transcript.jsonl` (by session id or path) into the same self-contained HTML, reusing the render/HTML core.
- **Reason**: Lets the agent/user render actual conversation content for inspection, not only synthetic fixtures.
- **User Evidence**: "先同步 spec/plan，再加真实 session 回放渲染".

## Out of Scope

### OUT-001: Runtime per-frame dump of real interactive sessions

- **Status**: confirmed
- **Statement**: Capturing a live TUI session's frames to disk during real interactive use is out of scope.
- **Reason**: Non-deterministic and not needed for verifying rendering after code changes.
- **User Evidence**: User chose the snapshot-command direction over "运行时实时转储".

### OUT-002: Strict pixel/byte regression gate in CI

- **Status**: confirmed
- **Statement**: No CI job asserts image equality (byte, hash, or pixel tolerance). The Go tests assert the HTML structure/content deterministically, not a rendered screenshot.
- **Reason**: Goal is visual inspection, not automated pixel regression gating.
- **User Evidence**: User selected "按需生成供我看图".

### OUT-003: Pixel-identical reproduction of a specific terminal

- **Status**: confirmed
- **Statement**: The feature does not reproduce a specific terminal (e.g. the user's iTerm2) pixel-for-pixel. It renders a fixed, self-contained appearance (Monokai palette + Sarasa font) via the browser.
- **Reason**: A terminal-adaptive theme rendered through the OS font-fallback chain has no single offline appearance; the self-contained HTML is browser-reproducible instead (much more stable than the earlier freeze output).

## Open Questions

### OPEN-001: freeze font/theme configuration for best fidelity to the amber theme

- **Status**: resolved
- **Resolution**: Moot — freeze was dropped (DEC-007). Fidelity is now achieved by the browser rendering a self-contained HTML with the embedded Sarasa Mono SC font and the fixed Monokai palette (DEC-008, DEC-009). No freeze font/theme tuning is needed.

## Superseded Entries

### DEC-000: Golden snapshot tests driving View() under go test

- **Status**: superseded
- **Replaced By**: DEC-001
- **Historical Note**: Initially framed (and briefly selected) as "golden 快照测试" run via `go test`. Superseded once the user clarified the goal is simply obtaining the latest code's rendered TUI image — the render core does not need a test-runner container or regression assertions.

## Confirmation Log

### Session 2026-08-07

- **Summary Presented**: Runnable `fox render` subcommand renders built-in fixture TUI scenes to PNG via freeze, with deterministic frame content (fixed size + injected clock + fixed spinner), generated on demand for agent visual inspection; no go test / no regression gate; graceful degradation when freeze is absent.
- **User Confirmation**: "确认" (after clarifying that `go test` is not required and confirming DEC-005 = fox subcommand and DEC-006 = built-in fixed scenes).
- **Entries Confirmed**: NEED-001, CON-001, CON-002, CON-003, DEC-001, DEC-002, DEC-003, DEC-004, DEC-005, DEC-006, OUT-001, OUT-002, OUT-003
- **Superseded**: DEC-000 → DEC-001

### Session 2026-08-08 (implementation amendment)

- **Summary Presented**: During implementation, freeze was found unable to render CJK or embed custom fonts (verified: tofu even with a font that contains the glyphs; blank frame when a font family is named; hangs on some fonts). VHS could not be installed (ffmpeg/ttyd download failures). Pivoted to `ANSI → self-contained HTML → browser`, embedding the Sarasa Mono SC font (WOFF2) and baking a fixed Monokai Pro dark palette + `#2d2a2e` background. Verified via Playwright that `⬢`, `✦`, CJK, colors, layout, and sidebar alignment all render faithfully. No product glyphs were changed (the user's principle: do not alter real styling to suit the renderer).
- **User Confirmation**: chose option "A" (embed Sarasa into a self-contained artifact); then "先同步 spec/plan，再加真实 session 回放渲染".
- **Entries Amended**: CON-002 → superseded by CON-004; DEC-002 → superseded by DEC-008; DEC-003 → superseded by DEC-007; OPEN-001 resolved.
- **Entries Added (confirmed)**: CON-004, DEC-007, DEC-008, DEC-009, DEC-010.
