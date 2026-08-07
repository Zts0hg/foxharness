# OpenHands Agent 架构与上下文压缩实现参考

## 1. 文档目的

本文记录对本地项目 `/Users/xiaoming/code/OpenHands-main` 的静态分析结果，重点回答以下问题：

1. OpenHands 的真实代码边界和主要模块是什么。
2. 用户消息如何经过上下文构造、模型调用、工具执行和状态更新形成完整 Agent 循环。
3. 项目是否具备较好的高内聚、低耦合特征。
4. 用户选择上下文压缩方式的功能如何从前端配置一直作用到 Agent 运行时。
5. 哪些设计适合 FoxHarness 参考，哪些复杂度不应直接复制。

分析日期为 2026-08-02。被分析目录没有 `.git` 目录，也没有安装 `node_modules`，因此本文不包含提交历史、代码变更频率或动态测试结论。核心运行时分析使用该项目固定的 `openhands-agent-server v1.39.1` 对应官方 `software-agent-sdk` 源码进行核对。

## 2. 核心结论

`/Users/xiaoming/code/OpenHands-main` 并不是完整的 OpenHands Agent 内核，而是 `@openhands/agent-canvas 1.8.0`，主要负责 React UI、桌面壳层、状态投影以及 Agent Server API 适配。

完整系统由三层组成：

```text
Agent Canvas
  UI / 表单 / REST与WebSocket适配 / 前端状态投影
                    |
                    v
Agent Server
  API / 会话持久化 / 事件发布 / 设置与Profile管理
                    |
                    v
software-agent-sdk
  Conversation循环 / Agent / Context / LLM / Tools / Condenser / Critic
                    |
                    v
Workspace / MCP / 浏览器 / 终端 / 外部LLM
```

总体评价如下：

- 宏观架构边界清晰。Canvas 是控制面和展示面，Agent Server 与 SDK 是执行面。
- SDK 中 Conversation、Agent、Context、Condenser、Tool、Event 的领域职责清楚，整体符合高内聚、低耦合方向。
- 追加式事件日志同时服务于状态恢复、实时 UI、审计以及上下文压缩，是整个架构最有参考价值的设计。
- Canvas 内部可维护性中等偏上，但 WebSocket 集成、设置兼容、local/cloud 双路径已经形成若干大型变更热点。
- 所谓“选择上下文压缩方式”，在当前固定版本中实际只有 `llm_summarizing` 和 `no_op` 两个选项。`no_op` 表示不压缩，并不是另一种摘要算法。

## 3. 分析范围与项目事实

### 3.1 本地项目事实

本地源码中的 `docs/architecture.md` 明确说明：

- Agent Canvas 是用于运行和监控 OpenHands Agent 的 React/TypeScript 前端。
- Canvas 将 UI 操作转换为 Agent Server API 调用。
- Canvas 不直接执行 Agent action。
- Canvas 不提供沙箱或 workspace 隔离。
- Canvas 不负责在后端之外托管 LLM 凭据。

`config/defaults.json` 给出的主要版本为：

| 组件 | 版本 |
|---|---:|
| Agent Canvas | 1.8.0 |
| Agent Server | 1.39.1 |
| Automation | 1.5.0 |
| 最低兼容 Agent Server | 1.28.0 |
| TypeScript Client | 1.36.1 |

静态统计结果：

| 指标 | 数量 |
|---|---:|
| `src` 下 TypeScript/TSX 文件 | 1098 |
| TypeScript/TSX 总行数 | 111743 |
| 扫描到的测试文件 | 554 |

主要大型文件包括：

| 文件 | 行数 | 主要职责 |
|---|---:|---|
| `src/components/features/backends/backend-form-modal.tsx` | 1637 | 后端配置 UI |
| `src/mocks/settings-handlers.ts` | 1222 | 设置相关 mock |
| `src/api/agent-server-adapter.ts` | 1161 | UI 设置到 Agent Server 请求的适配 |
| `src/contexts/conversation-websocket-context.tsx` | 1113 | 主会话和 planning 会话的实时事件集成 |
| `src/components/features/conversation-panel/conversation-panel.tsx` | 955 | 会话工作区 UI |
| `src/api/conversation-service/agent-server-conversation-service.api.ts` | 876 | 会话服务适配 |
| `src/services/telemetry.ts` | 874 | 前端遥测 |
| `src/api/settings-service/settings-service.api.ts` | 757 | 设置读取、兼容和保存 |

这些文件并非都违反单一职责，但它们是未来修改时最需要保护性测试和进一步拆分的区域。

