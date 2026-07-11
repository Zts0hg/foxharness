<p align="center">
  <img src="assets/banner.png" alt="foxharness" width="100%">
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <b>繁體中文</b> | <a href="README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go" alt="Go"></a>
  <a href="https://opensource.org/licenses/AGPL-3.0"><img src="https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge" alt="License"></a>
</p>

foxharness 是一個基於 Go 語言的 AI 程式設計助手。它可以在終端機中執行，能夠讀取目前專案、呼叫本地工具、在多次執行中保留工作階段歷史記錄，並提供豐富的互動式終端使用者介面（TUI）體驗。

預設命令為 `fox`。

## 功能特色

- 互動式 TUI：在專案目錄下執行 `fox` 即可進入持續對話模式。
- 單次命令列模式：使用 `fox exec` 或 `fox -p` 快速執行單一任務。
- 工作階段連續性：多次執行共享同一個工作階段和完整的訊息歷史記錄。
- 專案指令：自動載入目前工作區中的 `AGENTS.md` 作為專案指引。
- 技能與斜線命令：支援載入 `.foxharness/` 目錄下的原生命令，也相容 `.claude/` 目錄下的 Claude Code 格式。
- 正式規劃模式：在實作前明確產生、修訂並核准方案，規劃與執行清單僅儲存在目前工作階段中。
- 工具執行：支援檔案讀寫、模糊編輯、Bash 命令執行，以及委派子 Agent 任務。
- 本地追蹤資料：在 `~/.foxharness` 目錄下儲存對話記錄、效能指標、呼叫鏈追蹤和執行中繼資料。

## 安裝

### 方式一：下載預編譯二進位檔案

從以下網址下載適合你平台的二進位檔案：

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

Windows 使用者可以從發佈頁面下載以下壓縮檔：

- `fox_windows_amd64.zip`
- `fox_windows_arm64.zip`

解壓縮後將 `fox.exe` 所在目錄加入 `PATH` 環境變數中。

在 macOS 上，如果 Gatekeeper 阻擋了下載的二進位檔案執行，可以移除隔離標記：

```bash
xattr -d com.apple.quarantine /usr/local/bin/fox
```

### 方式二：從原始碼編譯安裝

需要 Go 1.25 或更高版本。

```bash
go install github.com/Zts0hg/foxharness/cmd/fox@latest
```

確保 `$GOPATH/bin` 已加入 `PATH` 環境變數。

## 設定

foxharness 不再內建預設 LLM 供應商。執行 `fox` 前，請在
`~/.foxharness/settings.json` 中設定使用者層級的 provider profile。

最便捷的方式是使用互動式精靈，它會自動寫入該 profile：

```bash
fox config
```

`fox config` 會引導你從內建預設（OpenAI、Anthropic、Zhipu、DeepSeek、
Moonshot、Qwen、MiniMax、Groq、Mistral、xAI、OpenRouter，或本地 Ollama）
中選擇，或填寫完全自訂的欄位；它會檢查 API Key 環境變數是否已設定，
可選擇性地測試連線，並持久化 profile。用 `fox config list` 檢視已設定的
profile，用 `fox config default <名稱>` 切換預設 provider。若未設定任何
provider 就啟動 `fox`，錯誤訊息會指向 `fox config`。

你也可以直接編輯該檔案：

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

設定優先順序為：CLI flags、`FOXHARNESS_LLM_*` 環境變數、
`~/.foxharness/settings.json`，最後沒有內建預設值。缺少必要 LLM 設定時會直接回報可操作的設定錯誤。

日常使用建議透過 `api_key_env` 引用金鑰，避免把金鑰寫入專案檔案：

```bash
export OPENAI_API_KEY="your-api-key"
```

可選的重試與逾時設定：

```bash
export FOXHARNESS_LLM_MAX_ATTEMPTS=4
export FOXHARNESS_LLM_RETRY_INITIAL_DELAY=750ms
export FOXHARNESS_LLM_RETRY_MAX_DELAY=8s
export FOXHARNESS_LLM_REQUEST_TIMEOUT=60s
```

### Provider Profile 與協定

使用 `-llm-provider` 在已命名的 profile 之間切換：

```bash
fox exec -llm-provider local "檢查這個專案有沒有潛在的 bug"
```

使用 `-protocol` 僅覆蓋本次執行使用的 OpenAI/Claude 相容協定：

```bash
fox exec -protocol claude -base-url https://api.anthropic.com -model claude-sonnet-4-20250514 -api-key-env ANTHROPIC_API_KEY "檢查這個專案有沒有潛在的 bug"
```

