# 当前架构图 ASCII 草案

本文用于在绘制 draw.io 前快速确认架构图内容和版式。它不是正式架构说明文档，不应被正式文档依赖。

## 图例

```text
实线箭头  ──▶   主要请求 / 控制流
虚线箭头  - -▶   状态读写 / 旁路观测 / 辅助输入
双向箭头  ◀─▶   外部协议交互 / 双向通信
包含关系  ┌─┐    架构分层 / 能力归属
```

## A2. 核心 Agent 运行链路

```text
┌──────────────────┐
│ 用户请求           │
│ prompt / command  │
└────────┬─────────┘
         │
         ▼
┌──────────────────────────────────────────────────────────────┐
│ Runner：一次运行的装配边界                                    │
│                                                              │
│  选择/创建 Session ──▶ 解析 LLM 配置 ──▶ 构造运行依赖           │
│          │                         │                         │
│          │ - -▶ message log         │ - -▶ provider profile   │
│          │ - -▶ working state       │ - -▶ slash/skill list   │
└─────────┬────────────────────────────────────────────────────┘
          │
          ▼
┌──────────────────────────────────────────────────────────────┐
│ Prompt / Context Composer                                    │
│                                                              │
│ Project Instructions │ Session Memory │ AutoMemory Index      │
│ Recent Messages      │ Skill Summary  │ Interactive Hints     │
└─────────┬────────────────────────────────────────────────────┘
          │
          ▼
┌──────────────────────────────────────────────────────────────┐
│ Engine Loop：多轮 Agent 执行                                  │
│                                                              │
│ Context Estimate ──▶ Optional Compaction ──▶ LLM Generate     │
│        ▲                                      │               │
│        │                                      ▼               │
│        └ - - - Tool Result Context ◀── Response Parse         │
│                                               │               │
│                                no tools ──────┴──▶ Final Text  │
│                                               │               │
│                                          tool calls            │
└───────────────────────────────────────────────┬──────────────┘
                                                │
                                                ▼
┌──────────────────────────┐       ┌──────────────────────────┐
│ Provider Adapter          │◀────▶│ LLM Providers             │
│ internal schema boundary  │       │ OpenAI / Claude-like      │
└──────────────────────────┘       └──────────────────────────┘

┌──────────────────────────┐
│ Tool Registry             │
│ tool definitions / dispatch│
└─────────────┬────────────┘
              │
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Tool Execution                                                │
│ read / write / edit / bash / TODO / skill / subagent / ask    │
└─────────────┬────────────────────────────────────────────────┘
              │
              │ - - observation / persistence
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Run Records                                                   │
│ message log │ transcript │ metrics │ trace │ persisted results │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────┐
│ Post-run AutoMemory       │
│ async extraction           │
└──────────────────────────┘
```

## A3. 工具与安全边界

```text
┌──────────────────────────────────────────────────────────────┐
│ Engine Loop                                                   │
│                                                              │
│ 模型响应 ──▶ 工具调用请求 ──▶ Tool Registry                   │
└──────────────────────────────────┬───────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────┐
│ Tool Registry：能力目录与执行入口                              │
│                                                              │
│  Tool Definitions  │ Alias Resolution │ Dispatch              │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ Control Boundary                                       │  │
│  │                                                        │  │
│  │ Middleware Chain ──▶ Checkpoint Guard                  │  │
│  │        │                                               │  │
│  │        ├ - -▶ Workdir Boundary                         │  │
│  │        ├ - -▶ Allowed-tools Filter                     │  │
│  │        └ - -▶ Interactive Ask Gate                     │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ Capability Surface                                     │  │
│  │                                                        │  │
│  │ File Tools: read / write / edit                        │  │
│  │ Shell Tool: bash                                       │  │
│  │ Session Tools: TODO read / update                      │  │
│  │ Extension Tools: skill invocation                      │  │
│  │ Delegation Tools: subagent                             │  │
│  │ Human Interaction: ask_user_question                   │  │
│  └────────────────────────────────────────────────────────┘  │
└─────────────┬─────────────────────────────┬──────────────────┘
              │                             │
              ▼                             ▼
┌──────────────────────────┐       ┌──────────────────────────┐
│ Project Workspace         │       │ Session Runtime State     │
│ files / shell side effects│       │ TODO / checkpoints        │
└──────────────────────────┘       └──────────────────────────┘
              │
              │ - - tool result / error / events
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Feedback to Runtime                                           │
│ Tool Result ──▶ Engine Context                                │
│ Errors - -▶ Recovery Signal                                   │
│ Patterns - -▶ Reminder Signal                                 │
│ Events  - -▶ Transcript / Trace                               │
└──────────────────────────────────────────────────────────────┘
```

