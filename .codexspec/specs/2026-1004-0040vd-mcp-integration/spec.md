# Feature Specification: MCP Integration

**Feature Branch**: `2026-1004-0040vd-mcp-integration`
**Created**: 2026-10-04
**Status**: Reviewed — PASS (review-spec.md)
**Authority**: [requirements.md](requirements.md), final confirmation 2026-10-04 02:13 +08:00
**Input**: Add external MCP tool integration to Fox, including first-release Playwright browser automation and model-visible screenshots.

## Context and Goals

Fox currently registers concrete tools in its execution entry points and has no MCP client integration. Its shared message and tool-result representations primarily carry strings. This feature introduces external MCP tools and an end-to-end image-result path while preserving the shared runtime architecture, existing permission boundaries, and readability of existing session history.

The intended workflow is to configure a server, authorize it when necessary, make its permitted tools available to the agent, and use them repeatedly within one Fox session. Playwright MCP is the explicit acceptance target: navigation, accessibility snapshots, clicking, typing, parent/child handoff, and screenshot-based analysis must work together. This is client integration; Fox does not become an MCP server.

## User Scenarios and Testing

All stories below are first-release scope. Priorities express validation order, not optional inclusion. Each story can be tested with controlled server/provider fixtures; the browser workflow also has a separately runnable real Playwright acceptance path.

### Story 1 — Configure and call an external tool (P1)

A user adds a local stdio server or a remote HTTP server, inspects its status, and asks Fox to perform an operation using its tools.

**Independent test**: A fixture server exposes a tool with a known input schema and result. Fox discovers it, advertises it to a scripted model, executes the requested call, and delivers the correlated result.

**Acceptance scenarios**:

1. Given an enabled, authorized server, when Fox establishes a session, then its permitted tools become discoverable and callable with their descriptions and schemas.
2. Given stdio, Streamable HTTP, or legacy HTTP+SSE fixture servers, when connected through the corresponding configuration, then the same tool workflow succeeds for each transport.
3. Given an unsupported negotiated protocol version or a server that cannot start, when connection is attempted, then that server is visibly unavailable and unrelated tools remain usable.

### Story 2 — Complete a stateful browser workflow (P1)

A user asks Fox to open a page, inspect it, enter text, click a control, and inspect the resulting page. An authorized child can continue work on the page opened by its parent.

**Independent test**: A stateful fixture validates session continuity; a pinned Playwright server operates on a controlled local test page for integration acceptance.

**Acceptance scenarios**:

1. Navigation, snapshot, typing, and clicking operate on the same browser state across ordinary Fox turns.
2. A permitted child continues the parent's page state through the shared MCP session, while a disallowed child call is denied before dispatch.
3. Two calls targeting one server do not execute concurrently by default, including calls issued by different parent/child callers.
4. Ending a child does not close the parent's shared connection. Ending the owning Fox session closes its owned connections and local processes.

### Story 3 — Inspect screenshots with the configured model (P1)

A user asks Fox to analyze the visible page. A screenshot returned by a tool reaches an image-capable model as image input, and the image remains available in session history.

**Independent test**: Inspect the actual provider request generated for a fixture image for each existing provider adapter, then load persisted history and verify that the image can be resolved again. A path string alone fails this test.

**Acceptance scenarios**:

1. With an image-capable configured model, screenshot bytes and their supported media type are represented as model image input together with relevant textual results.
2. With a model that cannot accept images, Fox retains the screenshot for user access, explicitly reports that visual analysis is unavailable, and continues supported text-snapshot operations without switching models.
3. Existing text-only session records still load and behave as before after image-result support is added.

### Story 4 — Manage configuration and authorization (P1)

A user manages servers through files, CLI commands, or a TUI entry point, choosing user or project scope and completing OAuth login where required.

**Independent test**: Exercise both management adapters against temporary configurations and a local OAuth fixture, then verify the effective configuration and connection behavior.

**Acceptance scenarios**:

1. A project definition replaces the entire same-named user definition; fields and credentials from the replaced definition do not silently merge into it. Differently named servers coexist.
2. Add/remove, enable/disable, status inspection, login/logout, and explicit reconnect are available through the appropriate management workflow.
3. Editing configuration leaves an established session's resolved configuration unchanged. A new session sees the edit; an explicit reconnect applies it to the current session after its active tool calls finish.
4. No-auth, configured token/header, and OAuth servers are usable. Refresh works when supported; otherwise the server reports that reauthorization is needed.
5. An unattended entry with no available interaction mechanism returns an actionable authorization/approval error instead of waiting indefinitely.

