package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ccContextRead/internal/model"
	"ccContextRead/internal/render"
)

// fakeEmitter is a test double for Emitter: it routes "session:updated",
// "error", and "sessions:changed" payloads to buffered channels so tests
// can block-wait on them instead of polling the filesystem.
type fakeEmitter struct {
	updates         chan UpdateEvent
	errors          chan ErrorEvent
	sessionsChanged chan struct{}
}

func newFakeEmitter() *fakeEmitter {
	return &fakeEmitter{
		updates:         make(chan UpdateEvent, 64),
		errors:          make(chan ErrorEvent, 64),
		sessionsChanged: make(chan struct{}, 64),
	}
}

func (f *fakeEmitter) Emit(event string, data any) {
	switch event {
	case "session:updated":
		f.updates <- data.(UpdateEvent)
	case "error":
		f.errors <- data.(ErrorEvent)
	case "sessions:changed":
		f.sessionsChanged <- struct{}{}
	}
}

const testPollInterval = 30 * time.Millisecond
const testDebounceInterval = 30 * time.Millisecond

func newTestOrchestrator(emitter Emitter) *Orchestrator {
	return NewOrchestrator(emitter, WithPollInterval(testPollInterval), WithDebounceInterval(testDebounceInterval))
}

func waitForUpdateContaining(t *testing.T, emitter *fakeEmitter, substr string, timeout time.Duration) UpdateEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev := <-emitter.updates:
			if strings.Contains(ev.Markdown, substr) {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out after %s waiting for an update containing %q", timeout, substr)
		}
	}
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open %q for append: %v", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		t.Fatalf("append to %q: %v", path, err)
	}
}

func userRecordLine(uuid, parentUUID, text string) string {
	return `{"type":"user","uuid":"` + uuid + `","parentUuid":"` + parentUUID + `","message":{"role":"user","content":"` + text + `"}}` + "\n"
}

func assistantRecordLine(uuid, parentUUID, text string) string {
	return `{"type":"assistant","uuid":"` + uuid + `","parentUuid":"` + parentUUID + `","message":{"role":"assistant","model":"claude-sonnet-5","content":[{"type":"text","text":"` + text + `"}]}}` + "\n"
}

func assistantToolUseLine(uuid, parentUUID, toolName string) string {
	return `{"type":"assistant","uuid":"` + uuid + `","parentUuid":"` + parentUUID + `","message":{"role":"assistant","model":"claude-sonnet-5","content":[{"type":"tool_use","id":"toolu_1","name":"` + toolName + `","input":{}}]}}` + "\n"
}

// TestOrchestrator_LiveAppend_EmitsUpdateWithinThreeSeconds covers M1
// acceptance item 2: append a record to a watched session file and expect
// session:updated within 3s, with the new content reflected in the output
// markdown file on disk.
func TestOrchestrator_LiveAppend_EmitsUpdateWithinThreeSeconds(t *testing.T) {
	sessionDir := t.TempDir()
	sessionPath := filepath.Join(sessionDir, "sess1.jsonl")
	if err := os.WriteFile(sessionPath, []byte(userRecordLine("u1", "", "hello there")), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "sess1.md")

	emitter := newFakeEmitter()
	orch := newTestOrchestrator(emitter)
	if err := orch.Start(sessionPath, outputPath, "sess1", model.DefaultFilterConfig(), render.Options{ImageMode: render.ImagePlaceholder}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer orch.Stop()

	// Drain the initial synchronous read triggered by Start.
	waitForUpdateContaining(t, emitter, "hello there", 3*time.Second)

	appendLine(t, sessionPath, assistantRecordLine("a1", "u1", "here is my live reply"))

	ev := waitForUpdateContaining(t, emitter, "here is my live reply", 3*time.Second)
	if ev.OutputPath != outputPath {
		t.Errorf("OutputPath = %q, want %q", ev.OutputPath, outputPath)
	}

	onDisk, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(onDisk), "here is my live reply") {
		t.Errorf("output file does not contain the new reply:\n%s", onDisk)
	}
}

