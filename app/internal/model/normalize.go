package model

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"ccContextRead/internal/claude"
)

// Normalize converts a session's parsed Records into a flat, ordered Event
// stream. A single Record may yield zero, one, or several Events (e.g. an
// assistant message with thinking + text + tool_use blocks becomes three).
// Unknown record/block/field shapes fall back to a generic EventSystemNote
// rather than being dropped (PLAN.md 2.2: observed value sets are a lower
// bound, not a closed set).
func Normalize(records []claude.Record) []Event {
	var events []Event
	turn := 0
	seenPrompt := false

	for idx := range records {
		rec := records[idx]

		var raw map[string]any
		if err := json.Unmarshal(rec.Raw, &raw); err != nil {
			continue // Raw was already validated JSON by claude.ParseRecords.
		}

		parentID := effectiveParentID(rec, raw)
		blocks := eventsForRecord(rec, raw)

		for i, ev := range blocks {
			ev.Timestamp = rec.Timestamp
			ev.SourceRecord = &records[idx]

			if i == 0 {
				ev.ID = rec.UUID
				ev.ParentID = parentID
			} else {
				ev.ID = rec.UUID + "#" + strconv.Itoa(i)
				ev.ParentID = events[len(events)-1].ID
			}

			if ev.Kind == EventUserPrompt {
				if seenPrompt {
					turn++
				}
				seenPrompt = true
			}
			ev.TurnIndex = turn

			events = append(events, ev)
		}
	}

	return events
}

// effectiveParentID resolves a record's causal parent, falling back to
// logicalParentUuid when parentUuid was recorded as null — the case at
// every compact boundary (PLAN.md 2.2, 2.8). A record with neither is a
// true session root.
func effectiveParentID(rec claude.Record, raw map[string]any) string {
	if rec.ParentUUID != "" {
		return rec.ParentUUID
	}
	return stringField(raw, "logicalParentUuid")
}

// eventsForRecord classifies one record into its constituent Events,
// without yet assigning ID/ParentID/TurnIndex (Normalize does that once it
// knows each block's position in the overall stream).
func eventsForRecord(rec claude.Record, raw map[string]any) []Event {
	switch rec.Type {
	case "user":
		return userEvents(rec, raw)
	case "assistant":
		return assistantEvents(raw)
	case "system":
		return systemEvents(raw)
	case "attachment":
		att, _ := raw["attachment"].(map[string]any)
		return []Event{{Kind: EventContextInjection, Role: "system", Text: stringField(att, "type")}}
	case "ai-title":
		return []Event{{Kind: EventTitle, Text: stringField(raw, "aiTitle")}}
	case "custom-title":
		return []Event{{Kind: EventTitle, Text: stringField(raw, "customTitle")}}
	case "agent-name":
		return []Event{{Kind: EventTitle, Text: stringField(raw, "agentName")}}
	case "file-history-snapshot", "file-history-delta":
		return []Event{{Kind: EventFileChange}}
	default:
		// mode, permission-mode, last-prompt, queue-operation,
		// agent-setting, and any future unknown type: keep as a generic,
		// non-dropped note rather than asserting a closed set.
		return []Event{{Kind: EventSystemNote, Role: rec.Type}}
	}
}

func userEvents(rec claude.Record, raw map[string]any) []Event {
	msg, _ := raw["message"].(map[string]any)
	content := msg["content"]

	if rec.IsMeta {
		return []Event{{Kind: EventSystemNote, Role: "user", Text: stripANSI(extractPlainText(content))}}
	}
	if boolField(raw, "isCompactSummary") {
		return []Event{{Kind: EventCompactSummary, Role: "user", Text: extractPlainText(content)}}
	}

	switch c := content.(type) {
	case string:
		return []Event{{Kind: EventUserPrompt, Role: "user", Text: stripANSI(c)}}
	case []any:
		events := make([]Event, 0, len(c))
		for _, item := range c {
			events = append(events, blockEvent("user", item))
		}
		if len(events) == 0 {
			return []Event{{Kind: EventUserPrompt, Role: "user"}}
		}
		return events
	default:
		return []Event{{Kind: EventUserPrompt, Role: "user"}}
	}
}

func assistantEvents(raw map[string]any) []Event {
	msg, _ := raw["message"].(map[string]any)
	model := stringField(msg, "model")
	usage := extractUsage(msg)

	var events []Event
	switch c := msg["content"].(type) {
	case string:
		events = []Event{{Kind: EventAssistantText, Text: stripANSI(c)}}
	case []any:
		events = make([]Event, 0, len(c))
		for _, item := range c {
			events = append(events, blockEvent("assistant", item))
		}
	}
	if len(events) == 0 {
		events = []Event{{Kind: EventAssistantText}}
	}

	for i := range events {
		events[i].Role = "assistant"
		events[i].Model = model
		events[i].Usage = usage
	}
	return events
}