### Story 5 — Continue useful work when a server fails (P1)

A user can continue using internal tools and healthy MCP servers even when another server disconnects, times out, or loses authorization.

**Independent test**: One healthy and one failing fixture server are connected alongside an internal tool; observe status, errors, cancellation, reconnect, and continued successful calls.

**Acceptance scenarios**:

1. The failing server is identifiable in status and affected operations return explicit errors. Healthy MCP and internal tools remain usable.
2. A timed-out operation is bounded and cancellable; Fox does not silently replay a call whose side effects are uncertain.
3. Replacing a server session reports that browser state may have been lost. Loading Fox history does not claim to restore a live browser session.
4. Cancelling a child operation leaves the shared session usable when the server remains healthy; cleanup is owned by the enclosing Fox session rather than by the child borrower.

### Story 6 — Use tools consistently from every applicable entry (P1)

The same configured MCP capability can be used by CLI, TUI, Feishu, AgentOps, Autodev, Benchmark, and authorized child execution without bypassing entry-specific policy.

**Independent test**: Use the same scripted MCP fixture through each actual entry composition and verify advertised tools, execution, result propagation, permission denial, and lifecycle ownership.

**Acceptance scenarios**:

1. Each named entry can use permitted configured MCP tools through the shared runtime integration.
2. Explicitly restricted profiles and child tool sets remain restricted; having access to a shared connection does not confer permission to call every tool.
3. External tool annotations alone do not grant permission, and entries without an available approval interaction do not wait indefinitely.

### Story 7 — Consume rich tool results without standalone resource browsing (P1)

A tool returns text, structured data, an image, embedded resource content, or a resource link needed to finish the task.

**Independent test**: A fixture returns each supported content type and a linked resource, plus an unsupported type; observe model/user presentation and authorized retrieval.

**Acceptance scenarios**:

1. Supported text, structured data, images, and embedded text/image contents are preserved with the originating tool result.
2. Resource links remain available and necessary reads can be requested under authorization, without a standalone Resources browsing or Prompts template feature.
3. Unsupported content is explicitly reported; Fox neither silently drops it nor reports that it interpreted it successfully.

## Functional Requirements

