# 验证单个 Session 多次连续 Run

本文档用于验证当前项目的 Session 语义：一个 Session 是连续对话容器，用户可以在同一个 Session 中发起多次 Run；每次 Run 有独立的 trace/metrics/run.json，但共享同一份原始消息历史 `messages.jsonl`。

## 前置条件

从项目根目录执行：

```bash
cd /Users/xiaoming/code/foxharness-go
export ZHIPU_API_KEY="你的智谱 API Key"
```

建议先关闭 Plan Mode，让验证更聚焦在 engine 的会话历史拼装上：

```bash
go run ./cmd/fox exec -plan=false -new "ping"
```

如果这一步能正常返回，并在结尾打印 `Session:`、`Run:`、`Metrics:`、`Trace:`，说明 CLI 基本链路可用。

## 示例 1：创建一个新 Session，并执行第 1 个 Run

这个 Run 写入一个很容易识别的验证短语。注意这里不依赖 Working Memory，总结文件或外部文件，只依赖 Session 的原始消息历史。

```bash
OUT1=$(go run ./cmd/fox exec \
  -plan=false \
  -new \
  "这是单个 Session 连续 Run 验证的第 1 轮。请不要调用工具，只回复一句：我已记住验证短语 SESSION_CHAIN_042，下一轮可以直接问我。")

echo "$OUT1"
SESSION_ID=$(printf '%s\n' "$OUT1" | awk '/^Session:/ {print $2}' | tail -1)
echo "SESSION_ID=$SESSION_ID"
```

期望结果：

- 输出末尾有 `Session: <session-id>`
- 输出末尾有 `Run: <run-id>`
- `SESSION_ID` 不为空
- 模型回复中包含 `SESSION_CHAIN_042`

## 示例 2：在同一个 Session 中执行第 2 个 Run

这一步显式使用 `-session "$SESSION_ID"` 继续同一个 Session。验证点是：第 2 个 Run 应该能从当前 Session 的原始消息历史中看到第 1 个 Run 的内容。

```bash
OUT2=$(go run ./cmd/fox exec \
  -plan=false \
  -session "$SESSION_ID" \
  "这是同一个 Session 的第 2 轮。不要调用工具，不要读取文件，不要猜测。请根据当前会话历史回答：上一轮我让你记住的验证短语是什么？")

echo "$OUT2"
```

期望结果：

- 输出末尾的 `Session:` 仍然等于 `$SESSION_ID`
- 输出末尾的 `Run:` 是一个新的 run id，不等于第 1 轮的 run id
- 模型回答中包含 `SESSION_CHAIN_042`

如果第 2 轮能回答出 `SESSION_CHAIN_042`，说明新的 Run 不是孤立执行，而是收到了同一个 Session 中上一轮的原始上下文。

## 示例 3：继续同一个 Session 执行第 3 个 Run

这一步继续使用同一个 Session，让模型总结前两轮任务。

```bash
OUT3=$(go run ./cmd/fox exec \
  -plan=false \
  -session "$SESSION_ID" \
  "这是同一个 Session 的第 3 轮。请根据当前会话历史，用两句话总结第 1 轮和第 2 轮分别做了什么。")

echo "$OUT3"
```

期望结果：

- 输出末尾的 `Session:` 仍然等于 `$SESSION_ID`
- 输出末尾的 `Run:` 又是一个新的 run id
- 回答能提到第 1 轮保存验证短语，第 2 轮询问验证短语

## 示例 4：使用 `-continue` 继续最近的 CLI Session

如果刚刚创建的 `$SESSION_ID` 是最新的 CLI Session，也可以不用显式传 session id：

```bash
go run ./cmd/fox exec \
  -plan=false \
  -continue \
  "继续最近的 CLI Session。请只回答：当前验证短语是什么？"
```

期望结果：

- 输出中的 `Session:` 等于刚才的 `$SESSION_ID`
- 模型回答包含 `SESSION_CHAIN_042`

如果你的本地已有多个 Session，更推荐使用 `-session "$SESSION_ID"`，这样验证结果不会受“最近 Session”选择影响。

## 示例 5：验证新 Session 与旧 Session 隔离

这一步强制创建一个新 Session。新 Session 不应该天然拥有旧 Session 的上下文。

```bash
go run ./cmd/fox exec \
  -plan=false \
  -new \
  "这是一个全新的 Session。不要调用工具。请回答：当前会话历史里是否出现过验证短语 SESSION_CHAIN_042？如果没有，请只回答：没有。"
```

