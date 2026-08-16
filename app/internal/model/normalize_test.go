package model

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ccContextRead/internal/claude"
)

func parseRecords(t *testing.T, lines ...string) []claude.Record {
	t.Helper()
	records, err := claude.ParseRecords(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}
	return records
}

func parseRecordsFromFile(t *testing.T, path string) []claude.Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer f.Close()
	records, err := claude.ParseRecords(f)
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}
	return records
}

func TestNormalize_StringContentIsUserPrompt(t *testing.T) {
	records := parseRecords(t, `{"type":"user","uuid":"u1","parentUuid":null,"message":{"role":"user","content":"hello there"}}`)

	events := Normalize(records)
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Kind != EventUserPrompt {
		t.Errorf("Kind = %v, want EventUserPrompt", events[0].Kind)
	}
	if events[0].Text != "hello there" {
		t.Errorf("Text = %q, want %q", events[0].Text, "hello there")
	}
	if events[0].ID != "u1" {
		t.Errorf("ID = %q, want %q", events[0].ID, "u1")
	}
}

func TestNormalize_AssistantBlockArray_ThinkingTextToolUse(t *testing.T) {
	records := parseRecords(t, `{"type":"assistant","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","model":"claude-sonnet-5","content":[`+
		`{"type":"thinking","thinking":"pondering","signature":"deadbeef=="},`+
		`{"type":"text","text":"here is my answer"},`+
		`{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"a.go"}}`+
		`]}}`)

	events := Normalize(records)
	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(events))
	}

	if events[0].Kind != EventThinking || events[0].Text != "pondering" {
		t.Errorf("events[0] = %+v, want Kind=EventThinking Text=%q", events[0], "pondering")
	}
	if events[1].Kind != EventAssistantText || events[1].Text != "here is my answer" {
		t.Errorf("events[1] = %+v, want Kind=EventAssistantText Text=%q", events[1], "here is my answer")
	}
	if events[2].Kind != EventToolUse || events[2].ToolName != "Read" {
		t.Errorf("events[2] = %+v, want Kind=EventToolUse ToolName=%q", events[2], "Read")
	}
	var input map[string]any
	if err := json.Unmarshal(events[2].ToolInput, &input); err != nil {
		t.Fatalf("unmarshal ToolInput: %v", err)
	}
	if input["file_path"] != "a.go" {
		t.Errorf("ToolInput.file_path = %v, want %q", input["file_path"], "a.go")
	}

	// Block-chain IDs/parents within one record.
	if events[0].ID != "a1" || events[0].ParentID != "u1" {
		t.Errorf("events[0] ID/ParentID = %q/%q, want a1/u1", events[0].ID, events[0].ParentID)
	}
	if events[1].ParentID != events[0].ID {
		t.Errorf("events[1].ParentID = %q, want %q", events[1].ParentID, events[0].ID)
	}
	if events[2].ParentID != events[1].ID {
		t.Errorf("events[2].ParentID = %q, want %q", events[2].ParentID, events[1].ID)
	}
}

func TestNormalize_RoleReclassification_AllToolResultVsMixedText(t *testing.T) {
	records := parseRecords(t,
		`{"type":"user","uuid":"u1","parentUuid":null,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok","is_error":false}]}}`,
		`{"type":"user","uuid":"u2","parentUuid":"u1","message":{"role":"user","content":[{"type":"text","text":"thanks"},{"type":"tool_result","tool_use_id":"t2","content":"log","is_error":false}]}}`,
	)

	events := Normalize(records)
	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(events))
	}
	if events[0].Kind != EventToolResult {
		t.Errorf("all-tool_result record: Kind = %v, want EventToolResult", events[0].Kind)
	}
	if events[1].Kind != EventUserPrompt || events[1].Text != "thanks" {
		t.Errorf("mixed record text block: %+v, want Kind=EventUserPrompt Text=%q", events[1], "thanks")
	}
	if events[2].Kind != EventToolResult {
		t.Errorf("mixed record tool_result block: Kind = %v, want EventToolResult", events[2].Kind)
	}
}

func TestNormalize_ToolResultContentThreeForms(t *testing.T) {
	records := parseRecords(t,
		`{"type":"user","uuid":"s1","parentUuid":null,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"plain string"}]}}`,
		`{"type":"user","uuid":"s2","parentUuid":"s1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t2","content":[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]}]}}`,
		`{"type":"user","uuid":"s3","parentUuid":"s2","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t3","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUJD"}}]}]}}`,
	)

	events := Normalize(records)
	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(events))
	}

	if events[0].Text != "plain string" {
		t.Errorf("string form: Text = %q, want %q", events[0].Text, "plain string")
	}

	if events[1].Text != "line1\nline2" {
		t.Errorf("text-array form: Text = %q, want %q", events[1].Text, "line1\nline2")
	}

	if len(events[2].Images) != 1 {
		t.Fatalf("image-array form: len(Images) = %d, want 1", len(events[2].Images))
	}
	img := events[2].Images[0]
	if img.MediaType != "image/png" || img.Data != "QUJD" {
		t.Errorf("Images[0] = %+v, want MediaType=image/png Data=QUJD", img)
	}
	if strings.Contains(events[2].Text, "QUJD") {
		t.Errorf("Text must never contain inlined base64 image data, got %q", events[2].Text)
	}
}

