# Confirmed Requirements: mcp-integration

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-1004-0040vd`
**Status**: Confirmed
**Last Confirmed**: 2026-10-04 (Feishu lifecycle clarification, DEC-010)

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: Consume external MCP tools

- **Status**: confirmed
- **Statement**: Fox acts as an MCP client, discovers tools exposed by external MCP servers, and makes authorized tools callable by the agent.
- **User Evidence**: The user selected option 1, external MCP tool integration.
- **Confirmation State**: Confirmed in the final consolidated stage summary on 2026-10-04 02:13 +08:00.

### NEED-002: Local and remote connections

- **Status**: confirmed
- **Statement**: Support local MCP servers over stdio and remote MCP servers over HTTP.
- **User Evidence**: The user selected option 1, both local and remote servers.
- **Confirmation State**: Confirmed in the final consolidated stage summary on 2026-10-04 02:13 +08:00.

### NEED-003: All applicable execution entry points

- **Status**: confirmed
- **Statement**: Make MCP tools available across CLI, TUI, Feishu, AgentOps, Autodev, and Benchmark. Child agents may use authorized MCP tools within their inherited and child-specific capability limits.
- **User Evidence**: "覆盖所有适用入口" in response to an option explicitly listing these entry points and constrained child-agent access.
- **Confirmation State**: Confirmed in the final consolidated stage summary on 2026-10-04 02:13 +08:00.

### NEED-004: User and project configuration

- **Status**: confirmed
- **Statement**: Support user-level MCP server configuration shared across projects and project-level configuration that supplements or overrides same-named servers.
- **User Evidence**: The user selected option 1, user-level plus project-level configuration with project precedence.
- **Confirmation State**: Confirmed in the final consolidated stage summary on 2026-10-04 02:13 +08:00.

### NEED-005: Playwright MCP compatibility

- **Status**: confirmed
- **Statement**: Treat Playwright MCP as an explicit compatibility acceptance target. Verify page navigation, accessibility snapshot retrieval, clicking, text entry, and preservation of browser state across consecutive tool calls in a session.
- **Rationale**: Compatibility must cover an actual browser workflow, beyond successful server connection or tool discovery.
- **User Evidence**: The user identified their regular Playwright MCP usage and replied "采纳建议。" to the proposed compatibility acceptance scope.
- **Confirmed At**: 2026-10-04 01:11 +08:00

### NEED-006: Model-visible browser screenshots in the first release

- **Status**: confirmed
- **Statement**: The first release must allow the model to receive and visually analyze screenshots returned by Playwright MCP tools. Saving an image file or returning its path alone does not satisfy this requirement.
- **User Evidence**: "另外，确认**首版是否也要求模型能直接查看 Playwright 返回的截图，并据此分析页面**" in response to the explicit first-release screenshot question.
- **Confirmed At**: 2026-10-04 01:11 +08:00
- **Open Detail**: DEC-003 and DEC-008 define fallback and persistence behavior; exact capability-detection and storage mechanisms are delegated to specification/design under DEC-009.

### NEED-007: First-release remote authentication coverage

- **Status**: confirmed
- **Statement**: The first release supports unauthenticated remote MCP servers, configured tokens/HTTP headers, and OAuth login authorization.
- **User Evidence**: The user selected option 1, including OAuth in the first release alongside unauthenticated access and configured credentials.
- **Confirmation State**: Confirmed in the final consolidated stage summary on 2026-10-04 02:13 +08:00.

### NEED-008: Configuration files, CLI commands, and TUI management

- **Status**: confirmed
- **Statement**: Provide configuration-file support, CLI management commands, and a TUI management entry point. Management covers adding/removing servers, enabling/disabling servers, inspecting connection status, and OAuth login/logout.
- **User Evidence**: The user selected option 1, configuration files plus CLI commands plus a TUI management entry point, after these management operations were listed.
- **Confirmation State**: Confirmed in the final consolidated stage summary on 2026-10-04 02:13 +08:00.

### NEED-009: Isolate unavailable MCP servers

- **Status**: confirmed
- **Statement**: When an MCP server cannot connect, times out, or requires reauthentication, clearly expose its unavailable state while allowing other MCP servers and built-in tools to remain usable. Operations that depend on the unavailable server report an explicit error.
- **User Evidence**: The user selected option 1, isolate the failure and continue, rather than requiring all servers to connect or introducing a required-server setting.
- **Confirmation State**: Confirmed in the final consolidated stage summary on 2026-10-04 02:13 +08:00.

## Constraints

### CON-001: Preserve permission and capability restrictions

- **Status**: confirmed
- **Statement**: MCP tool advertisement and execution respect the existing permissions and allowed-tool boundaries of each runtime profile, including constrained child agents.
- **User Evidence**: The selected all-entry-point option explicitly included constrained child access. The full constraint was accepted in the final consolidated stage summary.
- **Profile Evidence**: D-2026-0808-17255q-5 (vetted) preserves single-level constrained child execution; P-2026-0808-17255q-3 (candidate) warns against empty capability intersections restoring unrestricted tools.

### CON-002: Existing architecture and compatibility boundaries

- **Status**: confirmed
- **Statement**: integrate MCP through the shared runtime and existing tool abstractions, keep protocol transports out of the engine, and preserve existing session-data compatibility when adding image-bearing results. Update dependency documentation and architecture checks when package dependencies change.
- **Evidence Type**: Proposed during discovery and explicitly accepted by the user in the final consolidated stage summary.
- **Profile Evidence**: D-2026-0808-17255q-1, D-2026-0808-17255q-2, D-2026-0808-17255q-4, D-2026-0808-17255q-6, Con-2026-0808-17255q-1, and Con-2026-0808-17255q-2 (all vetted).

## Decisions

### DEC-001: Separate standalone resource features from tool-result handling

- **Status**: confirmed
- **Decision**: Excluding standalone Resources and Prompts entry points must not be interpreted as limiting MCP tool results to text. Tool-result handling and necessary resource retrieval are specified by DEC-008.
- **Reason**: The user's Playwright workflow includes model-visible screenshots; protocol feature exclusions must not accidentally exclude that workflow.
- **User Evidence**: "采纳建议。" in response to the revised exclusion and separate treatment of tool results and necessary reads.
- **Confirmed At**: 2026-10-04 01:11 +08:00

### DEC-002: Share MCP sessions between a parent and its children

- **Status**: confirmed
- **Decision**: The main agent and its child agents share the same MCP session, allowing a child to continue using the page opened by its parent in a Playwright workflow. Tool permissions are checked separately for each caller.
- **User Evidence**: The user selected option 1, shared MCP sessions with separate permission checks, instead of separate child connections/browser contexts.
- **Confirmation State**: Confirmed in the final consolidated stage summary on 2026-10-04 02:13 +08:00.
- **Profile Evidence**: D-2026-0808-17255q-5 (vetted) requires child execution to remain within inherited capability limits.

### DEC-003: Explicit fallback for models without image input

- **Status**: confirmed
- **Decision**: Preserve screenshots for user access and continue operations supported by textual page snapshots. Explicitly report that visual analysis is unavailable with the configured model, without automatically switching models or blocking unrelated work.
- **User Evidence**: The user selected option 1, explicit notice with continued text-based operations and no automatic model switch.
- **Confirmation State**: Confirmed in the final consolidated stage summary on 2026-10-04 02:13 +08:00.

### DEC-004: HTTP transport compatibility

- **Status**: confirmed
- **Decision**: support stdio, Streamable HTTP, and legacy HTTP+SSE. Negotiate protocol versions and report unsupported versions as a server-specific failure. Document the tested protocol/server versions.
- **Evidence Type**: Proposed during discovery and explicitly accepted by the user in the final consolidated stage summary.
- **Research Evidence**: https://modelcontextprotocol.io/specification/2025-11-25/basic/transports distinguishes Streamable HTTP from the legacy HTTP+SSE transport and describes compatibility handling.

### DEC-005: Configuration precedence and activation

- **Status**: confirmed
- **Decision**: a project-level definition replaces the entire same-named user-level server definition; different names coexist. Management operations explicitly target user or project scope. Configuration edits, including enable/disable settings, apply to new MCP sessions; applying them to an existing Fox session requires an explicit reconnect after active tool calls finish.
- **Evidence Type**: Proposed during discovery and explicitly accepted by the user in the final consolidated stage summary.

### DEC-006: Connection ownership and recovery

- **Status**: confirmed
- **Decision**: each independent Fox session owns its MCP connections; parent and child agents share within that session. Preserve connections across normal turns, serialize calls to each server by default, and close owned connections/processes when their owning session ends. Provide configurable bounded startup/call timeouts, cancellation, cleanup, and explicit reconnect. Do not automatically replay calls with uncertain side effects. A replaced or lost server session must be reported as potentially losing browser state; persisted Fox history does not imply browser-session restoration.
- **Boundary**: Separate Fox connections cannot guarantee isolation of state inside a remotely managed server. Playwright acceptance must use a server configuration that supports the intended session behavior.
- **Evidence Type**: Proposed during discovery and explicitly accepted by the user in the final consolidated stage summary.
- **Profile Evidence**: P-2026-0808-17255q-1 and P-2026-0808-17255q-4 (candidate guidance).

### DEC-010: Feishu conversation lifetime

- **Status**: confirmed
- **Decision**: Consecutive messages from the same Feishu chat/user retain one live Fox session, its MCP connections, browser state and resolved MCP configuration. Per-message completion, cancellation or failure does not end that session. An explicit new-session directive closes the previous live session before creating its replacement; service shutdown closes all retained sessions. Task-specific approval correlation, permission evidence, journals and memory extraction remain independent. Process restart may reopen history but does not restore browser state; connection loss and explicit reconnect retain DEC-006 semantics.
- **User Evidence**: The user selected "1" after the summary offered "共享浏览器状态，直到新建会话或服务退出（推荐）" versus per-message connections.
- **Confirmation State**: Confirmed directly by this explicit lifecycle choice on 2026-10-04; no further scope approval is pending.

### DEC-007: OAuth and unattended authorization

- **Status**: confirmed
- **Decision**: provide explicit login/logout, persist credentials in user-local storage separately from shareable project configuration, refresh credentials when supported, and expose reauthorization requirements. Unattended execution uses existing credentials and permissions; when user interaction is required but unavailable, return an actionable authorization/approval error without waiting indefinitely. Preserve existing permission modes; external server metadata alone does not grant permission.
- **Evidence Type**: Proposed during discovery and explicitly accepted by the user in the final consolidated stage summary.

### DEC-008: Tool-result handling and persistence

- **Status**: confirmed
- **Decision**: support text, structured data, images, and embedded text/image resource content. Preserve resource links and allow authorized on-demand reads needed to use tool results. Preserve image artifacts and references for session replay, provide image input through both existing provider adapters when their configured models support it, and retain compatibility with existing text-only history. Report unsupported result formats explicitly without silently discarding them or claiming successful interpretation.
- **Evidence Type**: Proposed during discovery and explicitly accepted by the user in the final consolidated stage summary.
- **Research Evidence**: internal/schema/message.go; internal/llmconfig/config.go; https://modelcontextprotocol.io/specification/2025-11-25/server/tools#tool-result.

### DEC-009: Deferred implementation details and acceptance approach

- **Status**: confirmed
- **Decision**: exact configuration paths/field names, command spelling, SDK and protocol-version matrix, pinned Playwright acceptance version, credential-storage backend, numeric timeout/output limits, and package layout are to be fixed during specification/design without reopening the agreed product behavior. Keep routine tests deterministic and independent of live servers; add a separately runnable Playwright integration acceptance workflow for the agreed browser operations and screenshots.
- **Evidence Type**: Proposed during discovery and explicitly accepted by the user in the final consolidated stage summary.
- **Profile Evidence**: Con-2026-0808-17255q-2 (vetted); project constitution testing and architecture principles.

## Out of Scope

### OUT-002: Standalone resource and prompt entry points; serving Fox over MCP

- **Status**: confirmed
- **Statement**: This feature does not provide standalone MCP Resources browsing or MCP Prompts template entry points, and does not expose Fox as an MCP server for other clients.
- **Boundary**: DEC-001 and DEC-008 include tool-result handling and necessary authorized resource retrieval; they are not excluded by this entry.
- **User Evidence**: "采纳建议。" accepting the narrowed exclusion after the Playwright compatibility explanation.
- **Confirmed At**: 2026-10-04 01:11 +08:00
- **Replaces**: OUT-001, an unconfirmed broader proposal.

## Open Questions

There are no blocking product-scope questions. OPEN-001 through OPEN-005 are explicitly deferred implementation details under confirmed DEC-009. Their resolutions must preserve the confirmed behavior and be recorded in specification/design before implementation.

### OPEN-001: Authentication implementation details

- **Status**: open
- **Blocking Specification**: no — explicitly deferred by DEC-009.
- **Resolved Product Behavior**: NEED-007, NEED-008, and DEC-007 cover authentication modes, login/logout, refresh, user-local credentials, and unattended authorization failures.
- **Deferred Detail**: OAuth compatibility details and the user-local credential-storage backend.
- **Owner**: Specification / Design

### OPEN-002: Configuration representation

- **Status**: open
- **Blocking Specification**: no — explicitly deferred by DEC-009.
- **Resolved Product Behavior**: NEED-004, NEED-008, and DEC-005 cover scope, management operations, whole-definition replacement, and activation timing.
- **Deferred Detail**: Exact paths, field names, validation representation, and CLI/TUI command spelling.
- **Owner**: Specification / Design

### OPEN-003: Lifecycle implementation details

- **Status**: open
- **Blocking Specification**: no — explicitly deferred by DEC-009.
- **Resolved Product Behavior**: NEED-009, CON-001, DEC-002, DEC-006, and DEC-007 cover failure isolation, per-caller permissions, session ownership/sharing, serial execution, bounded cancellation/cleanup, reconnect, and unattended errors.
- **Deferred Detail**: Numeric startup/call/output limits, process supervision mechanisms, and composition details for each existing profile.
- **Profile Evidence**: Con-2026-0808-17255q-4, P-2026-0808-17255q-1, and P-2026-0808-17255q-4 are candidate implementation guidance, not independent authority to expand confirmed scope.
- **Owner**: Specification / Design

### OPEN-004: Multimodal implementation details

- **Status**: open
- **Blocking Specification**: no — explicitly deferred by DEC-009.
- **Resolved Product Behavior**: NEED-006, DEC-001, DEC-003, and DEC-008 cover supported results, model-visible screenshots, image artifacts/replay, authorized resource reads, and fallback without model switching.
- **Deferred Detail**: Provider/model capability declaration or detection, image artifact representation, persistence migration, and provider-specific encoding.
- **Research Evidence**: internal/schema/message.go; internal/llmconfig/config.go; https://modelcontextprotocol.io/specification/2025-11-25/server/tools#tool-result.
- **Owner**: Specification / Design

### OPEN-005: Version and dependency selection

- **Status**: open
- **Blocking Specification**: no — explicitly deferred by DEC-009.
- **Resolved Product Behavior**: NEED-002, NEED-005, DEC-004, and DEC-009 cover all three transports, negotiation, documented compatibility, and independent Playwright acceptance.
- **Deferred Detail**: SDK/version selection, supported protocol-version matrix, pinned Playwright version, and exact package layout.
- **Owner**: Specification / Design

### OPEN-006: Final consolidated stage confirmation

- **Status**: resolved
- **Resolution**: The user replied "确认" to the final summary covering NEED-001 through NEED-009, CON-001 through CON-002, DEC-001 through DEC-009, and OUT-002, including proposed defaults and explicit deferrals.
- **Blocking Specification**: no.
- **Resolved At**: 2026-10-04 02:13 +08:00

## Superseded Entries

### OUT-001: Broad Resources and Prompts exclusion

- **Status**: superseded
- **Original Proposal**: Do not implement MCP Resources or Prompts, and do not expose Fox as an MCP server.
- **Replaced By**: OUT-002 and DEC-001.
- **Historical Note**: This was an AI assumption presented for confirmation, never binding authority. The user asked whether it would impair Playwright MCP usage; the revised proposal distinguishes standalone feature entry points from tool-result handling and necessary resource retrieval.

## Confirmation Log

### Feishu lifecycle clarification — 2026-10-04

- **Summary Presented**: Share browser state across consecutive Feishu messages until explicit new session or service exit.
- **User Confirmation**: "1" (the shared-state option).
- **Entries Confirmed**: DEC-010, clarifying DEC-005/006 for Feishu normal message boundaries.

### Session 2026-10-04 01:11 +08:00 — Playwright compatibility and screenshots

- **Summary Presented**: Use Playwright MCP as an explicit acceptance target for navigation, snapshots, clicking, typing, and browser-state continuity. Narrow the exclusion to standalone Resources/Prompts entry points and exposing Fox as a server. Specify tool-result handling and necessary resource reads separately. Explicitly ask whether first-release models must see and analyze Playwright screenshots.
- **User Confirmation**: "采纳建议。另外，确认**首版是否也要求模型能直接查看 Playwright 返回的截图，并据此分析页面**"
- **Entries Confirmed**: NEED-005, NEED-006, DEC-001, OUT-002.
- **Superseded Proposal**: OUT-001 (previously unconfirmed).
- **Stage**: Intermediate topic confirmation. Remaining discovery questions block final confirmation and automatic specification generation.

### Session 2026-10-04 02:13 +08:00 — Final consolidated requirements

- **Summary Presented**: External MCP tools across all applicable entry points; stdio, Streamable HTTP, and legacy HTTP+SSE; user/project configuration with whole-server replacement; CLI/TUI management; OAuth and static credentials; failure isolation; shared parent/child sessions with separate permissions; persistent browser state across normal turns; bounded lifecycle operations and explicit reconnect without uncertain-call replay; Playwright acceptance and model-visible screenshots; explicit fallback for non-image models; structured and embedded results plus authorized resource reads; user-local credentials; existing architecture, permissions, and persisted-history compatibility; explicit deferral of implementation details; deterministic ordinary tests and separately runnable Playwright acceptance. Preserve the narrowed Resources/Prompts/server exclusions.
- **User Confirmation**: "确认"
- **Entries Confirmed**: NEED-001, NEED-002, NEED-003, NEED-004, NEED-005, NEED-006, NEED-007, NEED-008, NEED-009, CON-001, CON-002, DEC-001, DEC-002, DEC-003, DEC-004, DEC-005, DEC-006, DEC-007, DEC-008, DEC-009, OUT-002.
- **Open Items**: OPEN-001 through OPEN-005 explicitly deferred, non-blocking; OPEN-006 resolved.
- **Stage**: Final confirmation. Discovery complete; workflow.auto_next is true, so invoke codexspec:generate-spec exactly once for this feature directory.
