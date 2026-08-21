# ccContextRead

[English](README.en.md) | 简体中文

一个 Windows 桌面小工具：把本机 [Claude Code](https://claude.com/product/claude-code) 的会话记录（JSONL）实时渲染成可读的 Markdown 文件，全程只读、零外部依赖。

## 这是什么

Claude Code 把每次对话写在 `~/.claude/projects/**/*.jsonl` 里，格式是给机器读的、不适合人直接看。ccContextRead 会：

1. 扫描并列出本机所有 Claude Code 会话，识别出哪些正在运行；
2. 你选定一个会话后，实时把它渲染成 Markdown，会话每往前推进一轮，文件就跟着增量更新（3 秒内）；
3. 通过勾选项精确控制内容粒度——默认只保留「用户输入 + 助手最终回复」，可按需打开工具调用、子 Agent、上下文注入等细节；
4. 支持浅色 / 深色 / 跟随系统三种主题，Mermaid 代码块会被渲染成图（渲染失败会优雅降级为代码块，不影响整页）。

单个 exe，双击即用，不需要额外安装 Go / Node / Pandoc 环境。

## 功能状态

- ✅ **M1 已完成**：会话发现与枚举、实时增量渲染、内容过滤、GUI 外壳、主题切换、Mermaid + 代码高亮。
- 🚧 **M2 规划中**：导出为 PDF / DOCX / HTML。
- 🚧 **M3 规划中**：Trajectory 视图——以只追加事件流的形式检查会话全部细节（思维链、工具调用与结果、子 Agent 调度、上下文注入、token 用量与计时）。

## 已知局限

如实告知，不做夸大：

- **拿不到系统提示词原文**。能拿到 attachment 形式的增量注入（skill 清单、MCP 说明、任务提醒等），但拿不到基础 system prompt——Claude Code 的 JSONL 日志本身不落这部分内容。
- **没有逐 chunk 的流式帧**，因此算不出首字延迟（TTFT），时间轴只能做到「按 turn 耗时」的粒度。
- **没有请求级 HTTP 头 / schema** 可供检查。
- 只在 Windows x64 上验证和分发；代码保持可移植，但不追求跨平台。

## 数据安全

- `~/.claude/` 目录**全程只读**，本程序不会修改、删除、移动其中任何文件。
- 唯一的写入目标是你在设置里指定的输出目录。
- 不联网、不做云同步、不做账号体系（导出 PDF 时的渲染也完全在本机完成）。

## 下载使用

前往 [Releases](../../releases) 页面下载最新的 `ccContextRead.exe`，双击运行即可（需要系统已安装 [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)，Windows 10/11 通常已自带）。

## 从源码构建

依赖：Go 1.25+、Node 18+、[Wails v2](https://wails.io/)。

```bash
# 开发模式（热重载）
cd app && wails dev

# 构建单文件 exe
cd app && wails build -platform windows/amd64
```

后端测试：

```bash
cd app && go test ./... -race -cover
```

前端测试与类型检查：

```bash
cd app/frontend && npm run test && npm run build
```

## 技术栈

- 后端：Go + Wails v2（`internal/claude` 解析、`internal/model` 归一化、`internal/render` 渲染，三层不依赖 Wails，可纯 `go test`）
- 前端：React + TypeScript，Markdown 渲染用 `marked` + `dompurify`，图表用 `mermaid`，代码高亮用 `highlight.js`

## 许可证

[MIT](LICENSE)