### 3.2 SDK 模块结构

与 Agent 执行直接相关的官方 SDK 目录包括：

```text
openhands/sdk/
├── agent/          Agent单步推理、响应分发、并行工具执行
├── conversation/   生命周期、循环、暂停、恢复、并发和状态
├── context/        AgentContext、Skills、Memory、View、Condenser
├── event/          Message、Action、Observation、Condensation等事件
├── llm/            模型调用、消息类型、异常和指标
├── tool/           Tool定义、注册、schema、内置工具
├── mcp/            MCP工具集成
├── subagent/       子Agent与委派
├── critic/         结果评估
├── security/       Action风险分析
├── hooks/          生命周期和停止钩子
├── observability/  追踪与观测
├── settings/       Agent配置、迁移、schema导出和对象构造
├── skills/         技能加载和触发
└── workspace/      工作区抽象
```

Agent Server 进一步提供 conversation、event、settings、profiles、skills、MCP、workspace、sub-agents 等 API 和持久化服务。

## 4. Agent 完整执行脉络

用户提出的执行脉络与 OpenHands 基本一致，但实际实现是前端、Agent Server、SDK 三层协作，而不是一个对象依次执行全部步骤。

```text
用户输入
 -> Canvas规范化文本、图片和附件
 -> WebSocket或REST发送Message，附带run:true
 -> Agent Server持久化MessageEvent
 -> Agent Server启动Conversation.run/arun
 -> Conversation检查状态、预算、暂停、确认和stuck detector
 -> Agent.step从state.view准备LLM消息
 -> 必要时先执行Condenser
 -> 调用LLM
 -> classify_response分类模型返回
 -> 解析tool calls或普通内容
 -> 安全分析和用户确认
 -> 执行工具并收集Observation
 -> 所有结果追加为Event
 -> 更新ConversationState和执行状态
 -> Agent Server经WebSocket发布Event
 -> Canvas去重并更新各类UI投影
 -> Conversation进入下一轮或结束
```

### 4.1 用户输入规范化

`src/hooks/use-send-message.ts` 将 UI 层的：

```typescript
{ action: "message", args: { content, image_urls } }
```

转换成 Agent Server 接受的 `SendMessageRequest`：

```typescript
{
  role: "user",
  content: [
    { type: "text", text: "..." },
    { type: "image", image_urls: ["..."] }
  ]
}
```

这属于协议适配，不承担任务意图判断。

### 4.2 启动运行

`conversation-websocket-context.tsx` 优先通过当前模式对应的 WebSocket 发送消息：

```typescript
currentSocket.send(JSON.stringify({ ...message, run: true }));
```

如果 WebSocket 尚未连接，则调用 `ConversationClient.sendEvent()`，同样携带 `{ run: true }`。因此消息写入和启动 Agent 是同一个服务端命令的两个语义。

Planning 模式使用独立的 sub-conversation 和独立 WebSocket。它不是 Canvas 在本地直接调用另一个 Agent 对象，而是创建并连接另一个后端会话。

### 4.3 Conversation 循环

固定版本 SDK 的 `LocalConversation.run()` 负责：

1. 初始化 Agent、插件和取消令牌。
2. 将可恢复状态切换为 `RUNNING`。
3. 在循环开始时检查 `PAUSED`、`STUCK` 和 `FINISHED`。
4. 在结束前执行 stop hooks，hook 可以拒绝结束并注入反馈。
5. 运行 stuck detector。
6. 处理等待用户确认后的恢复。
7. 调用一次 `agent.step()`。
8. 检查确认等待、预算限制、最大迭代数和错误。
9. 根据状态继续下一轮或退出。

Conversation 因而负责“是否继续运行”，Agent 负责“这一轮做什么”。这是一个合理且重要的职责边界。

### 4.4 Agent 单步推理

`Agent.step()` 的核心顺序是：

1. 如果存在已经确认的 pending actions，先执行这些 action，不进行新模型采样。
2. 检查用户消息是否被 hook 阻止。
3. 从 `state.view` 构造 LLM messages。
4. 如果上下文应压缩，返回 `Condensation`，将其作为事件追加后结束本次 step。
5. 处理非视觉模型收到图片的情况。
6. 调用 Agent LLM。
7. 对模型输出执行纯函数分类。
8. 根据分类结果分发到 tool calls、文本消息或空响应处理器。

模型响应分类优先级为：

```text
TOOL_CALLS
 -> CONTENT
 -> REASONING_ONLY
 -> EMPTY
```

