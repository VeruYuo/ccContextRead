package claude

import "testing"

func TestExtractText(t *testing.T) {
	tests := []struct {
		name    string
		content any
		want    string
	}{
		{
			name:    "string content",
			content: "hello world",
			want:    "hello world",
		},
		{
			name: "text block array",
			content: []any{
				map[string]any{"type": "text", "text": "first"},
				map[string]any{"type": "text", "text": "second"},
			},
			want: "first\nsecond",
		},
		{
			name: "image-only block does not leak base64 into the result",
			content: []any{
				map[string]any{
					"type": "image",
					"source": map[string]any{
						"type":       "base64",
						"media_type": "image/png",
						"data":       "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
					},
				},
			},
			want: "",
		},
		{
			name: "unknown block type passes through degraded, no panic",
			content: []any{
				map[string]any{"type": "some_future_block", "weird": 123},
			},
			want: "",
		},
		{
			name: "mixed known and unknown blocks",
			content: []any{
				map[string]any{"type": "text", "text": "keep me"},
				map[string]any{"type": "some_future_block", "weird": 123},
			},
			want: "keep me",
		},
		{
			name:    "empty array",
			content: []any{},
			want:    "",
		},
		{
			name:    "nil content",
			content: nil,
			want:    "",
		},
		{
			name:    "single block object (not wrapped in array)",
			content: map[string]any{"type": "text", "text": "solo block"},
			want:    "solo block",
		},
		{
			name:    "unsupported scalar type",
			content: 42,
			want:    "",
		},
		{
			name: "multi-byte chinese text preserved verbatim",
			content: []any{
				map[string]any{"type": "text", "text": "你好，世界"},
			},
			want: "你好，世界",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractText(tt.content)
			if got != tt.want {
				t.Errorf("extractText(%#v) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

func TestExtractTextFromItem(t *testing.T) {
	tests := []struct {
		name string
		item any
		want string
	}{
		{
			name: "tool_use with name",
			item: map[string]any{"type": "tool_use", "name": "Read", "input": map[string]any{}},
			want: "[Tool: Read]",
		},
		{
			name: "tool_use with empty name falls back to unknown",
			item: map[string]any{"type": "tool_use", "name": ""},
			want: "[Tool: unknown]",
		},
		{
			name: "tool_use with missing name field falls back to unknown",
			item: map[string]any{"type": "tool_use"},
			want: "[Tool: unknown]",
		},
		{
			name: "tool_result with string content recurses",
			item: map[string]any{"type": "tool_result", "content": "the result text"},
			want: "the result text",
		},
		{
			name: "tool_result with block array content recurses",
			item: map[string]any{
				"type": "tool_result",
				"content": []any{
					map[string]any{"type": "text", "text": "nested"},
				},
			},
			want: "nested",
		},
		{
			name: "tool_result with only image content yields empty, no base64 leak",
			item: map[string]any{
				"type": "tool_result",
				"content": []any{
					map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "deadbeef"}},
				},
			},
			want: "",
		},
		{
			name: "plain text field",
			item: map[string]any{"type": "text", "text": "plain"},
			want: "plain",
		},
		{
			name: "unknown type with nested content field falls back to extractText",
			item: map[string]any{"type": "some_future_block", "content": "buried text"},
			want: "buried text",
		},
		{
			name: "unknown type with neither text nor content yields empty",
			item: map[string]any{"type": "some_future_block", "weird": 123},
			want: "",
		},
		{
			name: "item is not a map",
			item: "just a string",
			want: "",
		},
		{
			name: "item is nil",
			item: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextFromItem(tt.item)
			if got != tt.want {
				t.Errorf("extractTextFromItem(%#v) = %q, want %q", tt.item, got, tt.want)
			}
		})
	}
}

func TestIsRealUserRecord(t *testing.T) {
	tests := []struct {
		name string
		v    map[string]any
		want bool
	}{
		{
			name: "type is user",
			v:    map[string]any{"type": "user"},
			want: true,
		},
		{
			name: "type is not user but message.role is user",
			v:    map[string]any{"type": "custom-title", "message": map[string]any{"role": "user"}},
			want: true,
		},
		{
			name: "type is not user and message.role is not user",
			v:    map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant"}},
			want: false,
		},
		{
			name: "no type and no message field at all",
			v:    map[string]any{"timestamp": "2026-01-01T00:00:00Z"},
			want: false,
		},
		{
			name: "message field present but not a map",
			v:    map[string]any{"message": "not a map"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRealUserRecord(tt.v)
			if got != tt.want {
				t.Errorf("isRealUserRecord(%#v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}
