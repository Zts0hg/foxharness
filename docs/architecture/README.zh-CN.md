# 架构文档维护说明

本目录保存 foxharness 的架构说明、draw.io 源文件和导出图片。目标读者是项目维护者和贡献者。

## 目录结构

```text
docs/architecture/
├── README.zh-CN.md
├── current-architecture.zh-CN.md
├── evolution/
│   ├── stage-00-core-agent-loop.zh-CN.md
│   ├── stage-01-tool-using-agent.zh-CN.md
│   ├── stage-02-session-context.zh-CN.md
│   ├── stage-03-tui-workbench.zh-CN.md
│   ├── stage-04-reliability-governance.zh-CN.md
│   ├── stage-05-extension-multi-entry.zh-CN.md
│   └── stage-06-codexspec-autodev.zh-CN.md
├── drafts/
│   ├── current-architecture-ascii.zh-CN.md
│   └── evolution-architecture-ascii.zh-CN.md
├── drawio/
│   ├── current-architecture.zh-CN.drawio
│   └── evolution-architecture.zh-CN.drawio
└── images/
    └── *.png
```

正式架构文档是面向维护者和贡献者阅读的成稿。`drafts/` 下的 ASCII 草案只用于绘制 draw.io 前快速确认内容和版式，不作为正式文档的依赖。

## 阅读入口

- [当前架构](current-architecture.zh-CN.md)：最新代码结构的完整说明，也是演化架构的最新阶段。
- [Stage 00：核心 Agent Loop](evolution/stage-00-core-agent-loop.zh-CN.md)：最小 Agent 运行时边界的当前架构说明。
- [Stage 01：Tool-Using Agent](evolution/stage-01-tool-using-agent.zh-CN.md)：工具型 Agent 的当前架构说明。
- [Stage 02：持久会话与工程上下文](evolution/stage-02-session-context.zh-CN.md)：会话、上下文、TODO 和 rewind 体系的当前架构说明。
- [Stage 03：交互式 TUI 与开发者工作台](evolution/stage-03-tui-workbench.zh-CN.md)：TUI、Reporter 和共享 Runner 的当前架构说明。
- [Stage 04：可靠性与治理层](evolution/stage-04-reliability-governance.zh-CN.md)：compaction、tool result persistence、metrics 和 tracing 的当前架构说明。
- [Stage 05：扩展生态与多入口集成](evolution/stage-05-extension-multi-entry.zh-CN.md)：slash/skill、allowed-tools 和多入口共享 Runtime 的当前架构说明。
- [Stage 06：CodexSpec + Autodev 自动化开发流水线](evolution/stage-06-codexspec-autodev.zh-CN.md)：确定性控制平面与 LLM 执行平面的当前架构说明。

## 维护流程

1. 必要时先在 `drafts/` 下更新 ASCII 草案，确认架构图内容和版式。
2. 更新正式架构 Markdown，使文本说明可以独立解释架构。演化架构应按阶段更新 `evolution/` 下的单独文档，不把多个阶段合并成一个正文文档。
3. 更新 `drawio/` 下的 `.drawio` 源文件。
4. 运行脚本导出 Markdown 可直接渲染的 PNG 图片：

   ```bash
   scripts/export-architecture-diagrams.sh
   ```

5. 在架构 Markdown 中通过相对路径引用导出的 PNG。

## 为什么不直接在 Markdown 中嵌入 draw.io

`.drawio` 文件是 diagrams.net/draw.io 的源文件，适合继续编辑，但不是 Markdown 渲染器通用支持的图片格式。为了让 GitHub 和常见 Markdown 预览器稳定显示架构图，文档中引用导出的 PNG，`.drawio` 作为可编辑源文件保留。

## draw.io 可执行文件

导出脚本会查找：

- `DRAWIO_BIN` 指定的可执行文件
- `drawio` 命令
- macOS 常见安装路径 `/Applications/draw.io.app/Contents/MacOS/draw.io`
- macOS 备用安装路径 `/Applications/drawio.app/Contents/MacOS/drawio`

如果未安装 draw.io Desktop，可以安装后重跑脚本，或显式指定：

```bash
DRAWIO_BIN="/Applications/draw.io.app/Contents/MacOS/draw.io" scripts/export-architecture-diagrams.sh
```
