<p align="center">
  <img src="assets/banner.png" alt="foxharness" width="100%">
</p>

<p align="center">
  <a href="README.md">English</a> | <b>简体中文</b> | <a href="README.zh-TW.md">繁體中文</a> | <a href="README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go" alt="Go"></a>
  <a href="https://opensource.org/licenses/AGPL-3.0"><img src="https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge" alt="License"></a>
</p>

foxharness 是一个基于 Go 语言的 AI 编程助手。它可在终端中运行，能够读取当前项目、调用本地工具、在多次运行中保留会话历史记录，并提供丰富的交互式终端用户界面（TUI）体验。

默认命令为 `fox`。

## 功能特性

- 交互式 TUI：在项目目录下运行 `fox` 即可进入持续对话模式。
- 单次命令行模式：使用 `fox exec` 或 `fox -p` 快速执行单个任务。
- 会话连续性：多次运行共享同一个会话和完整的消息历史记录。
- 项目指令：自动加载当前工作区中的 `AGENTS.md` 作为项目指引。
- 技能与斜杠命令：支持加载 `.foxharness/` 目录下的原生命令，也兼容 `.claude/` 目录下的 Claude Code 格式。
- 正式计划模式：在实施前显式生成、修订并审批方案，计划与执行清单仅保存在当前会话中。
- 工具执行：支持文件读写、模糊编辑、Bash 命令执行，以及委托子 Agent 任务。
- 本地追踪数据：在 `~/.foxharness` 目录下保存会话记录、性能指标、调用链追踪和运行元数据。

## 安装

### 方式一：下载预编译二进制文件

从以下地址下载适用于你平台的二进制文件：

```text
https://github.com/Zts0hg/foxharness/releases
```

macOS Apple Silicon：

```bash
curl -fL https://github.com/Zts0hg/foxharness/releases/latest/download/fox_darwin_arm64.tar.gz | tar xz
chmod +x fox
sudo mv fox /usr/local/bin/fox
```

macOS Intel：

```bash
curl -fL https://github.com/Zts0hg/foxharness/releases/latest/download/fox_darwin_amd64.tar.gz | tar xz
chmod +x fox
sudo mv fox /usr/local/bin/fox
```

Linux amd64：

```bash
curl -fL https://github.com/Zts0hg/foxharness/releases/latest/download/fox_linux_amd64.tar.gz | tar xz
chmod +x fox
sudo mv fox /usr/local/bin/fox
```

Linux arm64：

```bash
curl -fL https://github.com/Zts0hg/foxharness/releases/latest/download/fox_linux_arm64.tar.gz | tar xz
chmod +x fox
sudo mv fox /usr/local/bin/fox
```

Windows 用户可以从发布页面下载以下压缩包：

- `fox_windows_amd64.zip`
- `fox_windows_arm64.zip`

解压后将 `fox.exe` 所在目录添加到 `PATH` 环境变量中。

在 macOS 上，如果 Gatekeeper 阻止了下载的二进制文件运行，可以移除隔离标记：

```bash
xattr -d com.apple.quarantine /usr/local/bin/fox
```

### 方式二：从源码编译安装

需要 Go 1.25 或更高版本。

```bash
go install github.com/Zts0hg/foxharness/cmd/fox@latest
```

确保 `$GOPATH/bin` 已加入 `PATH` 环境变量。

## 配置

foxharness 不再内置默认 LLM 提供商。运行 `fox` 前，请在
`~/.foxharness/settings.json` 中配置用户级 provider profile。

最便捷的方式是使用交互式向导，它会自动写入该 profile：

```bash
fox config
```

`fox config` 会引导你从内置预设（OpenAI、Anthropic、Zhipu、DeepSeek、
Moonshot、Qwen、MiniMax、Groq、Mistral、xAI、OpenRouter，或本地 Ollama）
中选择，或填写完全自定义的字段；它会校验 API Key 环境变量是否已设置，
可选地测试连接，并持久化 profile。用 `fox config list` 查看已配置的
profile，用 `fox config default <名称>` 切换默认 provider。若未配置任何
provider 直接启动 `fox`，报错信息会指向 `fox config`。

你也可以直接编辑该文件：

```json
{
  "llm": {
    "default_provider": "primary",
    "providers": {
      "primary": {
        "protocol": "openai",
        "base_url": "https://api.openai.com/v1",
        "model": "gpt-4.1",
        "auth": "api-key",
        "api_key_env": "OPENAI_API_KEY"
      },
      "local": {
        "protocol": "openai",
        "base_url": "http://127.0.0.1:11434/v1",
        "model": "qwen2.5-coder",
        "auth": "none"
      },
      "zhipu": {
        "protocol": "openai",
        "base_url": "https://open.bigmodel.cn/api/coding/paas/v4",
        "model": "glm-4.5-air",
        "auth": "api-key",
        "api_key_env": "ZHIPU_API_KEY"
      }
    }
  }
}
```

配置优先级为：CLI flags、`FOXHARNESS_LLM_*` 环境变量、
`~/.foxharness/settings.json`，最后没有内置默认值。缺少必要 LLM 配置时会直接报出可操作的配置错误。