- **REQ-001**: Fox MUST act as an MCP client, establish server sessions, discover tool definitions, advertise permitted tools to the model, dispatch calls to the originating server, and return correlated success or error results. Sources: NEED-001, CON-001.
- **REQ-002**: Fox MUST support local subprocess servers over stdio, remote Streamable HTTP servers, and legacy HTTP+SSE servers, including protocol-version negotiation and identifiable server-specific incompatibility errors. Sources: NEED-002, DEC-004, NEED-009.
- **REQ-003**: Local server configuration MUST express the launch command, arguments, and environment; remote configuration MUST express the server URL and applicable authentication inputs. Exact representation is a delegated specification/design detail. Sources: NEED-002, NEED-007, DEC-009.
- **REQ-004**: Fox MUST resolve user-level and project-level server definitions with whole-definition project precedence for equal names and coexistence for different names. Sources: NEED-004, DEC-005.
- **REQ-005**: Fox MUST provide file-based configuration, CLI management commands, and a TUI management entry point covering add/remove, enable/disable, connection status, and OAuth login/logout. Scope-targeting MUST be explicit for management mutations. Sources: NEED-008, DEC-005.
- **REQ-006**: Configuration changes, including enable/disable settings, MUST apply to new MCP sessions. An existing Fox session MUST retain its resolved configuration until an explicit reconnect applies the changed configuration after active tool calls finish. Sources: DEC-005.
- **REQ-007**: Remote authentication MUST support unauthenticated operation, configured tokens/HTTP headers, and OAuth login authorization in the first release. Sources: NEED-007.
- **REQ-008**: Fox MUST support explicit OAuth login/logout, persist credentials in user-local storage separate from shareable project configuration, refresh when supported, and expose reauthorization requirements. Sources: NEED-007, NEED-008, DEC-007.
- **REQ-009**: When required authorization or approval cannot be obtained in an unattended entry, Fox MUST report an actionable error without waiting indefinitely. Previously available credentials and permissions remain usable where valid. Sources: DEC-007, CON-001, NEED-009.
- **REQ-010**: MCP capabilities MUST be available through CLI, TUI, Feishu, AgentOps, Autodev, Benchmark, and authorized child-agent execution, within each entry's existing permission and capability restrictions. Sources: NEED-003, CON-001, CON-002.
- **REQ-011**: Tool advertisement and execution MUST both enforce effective caller restrictions. Server metadata alone MUST NOT grant permission. Sources: CON-001, DEC-007.
- **REQ-012**: A main agent and its children MUST share the enclosing Fox session's MCP session and browser state, while authorization is checked separately for each caller. Child completion MUST NOT close connections owned by the enclosing session. Sources: DEC-002, DEC-006, CON-001.
- **REQ-013**: Independent Fox sessions MUST own separate client connections. Connections MUST persist across normal turns and calls to an individual server MUST execute serially by default, including parent/child calls. Feishu messages from the same chat/user MUST retain the live session, browser state and frozen MCP configuration across task completion, cancellation and failure, until an explicit new-session directive or service shutdown closes the owner. Per-message permissions, approval correlation and run artifacts remain independent. Restarting the service restores history only, subject to REQ-016. Separate connections MUST NOT be represented as a guarantee of isolation inside a remotely managed server. Sources: DEC-002, DEC-006, DEC-010.
- **REQ-014**: Fox MUST expose configurable bounded startup/call timeouts, cancellation, cleanup, and explicit reconnect. The owning session MUST close its owned connections and local server processes when it ends. Sources: DEC-006.
- **REQ-015**: Connection failure, timeout, incompatible protocol, and reauthorization requirements MUST make the affected server's state visible without blocking unrelated servers or internal tools; dependent operations MUST return explicit errors. Sources: NEED-009, DEC-004, DEC-007.
- **REQ-016**: Fox MUST NOT automatically replay a tool call whose side effects are uncertain. If a server session is lost or replaced, Fox MUST report possible loss of browser state and MUST NOT equate restored conversation history with restored live browser state. Sources: DEC-006.
- **REQ-017**: Fox MUST support text, structured data, images, and embedded text/image resource contents in tool results, preserving supported content with its originating call. Sources: DEC-001, DEC-008.
- **REQ-018**: Fox MUST preserve tool-returned resource links and support authorized on-demand resource reads necessary to use those results, without requiring standalone Resources browsing. Sources: DEC-001, DEC-008, OUT-002.
- **REQ-019**: Playwright screenshots MUST reach the configured image-capable model as image input through both existing provider adapter families. A file path or textual description alone MUST NOT count as model-visible image support. Sources: NEED-006, DEC-008.
- **REQ-020**: Fox MUST preserve screenshot artifacts and references for subsequent session replay and user access while continuing to load existing text-only history. Sources: DEC-008, CON-002.
- **REQ-021**: When the configured model cannot accept images, Fox MUST retain user access to screenshots, explicitly report unavailable visual analysis, continue supported text-snapshot operations, and refrain from automatic model switching. Sources: DEC-003, NEED-006.
- **REQ-022**: Unsupported result formats MUST produce an explicit indication rather than silent loss or a false claim of successful interpretation. Sources: DEC-008.
- **REQ-023**: Playwright acceptance MUST cover navigation, accessibility snapshots, clicking, typing, state continuity across turns and an authorized parent/child handoff, and model-visible screenshot analysis. Sources: NEED-005, NEED-006, DEC-002, DEC-009.
- **REQ-024**: Fox MUST document the tested transport/protocol/server compatibility matrix and provide a separately runnable, pinned Playwright acceptance workflow. The exact SDK, supported protocol-version set, and pin are delegated implementation choices. Sources: DEC-004, DEC-009.

## Non-Functional Requirements

