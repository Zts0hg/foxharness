# MCP client support

Fox connects to MCP servers and exposes their tools through its normal runtime.
CLI Exec, TUI, Feishu, AgentOps, Autodev, Benchmark, and authorized writable child
agents use the same integration. MCP Resources browsing, Prompts, and exposing
Fox as an MCP server are outside this release. Tools can still return embedded
resources and resource links; a per-server helper reads known links when the
caller also has permission for the originating tool.

## Playwright quick start

Install Node.js 18 or newer and Chrome, then run:

```sh
fox mcp add browser --scope project --transport stdio -- npx -y @playwright/mcp@0.0.83 --headless --isolated
fox mcp status
fox exec "Open the page, take a screenshot, and analyze its layout"
```

`fox mcp --workdir /path/to/project status` selects a project explicitly.
The command runs before LLM configuration is loaded. `status` opens and closes
**temporary probe connections**; it does not inspect or reconnect another Fox
process. `status NAME` probes only the selected server; `list` shows the effective
redacted configuration without opening connections. In TUI, `/mcp` and
`/mcp status` report the active session's actual connections.
Playwright 0.0.83 returns some automatic snapshots as file links; call
`browser_snapshot` for the inline accessibility snapshot. Its interaction tools
accept `target` (an element reference or unique selector). Fox uses discovered
schemas rather than hard-coding Playwright tool arguments.

## Configuration and scope

Fox reads `~/.foxharness/mcp.json` and `<workDir>/.foxharness/mcp.json`.
A project definition **replaces the entire** same-named user definition,
including credentials and enablement. Fox does not search parent directories.
Missing files mean no MCP servers. Special files such as named pipes are rejected
before reading. A malformed file is reported and is never
overwritten by a management edit; invalid definitions disable only that server.

```json
{
  "mcpServers": {
    "browser": {
      "transport": "stdio",
      "command": "npx",
      "args": ["-y", "@playwright/mcp@0.0.83", "--headless", "--isolated"],
      "enabled": true,
      "startup_timeout_seconds": 30,
      "call_timeout_seconds": 120
    },
    "remote": {
      "transport": "streamable-http",
      "url": "https://mcp.example.com/mcp",
      "headers_from_env": {"X-Service-Key": "SERVICE_KEY"},
      "auth": {"type": "bearer", "token_env": "MCP_TOKEN"}
    }
  }
}
```

Available transports: `stdio`, `streamable-http`, legacy `sse`, and `http`.
`http` tries Streamable HTTP, falling back to SSE only on eligible initialize
400/404/405 responses. It never switches transport on 401/403.
Use `env` for literal subprocess environment values, `env_from` for mappings
from subprocess variable names to host environment-variable names, `headers`
for literal HTTP headers, and `headers_from_env` for environment references.
Prefer references for secrets. Status omits raw headers, arguments, and tokens.
Subprocess working directories are the effective definition's scope directory.

## Management

The same actions work as `fox mcp ...` and `/mcp ...` in TUI:

```text
add NAME --scope user|project --transport stdio -- COMMAND ARGS...
add NAME --scope user|project --transport http|streamable-http|sse --url URL
add NAME --scope user|project --json '{"transport":"stdio","command":"server"}'
remove NAME --scope user|project
enable NAME --scope user|project
disable NAME --scope user|project
list
status [NAME]
login NAME
logout NAME
```

Mutations require an explicit scope. `add` refuses to overwrite an existing
name; edit the file or remove/add within the chosen scope. `--json` accepts the
complete definition for environment, header, timeout, and OAuth options.
TUI parsing supports quotes and escapes without executing shell expansions.

Configuration edits affect new sessions. `/mcp reconnect NAME` waits for the
active run and in-flight server call, reloads the effective definition, and
replaces its tool generation. Existing adapters cannot dispatch against a new
generation. Browser state may be lost. `/mcp cancel` cancels a pending management
operation, including login. CLI cancellation uses Ctrl-C. Status distinguishes
disabled, connecting, ready, authorization-required, failed, and closed states.

## Authentication

`auth.type` accepts `none`, `headers`, `bearer`, or `oauth`. `headers` uses the
configured header fields. OAuth is explicit:

```sh
fox mcp add remote --scope user --transport streamable-http --url https://mcp.example.com/mcp --auth oauth
fox mcp login remote
```

Open the displayed URL in a browser. Fox owns a temporary loopback callback,
validates state/issuer, and uses PKCE. Login expires after five minutes and closes
the listener on cancellation. OAuth configuration also accepts `client_id`,
`issuer`, `client_secret_env`, `client_metadata_url`, `scopes`, and `redirect_port`
through JSON. A client secret requires both `client_id` and the trusted `issuer`
URL from that client registration. Fox rejects conflicting discovered issuers
before presenting authorization or sending the secret. Changing the configured
issuer also changes credential identity; log in again after such a change.
Without an existing client ID or metadata URL, the authorization server must support dynamic client registration. Unsupported server registration is an
explicit login failure, not an unattended browser prompt.