有工具调用时，响应会被解析为 `ActionEvent`。普通文本响应会生成 `MessageEvent` 并将会话置为 `FINISHED`，含义是当前 Agent 回合结束并等待下一条用户消息。只有 reasoning 或空响应时，框架会注入纠正消息并继续循环。

### 4.5 工具执行和确认

工具调用通过 `ParallelToolExecutor` 批量执行。执行前会：

- 截断 `FinishTool` 之后的多余调用。
- 区分被 hook 阻止和可执行的 action。
- 根据 confirmation mode 与安全分析结果决定是否等待用户确认。
- 保留 action 顺序发布执行结果，即使底层工具并行运行。

工具结果转换为 Observation 事件并进入同一事件日志。Agent 不通过某个临时返回值私下保存工具结果，下一轮统一从更新后的 View 中获取上下文。

### 4.6 前端状态投影

Canvas 的 `use-event-store.ts` 同时维护：

- `events`：收到的原始事件。
- `eventIds`：用于 O(1) 去重的 ID 集合。
- `uiEvents`：经 UI 规则转换后的显示事件。
- `loadedConversationId`：当前全局 store 属于哪个会话。

初始和历史事件通过 REST 分页加载，实时事件通过 WebSocket 到达。WebSocket 重连可能重放旧事件，因此 store 和副作用处理器都会检查事件 ID。

WebSocket Provider 还会根据事件类型更新：

- 执行状态。
- LLM 与 condenser 指标。
- 终端输入输出。
- 浏览器截图和 URL。
- goal 状态。
- 模型切换状态。
- 文件与 Git 相关缓存。
- UI client tool action。

这说明事件日志是权威事实，Zustand stores 是读模型或 UI 投影。

## 5. 与常见 Agent 项目组成的映射

| 常见组成 | OpenHands 对应实现 | 分析结论 |
|---|---|---|
| 用户输入规范化 | Canvas chat hooks、附件处理、Agent Server adapter | 明确存在 |
| 意图识别 | slash commands、skill triggers、LLM工具选择 | 没有统一的前置意图分类器 |
| Agent运行 | `LocalConversation.run/arun` 与 `Agent.step/astep` | 核心职责分离 |
| 上下文管理 | `AgentContext`、Skills、SystemPrompt、View、Condenser | 完整且边界清晰 |
| 检索/RAG | public/user/project skills、keyword/task triggers | 不属于传统向量RAG |
| 评估与反馈 | Critic、iterative refinement、stop hooks、stuck detector | 与循环集成 |
| 工具管理 | Tool registry、built-ins、MCP、client tools、并行执行 | 功能完整 |
| Agent委派 | SDK subagent、Canvas planning sub-conversation | 通过工具和独立会话实现 |
| 用户确认 | confirmation policy、风险分析、confirmation response API | 事件化处理 |
| 可观测性 | 事件日志、Laminar、PostHog、独立LLM指标 | 边界较好 |

### 5.1 关于“意图识别”

OpenHands 没有在所有用户请求前执行统一的“编码任务、问答任务、搜索任务”分类。主要决策来自：

- Canvas 识别 slash command 等显式 UI 命令。
- Skill trigger 根据关键词或任务条件激活知识。
- LLM 根据上下文与 tool schemas 直接选择下一步行动。

因此更准确的描述是“命令拦截和条件上下文激活 + LLM 决策”，而不是一个独立意图识别流水线。

### 5.2 关于 RAG

SDK Context 提供 AgentContext 与 Skill。Skill 可以：

- 对所有会话生效。
- 由关键词触发。
- 由任务条件触发。
- 来自 public、user 或 project 范围。

扫描固定版本的 SDK 模块没有发现核心向量数据库、embedding 或统一 retriever。因此 OpenHands 的默认上下文增强更接近结构化技能和项目知识注入，不应笼统称为传统向量 RAG。外部 MCP 或工具仍可以自行提供检索能力。

## 6. 上下文压缩设置如何实现

### 6.1 后端配置模型

在 `software-agent-sdk v1.39.1` 中，`OpenHandsAgentSettings.condenser` 的类型是 `CondenserSettingsConfig`。它是 Pydantic discriminated union，包含两个变体：

```python
CondenserSettingsConfig = Annotated[
    Annotated[LLMSummarizingCondenserSettings, Tag("llm_summarizing")]
    | Annotated[NoOpCondenserSettings, Tag("no_op")],
    Discriminator(_condenser_settings_discriminator),
]
```

