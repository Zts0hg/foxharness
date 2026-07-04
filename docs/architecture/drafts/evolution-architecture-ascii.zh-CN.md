# 项目演进架构图 ASCII 草案

本文用于在绘制 draw.io 前快速确认项目演进架构图的内容和版式。它不是正式架构说明文档，不应被正式文档依赖。

## 图例

```text
实线箭头  ──▶   演进后的主要控制流 / 请求流
虚线箭头  - -▶   新增的状态读写 / 观测 / 治理能力
双向箭头  ◀─▶   外部协议交互 / 双向通信
包含关系  ┌─┐    阶段内架构边界 / 能力归属
```

## B1. Stage 0：Hello Agent Demo

```text
┌──────────────────────────┐
│ User Prompt               │
│ single text input          │
└─────────────┬────────────┘
              │
              ▼
┌──────────────────────────┐
│ Minimal CLI / Demo Runner  │
│ parse prompt               │
│ choose provider config     │
└─────────────┬────────────┘
              │
              ▼
┌──────────────────────────┐       ┌──────────────────────────┐
│ Simple LLM Client          │◀────▶│ LLM Provider              │
│ request / response          │       │ chat completion API       │
└─────────────┬────────────┘       └──────────────────────────┘
              │
              ▼
┌──────────────────────────┐
│ Final Text Output          │
│ print answer               │
└──────────────────────────┘

架构特征：
- 只有一次请求和一次模型响应。
- 没有工具、会话、记忆、上下文治理和观测系统。
- Provider 协议与运行逻辑容易耦合在一起。
```

## B2. Stage 1：Tool-Using Agent

```text
┌──────────────────────────┐
│ User Task                 │
│ ask agent to inspect/change│
└─────────────┬────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Agent Loop                                                    │
│                                                              │
│ Prompt ──▶ LLM Generate ──▶ Response Parse ──▶ Final Text     │
│                         │                                    │
│                         └── tool calls ──▶ Tool Dispatcher    │
│                                                   │          │
│                                                   ▼          │
│                         Tool Result Context ◀────┘           │
└─────────────────────────┬────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────┐
│ Initial Tool Surface                                          │
│                                                              │
│ File Read │ File Write/Edit │ Shell Command                   │
└─────────────┬────────────────────────────────────────────────┘
              │
              ▼
┌──────────────────────────┐
│ Project Workspace         │
│ files / command effects    │
└──────────────────────────┘

架构特征：
- 从单轮问答演进为“模型决定是否调用工具”的循环。
- 工具开始引入本地副作用，需要统一调度入口。
- 仍缺少稳定的权限边界、会话状态和可审计记录。
```

## B3. Stage 2：持久会话与工程上下文

```text
┌──────────────────────────┐
│ User Request              │
│ continue a work session    │
└─────────────┬────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Session-aware Runner                                          │
│                                                              │
│ Select/Create Session ──▶ Restore Working State ──▶ Start Run │
│          │                         │                         │
│          │ - -▶ message log         │ - -▶ PLAN / TODO         │
└──────────┬─────────────────────────┬─────────────────────────┘
           │                         │
           ▼                         ▼
┌──────────────────────────┐    ┌──────────────────────────────┐
│ Project Instructions      │    │ Context Composer              │
│ repo guidance / rules      │──▶│ prompt + history + state      │
└──────────────────────────┘    └──────────────┬───────────────┘
                                                │
                                                ▼
┌──────────────────────────────────────────────────────────────┐
│ Agent Loop + Tools                                            │
│ multi-turn reasoning / tool execution                         │
└─────────────┬────────────────────────────────────────────────┘
              │ - - persist
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Session Store                                                 │
│ metadata │ message log │ working memory │ artifacts           │
└──────────────────────────────────────────────────────────────┘

架构特征：
- Agent 从一次性执行变成可延续的工程工作会话。
- 项目指令、历史消息和工作状态进入上下文组装。
- 状态生命周期开始分层：当前运行视图与持久 session 记录分离。
```

## B4. Stage 3：交互式 TUI 与开发者工作台

```text
┌──────────────────────────────────────────────────────────────┐
│ Human Operator                                                │
│ reads progress / steers task / answers questions              │
└──────────────────────────┬───────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│ TUI Workbench                                                 │
│                                                              │
│ Conversation View │ Input Bar │ Status │ Tool Events          │
│ Session Browser   │ Settings  │ Ask/Approval UI               │
└─────────────┬──────────────────────────────────┬─────────────┘
              │                                  │
              ▼                                  ▼
┌──────────────────────────┐          ┌────────────────────────┐
│ Interactive Runner        │          │ Reporter / Event Stream │
│ session + runtime deps     │          │ progress / transcript   │
└─────────────┬────────────┘          └────────────▲───────────┘
              │                                    │
              ▼                                    │
┌──────────────────────────────────────────────────┴───────────┐
│ Agent Runtime                                                 │
│ engine loop / tools / provider / session state                │
└──────────────────────────────────────────────────────────────┘

架构特征：
- 用户交互从“命令执行后看结果”演进为持续协作工作台。
- TUI 负责输入、展示、审批和会话导航，不复制 Agent Runtime。
- Reporter/Event Stream 成为运行时和界面之间的稳定桥梁。
```