期望结果：

- 输出中的 `Session:` 是一个新的 session id，不等于 `$SESSION_ID`
- 模型应该回答没有，或表达当前会话历史中没有该短语

这一步验证的是物理隔离：不同 Session 的 `messages.jsonl` 不会互相混入。

## 检查磁盘结构

回到第一个 Session，检查它的物理目录：

```bash
PROJECT_KEY="$(pwd | sed 's#/#-#g' | tr -d ':')"
SESSION_DIR="$HOME/.foxharness/projects/$PROJECT_KEY/sessions/$SESSION_ID"
find "$SESSION_DIR" -maxdepth 3 -type f | sort
```

期望至少能看到类似结构：

```text
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/messages.jsonl
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/session.json
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/transcript.jsonl
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/working_memory.md
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/runs/<run-id-1>/run.json
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/runs/<run-id-1>/metrics.jsonl
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/runs/<run-id-1>/trace.jsonl
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/runs/<run-id-2>/run.json
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/runs/<run-id-2>/metrics.jsonl
~/.foxharness/projects/<encoded-workdir>/sessions/<session-id>/runs/<run-id-2>/trace.jsonl
```

检查这个 Session 下有多少个 Run：

```bash
ls -1 "$SESSION_DIR/runs"
```

如果执行了示例 1、2、3，期望至少有 3 个 run 目录。

## 检查原始消息历史是否跨 Run 连续追加

`messages.jsonl` 是单个 Session 的原始模型可见消息历史。多个 Run 会追加到同一个文件里，并通过 `run_id` 区分来源。

```bash
wc -l "$SESSION_DIR/messages.jsonl"
sed -n '1,12p' "$SESSION_DIR/messages.jsonl"
```

如果安装了 `jq`，可以用更清晰的方式查看：

```bash
jq -r '[.seq, .run_id, .message.role, (.message.content | gsub("\n"; " ") | .[0:100])] | @tsv' \
  "$SESSION_DIR/messages.jsonl"
```

期望结果：

- `seq` 从 0 开始递增
- 多个不同的 `run_id` 出现在同一个 `messages.jsonl` 中
- 第 1 个 Run 的 user/assistant 消息在前
- 第 2 个 Run 的 user/assistant 消息追加在后
- 第 3 个 Run 的 user/assistant 消息继续追加

这说明上下文连续性来自原始聊天记录，而不是依赖 LLM 总结写入的非原始内容。

## 检查 transcript 是否记录每个 Run

`transcript.jsonl` 记录事件流，并且事件带有 `run_id`。

```bash
sed -n '1,20p' "$SESSION_DIR/transcript.jsonl"
```

如果安装了 `jq`：

```bash
jq -r '[.time, .run_id, .type] | @tsv' "$SESSION_DIR/transcript.jsonl"
```

期望结果：

- 同一个 `transcript.jsonl` 中出现多个不同 `run_id`
- 每个 Run 至少有 `user_prompt` 事件
- 如果触发了上下文压缩，会看到 `context_compacted` 事件

## 检查每个 Run 的独立产物

每个 Run 都有独立的运行记录：

```bash
for d in "$SESSION_DIR"/runs/*; do
  echo "== $d =="
  sed -n '1,120p' "$d/run.json"
  test -f "$d/metrics.jsonl" && echo "metrics: yes" || echo "metrics: no"
  test -f "$d/trace.jsonl" && echo "trace: yes" || echo "trace: no"
done
```

期望结果：

- 每个 `run.json` 的 `session_id` 都等于 `$SESSION_ID`
- 每个 `run.json` 的 `id` 都不同
- 每个 `run.json` 都记录了自己的 `prompt`
- 每个 Run 的 `metrics.jsonl` 和 `trace.jsonl` 独立存在

## 代码级回归验证

可以运行现有测试，确认 Session、Engine、Reporter 的基础行为没有破坏：

```bash
go test ./internal/session ./internal/engine
go test ./...
```

期望全部通过。

## 判断标准

这次能力验证通过需要同时满足：

- 同一个 `$SESSION_ID` 下能产生多个不同 Run
- 第 2 个或第 3 个 Run 能回答第 1 个 Run 中出现过的信息
- `messages.jsonl` 是 Session 级文件，并连续追加多个 Run 的原始消息
- `runs/<run-id>/` 是 Run 级目录，并分别保存 run metadata、metrics、trace
- 新建 Session 后无法自然看到旧 Session 的消息历史
