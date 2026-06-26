<p align="center">
  <img src="assets/banner.png" alt="foxharness" width="100%">
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.zh-TW.md">繁體中文</a> | <b>日本語</b>
</p>

<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go" alt="Go"></a>
  <a href="https://opensource.org/licenses/AGPL-3.0"><img src="https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge" alt="License"></a>
</p>

foxharness は Go 言語で作られた AI コーディングアシスタントです。ターミナル上で動作し、現在のプロジェクトを読み込んでローカルツールを呼び出すことができます。複数回の実行にわたってセッション履歴を保持し、リッチなインタラクティブターミナル UI（TUI）を提供します。

デフォルトのコマンド名は `fox` です。

## 機能

- インタラクティブ TUI：プロジェクトディレクトリで `fox` を実行するだけで、継続的な対話が可能です。
- ワンショット CLI モード：`fox exec` または `fox -p` で単一タスクを素早く実行できます。
- セッションの継続性：複数回の実行で同じセッションとメッセージ履歴を共有します。
- プロジェクト指示：ワークスペース内の `AGENTS.md` を自動的に読み込み、プロジェクトのガイドラインとして反映します。
- スキルとスラッシュコマンド：`.foxharness/` ディレクトリ配下のネイティブ形式に加え、`.claude/` ディレクトリ配下の Claude Code 互換形式にも対応します。
- プランモード：`PLAN.md`、`TODO.md`、`MEMORY.md` を自動的に生成・利用できます。
- ツール実行：ファイルの読み書き、ファジー編集、Bash コマンドの実行、サブエージェントへのタスク委譲をサポートします。
- ローカルトレースデータ：`~/.foxharness` 配下に会話記録、パフォーマンスメトリクス、トレース、実行メタデータを保存します。

## インストール

### 方法 1：リリースバイナリをダウンロード

以下のページから、お使いのプラットフォームに合ったバイナリをダウンロードしてください：

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

Windows ユーザーはリリースページから以下のいずれかのアーカイブをダウンロードできます：

- `fox_windows_amd64.zip`
- `fox_windows_arm64.zip`

展開後、`fox.exe` があるディレクトリを `PATH` 環境変数に追加してください。

macOS で Gatekeeper がダウンロードしたバイナリの実行をブロックする場合は、検疫フラグを削除してください：

```bash
xattr -d com.apple.quarantine /usr/local/bin/fox
```

### 方法 2：ソースからビルドしてインストール

Go 1.25 以降が必要です。

```bash
go install github.com/Zts0hg/foxharness/cmd/fox@latest
```

`$GOPATH/bin` が `PATH` に含まれていることを確認してください。

## 設定

foxharness はデフォルトで智譜（Zhipu）BigModel のコーディングエンドポイントを使用します。デフォルトの通信プロトコルは OpenAI 互換形式ですが、Claude 互換の Anthropic Messages プロトコルアダプターも利用可能です。`fox` を実行する前に、API キーを設定してください：

```bash
export ZHIPU_API_KEY="your-api-key"
```

リトライとタイムアウトのオプション設定：

```bash
export FOXHARNESS_LLM_MAX_ATTEMPTS=4
export FOXHARNESS_LLM_RETRY_INITIAL_DELAY=750ms
export FOXHARNESS_LLM_RETRY_MAX_DELAY=8s
export FOXHARNESS_LLM_REQUEST_TIMEOUT=60s
```

### プロトコルの種類

デフォルトの OpenAI 互換 Chat Completions プロトコルを使う場合：

```bash
fox exec -provider openai "このプロジェクトに潜在的なバグがないか調べて"
```

Claude 互換の Anthropic Messages プロトコルを使う場合：

```bash
fox exec -provider claude "このプロジェクトに潜在的なバグがないか調べて"
```

どちらのプロトコルでも、内部の Agent メッセージとツール呼び出しは共通です。プロバイダーアダプターが対象プロトコルの形式に変換します：