func TestNormalize_TurnIndexIncrementsOnRealUserPrompts(t *testing.T) {
	records := parseRecords(t,
		`{"type":"user","uuid":"u1","parentUuid":null,"message":{"role":"user","content":"first"}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":[{"type":"text","text":"reply1"}]}}`,
		`{"type":"user","uuid":"u2","parentUuid":"a1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"result-not-a-new-turn"}]}}`,
		`{"type":"user","uuid":"u3","parentUuid":"u2","message":{"role":"user","content":"second"}}`,
		`{"type":"assistant","uuid":"a2","parentUuid":"u3","message":{"role":"assistant","content":[{"type":"text","text":"reply2"}]}}`,
	)

	events := Normalize(records)
	if len(events) != 5 {
		t.Fatalf("len(events) = %d, want 5", len(events))
	}
	want := []int{0, 0, 0, 1, 1}
	for i, ev := range events {
		if ev.TurnIndex != want[i] {
			t.Errorf("events[%d].TurnIndex = %d, want %d (Kind=%v Text=%q)", i, ev.TurnIndex, want[i], ev.Kind, ev.Text)
		}
	}
}

func TestNormalize_IsMetaRecordDoesNotBecomeUserPromptButIsKept(t *testing.T) {
	records := parseRecords(t, `{"type":"user","uuid":"m1","parentUuid":null,"isMeta":true,"message":{"role":"user","content":"<command-name>/compact</command-name>"}}`)

	events := Normalize(records)
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Kind == EventUserPrompt {
		t.Errorf("isMeta:true record must not be classified as EventUserPrompt, got Kind = %v", events[0].Kind)
	}
	if events[0].Text != "<command-name>/compact</command-name>" {
		t.Errorf("Text = %q, want content preserved verbatim", events[0].Text)
	}
}

func TestNormalize_UsageModelDurationMounted(t *testing.T) {
	records := parseRecords(t,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","model":"claude-sonnet-5","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":100,"output_tokens":20,"cache_creation_input_tokens":5,"cache_read_input_tokens":50,"service_tier":"standard"}}}`,
		`{"type":"system","uuid":"s1","parentUuid":"a1","subtype":"turn_duration","level":"info","durationMs":4213,"messageCount":8}`,
	)

	events := Normalize(records)
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}

	assistantEv := events[0]
	if assistantEv.Model != "claude-sonnet-5" {
		t.Errorf("Model = %q, want claude-sonnet-5", assistantEv.Model)
	}
	if assistantEv.Usage == nil {
		t.Fatal("Usage is nil, want populated")
	}
	if assistantEv.Usage.InputTokens != 100 || assistantEv.Usage.OutputTokens != 20 ||
		assistantEv.Usage.CacheCreationInputTokens != 5 || assistantEv.Usage.CacheReadInputTokens != 50 ||
		assistantEv.Usage.ServiceTier != "standard" {
		t.Errorf("Usage = %+v, want all fields mounted from JSON", assistantEv.Usage)
	}

	if events[1].DurationMs != 4213 {
		t.Errorf("DurationMs = %d, want 4213", events[1].DurationMs)
	}
}

func TestNormalize_SyntheticModelIsPassedThroughVerbatim(t *testing.T) {
	records := parseRecords(t, `{"type":"assistant","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","model":"<synthetic>","content":[{"type":"text","text":"synthetic reply"}]}}`)

	events := Normalize(records)
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Model != "<synthetic>" {
		t.Errorf("Model = %q, want <synthetic>", events[0].Model)
	}
}

