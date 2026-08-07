# FoxHarness Agent 架构与可维护性审查

## 1. 文档目的

本文档记录对 FoxHarness 当前代码的系统性审查结果，重点回答以下问题：

1. 当前实现是否符合典型 Agent 的执行脉络。
2. 项目中的职责是否清晰，包和模块是否高内聚、低耦合。
3. 当前代码是否易读、易懂、易测试、易扩展和易维护。
4. 哪些问题会影响正确性、安全性、可靠性或后续演进。
5. 应当按照什么顺序改进，才能在控制改动风险的同时逐步收敛架构。

本文档有意区分三类内容：

- **事实**：可以通过代码、测试或命令直接复现。
- **分析**：根据事实得到的架构和维护性判断。
- **建议**：面向后续演进的方案，不代表当前代码已经采用。

审查基线：

- Git revision：`5203770`
- Git tag：`v0.1.33`
- 审查日期：2026-07-29
- Go package 数量：39
- 非测试 Go 文件：184
- Go 测试文件：132
- 非测试 Go 代码：32,915 行
- Go 测试代码：31,110 行

## 2. 审查范围与方法

### 2.1 审查范围

本次审查覆盖：

- `cmd/fox`、`cmd/feishu`、`cmd/agentops`、`cmd/bench` 等入口。
- Agent 主循环及其上下文、模型、工具和状态更新路径。
- CLI、TUI、Feishu、AgentOps、benchmark、subagent 的运行时组装。
- 权限、审批、中间件、工具执行和子 Agent 委派边界。
- session、memory、compaction、recovery、reminder、metrics、tracing。
- autodev 和 benchmark 所代表的评估与反馈能力。
- 测试覆盖、race、vet、格式化和安装脚本验证。
- 项目宪法中有关代码质量、测试和架构的强制约束。

未覆盖：

- 对外部模型服务进行真实端到端调用。
- 生产环境容量压测和故障注入。
- ShellCheck 静态分析，因为当前环境未安装 `shellcheck`。
- 对 `vendor/` 的审查和修改。

### 2.2 执行的验证

以下命令均已实际执行：

```text
go test ./...
go test -race ./...
go vet ./...
gofmt -l cmd internal benchmarks
go test -coverprofile=/tmp/foxharness-coverage.out ./...
go tool cover -func=/tmp/foxharness-coverage.out
sh scripts/install_test.sh
git status --short
```

验证结果：

- `go test ./...`：通过。
- `go test -race ./...`：通过。
- `go vet ./...`：通过。
- `gofmt -l`：无输出，检查通过。
- 总语句覆盖率：73.0%。
- `scripts/install_test.sh`：51/51 通过。
- 创建本文档前工作区干净；当前除新增本文档外没有代码改动。

这些结果说明项目具有较好的基础工程质量，但不能排除未被测试覆盖的逻辑错误。本文后续发现的 `edit_file` 文件破坏问题正是一个例子。

### 2.3 独立核实

文档初稿完成后，使用两个 subagent 进行独立复核：

- **事实核查 subagent**：重新计算 Git、文件、代码行、包依赖和覆盖率数据，重新运行测试、race、vet、gofmt、coverage 和安装脚本，并逐条检查代码链接及问题链路。
- **架构核查 subagent**：独立检查执行链映射、内聚/耦合判断、严重性、评分、目标架构和迁移建议是否由代码支持，重点寻找夸大结论和替代解释。

核实结果：

- 两个 subagent 均确认 `edit_file`、远程权限、Bash 失败语义、重复运行时组装、远程资源、可观测性、大文件读取、工具顺序和 compaction 判断等核心事实成立。
- 事实核查重新执行的测试和静态检查结果与初稿一致。
- 核实发现 `internal/app` 的直接内部依赖数应为 20，而非初稿中的 19，本文已修正。
- 核实指出远程权限问题不限于 writable subagent，父 Agent 本身也未接入新权限协调器，本文已扩大问题范围。
- 核实指出“宪法应视为发布阻断”和“当前 CI 实际阻断”必须区分，本文已补充当前 release workflow 的门禁缺口。
- 架构核查指出单一 RuntimeFactory 只是候选设计，真正必须统一的是安全和能力不变量，本文已据此收紧建议和验收标准。

除明确标注为推断、评分或建议的内容外，本文事实均已完成主审查者与至少一个 subagent 的交叉核实。

## 3. 当前 Agent 执行链

用户提出的典型执行脉络是：

```text
用户消息
 -> 构造上下文
 -> 调用模型
 -> 解析模型返回
 -> 分发工具 / 委派其他 Agent / 询问用户
 -> 收集执行结果
 -> 更新状态
 -> 进行下一轮，或者结束
```

FoxHarness 当前实现与这一脉络在概念上基本一致。

### 3.1 用户输入与入口适配

当前存在多种入口：

