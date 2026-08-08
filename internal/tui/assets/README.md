# Embedded fonts

`SarasaMonoSC.woff2` is **Sarasa Mono SC** (更纱黑体) by be5invis, licensed under the
SIL Open Font License 1.1 (https://github.com/be5invis/Sarasa-Gothic).

It is embedded (via `go:embed`) so `fox render --format html` produces self-contained,
browser-faithful snapshots that render CJK, box-drawing, and symbol glyphs correctly —
something a single Latin font (or freeze's renderer) cannot do.