// TestOrchestrator_HalfLineWrite_NoEventUntilCompleted covers 4.5 edge case
// 1: a half-written JSON line must not be consumed or trigger an event;
// completing it with a trailing newline must then fire normally.
func TestOrchestrator_HalfLineWrite_NoEventUntilCompleted(t *testing.T) {
	sessionDir := t.TempDir()
	sessionPath := filepath.Join(sessionDir, "sess1.jsonl")
	if err := os.WriteFile(sessionPath, []byte(userRecordLine("u1", "", "hello there")), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "sess1.md")

	emitter := newFakeEmitter()
	orch := newTestOrchestrator(emitter)
	if err := orch.Start(sessionPath, outputPath, "sess1", model.DefaultFilterConfig(), render.Options{ImageMode: render.ImagePlaceholder}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer orch.Stop()

	waitForUpdateContaining(t, emitter, "hello there", 3*time.Second)

	before, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	// Half-written line: no trailing newline.
	half := `{"type":"user","uuid":"u2","parentUuid":"u1","message":{"role":"user","content":"unfinished`
	appendLine(t, sessionPath, half)

	// Give the watcher/poll loop several cycles to (wrongly) fire.
	select {
	case ev := <-emitter.updates:
		t.Fatalf("unexpected session:updated for a half-written line: %+v", ev)
	case <-time.After(10 * testPollInterval):
	}

	after, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("output file changed on a half-written line:\nbefore=%s\nafter=%s", before, after)
	}

	// Complete the line.
	appendLine(t, sessionPath, `"}}`+"\n")
	waitForUpdateContaining(t, emitter, "unfinished", 3*time.Second)
}