配置缺少 `condenser_kind` 时会默认解释为 `llm_summarizing`，用于兼容旧配置。

基础设置字段包括：

| 字段 | 默认值 | 含义 |
|---|---:|---|
| `enabled` | `true` | 是否启用上下文压缩 |
| `max_size` | `240` | View 中事件数上限 |

LLM 摘要变体还包括：

| 字段 | 默认值 | 含义 |
|---|---:|---|
| `condenser_kind` | `llm_summarizing` | 策略判别字段 |
| `max_tokens` | `null` | 可选 token 上限 |
| `keep_first` | `2` | 永不压缩的开头事件数 |
| `minimum_progress` | `0.1` | 一次压缩至少应遗忘的事件比例 |
| `hard_context_reset_max_retries` | `5` | 硬重置最大重试次数 |
| `hard_context_reset_context_scaling` | `0.8` | 重试时事件文本缩短比例 |

`NoOpCondenserSettings` 只保留 `enabled` 和 `condenser_kind="no_op"`，构建出的 NoOpCondenser 原样返回当前 View。

### 6.2 Schema 导出

Agent Server 并不要求 Canvas 硬编码这些设置，而是将 Pydantic 配置模型导出为统一的 `SettingsSchema`：

```text
SettingsSchema
 -> sections[]
     -> fields[]
         key
         label
         description
         value_type
         default
         choices
         depends_on
         prominence
         secret
         required
```

导出器遍历 `CondenserSettingsConfig` 的两个嵌套模型。两个模型都包含 `condenser_kind`，导出器按字段名合并，并将 Literal 中不同的值累积成 choices：

```json
{
  "key": "condenser.condenser_kind",
  "value_type": "string",
  "choices": [
    { "value": "llm_summarizing", "label": "llm_summarizing" },
    { "value": "no_op", "label": "no_op" }
  ]
}
```

这就是用户可以选择策略的后端来源。

### 6.3 前端设置页面

Canvas 路由 `settings/condenser` 渲染通用 `SdkSectionPage`，只选择 `agent_settings` schema 中的 `condenser` section。

页面的数据流为：

```text
GET /api/settings/agent-schema
 -> useAgentSettingsSchema
 -> SdkSectionPage筛选condenser section
 -> SchemaField按value_type和choices选择控件
 -> 用户修改字段
 -> dotted key重建嵌套配置
 -> PATCH agent_settings_diff
 -> 后端深度合并并持久化
```

`SchemaField` 的控件规则为：

- 有 `choices` 时渲染下拉框。
- boolean 渲染 switch。
- array/object 渲染 JSON textarea。
- integer/number 渲染数值输入框。
- 其他字段渲染普通文本输入框。

由于 `condenser_kind` 的 metadata 没有提高 prominence，它采用默认 minor 级别，通常需要切换到“All”设置视图才能看到。`enabled` 是 critical，`max_size` 等高级参数也是 minor。

### 6.4 保存和启动会话

表单内部以 dotted key 保存值，例如：

```text
condenser.enabled
condenser.condenser_kind
condenser.max_size
```

保存时由 `setDotted()` 还原成嵌套对象，生成：

```json
{
  "agent_settings_diff": {
    "condenser": {
      "enabled": true,
      "condenser_kind": "llm_summarizing",
      "max_size": 240
    }
  }
}
```

Settings Service 使用 PATCH 提交增量配置，Agent Server 将 diff 深度合并进已有 `agent_settings`。

创建本地会话时，Canvas 获取可安全回传的加密设置，将 `agent_settings` 放入 create-conversation payload。若用户选择 Agent Profile，则只发送 `agent_profile_id`，由 Agent Server 在服务端解析 Profile。`agent_profile_id` 与 inline `agent_settings` 互斥。

### 6.5 兼容路径

Canvas 仍同时维护旧的平面字段：

```text
enable_default_condenser
condenser_max_size
```

以及新的嵌套字段：

```text
agent_settings.condenser.enabled
agent_settings.condenser.max_size
```

Settings Service 在读取时会把嵌套值同步到旧字段，Cloud adapter 也会进行平面与嵌套配置转换。这保证旧 UI 和旧服务兼容，但增加了状态来源和迁移复杂度。

## 7. 上下文压缩运行原理

### 7.1 不修改原始事件日志

OpenHands 的会话历史是 append-only event log。压缩不能删除日志中的旧事件，否则会损害：

- 故障排查。
- 会话恢复。
- 审计能力。
- UI 完整历史展示。
- 下游事件处理。