常规使用建议通过 `api_key_env` 引用密钥，避免把密钥写入项目文件：

```bash
export OPENAI_API_KEY="your-api-key"
```

可选的重试与超时配置：

```bash
export FOXHARNESS_LLM_MAX_ATTEMPTS=4
export FOXHARNESS_LLM_RETRY_INITIAL_DELAY=750ms
export FOXHARNESS_LLM_RETRY_MAX_DELAY=8s
export FOXHARNESS_LLM_REQUEST_TIMEOUT=60s
```

### Provider Profile 与协议

使用 `-llm-provider` 在已命名的 profile 之间切换：

```bash
fox exec -llm-provider local "检查这个项目有没有潜在的 bug"
```

使用 `-protocol` 仅覆盖本次运行使用的 OpenAI/Claude 兼容协议：

```bash
fox exec -protocol claude -base-url https://api.anthropic.com -model claude-sonnet-4-20250514 -api-key-env ANTHROPIC_API_KEY "检查这个项目有没有潜在的 bug"
```

foxharness 没有 `-provider` 参数。它出现在 flag 位置时会被当作未知 flag；
请选择 `-llm-provider` 切换 provider profile，或用 `-protocol` 指定
OpenAI/Claude 兼容协议。

两种协议模式下，内部的 Agent 消息和工具调用完全相同，区别仅在于提供商适配器将消息转换为目标协议的格式：

| 方面 | OpenAI 兼容协议 | Claude 兼容协议 |
| --- | --- | --- |
| 系统提示词 | 以 `system` 角色消息的形式发送。 | 通过顶层的 `system` 字段发送。 |
| 工具调用 | 助手消息中包含 `tool_calls` 字段；工具结果以 `tool` 角色消息返回。 | 助手消息内容中包含 `tool_use` 块；工具结果以用户消息中的 `tool_result` 块返回。 |
| 工具 Schema | 函数参数嵌套在 `tools[].function.parameters` 中。 | 输入 Schema 通过 `tools[].input_schema` 字段发送。 |
| 响应内容 | 文本和工具调用分属助手消息的不同字段。 | 文本和工具调用混合在内容块中，再由框架统一转换为 foxharness 消息格式。 |

## 快速开始

进入任意项目目录，启动 TUI：

```bash
cd /path/to/your/project
fox
```

也可以直接指定项目路径：

```bash
fox -C /path/to/your/project
```

执行单次任务并输出结果：

```bash
fox exec "检查当前项目是否存在潜在问题"
```

也支持无交互的打印模式，方便在脚本中使用：

```bash
fox -p "总结一下这个项目的架构"
```

从标准输入读取任务：

```bash
echo "跑一下测试，说明哪些用例失败了" | fox exec -
```

## TUI 操作指南

TUI 中的快捷键与操作：

- `Enter`：发送当前消息。
- `Shift+Tab`：为下一次提交在 Default 与正式计划模式之间切换。
- `Up` / `Down`：在多行输入中上下移动光标；光标位于首行或末行时，切换浏览历史输入。
- `PgUp` / `PgDown` 或鼠标滚轮：上下滚动对话内容。
- 拖选对话文本：将选中内容复制到 macOS 剪贴板。
- `Ctrl+F`：聚焦右侧边栏，之后可用 `Tab`、`Up` / `Down`、`PgUp` / `PgDown`、`Home`、`End` 浏览边栏内容。
- `/`：弹出斜杠命令列表。
- `!<命令>`：执行本地 shell 命令并显示输出，不发送给模型。
- `Esc`：取消当前正在执行的任务。
- 两秒内连按两次 `Ctrl+C`：退出程序。

斜杠命令一览：

| 命令 | 说明 |
| --- | --- |
| `/session` | 显示当前会话的文件路径。 |
| `/clear` | 清空当前显示的对话内容。 |
| `/new` | 新建一个会话。 |
| `/plan` | 进入正式计划模式；使用 `/plan off` 返回 Default。 |
| `/cancel` | 取消当前正在执行的任务。 |
| `/help` | 显示可用命令列表。 |
| `/exit` | 退出 TUI。 |

## 命令行用法

```bash
fox [选项] [提示词]       # 启动交互式 TUI
fox exec [选项] [提示词]  # 执行单次任务并输出结果
fox -p [选项] [提示词]    # 执行单次任务并输出结果
```

常用选项：

| 选项 | 说明 |
| --- | --- |
| `-C`、`-workdir` | 工作目录，默认为当前目录（`.`）。 |
| `-llm-provider` | 从 `llm.providers` 中选择命名 provider profile。 |
| `-protocol` | 协议覆盖：`openai` 或 `claude`。 |
| `-base-url` | API base URL 覆盖。 |
| `-model` | 模型 id 覆盖。 |
| `-auth` | 认证模式覆盖：`api-key` 或 `none`。 |
| `-api-key-env` | 保存 API key 的环境变量名。 |
| `-api-key` | 直接传入 API key；常规使用优先选择 `-api-key-env`。 |
| `-thinking` | 使用旧版的逐轮思考模式。 |
| `-max-turns` | Agent 最大执行轮次。默认不限制；设为正整数可限制轮次。 |
| `-c`、`-continue` | 继续上一次的 CLI 会话。 |
| `-r`、`-session` | 恢复指定 ID 的会话。 |
| `-new` | 强制创建新会话。 |
| `-p`、`-print` | 执行单次任务并输出结果，不启动 TUI。 |