- **NFR-001**: MCP integration MUST use the existing shared runtime and tool abstractions; protocol transport mechanisms MUST remain outside the engine. Package dependency changes MUST update dependency documentation and executable architecture checks together. Sources: CON-002.
- **NFR-002**: New capabilities MUST preserve existing permission modes, constrained single-level child execution, and existing persisted-session compatibility. Sources: CON-001, CON-002, DEC-002, DEC-008.
- **NFR-003**: Startup, calls, and shutdown cleanup MUST be bounded; an unavailable external service MUST NOT render unrelated capabilities unusable. Numeric bounds and output limits are determined during design. Sources: NEED-009, DEC-006, DEC-009.
- **NFR-004**: Authentication state MUST remain in user-local storage separately from shareable project configuration, and external tool metadata MUST NOT substitute for caller authorization. Sources: DEC-007, CON-001.
- **NFR-005**: Ordinary automated tests MUST be deterministic and independent of live external servers. Playwright integration acceptance MUST be separately runnable and exercise the actual MCP integration. Sources: DEC-009, NEED-005, NEED-006.
- **NFR-006**: Image-result changes MUST maintain readable, usable old text-only session data and resolvable image references on replay. Sources: CON-002, DEC-008.

The project constitution additionally governs implementation: test-first code changes, Go formatting, focused testability, validation at boundaries, and the full repository test command `go test ./...`. These are existing project obligations, not newly inferred product scope.

## Expected Errors and Boundaries

| Condition | Required observable outcome | Coverage |
|---|---|---|
| One server cannot start/connect or negotiates an unsupported version | Identify unavailable server; allow other services/tools to continue | REQ-002, REQ-015 |
| Token expires and cannot refresh, or interaction is unavailable | Actionable reauthorization/approval error; no indefinite wait | REQ-008, REQ-009 |
| Caller is forbidden from using a tool or linked resource | Do not dispatch the forbidden operation | REQ-011, REQ-018 |
| Timeout/cancellation occurs after dispatch | Bounded terminal handling; report error/cancellation; no automatic uncertain-call replay | REQ-014, REQ-016 |
| Configuration changes during an active call | Keep that call on its established session; apply changes only through new-session or explicit-reconnect semantics | REQ-006 |
| Child completes or is cancelled | Do not dispose of the parent's shared connection solely because the child ends | REQ-012, REQ-014 |
| Connection/session is replaced | Surface potential browser-state loss; do not claim history restores the browser | REQ-016 |
| Screenshot returned to a non-image model | Preserve image for user; explicit limitation; supported textual work continues | REQ-020, REQ-021 |
| Unsupported result content | Explicit unsupported-content indication, without false successful interpretation | REQ-022 |
| Remote server itself shares global browser state | Do not claim client connection separation guarantees server-side isolation | REQ-013 |

## Key Entities

- **Server definition**: Named connection, launch or endpoint settings, authentication references, enabled state, configuration scope, and lifecycle limits.
- **Resolved server configuration**: The effective definition after user/project precedence, held stable for an established MCP session.
- **MCP session**: Negotiated connection owned by one independent Fox session and borrowed by its authorized children; status, serialized calls, and cleanup belong to that owner.
- **External tool identity**: The origin server and remote tool name, mapped into Fox's callable tool namespace while preserving caller authorization and result correlation.
- **Credential record**: User-local authorization state associated with the effective remote service; kept separate from project configuration.
- **Rich tool result and artifact**: Ordered result content and associated structured data, images, resource references, error indication, and persisted artifacts necessary for model input and replay.

These entities describe responsibilities; concrete Go types and package names remain design choices under DEC-009.

## Success Criteria and Verification Matrix

| Criterion | Required evidence | Requirements |
|---|---|---|
| SC-001: Three transports function | Hermetic tool discovery/call tests for stdio, Streamable HTTP, and legacy HTTP+SSE, including failure cases | REQ-001–003, REQ-015, REQ-024 |
| SC-002: User configuration is predictable | Whole-server precedence, scope-specific management, new-session activation and explicit reconnect tests for files/CLI/TUI | REQ-004–006 |
| SC-003: Authentication is usable | No-auth/static/OAuth fixture tests including login/logout, refresh, and unavailable-interaction errors | REQ-007–009, NFR-004 |
| SC-004: All entry compositions work | Entry matrix proves successful authorized use and denied restricted use for all seven named entry categories | REQ-010–012, NFR-001–002 |
| SC-005: Stateful lifecycle is correct | Parent/child continuity, independent connections, serial dispatch, bounded cancellation/cleanup, and no uncertain replay | REQ-012–016, NFR-003 |
| SC-006: Rich results survive the full path | Fixture content tests, authorized linked reads, actual outgoing image requests for both provider families, user artifacts and persisted replay | REQ-017–022, NFR-006 |
| SC-007: Playwright workflow succeeds | Pinned integration run against a controlled test page covers navigation, snapshot, type, click, child continuation, and screenshot input; visual-analysis demonstration uses a configured image-capable model | REQ-023–024 |
| SC-008: Existing behavior stays compatible | Existing history fixtures, architecture checks, and `go test ./...` pass without requiring live MCP services | NFR-001–002, NFR-005–006 |