- `cmd/fox`：命令行、一次性执行和 TUI。
- `internal/tui`：交互式终端输入、快捷键、slash command、表单和选择器。
- `cmd/feishu` 与 `internal/feishu`：飞书事件转换为任务。
- `cmd/agentops` 与 `internal/agentops`：AgentOps 任务入口。
- `cmd/bench` 与 `internal/benchmark`：benchmark 入口。
- `internal/subagent`：父 Agent 发起的委派任务。

**事实**：这些入口没有共同的、显式的规范化任务类型，也没有全部进入同一个运行时工厂。Feishu、AgentOps、benchmark 和 subagent 会分别组装 `AgentEngine`。

**分析**：输入规范化能力存在，但它是按入口分散实现的。项目的架构图表达了统一编排方向，实际代码还没有完全达到该目标。

### 3.2 上下文构造

上下文构造主要由以下模块协作：

- `internal/context`：系统提示、项目指令、skills、session memory 和 auto memory。
- `internal/session`：消息日志、运行记录和持久化路径。
- `internal/automemory`：跨 session 的持久记忆索引和运行后提取。
- `internal/compaction`：上下文接近限制时的压缩。
- `internal/engine`：把历史消息、当前输入、提醒和压缩结果组装为每轮请求。

**分析**：这一部分的包边界总体清晰。静态提示构造、会话持久化、自动记忆和上下文压缩分别位于独立包中，属于项目中内聚度较好的区域。

### 3.3 模型调用与返回解析

`internal/provider` 提供模型调用抽象，不同 provider 将供应商返回统一转换为 `schema.Message`。`internal/engine` 只消费统一消息和工具调用结构。

**分析**：模型适配和 Agent 编排之间的边界清晰。供应商差异被限制在 provider 层，是当前项目较好的扩展点之一。

### 3.4 工具、委派与用户询问

相关模块包括：

- `internal/tools`：工具定义、注册、查找、并行安全标记和执行。
- `internal/middleware`：旧的工具中间件和危险命令审批。
- `internal/permission`：新的权限协调器、策略和 registry decorator。
- `internal/toolpolicy`：工具风险和策略判断。
- `internal/subagent`：委派工具及子 Agent manager。
- TUI ask form 和远程 approver：向用户请求补充信息或审批。

**分析**：功能覆盖完整，但安全策略存在新旧两套路径。工具执行、审批和子 Agent 权限并未在所有入口形成一致的端到端边界。

### 3.5 结果收集和状态更新

结果收集和更新涉及：

- `internal/toolresult`：大型工具结果的截断和持久化。
- `internal/session`：消息、run 和 transcript。
- `internal/recovery`：连续工具失败检测。
- `internal/reminder`：根据调用模式注入提醒。
- `internal/metrics` 与 `internal/tracing`：运行指标和轨迹。
- `internal/checkpoint`：工作区 checkpoint。
- `internal/automemory`：运行结束后的记忆提取。

这些能力最终由 `AgentEngine.RunWithReporter` 在主循环中统一协调。

**分析**：状态更新需要的能力齐全，但大量能力集中在一个主循环函数内，造成较高的编排耦合。

### 3.6 下一轮与结束

`AgentEngine` 根据以下条件继续或结束：

- 模型返回工具调用时执行工具并进入下一轮。
- 模型返回无工具调用的最终消息时尝试结束。
- completion gate 拒绝结束时继续。
- 达到最大轮次时结束。
- context 取消或发生致命错误时终止。

因此，当前实现完整覆盖了用户描述的 Agent 任务循环。

## 4. Agent 项目组成映射

| 典型组成 | 当前实现 | 判断 |
|---|---|---|
| 用户输入规范化 | CLI/TUI/Feishu/AgentOps 各自转换 | 存在，但分散 |
| 意图识别 | 模型工具选择、slash command 和少量显式路由 | 隐式存在 |
| Agent 运行循环 | `internal/engine` | 功能完整，职责过重 |
| 上下文管理 | `context`、`session`、`compaction` | 分层较好 |
| 检索/RAG | `automemory` 的索引和按需读取 | 是持久记忆，不是通用向量 RAG |
| 评估与反馈 | `benchmark`、autodev gate/reviewer | 存在，但不是统一反馈平面 |
| 工具管理 | `tools`、`middleware`、`permission`、`toolpolicy` | 完整，但策略分裂 |
| 可观测性 | metrics、tracing、transcript、reporter | 能力存在，错误处理不足 |

当前没有独立的“意图识别服务”。这本身不构成架构缺陷：在通用 Agent 中，模型的工具选择通常就是意图路由。只有在需要确定性安全预分类、成本路由、模型分流或业务 SLA 时，才值得引入显式 intent router。

## 5. 依赖、规模与复杂度事实

### 5.1 依赖方向

Go 编译和测试均通过，说明包之间不存在 import cycle。较稳定的底层边界包括：

- `schema`：跨层消息和工具结果结构。
- `provider`：模型端口及供应商适配。
- `tools`：工具端口和 registry。
- `session`：持久化模型。
- `context`：提示构造。
- `compaction`：上下文压缩。
- `permission`：权限决策和装饰。

直接依赖内部包数量较多的包：