Tokens and refresh configuration are stored in the user-local
`~/.foxharness/mcp-credentials.json` with mode 0600 (directory 0700), separately
from project configuration. Refresh is serialized and persisted atomically.
The authorized MCP resource is persisted and included in refresh requests,
including after restart.
Credentials are bound to endpoint, client and effective configuration scope.
`logout` removes the local credential; reconnect active sessions afterward.
Unattended entries never launch login and report authorization requirements.
Credential-bearing requests and redirects are confined to their configured
origin. OAuth discovery/token requests have a 30-second HTTP bound.

## Ownership, permissions, and failures

A live root session owns connections across its runs. Children borrow those
connections, so an authorized writable child can continue the parent's browser
page. Child completion does not close the parent connection. Independent roots,
benchmark repeats, reconnects, and reopened sessions create separate connections;
external shared browser profiles can still share state outside Fox. Use
Playwright `--isolated` when independent browser state is needed. Remote task
completion ends that task's live runtime; reopening persisted conversation
history cannot restore an old browser process.

Host-generated tool names include a bounded server/tool prefix and a stable
hash. External names enter capability ceilings before the normal profile,
parent, persona, and explicit-tool restrictions. Explicit empty restrictions
remain deny-all. Unknown-effect MCP tools are unavailable to read-only children.
Server read-only annotations do not authorize calls. TUI and remote entries use
their existing approvals; full-access and existing no-coordinator entries retain
their existing policies. Formal Plan continues to expose only its constrained
tool surface.

Calls to one server are serial; queue wait counts toward its timeout. Default
startup/call bounds are 30/120 seconds, individually configurable. Connection
cleanup is bounded to five seconds and subprocess descendants are terminated.
Terminal close still releases external connections if persisting run completion
fails; the persistence error remains available for recovery. Cleanup runs once.
Protocol frames/SSE events are limited to 32 MiB, subprocess stderr to 64 KiB.
MCP tool `isError` is distinct from a transport failure. A call whose response is
lost may already have executed: Fox reports uncertainty and does not replay it.
Reconnect is explicit and can lose browser state. Other servers/internal tools
remain usable when a server fails; status and startup diagnostics identify it.
Active call timeouts and resource connection failures mark the server unavailable
until an explicit reconnect. Post-connect authorization failures instead request
login followed by reconnect. Canceling a queued request or a caller's otherwise
healthy operation does not invalidate the shared connection.

## Screenshots and models

PNG/JPEG screenshots are validated (20 MiB maximum, 100 million pixels maximum)
and stored under the session's `media/` directory before their references enter
history. Tool output identifies the artifact for user access; history stores
references rather than base64. Replay hydrates verified, confined artifacts.
Missing/corrupt images produce an explicit unavailable-image notice and preserve
textual history. Compaction budgets images and does not claim omitted images were
viewed; rewind does not delete durable image artifacts.

Both OpenAI-compatible and Anthropic adapters send actual image content. Set
`image_input` in the configured LLM provider profile to `auto` (default),
`enabled`, or `disabled`. Disabled mode retains screenshots and reports that
visual analysis is unavailable. Auto mode makes at most one same-model text
fallback for a verified unsupported-image rejection; ordinary network/auth
failures do not silently discard images, and tools are never rerun for fallback.
Unsupported audio and other content formats receive explicit text notices.

## Compatibility and verification

The Go SDK is pinned to `github.com/modelcontextprotocol/go-sdk v1.8.0`.
Fixtures cover 2024-11-05, 2025-03-26, 2025-06-18, 2025-11-25, and 2026-07-28.
Legacy SSE negotiates at most 2025-11-25; a requested 2026-07-28 version downgrades.
The 2026-07-28 HTTP fixture uses the SDK's stateless transport semantics. These
are tool-client compatibility tests, not a claim to implement every optional
MCP capability.

```sh
go test ./...
go test -race ./internal/mcpconfig ./internal/mcpauth ./internal/mcpclient ./internal/mcpintegration ./internal/runtime ./internal/childruntime
FOX_MCP_PLAYWRIGHT=1 go test ./internal/mcpintegration -run TestPlaywrightAcceptance -count=1 -v
```

Ordinary tests use local fixtures and skip live browser dependencies. The
Playwright acceptance uses a controlled local page, navigation, snapshot, typing,
clicking, child continuity, screenshots, persistence, and actual HTTP request
encoding for both provider families. It passed with Node 22.18.0 and Playwright
MCP 0.0.83. It requires an installed Chrome browser.

The separate live-model subtest runs only when `FOX_MCP_LIVE_MODEL` is set, with
`FOX_MCP_LIVE_PROTOCOL`, `FOX_MCP_LIVE_BASE_URL`, and `FOX_MCP_LIVE_API_KEY`.
It asks the model to identify a visual property from the screenshot. On
2026-10-04, the configured `zhipu` profile (`glm-5.2`, Claude-compatible protocol)
returned “blue” for the controlled page’s orange badge, so that live-model
acceptance **failed**. Browser operation and exact provider image encoding passed.
A successful demonstration with an image-capable model remains required; neither
a skipped test nor successful request encoding proves model visual reasoning.
