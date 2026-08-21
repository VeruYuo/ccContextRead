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

## Status

- ✅ **M1 complete**: session discovery & enumeration, real-time incremental rendering, content filtering, GUI shell, theme switching, Mermaid + syntax highlighting.
- 🚧 **M2 planned**: export to PDF / DOCX / HTML.
- 🚧 **M3 planned**: Trajectory view — an append-only event stream for inspecting every detail of a session (chain of thought, tool calls and results, sub-agent dispatch, context injection, token usage and timing).

## Known limitations

Stated plainly, without overselling:

- **No raw system prompt.** We can capture incremental injections in attachment form (skill lists, MCP descriptions, task reminders, etc.), but the base system prompt itself is never written to the JSONL log.
- **No per-chunk streaming frames**, so time-to-first-token (TTFT) cannot be computed; the timeline can only go down to per-turn duration.
- **No request-level HTTP headers / schema** available for inspection.
- Verified and distributed for Windows x64 only. The code stays portable, but cross-platform support isn't a goal.

## Data safety

- `~/.claude/` is treated as **read-only at all times** — this program never modifies, deletes, or moves anything in it.
- The only directory it ever writes to is the output directory you configure in settings.
- No network access, no cloud sync, no account system (PDF rendering, when you export, happens entirely locally too).

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