| 包 | 直接引用的内部包数量 |
|---|---:|
| `internal/app` | 20 |
| `internal/tui` | 14 |
| `internal/agentops` | 12 |
| `internal/engine` | 11 |
| `internal/feishu` | 10 |
| `internal/subagent` | 9 |

依赖数量本身不是缺陷。`app` 作为 composition root，本来就需要知道多个实现。问题在于它同时承担运行用例、可变状态管理、会话操作和 UI facade，导致依赖变更容易穿透到同一个对象。

### 5.2 大文件和大函数

最大的非测试 Go 文件：

| 文件 | 行数 |
|---|---:|
| `internal/tui/model.go` | 3,974 |
| `internal/tui/view.go` | 1,777 |
| `internal/app/runner.go` | 1,233 |
| `internal/engine/loop.go` | 1,006 |
| `internal/tui/markdown.go` | 765 |
| `internal/context/prompt.go` | 528 |
| `internal/compaction/compactor.go` | 521 |

其中：

- `internal/tui/model.go` 包含 181 个函数或方法。
- `AgentEngine.RunWithReporter` 约 500 行。
- `AgentRunner.runInternal` 约 180 行。
- TUI `Update` 和主键盘处理函数都超过 200 行。

文件长度不是单独的质量判据。但这些文件同时表现出状态字段多、分支多、跨领域依赖多，因此长度是职责聚集的外部信号，而不仅是代码风格问题。

## 6. 做得较好的部分

### 6.1 核心端口清晰

provider 和 tool registry 为上层提供稳定接口，供应商协议、具体工具和 Agent 主循环没有完全粘连。新增 provider 或普通工具通常不需要修改主循环。

### 6.2 上下文相关能力已经模块化

prompt composer、session、auto memory、compaction、tool result persistence 均有相对独立的包和测试。这使上下文能力可以单独验证和演进。

### 6.3 测试投入较高

测试代码 31,110 行，与生产 Go 代码 32,915 行接近 1:1。race、vet 和全部测试均通过，`autodev`、`automemory`、`compaction`、`context` 等包覆盖率较高。

### 6.4 并发取消基础较好

模型和工具调用普遍传播 `context.Context`。Bash 工具对进程组取消和超时有专门处理及 race 测试。权限 decorator 也会避免需要交互审批的工具被不安全地并行执行。

### 6.5 autodev 的控制面和执行面相对清晰

`docs/autodev.md` 描述并实现了状态机、确定性 gate 和 reviewer 的分离。相较于主 Agent 编排，这一子系统的职责边界更明确。

## 7. 主要问题与证据

### 7.1 CRITICAL：`edit_file` 模糊替换可能破坏文件

位置：