## Confirmed Decisions and Constraints

The acceptance boundary is external tool use rather than all MCP server features. Existing permissions apply at advertisement and dispatch; shared state never grants broader child access. Configurations are frozen for an established session, and deliberate reconnect applies changes. Failures remain localized. Both multimodal capability and its explicit non-image fallback are first-release behavior. Existing runtime, engine, and session boundaries remain in force.

Exact numeric limits, storage mechanisms, configuration representation, dependency selection, protocol-version matrix, and command naming may be chosen during specification/design under DEC-009, but those choices cannot remove one of the three transports, omit a named entry point, weaken caller permissions, or replace image input with a path string.

## Open Items and Deferred Decisions

There are no blocking product-scope questions. The following user-approved deferrals must be resolved in design before implementation:

| Upstream item | Delegated detail | Required boundary |
|---|---|---|
| OPEN-001 | OAuth implementation compatibility and credential backend | REQ-007–009, NFR-004 |
| OPEN-002 | Configuration paths/fields and CLI/TUI command spelling | REQ-003–006 |
| OPEN-003 | Numeric lifecycle/output bounds and profile composition | REQ-010–016, NFR-003 |
| OPEN-004 | Image capability detection, encoding, and artifact representation | REQ-017–022, NFR-006 |
| OPEN-005 | SDK, protocol versions, Playwright pin, and package layout | REQ-002, REQ-024, NFR-001 |

OPEN-006 was resolved by the user's final confirmation. Open entries themselves are not promoted into binding product requirements; the authority to make these implementation choices is confirmed DEC-009.

## Out of Scope

- Standalone MCP Resources browsing and MCP Prompts template entry points. Tool-returned embedded resources and necessary authorized linked-resource reads remain included. Sources: OUT-002, DEC-001, DEC-008.
- Exposing Fox as an MCP server to other clients. Sources: OUT-002.

## Assumptions and External Dependencies

No additional product assumptions are needed. Live acceptance requires an installed/pinned Playwright MCP server and browser runtime; actual screenshot analysis requires a configured image-capable model. Remote server internal state is outside Fox's control. These are explicit test/environment dependencies, not guarantees that Fox installs all external prerequisites or isolates a server's global state.

## Requirements Traceability

| Confirmed entry | Specification coverage | Result |
|---|---|---|
| NEED-001 | REQ-001; Story 1 | Full |
| NEED-002 | REQ-002–003; Story 1 | Full |
| NEED-003 | REQ-010; Story 6; SC-004 | Full |
| NEED-004 | REQ-004; Story 4 | Full |
| NEED-005 | REQ-023–024; Story 2; SC-007 | Full |
| NEED-006 | REQ-019–021, REQ-023; Story 3 | Full |
| NEED-007 | REQ-003, REQ-007–008; Story 4 | Full |
| NEED-008 | REQ-005, REQ-008; Story 4 | Full |
| NEED-009 | REQ-002, REQ-009, REQ-015; NFR-003; Story 5 | Full |
| CON-001 | REQ-001, REQ-009–012; NFR-002, NFR-004 | Full |
| CON-002 | REQ-010, REQ-020; NFR-001–002, NFR-006 | Full |
| DEC-001 | REQ-017–018; Story 7; Out of Scope | Full |
| DEC-002 | REQ-012–013, REQ-023; NFR-002; Story 2 | Full |
| DEC-003 | REQ-021; Story 3; expected-errors table | Full |
| DEC-004 | REQ-002, REQ-024; SC-001 | Full |
| DEC-005 | REQ-004–006; Story 4 | Full |
| DEC-006 | REQ-012–016; NFR-003; Story 5 | Full |
| DEC-010 | REQ-006, REQ-013, REQ-014, REQ-016; Feishu message continuity | Full |
| DEC-007 | REQ-008–009, REQ-011; NFR-004 | Full |
| DEC-008 | REQ-017–022; NFR-002, NFR-006; Story 7 | Full |
| DEC-009 | REQ-003, REQ-024; NFR-003, NFR-005; deferred-decisions table | Full |
| OUT-002 | Out of Scope; REQ-018 | Full |
