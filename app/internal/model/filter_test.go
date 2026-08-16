package model

import "testing"

// allKindsSample returns one Event per EventKind, in the fixed EventKind
// declaration order, so a filter config's effect on each kind can be
// checked independently of any particular Normalize output.
func allKindsSample() []Event {
	kinds := []EventKind{
		EventUserPrompt, EventAssistantText, EventThinking, EventToolUse,
		EventToolResult, EventSubagent, EventContextInjection, EventSystemNote,
		EventFileChange, EventTitle, EventCompactSummary, EventCompactBoundary,
	}
	events := make([]Event, len(kinds))
	for i, k := range kinds {
		events[i] = Event{Kind: k}
	}
	return events
}

func kindsOf(events []Event) map[EventKind]bool {
	out := map[EventKind]bool{}
	for _, e := range events {
		out[e.Kind] = true
	}
	return out
}

func TestApply_DefaultConfig_OnlyUserPromptAndAssistantText(t *testing.T) {
	got := Apply(allKindsSample(), DefaultFilterConfig())

	present := kindsOf(got)
	// CompactBoundary is the one always-on exception (4.3); everything else
	// under the default config must be gated by its switch.
	want := map[EventKind]bool{
		EventUserPrompt:      true,
		EventAssistantText:   true,
		EventCompactBoundary: true,
	}
	for k := range present {
		if !want[k] {
			t.Errorf("default config kept unexpected kind %v", k)
		}
	}
	for k := range want {
		if !present[k] {
			t.Errorf("default config dropped expected kind %v", k)
		}
	}
}

func TestApply_CompactBoundaryAlwaysPassesEvenWithAllSwitchesOff(t *testing.T) {
	cfg := FilterConfig{} // every bool zero-value false, FileChange = FileChangeNone
	got := Apply(allKindsSample(), cfg)

	if len(got) != 1 || got[0].Kind != EventCompactBoundary {
		t.Fatalf("Apply() = %+v, want only EventCompactBoundary", got)
	}
}

func TestApply_TitleNeverIncluded(t *testing.T) {
	cfg := FilterConfig{
		UserPrompt: true, AssistantText: true, Thinking: true, ToolUse: true,
		ToolResult: true, Subagent: true, ContextInjection: true, SystemNote: true,
		CompactSummary: true, FileChange: FileChangeFull,
	}
	got := Apply(allKindsSample(), cfg)

	for _, e := range got {
		if e.Kind == EventTitle {
			t.Fatalf("EventTitle must never appear in Apply() output, even with every switch on")
		}
	}
}

func TestApply_EachSwitchIndividually(t *testing.T) {
	tests := []struct {
		name string
		cfg  FilterConfig
		want EventKind
	}{
		{"Thinking", FilterConfig{Thinking: true}, EventThinking},
		{"ToolUse", FilterConfig{ToolUse: true}, EventToolUse},
		{"ToolResult", FilterConfig{ToolResult: true}, EventToolResult},
		{"Subagent", FilterConfig{Subagent: true}, EventSubagent},
		{"ContextInjection", FilterConfig{ContextInjection: true}, EventContextInjection},
		{"SystemNote", FilterConfig{SystemNote: true}, EventSystemNote},
		{"CompactSummary", FilterConfig{CompactSummary: true}, EventCompactSummary},
		{"UserPrompt", FilterConfig{UserPrompt: true}, EventUserPrompt},
		{"AssistantText", FilterConfig{AssistantText: true}, EventAssistantText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply(allKindsSample(), tt.cfg)
			present := kindsOf(got)
			if !present[tt.want] {
				t.Errorf("switch %s on: kind %v missing from output", tt.name, tt.want)
			}
			// Everything else, apart from the always-on compact boundary,
			// must stay excluded.
			for k := range present {
				if k != tt.want && k != EventCompactBoundary {
					t.Errorf("switch %s on: unexpected kind %v also present", tt.name, k)
				}
			}
		})
	}
}

func TestApply_FileChangeModes(t *testing.T) {
	tests := []struct {
		name string
		mode FileChangeMode
		want bool
	}{
		{"None_excluded", FileChangeNone, false},
		{"Summary_included", FileChangeSummary, true},
		{"Full_included", FileChangeFull, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply([]Event{{Kind: EventFileChange}}, FilterConfig{FileChange: tt.mode})
			present := len(got) == 1
			if present != tt.want {
				t.Errorf("FileChange=%v: present = %v, want %v", tt.mode, present, tt.want)
			}
		})
	}
}

func TestApply_TruncatesToolResultAndContextInjectionText(t *testing.T) {
	long := "0123456789abcdef" // 16 runes
	cfg := FilterConfig{ToolResult: true, ContextInjection: true, TruncateChars: 10}

	got := Apply([]Event{
		{Kind: EventToolResult, Text: long},
		{Kind: EventContextInjection, Text: long},
	}, cfg)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	for _, e := range got {
		want := "0123456789…"
		if e.Text != want {
			t.Errorf("Kind %v Text = %q, want %q", e.Kind, e.Text, want)
		}
	}
}

func TestApply_DoesNotTruncateUserPromptOrAssistantText(t *testing.T) {
	long := "0123456789abcdef"
	cfg := FilterConfig{UserPrompt: true, AssistantText: true, TruncateChars: 10}

	got := Apply([]Event{
		{Kind: EventUserPrompt, Text: long},
		{Kind: EventAssistantText, Text: long},
	}, cfg)

	for _, e := range got {
		if e.Text != long {
			t.Errorf("Kind %v Text = %q, want untruncated %q", e.Kind, e.Text, long)
		}
	}
}

func TestApply_TruncateCharsZeroOrNegative_DisablesTruncation(t *testing.T) {
	long := "0123456789abcdef"
	for _, limit := range []int{0, -1} {
		cfg := FilterConfig{ToolResult: true, TruncateChars: limit}
		got := Apply([]Event{{Kind: EventToolResult, Text: long}}, cfg)
		if len(got) != 1 || got[0].Text != long {
			t.Errorf("TruncateChars=%d: got = %+v, want untruncated text", limit, got)
		}
	}
}

func TestApply_ShortTextUnderThreshold_NotTruncated(t *testing.T) {
	cfg := FilterConfig{ToolResult: true, TruncateChars: 2000}
	got := Apply([]Event{{Kind: EventToolResult, Text: "short"}}, cfg)
	if len(got) != 1 || got[0].Text != "short" {
		t.Errorf("got = %+v, want unchanged short text", got)
	}
}

func TestApply_EmptyInput(t *testing.T) {
	got := Apply(nil, DefaultFilterConfig())
	if len(got) != 0 {
		t.Errorf("Apply(nil) = %+v, want empty", got)
	}
}

func TestDefaultFilterConfig_MatchesTaskBriefDefaults(t *testing.T) {
	cfg := DefaultFilterConfig()
	if !cfg.UserPrompt || !cfg.AssistantText {
		t.Errorf("DefaultFilterConfig() = %+v, want UserPrompt and AssistantText both true", cfg)
	}
	if cfg.Thinking || cfg.ToolUse || cfg.ToolResult || cfg.Subagent ||
		cfg.ContextInjection || cfg.SystemNote || cfg.CompactSummary {
		t.Errorf("DefaultFilterConfig() = %+v, want every other switch false", cfg)
	}
	if cfg.FileChange != FileChangeNone {
		t.Errorf("DefaultFilterConfig().FileChange = %v, want FileChangeNone", cfg.FileChange)
	}
	if cfg.TruncateChars != 2000 {
		t.Errorf("DefaultFilterConfig().TruncateChars = %d, want 2000", cfg.TruncateChars)
	}
}
