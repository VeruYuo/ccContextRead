package model

// FileChangeMode controls how much detail about file changes survives
// filtering (PLAN.md 4.3): none drops file-change events entirely, while
// summary/full both keep them — the distinction between a path-only
// summary and a full diff is a rendering concern (T1.7), not a filtering
// one.
type FileChangeMode int

const (
	FileChangeNone FileChangeMode = iota
	FileChangeSummary
	FileChangeFull
)

// FilterConfig mirrors the nine content-filter switches plus the two
// global options from PLAN.md 4.3. The zero value has every switch off,
// FileChange == FileChangeNone, and TruncateChars == 0 (which disables
// truncation) — use DefaultFilterConfig for the actual UI defaults.
type FilterConfig struct {
	UserPrompt       bool           `json:"userPrompt"`
	AssistantText    bool           `json:"assistantText"`
	Thinking         bool           `json:"thinking"`
	ToolUse          bool           `json:"toolUse"`
	ToolResult       bool           `json:"toolResult"`
	Subagent         bool           `json:"subagent"`
	ContextInjection bool           `json:"contextInjection"`
	SystemNote       bool           `json:"systemNote"`
	CompactSummary   bool           `json:"compactSummary"`
	FileChange       FileChangeMode `json:"fileChange"`
	TruncateChars    int            `json:"truncateChars"` // <= 0 disables truncation
}

// DefaultFilterConfig returns the task brief's default: only real user
// input and the assistant's final reply are shown, with a 2000-character
// truncation threshold for the (currently excluded) long-text kinds.
func DefaultFilterConfig() FilterConfig {
	return FilterConfig{
		UserPrompt:    true,
		AssistantText: true,
		FileChange:    FileChangeNone,
		TruncateChars: 2000,
	}
}

// Apply filters events per cfg and truncates the text of the kinds that
// can carry arbitrarily long bodies (tool results, and attachment
// content such as attachment.type:"file"). EventCompactBoundary always
// passes through regardless of cfg: it is structural information about
// where the conversation was compacted, not optional content (4.3).
// EventTitle is metadata for the document header, never part of the
// filtered body timeline, so it never appears in the output.
func Apply(events []Event, cfg FilterConfig) []Event {
	out := make([]Event, 0, len(events))
	for _, ev := range events {
		if !keep(ev.Kind, cfg) {
			continue
		}
		out = append(out, truncateEvent(ev, cfg.TruncateChars))
	}
	return out
}

func keep(kind EventKind, cfg FilterConfig) bool {
	switch kind {
	case EventCompactBoundary:
		return true
	case EventUserPrompt:
		return cfg.UserPrompt
	case EventAssistantText:
		return cfg.AssistantText
	case EventThinking:
		return cfg.Thinking
	case EventToolUse:
		return cfg.ToolUse
	case EventToolResult:
		return cfg.ToolResult
	case EventSubagent:
		return cfg.Subagent
	case EventContextInjection:
		return cfg.ContextInjection
	case EventSystemNote:
		return cfg.SystemNote
	case EventCompactSummary:
		return cfg.CompactSummary
	case EventFileChange:
		return cfg.FileChange != FileChangeNone
	default:
		// EventTitle and any future kind not covered by a 4.3 switch: not
		// part of the filtered body stream.
		return false
	}
}

// truncateEvent truncates the text of the kinds whose bodies can be
// arbitrarily long (tool results, attachment content). Other kinds are
// returned unchanged even when they happen to carry long text.
func truncateEvent(ev Event, limit int) Event {
	if limit <= 0 {
		return ev
	}
	if ev.Kind != EventToolResult && ev.Kind != EventContextInjection {
		return ev
	}
	ev.Text = truncateRunes(ev.Text, limit)
	return ev
}

// truncateRunes truncates s to at most limit runes, appending an ellipsis
// if truncation occurred. Rune-based so multi-byte (e.g. CJK) text is
// never split mid-character.
func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "…"
}