func systemEvents(raw map[string]any) []Event {
	subtype := stringField(raw, "subtype")
	switch subtype {
	case "compact_boundary":
		return []Event{{Kind: EventCompactBoundary, Role: "system", CompactMeta: extractCompactMeta(raw)}}
	case "turn_duration":
		return []Event{{Kind: EventSystemNote, Role: "system", DurationMs: int64(intField(raw, "durationMs"))}}
	default:
		return []Event{{Kind: EventSystemNote, Role: "system", Text: stripANSI(stringField(raw, "content"))}}
	}
}

// blockEvent classifies a single message.content block. role is the
// enclosing record's message.role, used to distinguish a user-typed text
// block (EventUserPrompt) from an assistant one (EventAssistantText).
func blockEvent(role string, item any) Event {
	m, ok := item.(map[string]any)
	if !ok {
		return Event{Kind: EventSystemNote, Role: role}
	}

	switch stringField(m, "type") {
	case "text":
		kind := EventUserPrompt
		if role == "assistant" {
			kind = EventAssistantText
		}
		return Event{Kind: kind, Role: role, Text: stripANSI(stringField(m, "text"))}
	case "thinking":
		// signature is an encrypted opaque string; deliberately not carried
		// through to Event (PLAN.md 2.3).
		return Event{Kind: EventThinking, Role: role, Text: stringField(m, "thinking")}
	case "tool_use":
		input, _ := json.Marshal(m["input"])
		return Event{Kind: EventToolUse, Role: role, ToolName: stringField(m, "name"), ToolInput: input}
	case "tool_result":
		text, images := extractToolResultContent(m["content"])
		return Event{Kind: EventToolResult, Role: role, Text: stripANSI(text), Images: images}
	default:
		return Event{Kind: EventSystemNote, Role: role}
	}
}

// extractToolResultContent handles all three observed tool_result.content
// shapes (PLAN.md 2.3): a plain string, an array of {type:"text"} blocks, or
// an array of {type:"image", source:{...}} blocks. Image bytes are always
// returned as ImageRef, never inlined into text.
func extractToolResultContent(content any) (string, []ImageRef) {
	switch c := content.(type) {
	case string:
		return c, nil
	case []any:
		var texts []string
		var images []ImageRef
		for _, item := range c {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch stringField(m, "type") {
			case "text":
				texts = append(texts, stringField(m, "text"))
			case "image":
				src, _ := m["source"].(map[string]any)
				images = append(images, ImageRef{
					MediaType: stringField(src, "media_type"),
					Data:      stringField(src, "data"),
				})
			}
		}
		return strings.Join(texts, "\n"), images
	default:
		return "", nil
	}
}

func extractUsage(msg map[string]any) *Usage {
	u, ok := msg["usage"].(map[string]any)
	if !ok {
		return nil
	}
	return &Usage{
		InputTokens:              intField(u, "input_tokens"),
		OutputTokens:             intField(u, "output_tokens"),
		CacheCreationInputTokens: intField(u, "cache_creation_input_tokens"),
		CacheReadInputTokens:     intField(u, "cache_read_input_tokens"),
		ServiceTier:              stringField(u, "service_tier"),
	}
}

func extractCompactMeta(raw map[string]any) *CompactMeta {
	cm, ok := raw["compactMetadata"].(map[string]any)
	if !ok {
		return nil
	}
	seg, _ := cm["preservedSegment"].(map[string]any)
	msgs, _ := cm["preservedMessages"].(map[string]any)

	return &CompactMeta{
		Trigger:                 stringField(cm, "trigger"),
		PreTokens:               intField(cm, "preTokens"),
		PostTokens:              intField(cm, "postTokens"),
		CumulativeDroppedTokens: intField(cm, "cumulativeDroppedTokens"),
		DurationMs:              int64(intField(cm, "durationMs")),
		PreservedSegment: PreservedSegment{
			HeadUUID:   stringField(seg, "headUuid"),
			AnchorUUID: stringField(seg, "anchorUuid"),
			TailUUID:   stringField(seg, "tailUuid"),
		},
		PreservedMessages: PreservedMessages{
			AnchorUUID: stringField(msgs, "anchorUuid"),
			UUIDs:      stringSlice(msgs["uuids"]),
			AllUUIDs:   stringSlice(msgs["allUuids"]),
		},
	}
}

// extractPlainText renders a message.content value (string or block array)
// down to plain text, for the isMeta/isCompactSummary paths where block-level
// splitting doesn't apply.
func extractPlainText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, item := range c {
			if m, ok := item.(map[string]any); ok {
				if t := stringField(m, "text"); t != "" {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// stripANSI removes ANSI CSI escape sequences (e.g. "\x1b[2m") from local
// command output before it reaches any renderer (PLAN.md 2.8 point 4).
func stripANSI(s string) string {
	if s == "" {
		return s
	}
	return ansiPattern.ReplaceAllString(s, "")
}

func stringField(v map[string]any, key string) string {
	if v == nil {
		return ""
	}
	s, _ := v[key].(string)
	return s
}

func boolField(v map[string]any, key string) bool {
	if v == nil {
		return false
	}
	b, _ := v[key].(bool)
	return b
}

func intField(v map[string]any, key string) int {
	if v == nil {
		return 0
	}
	f, _ := v[key].(float64)
	return int(f)
}

func stringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
