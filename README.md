# foxharness

foxharness is a Go-based AI coding agent. It runs in your terminal, reads the current project, calls local tools, keeps session history across multiple runs, and provides a TUI experience similar to Claude Code and Codex.

The default binary command is `fox`.

## Features

- Interactive TUI by default: run `fox` inside a project and chat continuously.
- One-shot CLI mode: run a single task with `fox exec` or `fox -p`.
- Session continuity: multiple runs can share one session and one raw message history.
- Project instructions: automatically loads `AGENTS.md` from the current workspace.
- Skills and slash commands: loads foxharness files under `.foxharness/` and Claude Code-compatible files under `.claude/`.
- Plan mode: can generate and use `PLAN.md`, `TODO.md`, and `MEMORY.md`.
- Tool execution: file reading, file writing, fuzzy edit, bash, and delegated subagent tasks.
- Local trace data: stores transcripts, metrics, traces, and run metadata under `~/.foxharness`.

## Install

### Option 1: Download a release binary

Download the binary for your platform from:

```text
https://github.com/Zts0hg/foxharness/releases
```

macOS Apple Silicon:

```bash
curl -fL https://github.com/Zts0hg/foxharness/releases/latest/download/fox_darwin_arm64.tar.gz | tar xz
chmod +x fox
sudo mv fox /usr/local/bin/fox
```

macOS Intel:

```bash
curl -fL https://github.com/Zts0hg/foxharness/releases/latest/download/fox_darwin_amd64.tar.gz | tar xz
chmod +x fox
sudo mv fox /usr/local/bin/fox
```

Linux amd64:

```bash
curl -fL https://github.com/Zts0hg/foxharness/releases/latest/download/fox_linux_amd64.tar.gz | tar xz
chmod +x fox
sudo mv fox /usr/local/bin/fox
```

Linux arm64:

```bash
curl -fL https://github.com/Zts0hg/foxharness/releases/latest/download/fox_linux_arm64.tar.gz | tar xz
chmod +x fox
sudo mv fox /usr/local/bin/fox
```

Windows users can download one of these archives from the release page:

- `fox_windows_amd64.zip`
- `fox_windows_arm64.zip`

Unzip it and add the directory containing `fox.exe` to your `PATH`.

On macOS, if Gatekeeper blocks the downloaded binary, remove the quarantine flag:

```bash
xattr -d com.apple.quarantine /usr/local/bin/fox
```

### Option 2: Install from source

This requires Go 1.25 or newer.

```bash
go install github.com/Zts0hg/foxharness/cmd/fox@latest
```

Make sure `$GOPATH/bin` is in your `PATH`.

## Configure

foxharness uses Zhipu BigModel's coding endpoint by default. The default provider protocol is OpenAI-compatible, and a Claude-compatible Anthropic Messages protocol adapter is also available. Set your API key before running `fox`:

```bash
export ZHIPU_API_KEY="your-api-key"
```

Optional retry and timeout settings:

```bash
export FOXHARNESS_LLM_MAX_ATTEMPTS=4
export FOXHARNESS_LLM_RETRY_INITIAL_DELAY=750ms
export FOXHARNESS_LLM_RETRY_MAX_DELAY=8s
export FOXHARNESS_LLM_REQUEST_TIMEOUT=60s
```

### Provider Protocols

Use `-provider openai` for the default OpenAI-compatible Chat Completions protocol:

```bash
fox exec -provider openai "Inspect this project for potential bugs"
```

Use `-provider claude` for the Claude-compatible Anthropic Messages protocol:

```bash
fox exec -provider claude "Inspect this project for potential bugs"
```

Both modes use the same internal agent messages and tools. The provider adapter translates them into the target protocol:

| Area | OpenAI-compatible protocol | Claude-compatible protocol |
| --- | --- | --- |
| System prompt | Sent as a `system` role message. | Sent through the top-level `system` field. |
| Tool calls | Assistant message includes `tool_calls`; tool results use `tool` role messages. | Assistant content includes `tool_use` blocks; tool results are user messages with `tool_result` blocks. |
| Tool schema | Function parameters are nested under `tools[].function.parameters`. | Input schema is sent as `tools[].input_schema`. |
| Response content | Text and tool calls are separate fields on the assistant message. | Text and tool calls are mixed content blocks and normalized back into foxharness messages. |

## Quick Start

Open any project directory and start the TUI:

```bash
cd /path/to/your/project
fox
```

Or specify the project directory explicitly:

```bash
fox -C /path/to/your/project
```

Run a one-shot task and print the answer:

```bash
fox exec "Inspect the current project for potential bugs"
```

Claude-style print mode is also supported:

```bash
fox -p "Summarize this project's architecture"
```

Read the task from stdin:

```bash
echo "Run the tests and explain any failures" | fox exec -
```

## TUI Usage

Inside the TUI:

- `Enter`: send the current message.
- `Shift+Tab`: toggle Plan Mode for future runs.
- `Up` / `Down`: navigate within multiline input; at the beginning or end, switch through input history.
- `PgUp` / `PgDown` or mouse wheel: scroll the conversation.
- Drag over transcript text: copy the selection to the macOS clipboard.
- `Ctrl+F`: focus the right sidebar, then use `Tab`, `Up` / `Down`, `PgUp` / `PgDown`, `Home`, and `End` to browse its boxes.
- `/`: open slash command suggestions.
- `Esc`: cancel the active run.
- `Ctrl+C` twice within two seconds: quit.

