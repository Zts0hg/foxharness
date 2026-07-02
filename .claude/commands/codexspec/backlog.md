---
description: 把一个想法经 specify 式确认后，追加成一条 BACKLOG.md 需求条目（供 autodev 消费）
argument-hint: "描述初始想法（作为讨论种子，可留空）"
---

# Backlog Item Authoring

## Language Preference

Read `.codexspec/config.yml`. Two independent language controls apply (each falls back to `language.output`, then English):

- **Interaction language** (`language.interaction`): language for all conversation with the user.
- **Document language** (`language.document`): language for generated artifact files.

Converse in the interaction language and author the backlog entry's `Description` in the document language. Apply the project's translation standard to both.

## User Input

`$ARGUMENTS` — the seed need (may be empty).

## Goal

Turn the user's idea into **one** `BACKLOG.md` entry whose content is a **user-confirmed** requirement, so that `autodev` (which treats each backlog item's `Description` as the confirmed input for its Requirements-first SDD pipeline) starts from genuinely confirmed material.

This is the backlog-authoring sibling of `/codexspec:specify`. It reuses specify's discovery-and-confirmation methodology but **changes the output target**:

- Do NOT create a CodexSpec feature workspace.
- Do NOT create a git branch.
- Do NOT write `requirements.md`.
- Do NOT auto-advance to `/codexspec:generate-spec`.

The single output is one appended backlog entry.

## Backlog Target Resolution

Resolve the file to append to, mirroring `autodev`:

1. If `.foxharness/autodev.yml` exists and sets `backlog_file`, use that path.
2. Otherwise use `BACKLOG.md` at the repository root.
3. On any trouble reading/parsing the config, default to `BACKLOG.md` (never fail just because the config is malformed).
4. If the target file does not exist, create it with a single `# Backlog` header line followed by a blank line before appending.

`autodev` reads the same key/default (`backlog_file`, default `BACKLOG.md`), so what you append is exactly what `autodev` will drain.

## Discovery & Confirmation

Run a `/codexspec:specify`-style discovery to refine the raw idea into a confirmed requirement. Use the existing `ask_user_question` tool for structured choices; do not ask in free-form prose when a small set of options suffices.

- Ask one material question at a time.
- Elicit, in the interaction language:
  - **Title** — a concise imperative heading for the item.
  - **Description** — the **full** confirmed requirement substance (goals, required behavior, key constraints, rationale). This is the material `autodev` will treat as confirmed input, so capture the substance, not a one-line summary.
  - **Priority** — `high`, `medium`, or `low` (default `medium` if the user has no preference).
  - **Type** — the bracketed category for the heading (default `[feature]`).
- Present a concise **stage summary** of the proposed entry (Title, Priority, Type, and the Description content) and ask the user to confirm or correct it.
- **Only after explicit user confirmation** may you append the entry. Silence or lack of objection is NOT confirmation.
- If the user declines or aborts, append nothing and stop.

## Entry Format Template

Append exactly one entry, formatted to match what `internal/autodev` parses. Use this literal shape, filling the bracketed placeholders with the confirmed content (the `Description` may span multiple lines — continuation lines are appended to it):

```markdown
## [<type>] <Title>

**Priority**: <high|medium|low>
**Status**: pending
**Description**: <full confirmed requirement substance>
```

Format rules (autodev silently mis-fields mistakes, so these are strict):

- `**Priority**` MUST be exactly `high`, `medium`, or `low` (anything else silently defaults to `low`).
- `**Status**` MUST be exactly `pending` (a new item is unstarted; the ledger — not this field — is authoritative).
- The heading MUST be `## [<type>] <Title>`.
- **Append** the entry after any existing content. Read the current file, append the new entry (preceded by a blank line if the file does not already end with one), and write the result back. Never rewrite, reorder, or delete existing entries.

## Abort Conditions

If interactive confirmation is unavailable — i.e. the `ask_user_question` tool is not registered (non-TUI / no interactive user) — stop immediately and append **nothing**. Do not fall back to appending unconfirmed content.

## Completion

After a confirmed append, report:

- The target file path.
- The Title and Priority of the appended entry.
- That `autodev` can now drain the item (it will materialize the worktree `requirements.md` from the `Description`).
- That no feature workspace, branch, or `requirements.md` was created.
