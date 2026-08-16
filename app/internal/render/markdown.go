// Package render turns a filtered model.Event stream into output formats.
// It must have zero GUI dependencies and stay fully testable via `go test`
// (PLAN.md layering rule 1).
package render

import (
	"fmt"
	"strings"
	"time"

	"ccContextRead/internal/model"
)

const timeLayout = "2006-01-02 15:04:05"

// Header carries the session-level metadata shown at the top of the
// rendered document (PLAN.md 4.3/T1.7): project name, session id,
// start/end time, model, and Claude Code version.
type Header struct {
	ProjectName string
	SessionID   string
	StartedAt   time.Time
	EndedAt     time.Time
	Model       string
	CCVersion   string
}

// ImageMode controls how a model.ImageRef is rendered (PLAN.md 4.3).
type ImageMode int

const (
	ImagePlaceholder ImageMode = iota
	ImageAttachment
)

// Options are the render-time settings from PLAN.md 4.3 that decide how
// (not whether) already-filtered content is shown: image handling, and
// the level of detail for file changes.
type Options struct {
	ImageMode  ImageMode
	FileChange model.FileChangeMode
}

// RenderMarkdown renders a filtered event stream into a single Markdown
// document. It returns an error only for a malformed Options value (e.g.
// an out-of-range ImageMode) — the function otherwise never fails, since
// unknown/future Event kinds degrade to a generic rendering rather than
// asserting a closed set (PLAN.md 2.2's parsing philosophy applies to
// rendering too).
func RenderMarkdown(h Header, events []model.Event, opts Options) (string, error) {
	if opts.ImageMode != ImagePlaceholder && opts.ImageMode != ImageAttachment {
		return "", fmt.Errorf("render: unknown ImageMode %d", opts.ImageMode)
	}

	var b strings.Builder
	writeHeader(&b, h)
	for _, ev := range events {
		writeEvent(&b, ev, opts)
	}
	return b.String(), nil
}

func writeHeader(b *strings.Builder, h Header) {
	fmt.Fprintf(b, "# %s\n\n", h.ProjectName)
	fmt.Fprintf(b, "- **Session ID**: %s\n", h.SessionID)
	fmt.Fprintf(b, "- **Started**: %s\n", formatTime(h.StartedAt))
	fmt.Fprintf(b, "- **Ended**: %s\n", formatTime(h.EndedAt))
	fmt.Fprintf(b, "- **Model**: %s\n", h.Model)
	fmt.Fprintf(b, "- **Claude Code Version**: %s\n", h.CCVersion)
	b.WriteString("\n---\n\n")
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format(timeLayout)
}

func writeEvent(b *strings.Builder, ev model.Event, opts Options) {
	switch ev.Kind {
	case model.EventUserPrompt:
		fmt.Fprintf(b, "## 用户\n\n%s\n\n", ev.Text)
	case model.EventAssistantText:
		fmt.Fprintf(b, "## 助手\n\n%s\n\n", ev.Text)
	case model.EventThinking:
		writeDetails(b, "💭 思考", ev.Text, "")
	case model.EventToolUse:
		writeDetails(b, "🔧 工具调用："+ev.ToolName, string(ev.ToolInput), "json")
	case model.EventToolResult:
		writeToolResult(b, ev, opts)
	case model.EventSubagent:
		writeDetails(b, "🤖 子 Agent", ev.Text, "")
	case model.EventContextInjection:
		writeDetails(b, "📎 上下文注入："+ev.Role, ev.Text, "")
	case model.EventSystemNote:
		writeSystemNote(b, ev)
	case model.EventFileChange:
		writeFileChange(b, ev, opts.FileChange)
	case model.EventCompactSummary:
		writeDetails(b, "📄 压缩摘要", ev.Text, "")
	case model.EventCompactBoundary:
		writeCompactBoundary(b, ev)
	case model.EventTitle:
		// Metadata for the document header, not a body-timeline item; the
		// filter already excludes it, but degrade harmlessly if it ever
		// reaches here directly.
	default:
		// Unknown/future kind: never silently drop non-empty text
		// (PLAN.md 2.2's "observed values are a lower bound" applies here).
		if ev.Text != "" {
			fmt.Fprintf(b, "*%s*\n\n", ev.Text)
		}
	}
}

func writeSystemNote(b *strings.Builder, ev model.Event) {
	if ev.Text == "" {
		return
	}
	fmt.Fprintf(b, "*%s*\n\n", ev.Text)
}

func writeDetails(b *strings.Builder, summary, body, lang string) {
	fmt.Fprintf(b, "<details>\n<summary>%s</summary>\n\n", summary)
	if body != "" {
		writeFenced(b, body, lang)
	}
	b.WriteString("\n</details>\n\n")
}

func writeToolResult(b *strings.Builder, ev model.Event, opts Options) {
	var body strings.Builder
	body.WriteString(ev.Text)
	for i, img := range ev.Images {
		if body.Len() > 0 {
			body.WriteString("\n")
		}
		body.WriteString(renderImage(img, i, opts.ImageMode))
	}
	writeDetails(b, "↩️ 工具结果", body.String(), "")
}

func renderImage(img model.ImageRef, index int, mode ImageMode) string {
	if mode == ImageAttachment {
		return fmt.Sprintf("![image](attachments/image-%d.%s)", index+1, extFromMediaType(img.MediaType))
	}
	return fmt.Sprintf("*[图片: %s]*", img.MediaType)
}

func extFromMediaType(mt string) string {
	switch mt {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return "bin"
	}
}

func writeFileChange(b *strings.Builder, ev model.Event, mode model.FileChangeMode) {
	if mode == model.FileChangeNone || len(ev.FileOps) == 0 {
		return
	}
	for _, op := range ev.FileOps {
		fmt.Fprintf(b, "%s %s %s\n", fileChangeEmoji(op.Op), fileChangeVerb(op.Op), op.Path)
	}
	if mode == model.FileChangeFull && ev.Text != "" {
		b.WriteString("\n")
		writeFenced(b, ev.Text, "diff")
	}
	b.WriteString("\n")
}

func fileChangeEmoji(op string) string {
	switch op {
	case "create":
		return "➕"
	case "delete":
		return "🗑️"
	default:
		return "📝"
	}
}

func fileChangeVerb(op string) string {
	switch op {
	case "create":
		return "新建"
	case "delete":
		return "删除"
	case "edit":
		return "修改"
	default:
		return op
	}
}

func writeCompactBoundary(b *strings.Builder, ev model.Event) {
	if ev.CompactMeta == nil {
		b.WriteString("─── 上下文已压缩 ───\n\n")
		return
	}
	cm := ev.CompactMeta
	fmt.Fprintf(b, "─── 上下文已压缩：%d → %d tokens，耗时 %.1fs ───\n\n",
		cm.PreTokens, cm.PostTokens, float64(cm.DurationMs)/1000)
}

// writeFenced wraps content in a code fence long enough to not collide
// with any backtick run already inside content (T1.7: "代码块围栏冲突处理").
func writeFenced(b *strings.Builder, content, lang string) {
	fence := fenceFor(content)
	fmt.Fprintf(b, "%s%s\n%s\n%s\n", fence, lang, content, fence)
}

// fenceFor returns a backtick fence at least one character longer than the
// longest run of consecutive backticks in s, with a floor of three.
func fenceFor(s string) string {
	maxRun, cur := 0, 0
	for _, r := range s {
		if r == '`' {
			cur++
			if cur > maxRun {
				maxRun = cur
			}
		} else {
			cur = 0
		}
	}
	n := max(maxRun+1, 3)
	return strings.Repeat("`", n)
}