- [`internal/tools/edit_file.go:296`](../internal/tools/edit_file.go#L296)
- [`internal/tools/edit_file.go:367`](../internal/tools/edit_file.go#L367)

`replaceLineRange` 先通过 `lineByteRange` 得到字节区间：

```go
start, end := lineByteRange(content, startLine, endLine)
```

但返回结果时使用的是 `endLine`：

```go
return content[:start] + replacement + content[endLine:]
```

`endLine` 是逻辑行号，`end` 才是字符串字节偏移。对于第二行、第三行等常见替换，二者通常完全不同。该错误位于 trimmed/fuzzy fallback 共用路径，可能把原文件从错误的字节位置拼回，造成内容重复或损坏。

同一文件中的模糊窗口循环使用：

```go
for i := 0; i+window < len(lines); i++ {
```

正确边界应允许 `i+window == len(lines)`。当前实现遗漏最后一个窗口，并且当窗口覆盖整个文件时一次也不会执行。

现有工具测试没有针对 trimmed/fuzzy fallback 的文件完整性断言，因此 `go test ./...` 无法发现该问题。

结论：这是直接影响用户文件完整性的正确性缺陷，应当在其他重构之前处理。

### 7.2 CRITICAL：Feishu/AgentOps 未接入统一权限策略

这不是仅发生在子 Agent 中的单点问题，而是远程入口仍使用旧审批中间件，父 Agent 和嵌套 Agent 都没有获得新权限框架提供的完整保护。

父 Agent 路径：

1. Feishu 在 [`internal/feishu/runner.go:176`](../internal/feishu/runner.go#L176)、AgentOps 在 [`internal/agentops/runner.go:186`](../internal/agentops/runner.go#L186) 直接注册 read/write/edit/bash/delegate 等工具。
2. 两者只接入旧的 `DangerMiddleware`，没有用 `permission.Coordinator` 装饰 registry。
3. [`internal/middleware/danger.go:69`](../internal/middleware/danger.go#L69) 只分析 `bash`，并且只通过若干危险字符串识别风险；非 bash 工具直接放行。
4. read/write/edit 执行层只使用 `filepath.Join` 解析路径，例如 [`internal/tools/write_file.go:76`](../internal/tools/write_file.go#L76)，自身不会拒绝包含 `..` 的 workspace 外路径。
5. 新策略本来会把 workspace 内普通文件修改标记为 fast-allow，把 workspace 外路径升级为 reviewable，见 [`internal/tools/permission.go:23`](../internal/tools/permission.go#L23)。远程入口没有执行这一评估。
6. 新策略也会对可变 shell、无法解析的 shell 和其他高风险调用执行上下文判断；旧中间件不能提供等价覆盖。

嵌套委派路径：

1. Feishu 和 AgentOps 创建 `subagent.NewManager` 时没有调用 `WithPermission`。
2. `delegate_task` 允许模型传入 `read_only=false`，见 [`internal/subagent/tool.go:119`](../internal/subagent/tool.go#L119)。
3. 在新策略中，缺少 nested permission enforcement 的委派应当是 HumanOnly，见 [`internal/subagent/tool.go:81`](../internal/subagent/tool.go#L81)；旧中间件不会调用这个 assessment。
4. 子 Agent manager 在非只读模式注册 write/edit，见 [`internal/subagent/manager.go:113`](../internal/subagent/manager.go#L113)。
5. 当 coordinator 为 nil 时，`DecorateRegistry` 原样返回基础 registry，见 [`internal/permission/coordinator.go:203`](../internal/permission/coordinator.go#L203)。

因此，当前风险包括：

- 父 Agent 的 workspace 外文件访问不会按新策略升级审批。
- 大量可变 shell 仅在命中特定字符串时才审批。
- `delegate_task(read_only=false)` 不会触发缺少嵌套权限执行时应有的 HumanOnly 判定。
- writable 子 Agent 的 bash/write/edit 不执行新权限策略。

结论：这是远程信任边界上的策略覆盖缺口。修复范围必须是 Feishu/AgentOps 的完整 registry 及其嵌套委派链，而不是只给 `delegate_task` 增加名称黑名单。workspace 内 write/edit 是否自动允许，应继续由统一策略根据显式 profile 决定。

### 7.3 CRITICAL/Constitution：七个包没有测试文件

项目宪法在 [`.codexspec/memory/constitution.md:121`](../.codexspec/memory/constitution.md#L121) 明确规定：

```text
Each package MUST have at least one test file
Critical paths MUST have unit tests
Error paths MUST be tested explicitly
```

以下包没有任何 `*_test.go`：

- `internal/approval`
- `internal/benchmark`
- `internal/llmresolve`
- `internal/metrics`
- `internal/recovery`
- `internal/reminder`
- `internal/tracing`

这不仅是形式问题。`approval`、`recovery`、`metrics` 和 `tracing` 都位于关键运行路径；审批并发、恢复阈值、写入失败等行为目前缺少包级回归保护。

结论：按照项目自己的治理原则，这是宪法不合规项，并且应被视为发布阻断项。其运行时严重性不等同于前两个缺陷。当前 [release workflow](../.github/workflows/release.yml#L44) 执行构建、打包和 installer test，但没有运行 `go test ./...`、vet 或 race，因此这个治理要求尚未成为实际 CI 门禁。

### 7.4 HIGH：工具协议无法同时表达输出和失败

位置：

- [`internal/tools/bash.go:170`](../internal/tools/bash.go#L170)
- [`internal/tools/registry.go:240`](../internal/tools/registry.go#L240)

工具基础接口返回 `(string, error)`。Bash 为了把 stderr/stdout 返回给模型，在命令非零退出或超时时返回文本和 `nil error`：

```go
if result.TimedOut {
    return warningOutput, nil
}
if result.Err != nil {
    return formattedOutput, nil
}
```

Registry 只根据 `error` 设置 `schema.ToolResult.IsError`。于是非零退出、超时和启动失败都可能成为 `IsError=false`。

影响包括：

- recovery tracker 无法准确统计失败。
- metrics 和 tracing 会记录工具成功。
- reporter 展示的状态不可靠。
- 模型接收到的结构化状态与文本内容互相矛盾。
- 任何依赖 `IsError` 的完成或恢复策略都可能做出错误决策。

结论：问题不在 Bash 的文案，而在工具端口的信息表达能力不足。

建议将“业务执行结果”与“调用基础设施错误”分开：

```go
type ExecutionResult struct {
    Output   string
    Failed   bool
    ExitCode *int
    TimedOut bool
}

Execute(ctx context.Context, args json.RawMessage) (ExecutionResult, error)
```

其中 `error` 只用于参数解码、工具不可用、内部 I/O 等无法形成有效执行结果的情况。

### 7.5 HIGH：运行时组装重复并产生行为漂移

`AgentEngine` 至少在以下位置被独立组装：

- [`internal/app/runner.go`](../internal/app/runner.go)
- [`internal/feishu/runner.go`](../internal/feishu/runner.go)
- [`internal/agentops/runner.go`](../internal/agentops/runner.go)
- [`cmd/bench/main.go`](../cmd/bench/main.go)
- [`internal/subagent/manager.go`](../internal/subagent/manager.go)

这些入口的差异包括：

- 是否使用新 permission coordinator。
- 是否使用旧 DangerMiddleware。
- 是否启用 checkpoint。
- 是否注入 skills、slash 或 collaboration 能力。
- 是否启用 auto memory。
- 是否启用 compaction。
- 最大轮次数量。
- reporter、metrics 和 tracing 的接入方式。

已经出现的具体漂移：

- Feishu/AgentOps 的子 Agent 没有绑定权限协调器。
- subagent 默认最大 200 轮，但没有接入 compaction。
- benchmark 运行时不等价于主应用运行时，因此评估结果不能完全代表实际产品行为。

结论：当前低耦合主要体现在基础包，运行时组装层仍是复制式复用。每增加一项横切能力，都需要修改多个入口，否则就会产生安全和行为不一致。

### 7.6 HIGH：远程服务缺少有界并发和资源回收

Feishu Runner 在 [`internal/feishu/runner.go:61`](../internal/feishu/runner.go#L61) 对每个任务直接启动 goroutine：

```go
go r.runOne(ctx, task)
```

虽然输入来自 channel，但任务一旦取出就立即进入新 goroutine，因此 channel 容量不能限制实际执行并发。Runner 还用 `map[string]*sync.Mutex` 保存会话锁，键不会清理。

Feishu 会用 session mutex 串行化同一会话的任务，并为单任务设置 5 分钟 timeout，因此无界执行主要发生在不同会话之间。AgentOps 命令同样为每个任务启动 goroutine，并使用不会淘汰的去重 map；它从 `context.Background()` 启动任务且没有同等的任务 timeout，风险更高。Feishu gateway 使用默认 HTTP server，没有显式的 header/read/write/idle timeout，也没有 graceful shutdown。

可能后果：

- 外部任务突发时 goroutine 数量快速增长。
- 长期运行后会话锁和去重记录持续积累。
- 慢连接占用 server 资源。
- shutdown 时正在执行的请求和任务缺少完整收束。

结论：对本地 CLI 这不是核心问题，但 Feishu 和 AgentOps 属于远程、长期运行入口，需要明确的容量边界。

### 7.7 MEDIUM：`AgentEngine` 主循环职责过多

位置：

- [`internal/engine/loop.go:78`](../internal/engine/loop.go#L78)
- [`internal/engine/loop.go:295`](../internal/engine/loop.go#L295)
- [`internal/engine/config.go`](../internal/engine/config.go)

`RunWithReporter` 在一个函数中负责：

- 创建 run 和 transcript。
- 构造初始上下文。
- turn 前压缩。
- reminder 和 recovery 注入。
- thinking/action 两阶段模型调用。
- streaming fallback。
- 工具批次调度与并行执行。
- 大结果持久化。
- metrics、tracing 和 reporter。
- completion gate。
- 最终状态和错误返回。

`Config` 又通过多个 callback 承载 checkpoint、用户消息 ID、工具调用、提醒、completion gate 和 context estimate。

**内聚性判断**：这些工作都与“完成一次 Agent run”有关，因此具有用例级相关性；但它们具有不同变化原因和测试方式。模型调用重试、工具调度、上下文压缩、运行日志任一变化都需要修改同一个大函数，已经超出单一职责的合理范围。

### 7.8 MEDIUM：`AgentRunner` 混合 composition root、facade 和状态服务

位置：

- [`internal/app/runner.go:54`](../internal/app/runner.go#L54)
- [`internal/app/runner.go:418`](../internal/app/runner.go#L418)

`AgentRunner` 同时负责：

- provider 和工具运行时组装。
- 当前 session、memory store 和 checkpoint。
- model、effort、collaboration mode 和 permission 的可变配置。
- run 串行化。
- message history、rewind 和 compact。
- 给 TUI 提供查询和控制方法。

`internal/app` 依赖 20 个内部包。作为 composition root，这一数字可以接受；但将组装和长期可变运行状态放在同一对象中，使测试、锁设计和接口稳定性变得困难。

### 7.9 MEDIUM：TUI 状态和运行时接口过宽

位置：

- [`internal/tui/model.go:46`](../internal/tui/model.go#L46)
- [`internal/tui/model.go:213`](../internal/tui/model.go#L213)
- [`internal/tui/model.go:398`](../internal/tui/model.go#L398)
- [`internal/tui/model.go:1206`](../internal/tui/model.go#L1206)

TUI 的主 `Runner` 接口包含约 16 个方法，并暴露 session、checkpoint、compaction 和 collaboration 等领域类型。主 `Model` 持有大量输入、运行、选择、sidebar、dialog、approval 和渲染状态。

项目已经把 ask form、selector、markdown 等部分能力抽出，但主 reducer 仍需理解几乎所有 UI 状态和运行时能力。

影响：

- 新增一种 dialog 或运行状态时容易修改主 Model。
- 测试 double 必须实现宽接口。
- UI 与 app 内部领域对象直接耦合。
- 键盘处理和状态转换难以局部推理。

### 7.10 MEDIUM：可观测性写入错误被静默忽略

`AgentEngine` 多处以 `_ =` 方式忽略非关键 transcript、metrics 和 trace append 错误。`internal/tracing` 的记录方法也不向调用者返回写入失败。核心 message log 写入采用不同策略，其错误会作为运行错误返回，因此这里不应概括为“所有持久化错误都被忽略”。

这种设计可能是“可观测性不得影响主任务”的有意选择，但当前没有替代信号：

- 不会向 RunResult 报告 telemetry degraded。
- 没有一次性 warning。
- 没有记录落盘失败计数。
- metrics 和 tracing 包没有测试。

结论：非致命写入失败可以不终止 Agent，但不应完全不可见。

### 7.11 MEDIUM：大文件读取先完整载入内存

`ReadFileTool` 在 [`internal/tools/read_file.go:76`](../internal/tools/read_file.go#L76) 使用 `io.ReadAll`，之后才做输出截断。AgentOps log search 也先 `os.ReadFile` 并 `strings.Split` 整个日志。

这意味着“输出有限”并不等于“内存使用有限”。对仓库中的超大文件或长期增长日志，可能造成不必要的内存峰值。

### 7.12 LOW：工具定义顺序不稳定

`GetAvailableTools` 在 [`internal/tools/registry.go:187`](../internal/tools/registry.go#L187) 直接遍历 map。

Go map 遍历顺序不稳定，因此相同 registry 可能产生不同顺序的工具定义。这不会改变工具集合，但会影响：

- 模型输入的字节级稳定性。
- prompt cache 命中。
- benchmark 可复现性。
- snapshot 测试。

建议按工具名称稳定排序。

### 7.13 LOW：compaction 变化判断依赖 slice backing array

`sameMessages` 在 [`internal/engine/loop.go:842`](../internal/engine/loop.go#L842) 通过元素地址判断压缩结果是否与原 slice 相同。

这使 engine 依赖 compactor 的分配策略：

- 内容相同但复制到新 slice 会被认为已变化。
- 若未来 compactor 原地修改，可能被认为未变化。

更清晰的协议是让 compactor 返回显式的 `Changed` 状态。

## 8. 高内聚、低耦合评估

### 8.1 包级内聚

总体评价：**中上**。

表现较好的包：

- `provider`
- `schema`
- `session`
- `context`
- `compaction`
- `automemory`
- `permission`
- `toolresult`
- `autodev`

这些包大体都有清楚的领域名词、有限的变化原因和相对独立的测试。

表现较弱的包：

- `engine`：运行循环与多个横切能力集中。
- `app`：组装、运行、会话和 UI facade 混合。
- `tui`：主 Model 和主 reducer 状态面过大。
- `agentops`、`feishu`：入口适配与运行时组装重复。

### 8.2 用例级内聚

总体评价：**偏低**。

一次 Agent run 所需能力主要通过 `RunWithReporter` 和 `AgentRunner.runInternal` 聚合。它们不是简单 orchestrator，而是知道每个子系统的大量细节。修改压缩、工具、权限、checkpoint 或 observability 时，容易同时触碰这些中心函数。

### 8.3 结构耦合

总体评价：**中等**。

正面因素：

- 没有 import cycle。
- provider、tool 和 reporter 使用接口。
- 多数基础包可以独立测试。

负面因素：

- `engine` 直接持有具体 compactor、recovery、reminder 和 filesystem 类型。
- TUI 接口泄漏多个 app/domain 类型。
- `Config` callback bag 形成隐式协议。
- 不同入口复制运行时构造逻辑。

### 8.4 行为耦合

总体评价：**偏高**。

行为耦合比 import 耦合更值得关注：

- Bash 返回 `nil error` 会改变 recovery、metrics、tracing 和 reporter 的行为。
- permission coordinator 是否在组装时传入，会改变整个子 Agent 安全边界。
- compaction 是否被某个入口接入，会改变长任务是否可持续。
- 工具 registry 的构造差异会改变不同入口下同一模型可做的事情。

这类耦合无法仅通过包依赖图发现，必须通过端到端契约以及共享的安全和能力不变量解决。

### 8.5 总体判断

项目具备良好的模块化基础，但当前状态更准确的描述是：

> 基础能力包多数高内聚、依赖方向清晰；核心编排层职责集中，入口运行时复制，导致用例级内聚不足、行为耦合偏高。

因此，项目尚未达到“整体高内聚、低耦合”的状态。

## 9. 评分

评分用于表达相对风险和改进优先级，不是精确质量度量，也不代表当前 CI 的实际通过或失败状态。它采用以下启发式锚点：

- 100%：该维度没有已知高风险缺陷，关键契约有自动化验证，边界和失败行为明确。
- 50%：核心功能可用，但存在会放大变更成本或运行风险的实质缺口。
- 0%：核心行为不可依赖，缺少必要边界或验证。

总分阈值采用：80-100 为 `Pass`，50-79 为 `Needs Work`，0-49 为 `Fail`。各项扣分来自本报告列出的已确认问题，不是行业标准化测量。

| 维度 | 权重 | 得分 | 主要依据 |
|---|---:|---:|---|
| 正确性与安全性 | 30 | 12 | 文件破坏缺陷、远程权限策略缺失、工具失败语义错误 |
| 架构与边界 | 25 | 14 | 基础分层良好，但 engine/app/TUI 和多入口组装耦合明显 |
| 可读性与可维护性 | 20 | 13 | 命名和包结构较好，大函数和大状态模型降低局部理解能力 |
| 测试与可验证性 | 15 | 12 | 总覆盖率和测试体量较好，但七个包无测试且关键缺陷未覆盖 |
| 可观测性与运行可靠性 | 10 | 5 | 能力齐全，但写入失败静默、远程资源无界 |
| **总计** | **100** | **56** | **Needs Work，存在按治理原则应阻断的问题** |

## 10. 建议的目标架构

目标不是增加大量抽象，而是让变化原因形成清晰边界。

### 10.1 显式表达入口差异

各入口需要显式表达来源、权限、能力和运行预算差异。一个候选方案是把外部输入转换为稳定任务：

```go
type Task struct {
    Source      Source
    UserID      string
    SessionHint string
    Prompt      string
    RunProfile  RunProfile
}
```

`RunProfile` 显式描述入口差异，例如：

- 权限模式。
- 是否允许委派。
- 最大轮次。
- 是否启用 compaction、memory、checkpoint。
- 可用工具集合。
- reporter/approval adapter。

统一 `Task` 不是唯一方案。入口可以保留各自的 transport DTO 和 session/reporter adapter；关键要求是安全和能力差异必须由可审查的 profile/policy 表达，而不是隐藏在复制的 runtime builder 中。

### 10.2 统一安全和能力不变量

当前最需要形成唯一事实来源的是：

- 创建工具 registry。
- 装饰 permission。
- 构造 subagent manager，并传递父权限及证据链。
- 决定 compaction、memory、checkpoint 等关键能力是否启用。
- 校验 turn/context budget。

可以先提取 `RegistryBuilder` 和 `CapabilityPolicy`。是否进一步合并为单一 `RuntimeFactory + RunProfile`，应通过 ADR 比较后决定。benchmark、subagent 和远程入口可以保留不同 session、reporter 和 turn budget，但不能复制或绕过安全不变量。这样可以避免把当前多个 composition root 合并成新的大工厂。

### 10.3 收缩 AgentEngine

以下是候选协作者边界：

```text
AgentEngine
 ├─ ContextManager
 ├─ ModelInvoker
 ├─ ToolExecutor
 ├─ TurnPolicy
 └─ RunJournal
```

- `ContextManager`：初始上下文、token estimate、compaction。
- `ModelInvoker`：streaming、fallback、provider metadata。
- `ToolExecutor`：工具批次、并行性、结构化结果、结果持久化。
- `TurnPolicy`：reminder、recovery、completion gate、最大轮次。
- `RunJournal`：协调 session/transcript/metrics/tracing，分别明确核心 session 写入的致命失败策略，以及 transcript/metrics/tracing 的非致命降级策略。

顶层 engine 只保留可读的状态机：

```text
prepare turn
 -> invoke model
 -> if final: evaluate completion
 -> if tool calls: execute and append results
 -> update turn state
 -> continue or finish
```

不建议引入通用 event bus。少量、面向行为的小接口比全局事件系统更容易追踪和测试。

这些名称不是预先承诺的最终接口。应先提取边界最明确的 `ToolExecutor`，验证测试和调用关系确实变简单后，再决定是否提取其余协作者。

### 10.4 分离 AgentRunner 的角色

建议拆分：

- `RuntimeFactory` 或更小的 builders：依赖组装。
- `RunCoordinator`：串行化 run 和调用 engine。
- `SessionController`：new/switch/history/rewind/compact。
- `RuntimeSettings`：model、effort、collaboration、permission。

用单一 `RunOptions` 替代多个参数组合不同的 `RunWith...` 方法。

### 10.5 拆分 TUI 状态

主 Model 可以组合以下状态：

- `inputState`
- `runState`
- `selectionState`
- `sidebarState`
- `dialogState`
- `approvalState`

每种消息先由对应 reducer 处理，主 `Update` 只负责路由。TUI 不直接依赖 checkpoint/compaction 等具体领域对象，而通过小型 capability interface 或 UI DTO 访问。

## 11. 分阶段实施路线

### P0：正确性和安全边界

1. 为 `replaceLineRange` 添加会失败的回归测试。
2. 为 fuzzy 最后窗口和整文件窗口添加边界测试。
3. 修复 `content[endLine:]` 为 `content[end:]`，修复 `<` 为 `<=`。
4. 为 Feishu/AgentOps 的 workspace 外文件、可变 shell 和嵌套委派增加端到端权限测试。
5. 让远程父 registry 接入统一 permission coordinator。
6. 将父 permission coordinator 和 evidence provider 传给 subagent；没有 nested enforcement 时按策略拒绝或要求人工确认委派。
7. 引入结构化工具执行结果，修正 Bash 非零退出和超时状态。
8. 在 CI 中至少加入 `go test ./...`，让基础正确性检查成为实际门禁。

### P1：统一运行时和可靠性

1. 提取共享 `RegistryBuilder`、`CapabilityPolicy` 和嵌套权限构造路径。
2. 先迁移 Feishu 和 AgentOps，消除当前安全漂移。
3. 通过 ADR 决定是否进一步引入 `RunProfile` 和统一 `RuntimeFactory`，再迁移其他入口的共享不变量。
4. 为 subagent 接入 compaction，或降低默认轮次并明确 context budget。
5. 为远程入口增加 semaphore/worker pool。
6. 为锁和 dedupe map 增加 TTL、LRU 或引用计数回收。
7. 使用显式 `http.Server` 配置 timeout 和 shutdown。
8. 为七个无测试包补充行为和错误路径测试。

### P2：编排层解耦

1. 从 engine 提取 `ToolExecutor`，这是协议修复后的自然边界。
2. 提取 `ModelInvoker` 和 `ContextManager`。
3. 将 transcript/metrics/tracing 收敛为 `RunJournal`。
4. 将 recovery/reminder/completion gate 收敛为 `TurnPolicy`。
5. 拆分 `AgentRunner` 的 session 与 runtime 职责。
6. 分阶段拆分 TUI state 和 reducer。

### P3：一致性和维护体验

1. 稳定排序工具定义。
2. 使用 limited reader/scanner 处理大文件和日志。
3. 让 compactor 显式返回 `Changed`。
4. 为 architecture diagram 增加可审查的文字说明或 ADR。
5. 在基础测试门禁之上加入 race、vet、coverage threshold 和 ShellCheck。

## 12. 不建议立即做的事情

### 12.1 不要仅为补齐名词创建“意图识别包”

当前模型工具选择足以承担通用意图路由。没有确定性路由需求时，单独 intent service 只会增加层次和调试成本。

### 12.2 不要一次性重写 engine

engine 是高风险核心路径。应先通过工具结果协议和权限测试建立安全网，然后逐个提取协作者，每一步保持 `go test -race ./...` 通过。

### 12.3 不要用通用事件总线替代显式依赖

事件总线会让执行顺序、错误传播和测试边界更难理解。当前规模适合使用少量同步接口和明确的返回值。

### 12.4 不要只按文件大小机械拆包

拆分目标应是隔离变化原因和行为契约。仅把大文件按行数切成多个文件，不会降低状态耦合。

## 13. 验收标准

完成 P0 后至少应满足：

- trimmed/fuzzy edit 回归测试证明不会破坏未替换内容。
- 模糊匹配覆盖最后窗口和整文件窗口。
- Feishu/AgentOps 的 workspace 外路径、可变 shell 和嵌套委派均经过统一策略评估。
- 缺少 nested enforcement 的委派不能被旧中间件直接放行。
- Bash 非零退出和超时产生 `IsError=true` 或等价结构化失败状态。
- recovery、metrics、tracing 能观察到工具失败。
- CI 对每次相关变更执行 `go test ./...`。

完成 P1 后至少应满足：

- 所有入口共享同一组安全和能力不变量。
- 入口差异由可审查的 profile、policy 或专用 adapter 显式表达。
- 子 Agent 具有明确 context budget，并配置 compaction 或足以避免上下文溢出的轮次上限。
- 远程执行并发有硬上限。
- 长期运行的锁和去重状态可以回收。
- 每个 Go package 至少有一个测试文件。

完成 P2 后至少应满足：

- 工具执行、模型调用、上下文处理和 journal 失败策略均可独立注入和单元测试。
- engine 顶层循环不再直接实现上述协作者的内部策略。
- AgentRunner 不再同时承担 runtime factory 和 session controller。
- TUI 主 Runner 接口按能力拆分，主 Model 状态按领域组合。

## 14. 最终结论

FoxHarness 不是一个缺少架构的项目。它已经形成 provider、tools、context、session、compaction、permission、memory 和 observability 等基础模块，核心 Agent 流程完整，测试投入也明显高于一般早期项目。

当前主要矛盾是：

1. 基础模块已经逐渐成熟，但主编排仍以大函数和 callback bag 集成。
2. 产品入口不断增加，但安全和关键运行能力的组装没有形成唯一事实来源。
3. 权限和工具失败等端到端契约没有在所有入口保持一致。
4. 测试总量较高，但少数关键 fallback 和横切包存在明显盲区。

因此，对当前状态最准确的评价是：

> 包级模块化基础较好，核心扩展点基本合理；编排层和入口层尚未实现高内聚低耦合，并已出现由组装漂移导致的正确性和安全问题。

改进工作应先处理文件完整性和远程权限边界，再统一安全与能力不变量，最后渐进拆分 engine、AgentRunner 和 TUI。这个顺序能够最大化风险收益比，并避免在安全网不足时进行大规模重构。
