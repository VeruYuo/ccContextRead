// Package model normalizes parsed claude.Record shells into a flat,
// semantically-typed Event stream (see PLAN.md 4.2).
package model

import (
	"encoding/json"
	"time"

	"ccContextRead/internal/claude"
)

// EventKind is the normalized semantic kind of an Event.
type EventKind int

const (
	EventUserPrompt       EventKind = iota // real user input
	EventAssistantText                     // assistant final reply text
	EventThinking                          // chain-of-thought
	EventToolUse                           // tool invocation
	EventToolResult                        // tool result
	EventSubagent                          // sub-agent session (expandable)
	EventContextInjection                  // attachment
	EventSystemNote                        // system / mode / permission-mode / misc bookkeeping
	EventFileChange                        // derived from tool_use/tool_result/file-history
	EventTitle                             // ai-title / custom-title / agent-name
	EventCompactSummary                    // isCompactSummary:true — machine summary, not user input
	EventCompactBoundary                   // system:compact_boundary, carries CompactMeta
)

// Usage mirrors Claude Code's per-turn token accounting (2.4).
type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	ServiceTier              string
}

// ImageRef is a base64-encoded image block carried through from a
// tool_result. Normalization never drops it or inlines it into text — a
// later render stage decides whether to write it to disk or substitute a
// placeholder (2.3).
type ImageRef struct {
	MediaType string
	Data      string
}

// FileOp is a single file-level change derived from tool use or
// file-history tracking (populated by a later task).
type FileOp struct {
	Path string
	Op   string
}

// PreservedSegment mirrors compactMetadata.preservedSegment verbatim (2.8).
type PreservedSegment struct {
	HeadUUID   string
	AnchorUUID string
	TailUUID   string
}

// PreservedMessages mirrors compactMetadata.preservedMessages verbatim (2.8).
type PreservedMessages struct {
	AnchorUUID string
	UUIDs      []string
	AllUUIDs   []string
}

// CompactMeta carries the full compactMetadata payload of a
// system:compact_boundary record (2.8).
type CompactMeta struct {
	Trigger                 string
	PreTokens               int
	PostTokens              int
	CumulativeDroppedTokens int
	DurationMs              int64
	PreservedSegment        PreservedSegment
	PreservedMessages       PreservedMessages
}

// Event is one normalized, semantically-typed unit of a session's timeline.
// A single Record may yield zero, one, or several Events (e.g. an assistant
// message with thinking + text + tool_use blocks becomes three).
type Event struct {
	ID, ParentID string
	Kind         EventKind
	TurnIndex    int
	Timestamp    time.Time
	Role         string
	Model        string
	Usage        *Usage
	DurationMs   int64
	ToolName     string
	ToolInput    json.RawMessage
	Text         string
	Images       []ImageRef
	FileOps      []FileOp
	CompactMeta  *CompactMeta
	SourceRecord *claude.Record
	Children     []*Event
}