| 項目 | OpenAI 互換プロトコル | Claude 互換プロトコル |
| --- | --- | --- |
| システムプロンプト | `system` ロールのメッセージとして送信。 | トップレベルの `system` フィールドで送信。 |
| ツール呼び出し | アシスタントメッセージに `tool_calls` を含む。ツールの結果は `tool` ロールのメッセージとして返す。 | アシスタントのメッセージ内容に `tool_use` ブロックを含む。ツールの結果はユーザーメッセージ内の `tool_result` ブロックとして返す。 |
| ツール Schema | 関数のパラメータを `tools[].function.parameters` の中にネストして指定。 | 入力 Schema を `tools[].input_schema` フィールドで送信。 |
| レスポンス内容 | テキストとツール呼び出しがアシスタントメッセージの別々のフィールドに分かれている。 | テキストとツール呼び出しが混在するコンテンツブロックとして返り、フレームワークが foxharness のメッセージ形式に統一して変換する。 |

## クイックスタート

プロジェクトディレクトリに移動して TUI を起動：

```bash
cd /path/to/your/project
fox
```

プロジェクトパスを直接指定することもできます：

```bash
fox -C /path/to/your/project
```

単発タスクを実行して結果を出力：

```bash
fox exec "このプロジェクトに潜在する問題がないか調べて"
```

スクリプトでの利用に便利な、非対話型のプリントモードにも対応しています：

```bash
fox -p "このプロジェクトのアーキテクチャをまとめて"
```

標準入力からタスクを読み込むことも可能です：

```bash
echo "テストを実行して、失敗したケースを説明して" | fox exec -
```

## TUI の操作方法

TUI でのキーボードショートカット：

- `Enter`：メッセージを送信。
- `Shift+Tab`：プランモードのオン/オフを切り替え。
- `Up` / `Down`：複数行入力中にカーソルを上下に移動。先頭行または最終行で押すと入力履歴を切り替え。
- `PgUp` / `PgDown` またはマウスホイール：会話をスクロール。
- 会話テキストをドラッグ選択：選択範囲を macOS のクリップボードにコピー。
- `Ctrl+F`：右サイドバーにフォーカス。その後 `Tab`、`Up` / `Down`、`PgUp` / `PgDown`、`Home`、`End` で内容を閲覧。
- `/`：スラッシュコマンドの一覧を表示。
- `!<command>`：ローカル shell コマンドを実行し、モデルへ送信せずに出力を表示。
- `Esc`：現在実行中のタスクをキャンセル。
- 2 秒以内に `Ctrl+C` を 2 回連続で押す：終了。

スラッシュコマンド一覧：

| コマンド | 説明 |
| --- | --- |
| `/session` | 現在のセッションのファイルパスを表示。 |
| `/clear` | 表示中の会話をクリア。 |
| `/new` | 新しいセッションを開始。 |
| `/cancel` | 現在実行中のタスクをキャンセル。 |
| `/help` | 使用可能なコマンド一覧を表示。 |
| `/exit` | TUI を終了。 |

## コマンドラインでの使い方

```bash
fox [オプション] [プロンプト]       # インタラクティブ TUI を起動
fox exec [オプション] [プロンプト]  # 単発タスクを実行して結果を出力
fox -p [オプション] [プロンプト]    # 単発タスクを実行して結果を出力
```

主なオプション：

| オプション | 説明 |
| --- | --- |
| `-C`、`-workdir` | 作業ディレクトリ。デフォルトはカレントディレクトリ（`.`）。 |
| `-model` | モデル名。デフォルトは `glm-4.5-air`。 |
| `-provider` | プロトコルの種類：`openai` または `claude`。デフォルトは `openai`。 |
| `-plan` | プランモードを有効にするかどうか。デフォルトは `true`。 |
| `-thinking` | プランモードが無効の場合、従来のターンごとの思考モードを使用。 |
| `-max-turns` | Agent の最大実行ターン数。デフォルトは無制限。正の整数で上限を設定。 |
| `-c`、`-continue` | 前回の CLI セッションを再開。 |
| `-r`、`-session` | 指定した ID のセッションを再開。 |
| `-new` | 強制的に新しいセッションを作成。 |
| `-p`、`-print` | TUI を起動せずに単発タスクを実行し、結果を出力。 |

使用例：

