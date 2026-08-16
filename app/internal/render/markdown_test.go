package render

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ccContextRead/internal/model"
)

var update = flag.Bool("update", false, "update golden files in testdata/golden")

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

func TestRenderMarkdown_Golden(t *testing.T) {
	fixedHeader := Header{
		ProjectName: "ccContextRead",
		SessionID:   "11111111-1111-4111-8111-111111111111",
		StartedAt:   time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC),
		EndedAt:     time.Date(2026, 8, 10, 9, 3, 50, 0, time.UTC),
		Model:       "claude-sonnet-5",
		CCVersion:   "2.1.223",
	}

	tests := []struct {
		name   string
		header Header
		events []model.Event
		opts   Options
	}{
		{
			name:   "header_and_basic_turn",
			header: fixedHeader,
			events: []model.Event{
				{Kind: model.EventUserPrompt, Text: "帮我修一下这个 bug"},
				{Kind: model.EventAssistantText, Text: "已经修好了，问题在第 12 行。"},
			},
		},
		{
			name:   "code_fence_conflict",
			header: fixedHeader,
			events: []model.Event{
				{Kind: model.EventToolUse, ToolName: "Write", ToolInput: mustJSON(t, map[string]string{
					"snippet": "```python\nprint(1)\n```",
				})},
			},
		},
		{
			name:   "tool_result_image_placeholder",
			header: fixedHeader,
			events: []model.Event{
				{Kind: model.EventToolResult, Text: "截图如下", Images: []model.ImageRef{
					{MediaType: "image/png", Data: "base64=="},
				}},
			},
			opts: Options{ImageMode: ImagePlaceholder},
		},
		{
			name:   "tool_result_image_attachment",
			header: fixedHeader,
			events: []model.Event{
				{Kind: model.EventToolResult, Text: "截图如下", Images: []model.ImageRef{
					{MediaType: "image/png", Data: "base64=="},
				}},
			},
			opts: Options{ImageMode: ImageAttachment},
		},
		{
			name:   "file_change_summary",
			header: fixedHeader,
			events: []model.Event{
				{Kind: model.EventFileChange, FileOps: []model.FileOp{
					{Path: "src/main.go", Op: "edit"},
					{Path: "src/new.go", Op: "create"},
				}},
			},
			opts: Options{FileChange: model.FileChangeSummary},
		},
		{
			name:   "file_change_full",
			header: fixedHeader,
			events: []model.Event{
				{Kind: model.EventFileChange, Text: "-old line\n+new line", FileOps: []model.FileOp{
					{Path: "src/main.go", Op: "edit"},
				}},
			},
			opts: Options{FileChange: model.FileChangeFull},
		},
		{
			name:   "compact_boundary",
			header: fixedHeader,
			events: []model.Event{
				{Kind: model.EventCompactBoundary, CompactMeta: &model.CompactMeta{
					PreTokens: 87325, PostTokens: 13963, DurationMs: 86480,
				}},
			},
		},
		{
			name:   "empty_session",
			header: fixedHeader,
			events: nil,
		},
		{
			name:   "user_only_no_reply",
			header: fixedHeader,
			events: []model.Event{
				{Kind: model.EventUserPrompt, Text: "有人在吗？"},
			},
		},
		{
			name:   "system_note_title_and_unknown_kind",
			header: Header{ProjectName: "x"}, // zero-value StartedAt/EndedAt
			events: []model.Event{
				{Kind: model.EventSystemNote, Text: "会话模式已切换"},
				{Kind: model.EventSystemNote, Text: ""}, // empty note must not emit a blank paragraph
				{Kind: model.EventTitle, Text: "should never render"},
				{Kind: model.EventKind(999), Text: "未知类型仍需降级展示"},
				{Kind: model.EventKind(999), Text: ""}, // empty unknown kind emits nothing
				{Kind: model.EventCompactBoundary},     // no CompactMeta: generic fallback line
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderMarkdown(tt.header, tt.events, tt.opts)
			if err != nil {
				t.Fatalf("RenderMarkdown() error = %v", err)
			}

			goldenPath := filepath.Join("testdata", "golden", tt.name+".md")
			if *update {
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("ReadFile(%q): %v (run with -update to generate)", goldenPath, err)
			}
			if got != string(want) {
				t.Errorf("RenderMarkdown() mismatch for %q\n--- got ---\n%s\n--- want ---\n%s", tt.name, got, want)
			}
		})
	}
}

func TestRenderMarkdown_InvalidImageMode_ReturnsError(t *testing.T) {
	_, err := RenderMarkdown(Header{}, nil, Options{ImageMode: ImageMode(99)})
	if err == nil {
		t.Fatal("RenderMarkdown() error = nil, want error for unknown ImageMode")
	}
}

func TestFormatTime(t *testing.T) {
	if got := formatTime(time.Time{}); got != "—" {
		t.Errorf("formatTime(zero) = %q, want %q", got, "—")
	}
	tm := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	if got := formatTime(tm); got != "2026-08-10 09:00:00" {
		t.Errorf("formatTime(tm) = %q, want %q", got, "2026-08-10 09:00:00")
	}
}

func TestExtFromMediaType(t *testing.T) {
	tests := []struct{ mt, want string }{
		{"image/png", "png"},
		{"image/jpeg", "jpg"},
		{"image/gif", "gif"},
		{"image/webp", "webp"},
		{"application/octet-stream", "bin"},
	}
	for _, tt := range tests {
		if got := extFromMediaType(tt.mt); got != tt.want {
			t.Errorf("extFromMediaType(%q) = %q, want %q", tt.mt, got, tt.want)
		}
	}
}

func TestFileChangeEmojiAndVerb(t *testing.T) {
	tests := []struct{ op, wantEmoji, wantVerb string }{
		{"create", "➕", "新建"},
		{"edit", "📝", "修改"},
		{"delete", "🗑️", "删除"},
		{"rename", "📝", "rename"},
	}
	for _, tt := range tests {
		if got := fileChangeEmoji(tt.op); got != tt.wantEmoji {
			t.Errorf("fileChangeEmoji(%q) = %q, want %q", tt.op, got, tt.wantEmoji)
		}
		if got := fileChangeVerb(tt.op); got != tt.wantVerb {
			t.Errorf("fileChangeVerb(%q) = %q, want %q", tt.op, got, tt.wantVerb)
		}
	}
}

func TestFenceFor(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no backticks", "plain text", "```"},
		{"single run of one", "a `b` c", "```"},
		{"run of three forces four", "```python\nprint(1)\n```", "````"},
		{"run of five forces six", "`````", "``````"},
		{"empty string", "", "```"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fenceFor(tt.in); got != tt.want {
				t.Errorf("fenceFor(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
