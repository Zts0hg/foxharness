# Confirmed Requirements: auto-memory

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0621-18248t`
**Status**: Discovery Complete — Requirements Confirmed
**Last Confirmed**: 2026-06-21

## Context

foxharness is a Go-based agent harness positioned as a Claude Code-equivalent. This
feature ports a **suitable** subset of established agent-memory-system mechanisms into
foxharness. The current state, verified in code: `working_memory.md` is a session-scoped file whose
`Append`/`Replace` methods are dead code (zero callers) and whose maintenance is not
prompt-driven; the project-level `{workDir}/MEMORY.md` is a single flat file injected
each turn with no types, structure, extraction, or lifecycle guardrails. foxharness
has **no cross-session persistent memory** today.

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: Cross-session persistent memory layer

- **Status**: confirmed
- **Statement**: Add a persistent memory layer that survives across sessions so the agent accumulates durable knowledge about the user, their feedback, the project, and external references. Existing `PLAN.md`, `TODO.md`, compaction, and `AGENTS.md` instruction loading remain unchanged.
- **Rationale**: foxharness currently loses all context when a session ends; the agent cannot recall user preferences or project background across sessions.
- **User Evidence**: User selected "新增跨会话持久记忆层（推荐）" as the core objective.
- **Confirmed At**: 2026-06-21

### NEED-002: Four typed memories with YAML frontmatter

- **Status**: confirmed
- **Statement**: Support four memory types — `user`, `feedback`, `project`, `reference` — each stored as an individual Markdown file with YAML frontmatter (`name`, `description`, `type`). `feedback` and `project` memories carry a `Why` and `How to apply` structure. The four types and their semantics match Claude Code's definition.
- **Rationale**: Typed, structured memories enable targeted retrieval and prevent an undifferentiated log; the `description` field is the primary relevance signal.
- **User Evidence**: User accepted "4 类型与索引上限与 Claude Code 保持一致".
- **Confirmed At**: 2026-06-21

### NEED-003: Two-tier MEMORY.md index, always injected

- **Status**: confirmed
- **Statement**: Maintain a `MEMORY.md` entry-point index at each of two scopes, always injected into the system prompt (merged): user-global `~/.foxharness/memory/MEMORY.md` and project-level `~/.foxharness/projects/{encoded-workdir}/memory/MEMORY.md`. `user` memories live in the global scope; `project`, `feedback`, and `reference` memories live in the project scope. Index entries are one line each (target `< 150` chars) pointing to the typed files.
- **Rationale**: `user` knowledge is cross-project by nature; project/feedback/reference knowledge is per-project. Two tiers match foxharness's existing user-home storage layout.
- **User Evidence**: User selected "用户全局 + 项目级（推荐）".
- **Confirmed At**: 2026-06-21

### NEED-004: Two-layer write — inline plus post-run extraction hook

- **Status**: confirmed
- **Statement**: Memories are written by two complementary layers sharing one prompt-level save criteria: (1) **inline** — the main agent writes memories with the existing `write_file`/`edit_file` tools when it judges something worth saving; (2) **extraction hook** — at the end of each run, a Go-triggered LLM extraction pass reviews the run's messages and writes missed memories. The extraction layer is mutually exclusive with inline writes (skipped when the main agent already wrote to the memory dir during the run) and pre-injects the existing memory manifest to avoid duplicates.
- **Rationale**: Inline captures obvious signals and immediate "remember this" requests; the extraction hook backstops signals the main agent missed while rushing. This mirrors Claude Code's two-layer model without the cache-sharing fork optimization.
- **User Evidence**: User selected "inline + 后台提取钩子（推荐）".
- **Confirmed At**: 2026-06-21

### NEED-005: Full-index retrieval with on-demand reads

- **Status**: confirmed
- **Statement**: Each turn injects the full merged MEMORY.md index (descriptions only). The agent expands a specific memory's full content on demand via `read_file` when relevant. No per-turn AI relevance filtering.
- **Rationale**: At foxharness's memory scale, the index with descriptions is the relevance signal; per-turn AI filtering adds latency and cost without proportional value.
- **User Evidence**: User selected "全量索引 + 按需读取（推荐）".
- **Confirmed At**: 2026-06-21

### NEED-006: Prompt-level lifecycle guardrails

- **Status**: confirmed
- **Statement**: Encode the following guardrails in the memory system prompt (shared by inline and extraction layers): a "what NOT to save" list (code patterns, git history, fix recipes, CLAUDE.md/AGENTS.md-documented content, ephemeral task state are excluded); the "surprising / non-obvious" heuristic as the saving criterion; a memory-drift caveat (verify before relying on a memory); a verify-before-recommending rule (check files/functions/flags named in a memory before recommending); an explicit "ignore memory" directive (proceed as if the index were empty); and a dedup-first rule (update an existing file rather than creating a duplicate).
- **Rationale**: These are near-zero-cost prompt-level rules that prevent memory bloat, stale-recommendation errors, and the confirm-then-overwrite anti-pattern.
- **User Evidence**: User accepted the proposed guardrail set in the stage summary.
- **Confirmed At**: 2026-06-21

### NEED-007: Activate working_memory.md as session-scoped scratchpad

- **Status**: confirmed
- **Statement**: Wire up `working_memory.md` (currently dead — `Append`/`Replace` have no callers and no prompt drives its maintenance) as a usable session-scoped scratchpad by connecting a writeback path and adding prompt guidance to maintain it (Goal / Known Facts / Current Plan / Next Step). It is explicitly distinct from the cross-session persistent layer: `working_memory.md` is session-scoped and perishes with the session; persistent memory survives across sessions. Maintenance reuses the existing `write_file`/`edit_file` tools (no new dedicated tool).
- **Rationale**: The file is not redundant — it occupies the always-injected session-draft slot that `PLAN.md`/`TODO.md` (tool-gated, not auto-injected) do not fill, and has a "Known Facts" bucket with no counterpart. Bundling its activation avoids leaving a dormant mechanism.
- **User Evidence**: User selected "本次顺便激活它" after the redundancy overstatement was corrected.
- **Confirmed At**: 2026-06-21

### NEED-008: Persistent forget/delete of memories

- **Status**: confirmed
- **Statement**: The main agent MUST be able to persistently remove a memory when the user asks it to forget/delete one (or when a memory is confirmed stale) — by removing the memory file and dropping its index line. This is distinct from the temporary "ignore memory" directive (NEED-006), which only suppresses memory for the current request without removing anything. Removal is performed inline by the main agent using the existing file tools.
- **Rationale**: A memory system that can create and update but not delete forces stale or unwanted entries to accumulate; persistent forget/delete is a natural and expected maintenance capability.
- **User Evidence**: During spec generation, review surfaced that "delete" had been introduced into the spec without a confirmed source. User selected "纳入范围（推荐）", confirming persistent forget/delete is in scope.
- **Confirmed At**: 2026-06-21

## Constraints

### CON-001: Storage under user home, two-tier

- **Status**: confirmed
- **Statement**: All persistent memory lives under the user home directory (`~/.foxharness/`), never inside the repository. Two tiers: `~/.foxharness/memory/` (user-global) and `~/.foxharness/projects/{encoded-workdir}/memory/` (project-level). This matches foxharness's existing storage layout and avoids polluting the user's repository.
- **User Evidence**: User selected "用户全局 + 项目级（推荐）" (in-repo option explicitly rejected).
- **Confirmed At**: 2026-06-21

### CON-002: Fresh start for the legacy project MEMORY.md

- **Status**: confirmed
- **Statement**: The legacy flat `{workDir}/MEMORY.md` is no longer read or injected. The new system uses only the new paths. Existing legacy content becomes orphaned (the user handles it manually); no automatic migration is performed.
- **User Evidence**: User selected "全新开始".
- **Confirmed At**: 2026-06-21

### CON-003: Constitution compliance

- **Status**: confirmed
- **Statement**: All new code follows the project constitution: Test-Driven Development (Red → Green → Refactor), Go block-level documentation standards (no teaching line comments), dependency injection for testability, and small focused interfaces.
- **User Evidence**: Project constitution is binding per CLAUDE.md.
- **Confirmed At**: 2026-06-21

### CON-004: Extraction mutually exclusive and tool-narrowed

- **Status**: confirmed
- **Statement**: The extraction hook detects whether the main agent already wrote to the memory directory during the run (mutual exclusion) and skips itself when so. The extraction agent's tool permissions are narrowed to the memory directory: read-only file tools and bash are allowed; `edit_file`/`write_file` are allowed only for paths inside the memory directory; all other tools (MCP, subagent, write-capable bash, `rm`) are denied.
- **User Evidence**: User accepted the proposed extraction design in the stage summary.
- **Confirmed At**: 2026-06-21

### CON-005: Bounded index size

- **Status**: confirmed
- **Statement**: The MEMORY.md index and memory file count are bounded, matching Claude Code's limits: at most ~200 memory files per scope, the index truncated at ~200 lines / ~25 KB (with a truncation notice), and each index line under ~150 characters. Memory file content (frontmatter excluded) is capped (~40,000 characters).
- **User Evidence**: User accepted "4 类型与索引上限与 Claude Code 保持一致".
- **Confirmed At**: 2026-06-21

### CON-006: Extraction hook must be context-isolated

- **Status**: confirmed
- **Statement**: The extraction hook runs as a context-isolated execution. It reads the run's main messages as read-only input only; its own extraction turns occupy only its own message slice and are NEVER appended to the main message log (`messages.jsonl`), the main transcript, or the main system prompt. It runs asynchronously and out-of-band relative to the main loop. Its only feedback to the main agent is the memory files it writes (bounded by CON-005). The extraction therefore consumes its own token budget, not the main agent's context window.
- **Rationale**: User-raised concern (2026-06-21): the extraction hook must neither pollute nor crowd out the main agent's context. This is the core correctness constraint on the extraction design.
- **User Evidence**: User asked "这个提取钩子是否可能污染主agent上下文？或者挤占上下文空间？" and accepted the isolation-based answer.
- **Confirmed At**: 2026-06-21

## Decisions

### DEC-001: Objective — new cross-session persistent layer

- **Status**: confirmed
- **Decision**: Add a cross-session persistent memory layer. Keep `PLAN.md`/`TODO.md`/compaction/`AGENTS.md` unchanged; bundle the activation of `working_memory.md`.
- **Alternatives Rejected**: (a) Refactor/unify the existing `working_memory.md` into the new system — rejected due to session-draft vs cross-session semantic conflict and larger blast radius; (b) Full Claude Code parity including team memory — rejected as out of scope for a local CLI harness.
- **Reason**: The core value of an agent-memory system is cross-session persistent memory, which foxharness entirely lacks.
- **User Evidence**: User selected "新增跨会话持久记忆层（推荐）".
- **Confirmed At**: 2026-06-21

### DEC-002: Write mechanism — inline plus extraction hook

- **Status**: confirmed
- **Decision**: Inline main-agent writes plus a post-run extraction hook. Not a full forked-agent with prompt-cache sharing.
- **Alternatives Rejected**: (a) Inline-only — rejected, no backstop for missed memories; (b) Full two-layer with cache-sharing forked agent — rejected, foxharness lacks that primitive and the benefit is mainly token savings.
- **Reason**: Balances coverage (backstop) against architectural complexity and aligns with the user's "Go-coded flow + LLM-in-the-loop, no over-engineering" preference.
- **User Evidence**: User selected "inline + 后台提取钩子（推荐）".
- **Confirmed At**: 2026-06-21

### DEC-003: Two-tier storage (user-global + project-level)

- **Status**: confirmed
- **Decision**: User-global scope for `user` memories; project-level scope for `project`/`feedback`/`reference` memories; merged at injection.
- **Alternatives Rejected**: (a) Project-level only — rejected, `user` memories would be re-recorded per project; (b) In-repo committable storage — rejected, pollutes the repository and diverges from the `~/.foxharness/` layout.
- **Reason**: Matches Claude Code's model and foxharness's existing per-project user-home storage.
- **User Evidence**: User selected "用户全局 + 项目级（推荐）".
- **Confirmed At**: 2026-06-21

### DEC-004: Retrieval — full index injection plus on-demand reads

- **Status**: confirmed
- **Decision**: Inject the full merged index each turn; expand full memory content on demand via `read_file`. No per-turn AI relevance filtering.
- **Alternatives Rejected**: (a) Per-turn AI relevance filtering (`findRelevantMemories`) — deferred (OUT-005); (b) Hybrid threshold-based switch — rejected as premature complexity.
- **Reason**: At current scale the index descriptions are a sufficient relevance signal; AI filtering adds per-turn cost without proportional value.
- **User Evidence**: User selected "全量索引 + 按需读取（推荐）".
- **Confirmed At**: 2026-06-21

### DEC-005: working_memory.md — activate in this feature, reuse existing tools

- **Status**: confirmed
- **Decision**: Bundle the activation of `working_memory.md` (connect writeback + prompt guidance). Maintenance reuses `write_file`/`edit_file`; no new dedicated tool.
- **Alternatives Rejected**: (a) Mark for later refactor / out of scope — rejected by user who chose to activate now; (b) Remove `working_memory.md` — rejected, it is not redundant (corrected from an earlier overstatement).
- **Reason**: User chose to activate it now rather than defer; reusing existing tools avoids scope growth from a new tool.
- **User Evidence**: User selected "本次顺便激活它" and "复用现有工具".
- **Confirmed At**: 2026-06-21

### DEC-006: Legacy MEMORY.md — fresh start

- **Status**: confirmed
- **Decision**: The new system uses only the new `~/.foxharness/` paths; the legacy `{workDir}/MEMORY.md` is no longer injected and its content is orphaned.
- **Alternatives Rejected**: (a) Migrate legacy content into the new project memory — rejected; (b) Dual-path coexistence — rejected, adds complexity.
- **Reason**: Cleanest; user prefers a fresh start and will handle legacy content manually.
- **User Evidence**: User selected "全新开始".
- **Confirmed At**: 2026-06-21

### DEC-007: Extraction execution model and cadence

- **Status**: confirmed
- **Decision**: The extraction hook runs **asynchronously** (fire-and-forget, non-blocking on run completion), at the **end of every run**, and is **skipped when the main agent already wrote to the memory directory during that run** (mutual exclusion).
- **Alternatives Rejected**: (a) Synchronous at run end — rejected, would block perceived completion; (b) Every N runs throttling — rejected as unnecessary at this stage (mutual exclusion already bounds cost).
- **Reason**: Async avoids user-perceived latency; per-run with mutual-exclusion skip bounds cost while guaranteeing coverage.
- **User Evidence**: User answered "OPEN-001: 异步" and "OPEN-002: 每轮 run 结束 + 互斥跳过".
- **Confirmed At**: 2026-06-21

### DEC-008: Extraction reuses existing engine/provider/subagent primitives

- **Status**: confirmed
- **Decision**: The extraction hook is built on foxharness's existing engine/provider/subagent primitives, not a true cache-sharing forked agent.
- **Alternatives Rejected**: True forked-agent with prompt-cache sharing — rejected, foxharness lacks the primitive and the benefit is mainly token savings.
- **Reason**: User explicitly accepted reusing existing primitives as sufficient.
- **User Evidence**: User stated "提取钩子复用现有 engine/provider/subagent 原语可以接受".
- **Confirmed At**: 2026-06-21

### DEC-009: Persistent forget/delete is in scope

- **Status**: confirmed
- **Decision**: Persistent forget/delete is in scope. The main agent removes a memory by deleting its file and dropping its index line (inline, via the existing file tools). It honors explicit user requests to forget a specific memory.
- **Alternatives Rejected**: Drop delete from the spec (keep only create/update + temporary "ignore") — rejected; user confirmed persistent removal is desired.
- **Reason**: A memory system without persistent removal accumulates stale/unwanted entries; forget/delete is a natural maintenance capability the user wants.
- **User Evidence**: User selected "纳入范围（推荐）" when asked whether persistent forget/delete is in scope.
- **Confirmed At**: 2026-06-21

## Out of Scope

### OUT-001: Team Memory

- **Status**: confirmed
- **Statement**: Shared team-level memory with sync and sensitive-data scanning is excluded.
- **Reason**: Requires sync infrastructure and secret-scanning beyond a local CLI harness.
- **User Evidence**: Stage summary accepted.
- **Confirmed At**: 2026-06-21

### OUT-002: KAIROS daily-log mode and /dream nightly distillation

- **Status**: confirmed
- **Statement**: Append-only daily-log memory mode and a nightly distillation skill are excluded.
- **Reason**: Complex and niche; not needed for the core persistent-memory goal.
- **User Evidence**: Stage summary accepted.
- **Confirmed At**: 2026-06-21

### OUT-003: CLAUDE.md instruction hierarchy and conditional rules

- **Status**: confirmed
- **Statement**: Hierarchical instruction files with `@include` and frontmatter `paths` conditional rules are excluded from this feature.
- **Reason**: foxharness already loads `AGENTS.md` / `.foxharness/` / `.claude/` instructions; this is a separate concern from memory.
- **User Evidence**: Stage summary accepted.
- **Confirmed At**: 2026-06-21

### OUT-004: Per-subagent-type agent memory

- **Status**: confirmed
- **Statement**: Memory scoped per subagent type (e.g. separate dirs for `Explore`, `general-purpose`) is excluded.
- **Reason**: foxharness's subagent model differs; defer.
- **User Evidence**: Stage summary accepted.
- **Confirmed At**: 2026-06-21

### OUT-005: Per-turn AI relevance filtering

- **Status**: confirmed
- **Statement**: Claude Code's `findRelevantMemories` per-turn AI selection is excluded for now.
- **Reason**: Unnecessary at current memory scale (see DEC-004); revisit when memory count grows.
- **User Evidence**: Stage summary accepted.
- **Confirmed At**: 2026-06-21

### OUT-006: @include for memory files

- **Status**: confirmed
- **Statement**: `@path` include directives inside memory files are excluded.
- **Reason**: Adds parsing complexity without clear value for typed memory files.
- **User Evidence**: Stage summary accepted.
- **Confirmed At**: 2026-06-21

## Open Questions

None blocking specification generation. The three open questions raised during
discovery were resolved by the user and folded into DEC-005 (reuse existing tools)
and DEC-007 (async, per-run, mutual-exclusion skip).

## Superseded Entries

None. (An interim overstatement that "`working_memory.md` is redundant with
`PLAN.md`/`TODO.md`" was corrected during discovery before any binding entry was
written; the corrected understanding is captured in NEED-007 and DEC-005.)

## Confirmation Log

### Session 2026-06-21

- **Summary Presented**: A full stage summary grouped by candidate IDs covering 7 Needs, 6 Constraints (including the user-raised context-isolation constraint CON-006), 8 Decisions, and 6 Out-of-Scope items; plus three resolved open questions and two AI assumptions.
- **User Confirmation**: User confirmed all entries and resolved the three open questions (async execution; per-run cadence with mutual-exclusion skip; reuse existing tools) and the two AI assumptions (reuse existing primitives; adopt Claude Code's four types and index limits). User additionally raised and accepted the context-isolation constraint (CON-006) for the extraction hook.
- **Entries Confirmed**: NEED-001..007, CON-001..006, DEC-001..008, OUT-001..006.

### Amendment 2026-06-21 (during spec generation)

- **Trigger**: Spec review (W1) found that "delete" had been introduced into `spec.md` REQ-009 and User Story 2 without a confirmed source. The confirmed requirements at that point covered only writes (NEED-004) and a temporary "ignore memory" directive (NEED-006).
- **User Decision**: User selected "纳入范围（推荐）", confirming persistent forget/delete is in scope.
- **Entries Confirmed**: NEED-008, DEC-009.
- **Note**: "ignore memory" (NEED-006) is a temporary, per-request directive — the agent proceeds as if the index were empty for that request without removing anything. It is distinct from persistent forget/delete (NEED-008), which physically removes a memory file and its index line.
