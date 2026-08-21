# ccContextRead

English | [简体中文](README.md)

A small Windows desktop tool that renders local [Claude Code](https://claude.com/product/claude-code) session logs (JSONL) into readable Markdown in real time — read-only, zero external dependencies.

## What it does

Claude Code writes every conversation to `~/.claude/projects/**/*.jsonl` in a machine-oriented format that isn't meant for humans to read directly. ccContextRead:

1. Scans and lists every Claude Code session on your machine, and detects which ones are currently active;
2. Once you pick a session, renders it to Markdown and keeps it updated incrementally as the conversation progresses (within 3 seconds of a new turn);
3. Lets you control content granularity via checkboxes — by default only "user input + assistant's final reply" is kept, with optional detail like tool calls, sub-agents, and context injections;
4. Supports light / dark / follow-system themes; Mermaid code blocks are rendered as diagrams, gracefully degrading to a code block on render failure without breaking the rest of the page.

Single exe, double-click to run — no need to install Go / Node / Pandoc on the target machine.

## Download

Grab the latest `ccContextRead.exe` from the [Releases](../../releases) page and double-click to run (requires the [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/), which ships with Windows 10/11 in most cases).

## Building from source

Requirements: Go 1.25+, Node 18+, [Wails v2](https://wails.io/).

```bash
# Dev mode (hot reload)
cd app && wails dev

# Build a single exe
cd app && wails build -platform windows/amd64
```

Backend tests:

```bash
cd app && go test ./... -race -cover
```

Frontend tests and type checking:

```bash
cd app/frontend && npm run test && npm run build
```

## Tech stack

- Backend: Go + Wails v2 (`internal/claude` parsing, `internal/model` normalization, `internal/render` rendering — none of the three import Wails, so they're fully testable with plain `go test`)
- Frontend: React + TypeScript, Markdown rendering via `marked` + `dompurify`, diagrams via `mermaid`, syntax highlighting via `highlight.js`

## License

[MIT](LICENSE)
