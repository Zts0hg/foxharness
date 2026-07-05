# foxharness 当前架构：交互式 TUI 与开发者工作台

![Stage 03 交互式 TUI 与开发者工作台](../images/evolution-architecture-b4-tui-workbench.zh-CN.png)

本文面向 foxharness 的维护者和贡献者，解释当前交互式终端工作台架构。当前系统提供 one-shot CLI 和 TUI 两种人类入口，但两者共享同一个 AgentRunner 和 Agent Runtime。TUI 负责持续交互体验，Runner 和 Engine 负责实际 Agent 运行。

当前架构的重点是把界面状态、运行事件和 Agent 执行边界分开，让维护者可以独立理解 TUI 和 Runtime。

## 系统边界

当前系统由入口层、AgentRunner、Agent Runtime、Reporter/Event Stream 和 TUI Workbench 组成。

入口层包含一次性 CLI 和交互式 TUI。CLI 适合提交一个请求后等待结果，TUI 适合在同一 session 中持续输入、观察工具事件和阅读运行状态。

AgentRunner 是 CLI 与 TUI 共享的运行装配层。它持有 session、provider、memory store、工具 registry 和运行配置。入口通过 Runner 提交用户请求，而不是直接创建 Engine。

Agent Runtime 负责多轮模型推理、工具调用、上下文组装和 session 写入。Runtime 不关心终端布局，也不持有界面渲染状态。

Reporter/Event Stream 是 Runtime 与 TUI 的桥梁。Engine 在运行过程中发出 run start、thinking、compaction、tool call、tool result、assistant message、complete 和 error 等事件。Reporter 把这些事件转换为 TUI 可以处理的消息。

TUI Workbench 负责界面状态和渲染。它维护输入框、消息列表、运行状态、工具事件展示和视图布局。它展示运行事实，但不替代 session 记录。

## 交互运行链路

用户在 TUI 输入 prompt 后，TUI 将请求交给 Runner。Runner 复用当前 session，构造 Engine，并传入 TUI reporter。

Engine 执行运行时，通过 reporter 持续发送事件。TUI model 接收事件后更新界面状态，view 根据 model 重新渲染终端界面。用户可以在运行之间继续输入，从而在同一 session 内持续协作。

这条链路保持单向职责：TUI 提交请求并展示事件，Runner 装配运行，Engine 产生运行事实，Reporter 传递事实。

## 状态体系

当前系统同时存在三类状态。

Session 状态保存权威运行记录，包括 message log、transcript、TODO 和 memory。它属于 Agent 工作本身。

TUI 状态保存界面呈现，包括当前输入、滚动位置、消息视图、运行提示和临时 UI 标记。它属于交互体验，不应被当作运行事实。

Reporter 事件连接两者。事件从 Runtime 流向 TUI，让界面能实时展示运行进度。事件本身应该来源于 Runtime 的真实行为，而不是 TUI 自行推断。

## 日志与可观察性

TUI 运行时会把日志重定向到 session 目录下的 TUI 日志文件，避免普通 stdout/stderr 干扰终端界面。运行过程中的工具调用、工具结果和模型消息仍通过 reporter 和 session 记录进入可观察链路。

维护者排查交互问题时，应区分 UI 渲染问题、Reporter 事件问题和 Runtime 执行问题。三者的症状可能同时出现在终端里，但责任边界不同。

## 维护原则

维护当前架构时，应优先保护以下边界：

- TUI 负责交互和展示，不复制 Agent Runtime。
- Runner 是入口与 Runtime 的共享装配层。
- Reporter 是 Runtime 到 TUI 的事件边界。
- Session 记录是运行事实，TUI model 是展示状态。
- 新增交互能力应优先扩展 TUI model/view 或 reporter 事件，而不是把 UI 逻辑写入 Engine。

这样可以让 CLI 和 TUI 在体验上不同，但在 Agent 行为上保持一致。