// TestNormalize_CompactSequence exercises the full six-record compact
// sequence from PLAN.md 2.8, via the shared claude testdata fixture.
func TestNormalize_CompactSequence(t *testing.T) {
	records := parseRecordsFromFile(t, "../claude/testdata/projects/D--FixtureProject/33333333-3333-4333-8333-333333333333.jsonl")
	if len(records) != 6 {
		t.Fatalf("len(records) = %d, want 6", len(records))
	}

	events := Normalize(records)
	if len(events) != 6 {
		t.Fatalf("len(events) = %d, want 6 (one per record)", len(events))
	}

	// #1 "/compact" typed by the user — a genuine prompt, true DAG root.
	if events[0].Kind != EventUserPrompt {
		t.Errorf("events[0].Kind = %v, want EventUserPrompt", events[0].Kind)
	}
	if events[0].ParentID != "" {
		t.Errorf("events[0].ParentID = %q, want empty (session root)", events[0].ParentID)
	}

	// #3 compact_boundary — parentUuid is null, must fall back to
	// logicalParentUuid, and the full compactMetadata must be mounted.
	boundary := events[2]
	if boundary.Kind != EventCompactBoundary {
		t.Fatalf("events[2].Kind = %v, want EventCompactBoundary", boundary.Kind)
	}
	if boundary.ParentID != "33333333-3333-4333-8333-000000000001" {
		t.Errorf("boundary.ParentID = %q, want logicalParentUuid fallback", boundary.ParentID)
	}
	if boundary.CompactMeta == nil {
		t.Fatal("boundary.CompactMeta is nil, want populated")
	}
	cm := boundary.CompactMeta
	if cm.Trigger != "manual" || cm.PreTokens != 87325 || cm.PostTokens != 13963 ||
		cm.CumulativeDroppedTokens != 73362 || cm.DurationMs != 86480 {
		t.Errorf("CompactMeta scalar fields = %+v, want the 2.8 fixture values", cm)
	}
	wantSeg := PreservedSegment{
		HeadUUID:   "44444444-4444-4444-8444-000000000001",
		AnchorUUID: "44444444-4444-4444-8444-000000000002",
		TailUUID:   "33333333-3333-4333-8333-000000000001",
	}
	if cm.PreservedSegment != wantSeg {
		t.Errorf("PreservedSegment = %+v, want %+v", cm.PreservedSegment, wantSeg)
	}
	if cm.PreservedMessages.AnchorUUID != "44444444-4444-4444-8444-000000000002" ||
		len(cm.PreservedMessages.UUIDs) != 1 || len(cm.PreservedMessages.AllUUIDs) != 2 {
		t.Errorf("PreservedMessages = %+v, want anchor+1 uuid+2 allUuids", cm.PreservedMessages)
	}

	// #4 isCompactSummary:true — machine summary, must never be EventUserPrompt.
	summary := events[3]
	if summary.Kind != EventCompactSummary {
		t.Errorf("events[3].Kind = %v, want EventCompactSummary", summary.Kind)
	}
	if !strings.Contains(summary.Text, "合成摘要占位") {
		t.Errorf("summary.Text missing fixture content, got %q", summary.Text)
	}

	// #5 isMeta:true command echo — excluded from EventUserPrompt.
	echo := events[4]
	if echo.Kind == EventUserPrompt {
		t.Errorf("events[4].Kind = %v, isMeta:true must not become EventUserPrompt", echo.Kind)
	}
	if echo.Text != "<command-name>/compact</command-name>" {
		t.Errorf("echo.Text = %q, want command echo preserved", echo.Text)
	}

	// #6 isMeta:true local-command-stdout — ANSI escapes must be stripped.
	stdout := events[5]
	if stdout.Kind == EventUserPrompt {
		t.Errorf("events[5].Kind = %v, isMeta:true must not become EventUserPrompt", stdout.Kind)
	}
	const wantStdout = "<local-command-stdout>Compacted 87325 → 13963 tokens. (86.5s)</local-command-stdout>"
	if stdout.Text != wantStdout {
		t.Errorf("stdout.Text = %q, want ANSI-stripped %q", stdout.Text, wantStdout)
	}
	if strings.Contains(stdout.Text, "\x1b") {
		t.Errorf("stdout.Text still contains a raw ESC byte: %q", stdout.Text)
	}
}

// TestNormalize_FullFixtureCoversAllRecordTypes runs the shared "all
// 14 record types" fixture end to end and checks nothing silently vanishes.
func TestNormalize_FullFixtureCoversAllRecordTypes(t *testing.T) {
	records := parseRecordsFromFile(t, "../claude/testdata/projects/D--FixtureProject/11111111-1111-4111-8111-111111111111.jsonl")
	if len(records) != 24 {
		t.Fatalf("len(records) = %d, want 24", len(records))
	}

	events := Normalize(records)
	if len(events) != 29 {
		t.Fatalf("len(events) = %d, want 29 (block-split assistant/tool_result records included)", len(events))
	}

	kindCounts := map[EventKind]int{}
	for _, ev := range events {
		kindCounts[ev.Kind]++
	}
	for _, kind := range []EventKind{
		EventUserPrompt, EventAssistantText, EventThinking, EventToolUse,
		EventToolResult, EventContextInjection, EventSystemNote, EventFileChange,
		EventTitle,
	} {
		if kindCounts[kind] == 0 {
			t.Errorf("no Event of Kind %v produced from the full-type fixture", kind)
		}
	}

	// record #10's tool_result carries only an image block — its Text must
	// stay empty rather than silently swallowing the block.
	var imageEvent *Event
	for i := range events {
		if len(events[i].Images) > 0 {
			imageEvent = &events[i]
			break
		}
	}
	if imageEvent == nil {
		t.Fatal("no Event carrying an image was produced")
	}
	if imageEvent.Text != "" {
		t.Errorf("image-only tool_result Text = %q, want empty", imageEvent.Text)
	}
}