示例：

```bash
fox exec "只检查代码，不要修改文件"
fox exec -continue "修复上一轮发现的问题"
fox exec -session 20260517-192517-a504c5 "继续这个会话，总结一下当前进展"
fox exec -llm-provider local "给这个项目补充测试"
fox exec -llm-provider primary -model gpt-4.1 "总结一下这个项目的架构"
```

## 项目指令

foxharness 会从项目根目录的以下文件中加载项目级指令：

```text
AGENTS.md
```

你可以在其中编写编码规范、测试命令、风格约束等项目相关的指导内容。

示例：

```markdown
# AGENTS.md

## Commands

- Run all tests with `go test ./...`.
- Format Go files with `gofmt -w`.

## Rules

- Do not edit files under `vendor/`.
- Prefer focused edits over whole-file rewrites.
```

## 斜杠命令与技能

foxharness 会从 foxharness 原生目录和 Claude Code 兼容目录中加载项目级与用户级的斜杠命令：

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

项目级命令优先于用户级命令。在同一级别下，`.foxharness` 中的命令优先于同名 `.claude` 命令。

在提示词中通过 `$技能名` 来引用技能：

```bash
fox exec "Use $go-refactor to refactor internal/session"
```

`SKILL.md` 支持在开头添加可选的 frontmatter 元数据：

```markdown
---
name: go-refactor
description: Go refactoring guidance for this project
---

Follow the existing package boundaries and preserve public APIs unless asked.
```

## 会话与数据

会话数据保存在项目目录之外：

```text
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/
```

每个会话可以包含多次运行记录：

```text
messages.jsonl
session.json
transcript.jsonl
working_memory.md
PLAN.md
TODO.md
runs/<run-id>/run.json
runs/<run-id>/metrics.jsonl
runs/<run-id>/trace.jsonl
```

这意味着你可以先启动一个任务、查看结果，然后在同一个会话中继续对话。

正式计划模式会将最近一次提交的方案保存在当前会话的 `PLAN.md` 中。方案批准后，执行清单保存在同一会话的 `TODO.md` 中；这两个文件都不会写入项目根目录。

## 开发

克隆仓库：

```bash
git clone https://github.com/Zts0hg/foxharness.git
cd foxharness
```

运行测试：

```bash
go test ./...
```

格式化代码：

```bash
gofmt -w ./cmd ./internal
```

从源码直接运行：

```bash
go run ./cmd/fox
go run ./cmd/fox exec "检查一下当前项目"
```

本地编译：

```bash
go build -trimpath -ldflags="-s -w" -o fox ./cmd/fox
```

安装开发版（`foxdev`）：

把当前分支编译成一个全局可用的 `foxdev` 二进制，与正式发布的 `fox` 并存，这样就能在任意项目目录中运行开发中的 foxharness 进行测试：

```bash
./scripts/install-foxdev.sh                  # 安装到 /usr/local/bin（可能需要 sudo）
sudo ./scripts/install-foxdev.sh             # 写入 /usr/local/bin，避免权限错误
PREFIX=~/go/bin ./scripts/install-foxdev.sh  # 免 sudo 安装（需要 ~/go/bin 在 PATH 中）
./scripts/install-foxdev.sh --check          # 编译前先运行 `go test ./...`
```

`fox` 仍是正式发布版；`foxdev` 跟随你最近编译的分支 —— `git switch feat/x && ./scripts/install-foxdev.sh` 即可刷新。`foxdev` 同样需要设置 `ZHIPU_API_KEY`。

## 许可证

foxharness 采用 GNU Affero General Public License v3.0 或更高版本（`AGPL-3.0-or-later`）授权。

允许商业使用，但如果以网络服务的形式分发或提供修改版本，必须在相同许可证下开源。

## 发布

GitHub Actions 发布工作流会为以下平台构建二进制文件：

- macOS amd64
- macOS arm64
- Linux amd64
- Linux arm64
- Windows amd64
- Windows arm64

每次发布会同时上传带版本号的压缩包（如 `fox_vX.Y.Z_linux_amd64.tar.gz`）和固定名称的最新版压缩包（如 `fox_linux_amd64.tar.gz`）。

如果要基于最新的远程 `main` 分支发布下一个补丁版本：

```bash
scripts/release-patch.sh --dry-run
scripts/release-patch.sh
```

该脚本会找到最新的 `vMAJOR.MINOR.PATCH` 标签，自动递增补丁版本号，为 `origin/main` 打上新标签并推送。推送标签后会自动触发发布工作流。

如果需要手动发布指定版本，可以创建并推送版本标签：

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```
