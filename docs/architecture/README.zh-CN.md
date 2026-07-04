# 架构文档维护说明

本目录保存 foxharness 的架构说明、draw.io 源文件和导出图片。目标读者是项目维护者和贡献者。

## 目录结构

```text
docs/architecture/
├── README.zh-CN.md
├── current-architecture.zh-CN.md
├── drawio/
│   └── current-architecture.zh-CN.drawio
└── images/
    └── *.png
```

## 维护流程

1. 更新 `drawio/` 下的 `.drawio` 源文件。
2. 运行脚本导出 Markdown 可直接渲染的 PNG 图片：

   ```bash
   scripts/export-architecture-diagrams.sh
   ```

3. 在架构 Markdown 中通过相对路径引用导出的 PNG，并补充面向维护者阅读的图文说明。

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