foxharness 沒有 `-provider` 參數。它出現在 flag 位置時會被視為未知 flag；
請使用 `-llm-provider` 切換 provider profile，或用 `-protocol` 指定
OpenAI/Claude 相容協定。

兩種協定模式下，內部的 Agent 訊息和工具呼叫完全相同，差異僅在於供應商介面卡將訊息轉換為目標協定的格式：

| 方面 | OpenAI 相容協定 | Claude 相容協定 |
| --- | --- | --- |
| 系統提示詞 | 以 `system` 角色訊息的形式傳送。 | 透過頂層的 `system` 欄位傳送。 |
| 工具呼叫 | 助手訊息中包含 `tool_calls` 欄位；工具結果以 `tool` 角色訊息回傳。 | 助手訊息內容中包含 `tool_use` 區塊；工具結果以使用者訊息中的 `tool_result` 區塊回傳。 |
| 工具 Schema | 函式參數巢狀於 `tools[].function.parameters` 中。 | 輸入 Schema 透過 `tools[].input_schema` 欄位傳送。 |
| 回應內容 | 文字和工具呼叫分屬助手訊息的不同欄位。 | 文字和工具呼叫混合在內容區塊中，再由框架統一轉換為 foxharness 訊息格式。 |

## 快速開始

進入任意專案目錄，啟動 TUI：

```bash
cd /path/to/your/project
fox
```

也可以直接指定專案路徑：

```bash
fox -C /path/to/your/project
```

執行單次任務並輸出結果：

```bash
fox exec "檢查目前專案是否有潛在問題"
```

也支援無互動的列印模式，方便在腳本中使用：

```bash
fox -p "總結一下這個專案的架構"
```

從標準輸入讀取任務：

```bash
echo "跑一下測試，說明哪些案例失敗了" | fox exec -
```

## TUI 操作指南

TUI 中的快捷鍵與操作：

- `Enter`：傳送目前訊息。
- `Shift+Tab`：為下一次提交在 Default 與正式規劃模式之間切換。
- `Up` / `Down`：在多行輸入中上下移動游標；游標位於首行或末行時，切換瀏覽歷史輸入。
- `PgUp` / `PgDown` 或滑鼠滾輪：上下捲動對話內容。
- 拖選對話文字：將選取內容複製到 macOS 剪貼簿。
- `Ctrl+F`：聚焦右側邊欄，之後可用 `Tab`、`Up` / `Down`、`PgUp` / `PgDown`、`Home`、`End` 瀏覽邊欄內容。
- `/`：彈出斜線命令列表。
- `!<命令>`：執行本地 shell 命令並顯示輸出，不傳送給模型。
- `Esc`：取消目前正在執行的任務。
- 兩秒內連按兩次 `Ctrl+C`：結束程式。

斜線命令一覽：

| 命令 | 說明 |
| --- | --- |
| `/session` | 顯示目前工作階段的檔案路徑。 |
| `/clear` | 清空目前顯示的對話內容。 |
| `/new` | 新建一個工作階段。 |
| `/plan` | 進入正式規劃模式；使用 `/plan off` 返回 Default。 |
| `/cancel` | 取消目前正在執行的任務。 |
| `/help` | 顯示可用命令列表。 |
| `/exit` | 結束 TUI。 |

## 命令列用法

```bash
fox [選項] [提示詞]       # 啟動互動式 TUI
fox exec [選項] [提示詞]  # 執行單次任務並輸出結果
fox -p [選項] [提示詞]    # 執行單次任務並輸出結果
```

常用選項：

| 選項 | 說明 |
| --- | --- |
| `-C`、`-workdir` | 工作目錄，預設為目前目錄（`.`）。 |
| `-llm-provider` | 從 `llm.providers` 中選擇命名 provider profile。 |
| `-protocol` | 協定覆蓋：`openai` 或 `claude`。 |
| `-base-url` | API base URL 覆蓋。 |
| `-model` | 模型 id 覆蓋。 |
| `-auth` | 認證模式覆蓋：`api-key` 或 `none`。 |
| `-api-key-env` | 保存 API key 的環境變數名稱。 |
| `-api-key` | 直接傳入 API key；日常使用優先選擇 `-api-key-env`。 |
| `-thinking` | 使用舊版的逐輪思考模式。 |
| `-max-turns` | Agent 最大執行輪次。預設不限制；設為正整數可限制輪次。 |
| `-c`、`-continue` | 繼續上一次的 CLI 工作階段。 |
| `-r`、`-session` | 恢復指定 ID 的工作階段。 |
| `-new` | 強制建立新的工作階段。 |
| `-p`、`-print` | 執行單次任務並輸出結果，不啟動 TUI。 |