Slash commands:

| Command | Description |
| --- | --- |
| `/session` | Show current session paths. |
| `/clear` | Clear the visible transcript. |
| `/new` | Start a fresh session. |
| `/cancel` | Cancel the active run. |
| `/help` | Show available commands. |
| `/exit` | Quit the TUI. |

## CLI Usage

```bash
fox [options] [prompt]       # start the interactive TUI
fox exec [options] [prompt]  # run once and print the result
fox -p [options] [prompt]    # run once and print the result
```

Common options:

| Option | Description |
| --- | --- |
| `-C`, `-workdir` | Working directory. Defaults to `.`. |
| `-model` | Model name. Defaults to `glm-4.5-air`. |
| `-provider` | Provider protocol: `openai` or `claude`. Defaults to `openai`. |
| `-plan` | Enable Plan Mode. Defaults to `true`. |
| `-thinking` | Enable legacy per-turn thinking mode when Plan Mode is not used. |
| `-max-turns` | Maximum agent turns. Defaults to unlimited; use a positive value to cap turns. |
| `-c`, `-continue` | Resume the latest CLI session. |
| `-r`, `-session` | Resume a specific session ID. |
| `-new` | Force creation of a new session. |
| `-p`, `-print` | Run once and print the result without TUI. |

Examples:

```bash
fox exec -plan=false "Inspect the code only; do not modify files"
fox exec -continue "Fix the bugs found in the previous run"
fox exec -session 20260517-192517-a504c5 "Continue this session and summarize the current progress"
fox exec -model glm-4.5-air "Add tests for this project"
fox exec -provider claude "Summarize the architecture of this project"
```

## Project Instructions

foxharness loads project-level instructions from:

```text
AGENTS.md
```

Put coding rules, test commands, style constraints, and project-specific guidance there.

Example:

```markdown
# AGENTS.md

## Commands

- Run all tests with `go test ./...`.
- Format Go files with `gofmt -w`.

## Rules

- Do not edit files under `vendor/`.
- Prefer focused edits over whole-file rewrites.
```

## Slash Commands and Skills

foxharness loads project and user slash commands from both native foxharness
directories and Claude Code-compatible directories:

```text
.foxharness/commands/<command>.md
.foxharness/skills/<skill-name>/SKILL.md
.claude/commands/<command>.md
.claude/skills/<skill-name>/SKILL.md
~/.foxharness/commands/<command>.md
~/.foxharness/skills/<skill-name>/SKILL.md
~/.claude/commands/<command>.md
~/.claude/skills/<skill-name>/SKILL.md
```

Project-level commands override user-level commands. At the same level,
`.foxharness` commands override `.claude` commands with the same name.

Reference a skill in your prompt with `$skill-name`:

```bash
fox exec "Use $go-refactor to refactor internal/session"
```

`SKILL.md` can include optional frontmatter:

```markdown
---
name: go-refactor
description: Go refactoring guidance for this project
---

Follow the existing package boundaries and preserve public APIs unless asked.
```

## Sessions and Data

Session data is stored outside your project directory:

```text
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/
```

Each session can contain multiple runs:

```text
messages.jsonl
session.json
transcript.jsonl
working_memory.md
runs/<run-id>/run.json
runs/<run-id>/metrics.jsonl
runs/<run-id>/trace.jsonl
```

This means a user can start a task, inspect the answer, and then continue with a follow-up message in the same session.

Plan Mode may create or update these files in the project root:

```text
PLAN.md
TODO.md
MEMORY.md
```

If you want these files to stay local, add them to your project's `.gitignore`.

## Development

Clone the repository:

```bash
git clone https://github.com/Zts0hg/foxharness.git
cd foxharness
```

Run tests:

```bash
go test ./...
```

Format Go files:

```bash
gofmt -w ./cmd ./internal
```

Run from source:

```bash
go run ./cmd/fox
go run ./cmd/fox exec "Inspect the current project"
```

Build locally:

```bash
go build -trimpath -ldflags="-s -w" -o fox ./cmd/fox
```

## License

foxharness is licensed under the GNU Affero General Public License v3.0 or later (`AGPL-3.0-or-later`).

Commercial use is allowed, but modified versions distributed or offered as a network service must remain open source under the same license.

## Release

The GitHub Actions release workflow builds binaries for:

- macOS amd64
- macOS arm64
- Linux amd64
- Linux arm64
- Windows amd64
- Windows arm64

Each release uploads both versioned archives, such as `fox_vX.Y.Z_linux_amd64.tar.gz`, and stable latest-download archives, such as `fox_linux_amd64.tar.gz`.

To publish the next patch release from the latest remote `main`, run:

```bash
scripts/release-patch.sh --dry-run
scripts/release-patch.sh
```

The script finds the latest `vMAJOR.MINOR.PATCH` tag, increments the patch number, tags `origin/main`, and pushes the new tag. Pushing the tag triggers the release workflow.

To publish a specific version manually, create and push a version tag:

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```