因此压缩结果也被表示成一个新事件 `Condensation`：

```text
Condensation
├── forgotten_event_ids  本次从LLM View隐藏的事件ID
├── summary              可选摘要
├── summary_offset       摘要应插入View的位置
└── llm_response_id      摘要LLM响应ID
```

前端的 `CondensationEvent` 类型同样明确说明：旧事件只是从给 LLM 的 View 中移除，不是从原始会话中删除。

### 7.2 View 是 LLM 上下文投影

Agent 不直接读取所有历史事件，而是读取 `ConversationState.view`。View 会增量处理事件日志，并在遇到 Condensation 时：

1. 从当前 LLM 可见事件中移除 `forgotten_event_ids`。
2. 在 `summary_offset` 插入摘要事件。
3. 更新操作边界和压缩请求状态。

因此系统中存在三个不同但各自合理的表示：

```text
完整Event Log：用于持久化、审计、恢复
LLM View：用于模型上下文和压缩
UI Events：用于界面展示和事件分组
```

这是该项目最值得参考的状态设计。

### 7.3 触发条件

`LLMSummarizingCondenser` 检查三类触发原因：

| 原因 | 条件 | 要求等级 |
|---|---|---|
| `REQUEST` | View 中存在未处理的 CondensationRequest | HARD |
| `TOKENS` | token 数超过 `max_tokens` | HARD |
| `EVENTS` | 事件数超过 `max_size` | SOFT |

软触发表示当前只是希望维持上下文上限。如果无法找到合法的压缩区间，本轮可以保持原 View，下一轮再尝试。

硬触发表示如果不压缩，Agent 很可能无法继续。显式用户请求、Agent 捕获上下文窗口异常后发出的请求以及 token 超限都属于硬触发。

### 7.4 选择遗忘区间

常规压缩并不是简单删除固定数量的头部事件。算法会：

1. 永久保留最前面的 `keep_first` 个事件。
2. 根据不同触发原因计算要保留的尾部长度。
3. 如果同时有多个触发原因，选择最严格的压缩要求。
4. 通过 View 的 manipulation indices 修正开始和结束位置。
5. 确保不会把原子事件单元拆开，例如工具 action 与对应 observation。

目标通常是压缩到当前资源限制的一半附近：

- 显式请求时，目标约为当前 View 的一半。
- 事件超限时，目标约为 `max_size / 2`。
- token 超限时，目标约为 `max_tokens / 2`。

这种设计在摘要调用成本、prompt cache 重建、早期上下文保留和近期任务连续性之间做平衡。

### 7.5 生成摘要

选出旧事件后，Condenser 会：

1. 将每个待遗忘事件转换成字符串。
2. 渲染 `summarizing_prompt.j2`。
3. 调用专用的 condenser LLM。
4. 从模型返回中提取摘要文本。
5. 创建 Condensation 事件。

Agent Settings 构造 condenser 时会复制 Agent LLM 配置，但把 `usage_id` 设置为 `condenser` 并重置指标，因此同一会话能够分别统计：

```text
usage_to_metrics.agent
usage_to_metrics.condenser
```

Canvas 的 `ConversationStats` 类型也保留了这两个指标分组。

### 7.6 Agent 循环中的压缩位置

压缩发生在调用 Agent LLM 之前：

```text
Agent.step
 -> prepare_llm_messages(state.view, condenser, llm)
 -> 如果返回Condensation
      -> 追加Condensation事件
      -> 本次step直接返回
 -> 下一次Conversation循环
      -> View已应用Condensation
      -> 重新构造messages
      -> 调用Agent LLM
```

这避免同一个 step 同时修改 View 并继续使用压缩前的临时消息。

### 7.7 上下文异常恢复

如果 LLM 抛出上下文窗口超限或 malformed conversation history：

1. Agent 检查 condenser 是否支持显式请求。
2. 必要时重建 View。
3. 追加 `CondensationRequest`。
4. 当前 step 返回。
5. 下一轮将该请求识别为 HARD 压缩。

若常规平衡压缩无法执行，`hard_context_reset()` 会尝试总结整个 View。摘要调用失败时，它最多重试配置的次数，并按 `hard_context_reset_context_scaling` 逐步截短每个事件的字符串表示。

## 8. “可选择压缩方式”的准确边界

当前版本应准确理解为：用户可以选择是否使用默认 LLM 摘要 condenser，或者显式选择 no-op。