範例：

```bash
fox exec "只檢查程式碼，不要修改檔案"
fox exec -continue "修復上一輪發現的問題"
fox exec -session 20260517-192517-a504c5 "繼續這個工作階段，總結一下目前進展"
fox exec -llm-provider local "為這個專案補充測試"
fox exec -llm-provider primary -model gpt-4.1 "總結一下這個專案的架構"
```

## 專案指令

foxharness 會從專案根目錄的以下檔案中載入專案層級的指令：

```text
AGENTS.md
```

你可以在其中撰寫編碼規範、測試命令、風格約束等專案相關的指引內容。

範例：

```markdown
# AGENTS.md

## Commands

- Run all tests with `go test ./...`.
- Format Go files with `gofmt -w`.

## Rules

- Do not edit files under `vendor/`.
- Prefer focused edits over whole-file rewrites.
```

## 斜線命令與技能

foxharness 會從 foxharness 原生目錄和 Claude Code 相容目錄中載入專案層級與使用者層級的斜線命令：

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

專案層級的命令優先於使用者層級的命令。在同一層級下，`.foxharness` 中的命令優先於同名的 `.claude` 命令。

在提示詞中透過 `$技能名稱` 來引用技能：

```bash
fox exec "Use $go-refactor to refactor internal/session"
```

`SKILL.md` 支援在開頭加入選擇性的 frontmatter 中繼資料：

```markdown
---
name: go-refactor
description: Go refactoring guidance for this project
---

Follow the existing package boundaries and preserve public APIs unless asked.
```

## 工作階段與資料

工作階段資料儲存在專案目錄之外：

```text
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/
```

每個工作階段可以包含多次執行記錄：

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

這表示你可以先啟動一項任務、檢視結果，然後在同一個工作階段中繼續對話。

正式規劃模式會將最近一次提交的方案儲存在目前工作階段的 `PLAN.md` 中。方案核准後，執行清單儲存在同一工作階段的 `TODO.md` 中；這兩個檔案都不會寫入專案根目錄。

## 開發

複製儲存庫：

```bash
git clone https://github.com/Zts0hg/foxharness.git
cd foxharness
```

執行測試：

```bash
go test ./...
```

格式化程式碼：

```bash
gofmt -w ./cmd ./internal
```

從原始碼直接執行：

```bash
go run ./cmd/fox
go run ./cmd/fox exec "檢查一下目前專案"
```

在本機編譯：

```bash
go build -trimpath -ldflags="-s -w" -o fox ./cmd/fox
```

安裝開發版（`foxdev`）：

把目前分支編譯成一個全域可用的 `foxdev` 二進位檔，與正式發佈的 `fox` 並存，這樣就能在任意專案目錄中執行開發中的 foxharness 進行測試：

```bash
./scripts/install-foxdev.sh                  # 安裝到 /usr/local/bin（可能需要 sudo）
sudo ./scripts/install-foxdev.sh             # 寫入 /usr/local/bin，避免權限錯誤
PREFIX=~/go/bin ./scripts/install-foxdev.sh  # 免 sudo 安裝（需要 ~/go/bin 在 PATH 中）
./scripts/install-foxdev.sh --check          # 編譯前先執行 `go test ./...`
```

`fox` 仍是正式發佈版；`foxdev` 跟隨你最近編譯的分支 —— `git switch feat/x && ./scripts/install-foxdev.sh` 即可刷新。`foxdev` 同樣需要設定 `ZHIPU_API_KEY`。

## 授權

foxharness 採用 GNU Affero General Public License v3.0 或更高版本（`AGPL-3.0-or-later`）授權。

允許商業使用，但如果以網路服務的形式散布或提供修改版本，必須在相同授權下開放原始碼。

## 發佈

GitHub Actions 發佈工作流程會為以下平台建置二進位檔案：

- macOS amd64
- macOS arm64
- Linux amd64
- Linux arm64
- Windows amd64
- Windows arm64

每次發佈會同時上傳帶版本號的壓縮檔（如 `fox_vX.Y.Z_linux_amd64.tar.gz`）和固定名稱的最新版壓縮檔（如 `fox_linux_amd64.tar.gz`）。

如果要基於最新的遠端 `main` 分支發佈下一個修補程式版本：

```bash
scripts/release-patch.sh --dry-run
scripts/release-patch.sh
```

此指令碼會找到最新的 `vMAJOR.MINOR.PATCH` 標籤，自動遞增修補程式版本號，為 `origin/main` 貼上新標籤並推送。推送標籤後會自動觸發發佈工作流程。

如果需要手動發佈指定版本，可以建立並推送版本標籤：

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```