```bash
fox exec -plan=false "コードの確認だけして、ファイルは変更しないで"
fox exec -continue "前回見つかったバグを修正して"
fox exec -session 20260517-192517-a504c5 "このセッションを続けて、今の進捗をまとめて"
fox exec -model glm-4.5-air "このプロジェクトにテストを追加して"
fox exec -provider claude "このプロジェクトのアーキテクチャをまとめて"
```

## プロジェクト指示

foxharness はプロジェクトルートの以下のファイルから、プロジェクト固有の指示を読み込みます：

```text
AGENTS.md
```

コーディング規約、テストコマンド、スタイルの制約など、プロジェクトに関するガイドラインを自由に記述できます。

記述例：

```markdown
# AGENTS.md

## Commands

- Run all tests with `go test ./...`.
- Format Go files with `gofmt -w`.

## Rules

- Do not edit files under `vendor/`.
- Prefer focused edits over whole-file rewrites.
```

## スラッシュコマンドとスキル

foxharness はネイティブの foxharness ディレクトリと Claude Code 互換ディレクトリの両方から、プロジェクトレベルおよびユーザーレベルのスラッシュコマンドを読み込みます：

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

プロジェクトレベルのコマンドがユーザーレベルのコマンドより優先されます。同じレベルでは、`.foxharness` のコマンドが同名の `.claude` コマンドより優先されます。

プロンプト内で `$skill-name` を使ってスキルを呼び出せます：

```bash
fox exec "Use $go-refactor to refactor internal/session"
```

`SKILL.md` の先頭には、オプションで frontmatter メタデータを記述できます：

```markdown
---
name: go-refactor
description: Go refactoring guidance for this project
---

Follow the existing package boundaries and preserve public APIs unless asked.
```

## セッションとデータ

セッションデータはプロジェクトディレクトリの外に保存されます：

```text
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/
```

各セッションには複数回の実行記録を含めることができます：

```text
messages.jsonl
session.json
transcript.jsonl
working_memory.md
runs/<run-id>/run.json
runs/<run-id>/metrics.jsonl
runs/<run-id>/trace.jsonl
```

つまり、タスクを開始して結果を確認した後、同じセッションでそのまま対話を続けることができます。

プランモードでは、プロジェクトルートに以下のファイルが作成または更新される場合があります：

```text
PLAN.md
TODO.md
MEMORY.md
```

これらのファイルを Git 管理対象外にしたい場合は、`.gitignore` に追加してください。

## 開発

リポジトリをクローン：

```bash
git clone https://github.com/Zts0hg/foxharness.git
cd foxharness
```

テストを実行：

```bash
go test ./...
```

コードをフォーマット：

```bash
gofmt -w ./cmd ./internal
```

ソースから直接実行：

```bash
go run ./cmd/fox
go run ./cmd/fox exec "現在のプロジェクトをチェックして"
```

ローカルでビルド：

```bash
go build -trimpath -ldflags="-s -w" -o fox ./cmd/fox
```

## ライセンス

foxharness は GNU Affero General Public License v3.0 以降（`AGPL-3.0-or-later`）でライセンスされています。

商用利用は可能ですが、変更版をネットワークサービスとして配布・提供する場合は、同じライセンスの下で公開する必要があります。

## リリース

GitHub Actions のリリースワークフローは、以下のプラットフォーム向けにバイナリをビルドします：

- macOS amd64
- macOS arm64
- Linux amd64
- Linux arm64
- Windows amd64
- Windows arm64

各リリースでは、バージョン付きアーカイブ（例：`fox_vX.Y.Z_linux_amd64.tar.gz`）と、固定名の最新版アーカイブ（例：`fox_linux_amd64.tar.gz`）の両方がアップロードされます。

最新のリモート `main` ブランチから次のパッチバージョンをリリースする場合：

```bash
scripts/release-patch.sh --dry-run
scripts/release-patch.sh
```

このスクリプトは最新の `vMAJOR.MINOR.PATCH` タグを見つけ、パッチ番号を自動的にインクリメントし、`origin/main` に新しいタグを付けてプッシュします。タグのプッシュにより、リリースワークフローが自動的にトリガーされます。

特定のバージョンを手動でリリースする場合は、バージョンタグを作成してプッシュします：

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```