| 配置 | 运行时结果 |
|---|---|
| `enabled=false` | 不创建 condenser |
| `enabled=true, condenser_kind=llm_summarizing` | 创建 LLM 摘要 condenser |
| `enabled=true, condenser_kind=no_op` | 创建 NoOpCondenser，原样返回 View |

SDK 还包含 `PipelineCondenser`，可组合多个 condenser，但它没有包含在当前 `CondenserSettingsConfig` 联合中，因此用户设置页面无法选择任意 pipeline 或第三种压缩算法。

还存在一个重要例外：当前 `OpenHandsAgentSettings.create_agent()` 在 LLM 属于 subscription 模式时直接令 `condenser=None`。因此 UI 中保存的 condenser 设置并不保证在所有 Agent/LLM 模式下生效。此外 ACP Agent 由外部 ACP 进程驱动，OpenHands LLM 和 condenser 设置对 ACP Agent 也不生效。

## 9. 高内聚、低耦合分析

### 9.1 架构优势

#### 控制面与执行面分离

Canvas 不执行工具，不直接维护 Conversation 的权威状态。它只发送命令并消费事件。这使 React 生命周期、浏览器状态和网络断线不会直接破坏 Agent 运行时。

#### Conversation 与 Agent 职责分离

Conversation 负责运行生命周期、状态锁、预算、暂停、确认和循环；Agent 负责单步上下文准备、模型调用、响应分类和 action 生成。二者虽然协作紧密，但变化原因不同。

#### 事件驱动的一致状态模型

Message、Action、Observation、Condensation、Error 和 StateUpdate 都通过事件表达。实时通知、历史加载和恢复共享同一数据模型，避免为 UI、持久化和 Agent 上下文维护互不一致的事实来源。

#### Context 和 Condenser 独立

Condenser 接收 View 并返回 View 或 Condensation，不直接控制 Conversation 循环，也不修改事件存储。新的压缩实现可以在该协议下扩展。

#### 工具定义与执行分离

工具 schema、registry、内置工具、MCP 工具和 client tool 有清晰的类型边界。Agent 只生成 Action，具体执行由工具层和 executor 完成。

#### schema 驱动设置

设置字段、默认值、选择项、依赖和重要性来自后端模型。新增后端字段通常不需要创建独立 React 页面，有利于减少版本耦合。

#### API 访问约束

Canvas 规定组件不直接调用 service，数据读取和修改应封装在 TanStack Query hooks 中。项目还有测试扫描源码，禁止绕过 `@openhands/typescript-client` 使用低级 HTTP 调用。

### 9.2 主要耦合与风险

#### WebSocket Provider 职责过多

`conversation-websocket-context.tsx` 同时承担：

- 两条 WebSocket 的连接生命周期。
- 历史加载协调。
- 事件解析和去重。
- 错误上报。
- 终端、浏览器、goal、metrics、model、cache 等副作用。
- 主 Agent 与 planning Agent 的相似处理分支。

它在“会话事件同步”这个抽象层面有内聚性，但具体依赖了过多业务模块，已成为高耦合变更热点。

建议的演进方向是保留一个连接协调器，将副作用拆成按事件类型注册的 handlers，例如：

```text
WebSocketTransport
 -> EventIngestion
 -> EventDeduplicator
 -> EventProjectionRegistry
      -> ConversationStatusProjection
      -> TerminalProjection
      -> BrowserProjection
      -> MetricsProjection
      -> CacheInvalidationProjection
```

#### Local/Cloud 分支扩散

Cloud 历史存储、runtime sandbox、鉴权方式和本地 Agent Server 不同。虽然 backend registry 和 service layer 提供了隔离，但不少 service 仍在方法内部判断 `backend.kind`，容易导致两个路径行为漂移。

#### 多代设置协议共存

以下设置形式同时存在：

- 旧平面字段和新嵌套字段。
- Agent Profile 和 inline agent settings。
- OpenHands Agent 与 ACP Agent 两个联合变体。
- 本地设置与 Cloud 设置。
- 加密、脱敏和明文三种 secret exposure 模式。

这使 settings service 和 Agent Server adapter 成为复杂兼容中心。

#### 前后端版本和类型漂移

Canvas 固定 Agent Server 1.39.1，但 `@openhands/typescript-client` 是 1.36.1。Canvas 自行维护了一部分 Agent Server event TypeScript 类型和 type guards，以覆盖客户端未提供的结构。这能解决短期兼容，却可能在服务器升级时发生静默漂移。

#### Schema UI 表达能力有限

前端 `depends_on` 的判断只有：