## A4. 上下文、会话与记忆体系

```text
┌──────────────────────────────────────────────────────────────┐
│ Current Run Context：当前模型调用可见视图                      │
│                                                              │
│ User Prompt │ Project Instructions │ Recent Messages          │
│ Tool Definitions │ Skill Summary │ Interactive Hints          │
│ Working Memory Snapshot │ AutoMemory Index                    │
└──────────────────────────┬───────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│ Context Governance                                            │
│                                                              │
│ Context Estimate ──▶ Optional Compaction ──▶ Compact State    │
│        │                                                     │
│        └ - -▶ Token / Context Window Observation              │
└──────────────────────────┬───────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│ Session State：连续会话的工作记录                              │
│                                                              │
│ Session Metadata │ Message Log │ Working Memory               │
│ PLAN / TODO      │ Run Artifacts │ Checkpoints                 │
└─────────────┬────────────────────────────────────────────────┘
              │ - - persisted under session scope
              ▼
┌──────────────────────────────────────────────────────────────┐
│ Run Observability                                             │
│ Transcript │ Metrics │ Trace │ Tool Result References         │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ Cross-session Memory：跨运行长期知识                           │
│                                                              │
│ User-global Memory │ Project Memory │ Feedback / Reference     │
│ Memory Index                                                  │
└─────────────▲──────────────────────────────────┬─────────────┘
              │                                  │
              │ - - post-run extraction          │ - - injected as index
              │                                  ▼
┌──────────────────────────┐          ┌────────────────────────┐
│ Completed Run Signals     │          │ Prompt / Context       │
│ output / tool usage        │          │ next run assembly      │
└──────────────────────────┘          └────────────────────────┘
```

## A5. 扩展入口与自动化平面

```text
┌──────────────────────────────────────────────────────────────┐
│ Human-facing Entry Points                                     │
│                                                              │
│ TUI ───────────────┐                                         │
│ CLI one-shot ──────┼────▶ Shared Runner ────▶ Agent Runtime   │
│ Config Wizard ─────┘              │                          │
└───────────────────────────────────┼──────────────────────────┘
                                    │
                                    │ - - uses
                                    ▼
┌──────────────────────────────────────────────────────────────┐
│ Extension System                                               │
│                                                              │
│ Slash Commands │ Skills │ Conditional Activation              │
│ Allowed-tools Restrictions │ Fork Mode / Subagent Delegation  │
│                                                              │
│ Registry / Executor - -▶ Skill Tool / Shared Runner           │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ Service / Evaluation Entry Points                             │
│                                                              │
│ Feishu / Lark ───────▶ Adapter / Reporter ───▶ Agent Runtime  │
│ AgentOps ────────────▶ Incident Runner ──────▶ Agent Runtime  │
│ Benchmark ───────────▶ Case Runner ─────────▶ Agent Runtime   │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ Autodev Automation Plane                                      │
│                                                              │
│ Backlog ──▶ Go Control Plane                                  │
│             │                                                │
│             ├──▶ Select Item / Prepare Worktree               │
│             ├──▶ Drive CodexSpec Stages                       │
│             ├──▶ Verify Ground Truth                          │
│             ├──▶ Maintain Ledger                              │
│             └──▶ Coordinate Remote Flow                       │
│                          │                                    │
│                          ▼                                    │
│             LLM Execution Plane                               │
│             │                                                │
│             ├──▶ Core Agent in isolated worktree ──▶ Runner   │
│             └──▶ Engineer Agent - -▶ answers / corrections    │
└─────────────┬──────────────────────────────────┬─────────────┘
              │                                  │
              │ ◀──────── external ───────────▶ │
              ▼                                  ▼
┌──────────────────────────┐          ┌────────────────────────┐
│ Git / Worktree            │          │ GitHub CLI / Remote     │
│ branches / commits        │          │ issue / PR / push       │
└──────────────────────────┘          └────────────────────────┘
```