// TestOrchestrator_FilterChange_TriggersFullRewrite covers M1 acceptance
// item 4: flipping a filter switch mid-watch must force a full rewrite
// (Full=true) that surfaces content the previous filter excluded, not an
// incremental append.
func TestOrchestrator_FilterChange_TriggersFullRewrite(t *testing.T) {
	sessionDir := t.TempDir()
	sessionPath := filepath.Join(sessionDir, "sess1.jsonl")
	if err := os.WriteFile(sessionPath, []byte(userRecordLine("u1", "", "hello there")), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "sess1.md")

	emitter := newFakeEmitter()
	orch := newTestOrchestrator(emitter)
	defaultFilter := model.DefaultFilterConfig()
	if err := orch.Start(sessionPath, outputPath, "sess1", defaultFilter, render.Options{ImageMode: render.ImagePlaceholder}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer orch.Stop()

	waitForUpdateContaining(t, emitter, "hello there", 3*time.Second)

	appendLine(t, sessionPath, assistantToolUseLine("t1", "u1", "Bash"))
	// Tool calls are off by default, so this append produces an event
	// stream but no *new* visible text; just drain whatever fires so it
	// doesn't get mistaken for the full-rewrite event below.
	select {
	case <-emitter.updates:
	case <-time.After(1 * time.Second):
	}

	toolFilter := defaultFilter
	toolFilter.ToolUse = true
	if err := orch.UpdateFilter(toolFilter, render.Options{ImageMode: render.ImagePlaceholder}); err != nil {
		t.Fatalf("UpdateFilter: %v", err)
	}

	ev := waitForUpdateContaining(t, emitter, "工具调用：Bash", 3*time.Second)
	if !ev.Full {
		t.Errorf("Full = false, want true (filter change must force a full rewrite)")
	}
}

// TestOrchestrator_CompactMidWatch_KeepsWatchingAndFallsBackParent covers
// the DAG-continuity edge case: a compact_boundary record with parentUuid
// null appears mid-stream, and normalization must fall back to
// logicalParentUuid (PLAN.md 2.8/easy-to-miss point 2) without interrupting
// the watch.
func TestOrchestrator_CompactMidWatch_KeepsWatchingAndFallsBackParent(t *testing.T) {
	fixture, err := os.ReadFile(filepath.FromSlash(
		"../claude/testdata/projects/D--FixtureProject/33333333-3333-4333-8333-333333333333.jsonl"))
	if err != nil {
		t.Fatalf("read compact fixture: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(fixture), "\n"), "\n")
	if len(lines) != 6 {
		t.Fatalf("compact fixture has %d lines, want 6", len(lines))
	}

	sessionDir := t.TempDir()
	sessionPath := filepath.Join(sessionDir, "sess1.jsonl")
	// Start with only record #1 (the "/compact" prompt itself).
	if err := os.WriteFile(sessionPath, []byte(lines[0]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "sess1.md")

	emitter := newFakeEmitter()
	orch := newTestOrchestrator(emitter)
	if err := orch.Start(sessionPath, outputPath, "sess1", model.DefaultFilterConfig(), render.Options{ImageMode: render.ImagePlaceholder}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer orch.Stop()

	waitForUpdateContaining(t, emitter, "/compact", 3*time.Second)

	// Append the rest, including the compact_boundary (#3, parentUuid:null)
	// and the isMeta/isCompactSummary records (#4-6), one line at a time as
	// Claude Code itself would.
	for _, line := range lines[1:] {
		appendLine(t, sessionPath, line+"\n")
	}

	ev := waitForUpdateContaining(t, emitter, "上下文已压缩", 3*time.Second)
	if !strings.Contains(ev.Markdown, "87325") {
		t.Errorf("markdown missing compact token counts: %s", ev.Markdown)
	}

	select {
	case errEv := <-emitter.errors:
		t.Fatalf("unexpected error event during compact mid-watch: %+v", errEv)
	default:
	}

	// The watch must still be alive: one more append should still produce
	// an update. Parent is record #6's uuid from the fixture.
	const lastFixtureUUID = "33333333-3333-4333-8333-000000000006"
	appendLine(t, sessionPath, assistantRecordLine("post-compact", lastFixtureUUID, "still alive after compact"))
	waitForUpdateContaining(t, emitter, "still alive after compact", 3*time.Second)
}

// TestOrchestrator_LastUpdate_OkFalseBeforeAnyPass verifies that LastUpdate
// returns ok=false on a freshly created Orchestrator that has never run a pass.
func TestOrchestrator_LastUpdate_OkFalseBeforeAnyPass(t *testing.T) {
	orch := newTestOrchestrator(newFakeEmitter())
	_, ok := orch.LastUpdate()
	if ok {
		t.Error("LastUpdate() ok = true before any pass, want false")
	}
}

// TestOrchestrator_LastUpdate_ReturnsSnapshotAfterPass verifies that after
// Start runs its initial synchronous pass, LastUpdate returns ok=true with
// the rendered Markdown matching what was emitted.
func TestOrchestrator_LastUpdate_ReturnsSnapshotAfterPass(t *testing.T) {
	sessionDir := t.TempDir()
	sessionPath := filepath.Join(sessionDir, "sess1.jsonl")
	if err := os.WriteFile(sessionPath, []byte(userRecordLine("u1", "", "hello snapshot")), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "sess1.md")

	emitter := newFakeEmitter()
	orch := newTestOrchestrator(emitter)
	if err := orch.Start(sessionPath, outputPath, "sess1", model.DefaultFilterConfig(), render.Options{ImageMode: render.ImagePlaceholder}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer orch.Stop()

	<-emitter.updates // drain initial pass

	ev, ok := orch.LastUpdate()
	if !ok {
		t.Fatal("LastUpdate() ok = false after Start, want true")
	}
	if ev.SessionID != "sess1" {
		t.Errorf("LastUpdate().SessionID = %q, want %q", ev.SessionID, "sess1")
	}
	if !strings.Contains(ev.Markdown, "hello snapshot") {
		t.Errorf("LastUpdate().Markdown does not contain %q: %s", "hello snapshot", ev.Markdown)
	}
}

// TestOrchestrator_LastUpdate_EmptyFile_OkTrue verifies that Start on an
// empty session file (zero records) still sets LastUpdate to ok=true with a
// valid header-only document, so the frontend can distinguish "empty session"
// from "never started".
func TestOrchestrator_LastUpdate_EmptyFile_OkTrue(t *testing.T) {
	sessionDir := t.TempDir()
	sessionPath := filepath.Join(sessionDir, "empty.jsonl")
	if err := os.WriteFile(sessionPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "empty.md")

	orch := newTestOrchestrator(newFakeEmitter())
	if err := orch.Start(sessionPath, outputPath, "empty-sess", model.DefaultFilterConfig(), render.Options{ImageMode: render.ImagePlaceholder}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer orch.Stop()

	ev, ok := orch.LastUpdate()
	if !ok {
		t.Fatal("LastUpdate() ok = false for empty session file, want true")
	}
	if ev.Markdown == "" {
		t.Error("LastUpdate().Markdown is empty, want at least a header document")
	}
}

// TestOrchestrator_LastUpdate_UpdatedAfterFilterChange verifies that after
// UpdateFilter forces a full rewrite, LastUpdate returns the new filtered
// result, not the old snapshot.
func TestOrchestrator_LastUpdate_UpdatedAfterFilterChange(t *testing.T) {
	sessionDir := t.TempDir()
	sessionPath := filepath.Join(sessionDir, "sess1.jsonl")
	content := userRecordLine("u1", "", "hello") + assistantToolUseLine("t1", "u1", "Bash")
	if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "sess1.md")

	emitter := newFakeEmitter()
	orch := newTestOrchestrator(emitter)
	defaultFilter := model.DefaultFilterConfig()
	if err := orch.Start(sessionPath, outputPath, "sess1", defaultFilter, render.Options{ImageMode: render.ImagePlaceholder}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer orch.Stop()

	<-emitter.updates // drain initial pass

	beforeEv, _ := orch.LastUpdate()
	if strings.Contains(beforeEv.Markdown, "工具调用") {
		t.Fatal("tool call should be hidden before filter change")
	}

	toolFilter := defaultFilter
	toolFilter.ToolUse = true
	if err := orch.UpdateFilter(toolFilter, render.Options{ImageMode: render.ImagePlaceholder}); err != nil {
		t.Fatalf("UpdateFilter: %v", err)
	}

	<-emitter.updates // drain filter-change pass

	afterEv, ok := orch.LastUpdate()
	if !ok {
		t.Fatal("LastUpdate() ok = false after UpdateFilter, want true")
	}
	if !strings.Contains(afterEv.Markdown, "工具调用") {
		t.Errorf("LastUpdate().Markdown does not contain tool call after filter change: %s", afterEv.Markdown)
	}
}

// TestOrchestrator_OutputDirBecomesUnwritable_EmitsErrorAndStopsWatching
// covers the error path: if the output directory disappears mid-watch, the
// next write must surface an "error" event and the watcher must stop
// (no more session:updated events, even after the directory reappears),
// without panicking.
func TestOrchestrator_OutputDirBecomesUnwritable_EmitsErrorAndStopsWatching(t *testing.T) {
	sessionDir := t.TempDir()
	sessionPath := filepath.Join(sessionDir, "sess1.jsonl")
	if err := os.WriteFile(sessionPath, []byte(userRecordLine("u1", "", "hello there")), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir, err := os.MkdirTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(outDir, "sess1.md")

	emitter := newFakeEmitter()
	orch := newTestOrchestrator(emitter)
	if err := orch.Start(sessionPath, outputPath, "sess1", model.DefaultFilterConfig(), render.Options{ImageMode: render.ImagePlaceholder}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer orch.Stop()

	waitForUpdateContaining(t, emitter, "hello there", 3*time.Second)

	if err := os.RemoveAll(outDir); err != nil {
		t.Fatal(err)
	}

	appendLine(t, sessionPath, assistantRecordLine("a1", "u1", "this write should fail"))

	select {
	case errEv := <-emitter.errors:
		if errEv.SessionID != "sess1" {
			t.Errorf("ErrorEvent.SessionID = %q, want %q", errEv.SessionID, "sess1")
		}
		if errEv.Message == "" {
			t.Error("ErrorEvent.Message is empty")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an error event after the output directory vanished")
	}

	// Recreate the directory and append again: the watcher must have
	// stopped, so no further update should ever arrive.
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	appendLine(t, sessionPath, assistantRecordLine("a2", "a1", "should never be observed"))

	select {
	case ev := <-emitter.updates:
		t.Fatalf("received an update after a fatal error should have stopped watching: %+v", ev)
	case <-time.After(10 * testPollInterval):
	}
}