```typescript
values[dependency] === true
```

它只能表达“另一个布尔开关为 true”，不能表达：

```text
condenser_kind == llm_summarizing
```

因此选择 `no_op` 后，All 视图仍可能显示只对 LLM 摘要有效的参数。后端最终可能忽略这些字段，但 UI 语义不够精确。

#### 联合 schema 的前端处理脆弱

Agent schema 可能对同一个 section key 导出不同 Agent 变体的 section。`SdkSectionPage` 当前通过“每个 key 只取第一个 section”规避重复渲染，而不是完整使用后端提供的 variant 元数据。这依赖 section 顺序，属于兼容性 workaround。

#### Mock 与真实 schema 不一致

`src/mocks/settings-handlers.ts` 的 condenser schema 仍主要模拟旧式启用开关和 max size，没有覆盖 `condenser_kind` 的两个 choices。结果是 mock 模式和普通前端单元测试无法可靠验证用户选择压缩实现的完整链路。

### 9.3 综合评价

| 范围 | 评价 | 说明 |
|---|---|---|
| 宏观系统边界 | 良好 | UI、Server、SDK、Workspace职责明确 |
| Agent核心模型 | 良好 | Conversation、Agent、Event、Tool、Context边界清楚 |
| 上下文压缩 | 优秀 | 非破坏性事件模型、策略接口和恢复流程完整 |
| 前端数据访问 | 良好 | service、query hook、component分层并有约束测试 |
| 前端实时集成 | 一般 | 大型Provider、副作用集中、主/规划处理重复 |
| 设置兼容 | 一般 | schema驱动优秀，但多版本、多表示转换复杂 |
| 类型稳定性 | 中等 | typed client与server版本不同，存在本地类型复制 |
| 测试设计 | 较好 | 测试数量多，包含架构约束测试，但压缩选择mock不足 |

整体并非“所有模块都已达到理想高内聚低耦合”，更准确的结论是：核心领域架构清晰，外围产品兼容层正在承担较高复杂度。

## 10. 对 FoxHarness 的参考建议

### 10.1 建议采用

#### 追加式事件日志作为权威状态

建议 FoxHarness 将以下内容都建模为领域事件：

```text
UserMessage
AssistantMessage
ActionRequested
ActionConfirmed / ActionRejected
ObservationReceived
DelegationStarted / DelegationCompleted
CondensationApplied
RunStateChanged
ErrorOccurred
```

状态、UI 和 LLM context 都从事件派生，避免多个组件分别维护事实。

#### 分离 ConversationRunner 和 AgentStep

建议边界为：

```go
type Agent interface {
    Step(ctx context.Context, state AgentState) (StepResult, error)
}

type ConversationRunner struct {
    agent       Agent
    eventStore  EventStore
    dispatcher ActionDispatcher
}
```

Runner 负责循环和状态，Agent 负责单轮决策。工具执行器不应成为 Agent 的隐式全局依赖。

#### 区分完整历史和 LLM View

至少保留三个明确概念：

```text
EventLog       完整权威历史
ContextView    当前模型可见上下文
Presentation   面向CLI/TUI/UI的展示投影
```

不要用原地截断 `[]Message` 的方式实现上下文压缩。

#### 将压缩设计成策略接口

可以采用小接口：

```go
type Condenser interface {
    Condense(ctx context.Context, view ContextView) (CondensationResult, error)
}
```

策略配置使用显式判别字段：

```yaml
context:
  condenser:
    kind: llm_summary
    max_events: 240
    keep_first: 2
```

`none` 应直接表示禁用。如果 NoOp 策略没有组合或测试用途，不必同时提供 `enabled=false` 和 `kind=no_op` 两套用户语义。

#### 保留原子工具边界

任何 context trimming 或 summarization 都必须把一次 assistant tool call 与对应 observations 视为原子单元。否则压缩后消息结构可能不符合模型 API 的 tool-use 协议。

#### 分离指标

至少独立统计：

```text
agent_llm
condenser_llm
critic_llm
delegated_agent_llm
```

否则很难判断上下文压缩节省的 token 是否大于摘要本身的成本。

### 10.2 不建议直接复制

- 如果 FoxHarness 没有 Web UI，不应复制复杂的 REST 历史加 WebSocket 增量同步层。
- 如果只有一个本地后端，不应预先引入 local/cloud registry 和大量分支。
- 不应在核心包中同时维护旧设置、新设置和多种序列化形态，除非确有迁移要求。
- 不应复制前后端事件类型。若未来有多语言客户端，应从同一个 schema 生成类型。
- 不应让一个事件消费者直接依赖终端、浏览器、指标、缓存等所有投影，应使用独立 handlers 或 subscribers。
- 不应只支持布尔 `depends_on`。配置 schema 若需要条件显示，应支持字段、操作符和值三元条件。