## B5. Stage 4：可靠性与治理层

```text
┌──────────────────────────────────────────────────────────────┐
│ Agent Runtime                                                 │
│ engine loop / provider / tool registry                        │
└─────────────┬────────────────────────────────────────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Governance Boundary                                           │
│                                                              │
│ Context Estimate ──▶ Optional Compaction                      │
│ Tool Middleware ───▶ Workdir / Allowed-tools / Ask Gate       │
│ Checkpoint ────────▶ protect file changes                     │
│ Error Recovery ────▶ reminders / retry signals                │
└─────────────┬────────────────────────────────────┬───────────┘
              │                                    │
              ▼                                    ▼
┌──────────────────────────┐          ┌────────────────────────┐
│ Project Workspace         │          │ Runtime State           │
│ guarded side effects       │          │ TODO / session / memory │
└──────────────────────────┘          └────────────────────────┘
              │                                    │
              │ - - facts / events                 │ - - snapshots
              ▼                                    ▼
┌──────────────────────────────────────────────────────────────┐
│ Observability                                                 │
│ transcript │ metrics │ trace │ persisted tool results         │
└──────────────────────────────────────────────────────────────┘

架构特征：
- 系统开始把“能完成任务”提升为“长任务可控、可恢复、可诊断”。
- 上下文治理、工具治理、checkpoint 和观测成为共享基础设施。
- 副作用和运行事实都通过稳定边界记录，而不是散落在入口代码中。
```

## B6. Stage 5：扩展生态与多入口集成

```text
┌──────────────────────────────────────────────────────────────┐
│ Entry Points                                                  │
│                                                              │
│ TUI │ CLI one-shot │ Config │ Feishu │ AgentOps │ Benchmark   │
└─────────────┬────────────────────────────────────────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Shared Application Assembly                                   │
│                                                              │
│ Runner │ Reporter │ Provider Config │ Session Selection       │
└─────────────┬────────────────────────────────────┬───────────┘
              │                                    │
              ▼                                    ▼
┌────────────────────────────────────┐    ┌────────────────────┐
│ Extension System                    │    │ Specialized Runner  │
│ slash commands / skills / fork mode │    │ service / eval flow │
│ allowed-tools / conditional activate│    └─────────┬──────────┘
└─────────────┬──────────────────────┘              │
              │                                     │
              ▼                                     ▼
┌──────────────────────────────────────────────────────────────┐
│ Shared Agent Runtime                                          │
│ engine loop / tools / provider adapter / state systems        │
└─────────────┬────────────────────────────────────────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────┐
│ External Systems                                              │
│ model providers │ workspace │ platform APIs │ eval datasets   │
└──────────────────────────────────────────────────────────────┘

架构特征：
- 系统从单一产品形态演进为多入口共享能力平台。
- Slash/skill 扩展把可复用流程文件化，并通过统一 runtime 执行。
- 服务入口和评测入口通过适配层接入，避免产生平行运行时。
```

## B7. Stage 6：CodexSpec + Autodev 自动化开发流水线

```text
┌──────────────────────────┐
│ Backlog / Issue Source    │
│ requested work items       │
└─────────────┬────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Deterministic Go Control Plane                                │
│                                                              │
│ Select Item ──▶ Prepare Worktree ──▶ Drive CodexSpec Stages   │
│       │                 │                    │                │
│       │                 │                    ├── specify       │
│       │                 │                    ├── plan/tasks    │
│       │                 │                    └── implement     │
│       │                 │                                     │
│       └ - - ledger      └ - - git state                       │
│                                                              │
│ Verify Ground Truth ──▶ Commit / Push / PR / Issue Update     │
└─────────────┬────────────────────────────────────┬───────────┘
              │                                    │
              ▼                                    ▼
┌────────────────────────────────────┐    ┌────────────────────┐
│ LLM Execution Plane                 │    │ Remote Boundaries   │
│                                    │    │ Git / GitHub CLI    │
│ Core Agent in isolated worktree     │    │ issue / PR / push   │
│ Engineer Agent for clarification    │    └────────────────────┘
└─────────────┬──────────────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Shared Agent Runtime + Project Workspace                      │
│ tools / provider / session / memory / checks                  │
└──────────────────────────────────────────────────────────────┘

架构特征：
- 自动化开发不是把交互式 Agent 简单循环调用，而是新增确定性控制平面。
- CodexSpec 把需求、计划、任务和实现阶段显式化，形成可追溯流水线。
- Go 控制平面验证磁盘、Git 和远端事实；LLM 执行平面负责具体开发操作。
```