## 11. 建议的 FoxHarness 目标结构

结合 OpenHands 的优点并控制复杂度，可以考虑：

```text
internal/
├── conversation/
│   ├── runner.go          循环、暂停、终止、预算
│   └── state.go           会话运行状态
├── agent/
│   ├── agent.go           单步Agent接口
│   ├── prompt.go          上下文到模型消息
│   └── response.go        模型响应分类与解析
├── event/
│   ├── event.go           领域事件联合
│   ├── store.go           追加式事件存储接口
│   └── projection.go      投影接口
├── context/
│   ├── view.go            LLM可见View
│   ├── builder.go         system/skills/memory组合
│   └── condenser/
│       ├── condenser.go   策略接口
│       ├── summary.go     LLM摘要实现
│       └── none.go        可选禁用实现
├── tool/
│   ├── registry.go
│   ├── dispatcher.go
│   └── confirmation.go
├── delegation/
│   └── coordinator.go
├── evaluation/
│   └── critic.go
└── observability/
    ├── metrics.go
    └── tracing.go
```

关键依赖方向建议为：

```text
UI/TUI/CLI
 -> application/conversation
 -> agent、context、tool等领域接口
 -> LLM、存储、MCP等基础设施实现
```

领域包不应反向依赖具体 UI、具体数据库或具体模型 SDK。

## 12. 来源索引

### 12.1 本地 Agent Canvas 源码

- `docs/architecture.md`
- `AGENTS.md`
- `config/defaults.json`
- `package.json`
- `src/hooks/use-send-message.ts`
- `src/contexts/conversation-websocket-context.tsx`
- `src/stores/use-event-store.ts`
- `src/hooks/query/use-conversation-history.ts`
- `src/api/event-service/event-service.api.ts`
- `src/routes/condenser-settings.tsx`
- `src/hooks/query/use-agent-settings-schema.ts`
- `src/components/features/settings/sdk-settings/schema-field.tsx`
- `src/components/features/settings/sdk-settings/sdk-section-page.tsx`
- `src/utils/sdk-settings-schema.ts`
- `src/api/settings-service/settings-service.api.ts`
- `src/api/agent-server-adapter.ts`
- `src/api/conversation-service/agent-server-conversation-service.api.ts`
- `src/types/agent-server/core/events/condensation-event.ts`
- `src/types/agent-server/core/events/conversation-state-event.ts`
- `src/api/no-direct-agent-server-calls.test.ts`
- `src/mocks/settings-handlers.ts`

### 12.2 官方 SDK 固定版本源码

- [Agent](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/agent/agent.py)
- [Response dispatch](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/agent/response_dispatch.py)
- [LocalConversation](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/conversation/impl/local_conversation.py)
- [Settings model](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/settings/model.py)
- [Condenser architecture](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/context/condenser/README.md)
- [Condenser base](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/context/condenser/base.py)
- [LLM summarizing condenser](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/context/condenser/llm_summarizing_condenser.py)
- [No-op condenser](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/context/condenser/no_op_condenser.py)
- [Pipeline condenser](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/context/condenser/pipeline_condenser.py)
- [Condensation event](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/event/condenser.py)
- [Context View](https://github.com/OpenHands/software-agent-sdk/blob/v1.39.1/openhands-sdk/openhands/sdk/context/view/view.py)

## 13. 最终判断

OpenHands 最值得借鉴的不是具体 React 页面或庞大的服务兼容层，而是以下架构原则：

1. Conversation 循环与 Agent 单步决策分离。
2. 所有关键变化通过追加式领域事件表达。
3. 完整历史、LLM View 和 UI View 是不同投影。
4. 上下文压缩通过 Condensation 事件实现，不破坏原始历史。
5. 工具执行、用户确认、评估和观测围绕事件集成。
6. 配置通过判别联合选择策略，并由运行时工厂构造具体实现。

其核心领域总体具备较好的高内聚、低耦合特征；产品外围层因多后端、多配置代际和丰富 UI 功能产生了明显耦合。FoxHarness 应优先吸收事件、View、Runner、Condenser 等核心设计，不必复制 OpenHands 为 Cloud、浏览器 UI 和长期兼容承担的全部复杂度。
