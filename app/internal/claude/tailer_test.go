package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func mustAppendFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(%q): %v", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
}

func TestTailer_InitialPollReadsAllRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, `{"type":"user","uuid":"a"}`+"\n"+`{"type":"assistant","uuid":"b"}`+"\n")

	tl := NewTailer(path)
	recs, fullRescan, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if fullRescan {
		t.Error("fullRescan = true, want false for a fresh tailer's first poll")
	}
	if len(recs) != 2 || recs[0].UUID != "a" || recs[1].UUID != "b" {
		t.Fatalf("recs = %+v, want uuid a, b", recs)
	}
}

func TestTailer_ThreeSegmentWritesAccumulateWithoutDuplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, "")

	tl := NewTailer(path)
	var all []Record

	mustAppendFile(t, path, `{"type":"user","uuid":"1"}`+"\n")
	recs, _, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll #1: %v", err)
	}
	all = append(all, recs...)

	mustAppendFile(t, path, `{"type":"user","uuid":"2"}`+"\n"+`{"type":"user","uuid":"3"}`+"\n")
	recs, _, err = tl.Poll()
	if err != nil {
		t.Fatalf("Poll #2: %v", err)
	}
	all = append(all, recs...)

	mustAppendFile(t, path, `{"type":"assistant","uuid":"4"}`+"\n")
	recs, _, err = tl.Poll()
	if err != nil {
		t.Fatalf("Poll #3: %v", err)
	}
	all = append(all, recs...)

	wantUUIDs := []string{"1", "2", "3", "4"}
	if len(all) != len(wantUUIDs) {
		t.Fatalf("total records = %d, want %d: %+v", len(all), len(wantUUIDs), all)
	}
	for i, want := range wantUUIDs {
		if all[i].UUID != want {
			t.Errorf("all[%d].UUID = %q, want %q", i, all[i].UUID, want)
		}
	}
}

func TestTailer_PartialLineNotConsumed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	line1 := `{"type":"user","uuid":"a"}` + "\n"
	mustWriteFile(t, path, line1+`{"type":"user","uuid":"b","extra":`)

	tl := NewTailer(path)
	recs, _, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(recs) != 1 || recs[0].UUID != "a" {
		t.Fatalf("recs = %+v, want only uuid=a (partial line must not be consumed)", recs)
	}

	state := tl.State()
	if state.Offset != int64(len(line1)) {
		t.Errorf("Offset = %d, want %d (must stop at last complete newline)", state.Offset, len(line1))
	}
}

func TestTailer_PartialLineConsumedAfterCompletion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, `{"type":"user","uuid":"a"}`+"\n"+`{"type":"user","uuid":"b"`)

	tl := NewTailer(path)
	if _, _, err := tl.Poll(); err != nil {
		t.Fatalf("Poll #1: %v", err)
	}

	mustAppendFile(t, path, `}`+"\n")

	recs, _, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll #2: %v", err)
	}
	if len(recs) != 1 || recs[0].UUID != "b" {
		t.Fatalf("recs = %+v, want only uuid=b now that the line is complete", recs)
	}
}

func TestTailer_TruncationTriggersFullRescan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, strings.Repeat(`{"type":"user","uuid":"a"}`+"\n", 5))

	tl := NewTailer(path)
	if _, _, err := tl.Poll(); err != nil {
		t.Fatalf("Poll #1: %v", err)
	}

	// Simulate CC rewriting the file smaller (e.g. after /compact).
	mustWriteFile(t, path, `{"type":"user","uuid":"compact"}`+"\n")

	recs, fullRescan, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll #2: %v", err)
	}
	if !fullRescan {
		t.Error("fullRescan = false, want true after truncation")
	}
	if len(recs) != 1 || recs[0].UUID != "compact" {
		t.Fatalf("recs = %+v, want single record uuid=compact", recs)
	}
}

func TestTailer_MTimeRegressionTriggersFullRescan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, `{"type":"user","uuid":"a"}`+"\n")

	tl := NewTailer(path)
	if _, _, err := tl.Poll(); err != nil {
		t.Fatalf("Poll #1: %v", err)
	}

	// Replace with different content stamped with an earlier mtime, as if a
	// stale backup were restored over the live file.
	mustWriteFile(t, path, `{"type":"user","uuid":"replaced"}`+"\n")
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	recs, fullRescan, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll #2: %v", err)
	}
	if !fullRescan {
		t.Error("fullRescan = false, want true after mtime regression")
	}
	if len(recs) != 1 || recs[0].UUID != "replaced" {
		t.Fatalf("recs = %+v, want single record uuid=replaced", recs)
	}
}

func TestTailer_StateRoundTripAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, `{"type":"user","uuid":"a"}`+"\n")

	tl1 := NewTailer(path)
	if _, _, err := tl1.Poll(); err != nil {
		t.Fatalf("Poll #1: %v", err)
	}
	state := tl1.State()

	mustAppendFile(t, path, `{"type":"user","uuid":"b"}`+"\n")

	tl2 := NewTailerWithState(path, state)
	recs, _, err := tl2.Poll()
	if err != nil {
		t.Fatalf("Poll #2: %v", err)
	}
	if len(recs) != 1 || recs[0].UUID != "b" {
		t.Fatalf("recs = %+v, want only uuid=b (restored offset must skip already-consumed data)", recs)
	}
}

func TestTailer_BOMStrippedOnInitialRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	bom := "\xEF\xBB\xBF"
	mustWriteFile(t, path, bom+`{"type":"user","uuid":"a"}`+"\n")

	tl := NewTailer(path)
	recs, _, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(recs) != 1 || recs[0].UUID != "a" {
		t.Fatalf("recs = %+v, want single record uuid=a with BOM stripped", recs)
	}
	if string(recs[0].Raw) != `{"type":"user","uuid":"a"}` {
		t.Errorf("Raw = %q, want no BOM bytes", recs[0].Raw)
	}
}

func TestTailer_CRLFLineEndingsHandled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, `{"type":"user","uuid":"a"}`+"\r\n"+`{"type":"user","uuid":"b"}`+"\r\n")

	tl := NewTailer(path)
	recs, _, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(recs) != 2 || recs[0].UUID != "a" || recs[1].UUID != "b" {
		t.Fatalf("recs = %+v, want uuid a, b", recs)
	}
	if string(recs[0].Raw) != `{"type":"user","uuid":"a"}` {
		t.Errorf("Raw = %q, want no CR byte", recs[0].Raw)
	}
}

func TestTailer_EmptyFilePollReturnsNoRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, "")

	tl := NewTailer(path)
	recs, fullRescan, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("recs = %+v, want empty", recs)
	}
	if fullRescan {
		t.Error("fullRescan = true, want false for empty file")
	}
}

func TestTailer_FileNotExistReturnsError(t *testing.T) {
	tl := NewTailer(filepath.Join(t.TempDir(), "missing.jsonl"))
	_, _, err := tl.Poll()
	if err == nil {
		t.Fatal("Poll() error = nil, want error for missing file")
	}
}

func TestTailer_NoNewDataReturnsEmptyOnSecondPoll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, `{"type":"user","uuid":"a"}`+"\n")

	tl := NewTailer(path)
	if _, _, err := tl.Poll(); err != nil {
		t.Fatalf("Poll #1: %v", err)
	}

	recs, fullRescan, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll #2: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("recs = %+v, want empty on unchanged file", recs)
	}
	if fullRescan {
		t.Error("fullRescan = true, want false")
	}
}

func TestTailer_BlankLinesSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, `{"type":"user","uuid":"a"}`+"\n\n"+`{"type":"user","uuid":"b"}`+"\n")

	tl := NewTailer(path)
	recs, _, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(recs) != 2 || recs[0].UUID != "a" || recs[1].UUID != "b" {
		t.Fatalf("recs = %+v, want uuid a, b (blank line skipped)", recs)
	}
	if recs[1].LineNo != 3 {
		t.Errorf("recs[1].LineNo = %d, want 3", recs[1].LineNo)
	}
}

func TestTailer_LineNoAndByteOffsetContinueAcrossPolls(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	line1 := `{"type":"user","uuid":"1"}` + "\n"
	mustWriteFile(t, path, line1)

	tl := NewTailer(path)
	recs, _, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll #1: %v", err)
	}
	if len(recs) != 1 || recs[0].LineNo != 1 || recs[0].ByteOffset != 0 {
		t.Fatalf("recs[0] = %+v, want LineNo=1 ByteOffset=0", recs[0])
	}

	line2 := `{"type":"user","uuid":"2"}` + "\n"
	mustAppendFile(t, path, line2)
	recs, _, err = tl.Poll()
	if err != nil {
		t.Fatalf("Poll #2: %v", err)
	}
	if len(recs) != 1 || recs[0].LineNo != 2 {
		t.Fatalf("recs[0].LineNo = %d, want 2", recs[0].LineNo)
	}
	if recs[0].ByteOffset != int64(len(line1)) {
		t.Errorf("recs[0].ByteOffset = %d, want %d", recs[0].ByteOffset, len(line1))
	}
}

func TestTailer_FullRescanResetsLineNumbering(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, strings.Repeat(`{"type":"user","uuid":"a"}`+"\n", 3))

	tl := NewTailer(path)
	if _, _, err := tl.Poll(); err != nil {
		t.Fatalf("Poll #1: %v", err)
	}

	mustWriteFile(t, path, `{"type":"user","uuid":"fresh"}`+"\n")
	recs, fullRescan, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll #2: %v", err)
	}
	if !fullRescan {
		t.Fatal("fullRescan = false, want true")
	}
	if len(recs) != 1 || recs[0].LineNo != 1 {
		t.Fatalf("recs = %+v, want single record with LineNo=1", recs)
	}
}

func TestTailer_ForceFullRescan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, `{"type":"user","uuid":"a"}`+"\n")

	tl := NewTailer(path)
	if _, _, err := tl.Poll(); err != nil {
		t.Fatalf("Poll #1: %v", err)
	}

	tl.ForceFullRescan()
	recs, fullRescan, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll #2: %v", err)
	}
	if !fullRescan {
		t.Error("fullRescan = false, want true after ForceFullRescan")
	}
	if len(recs) != 1 || recs[0].UUID != "a" {
		t.Fatalf("recs = %+v, want the same record re-parsed", recs)
	}
}

func TestTailer_InvalidJSONLineSkippedWithoutAborting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, `{"type":"user","uuid":"a"}`+"\n"+"not-json{{{"+"\n"+`{"type":"user","uuid":"b"}`+"\n")

	tl := NewTailer(path)
	recs, _, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(recs) != 2 || recs[0].UUID != "a" || recs[1].UUID != "b" {
		t.Fatalf("recs = %+v, want uuid a, b", recs)
	}
	if recs[1].LineNo != 3 {
		t.Errorf("recs[1].LineNo = %d, want 3", recs[1].LineNo)
	}
}

func TestTailer_ConcurrentPollAndWrite_NoRace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, path, "")

	tl := NewTailer(path)
	const total = 30

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Errorf("OpenFile: %v", err)
			return
		}
		defer f.Close()
		for i := 0; i < total; i++ {
			line := fmt.Sprintf(`{"type":"user","uuid":"u-%02d"}`+"\n", i)
			if _, err := f.WriteString(line); err != nil {
				t.Errorf("WriteString: %v", err)
				return
			}
		}
	}()

	seen := map[string]bool{}
	go func() {
		defer wg.Done()
		for i := 0; i < total*3; i++ {
			recs, _, err := tl.Poll()
			if err != nil {
				t.Errorf("Poll: %v", err)
				return
			}
			for _, r := range recs {
				if seen[r.UUID] {
					t.Errorf("duplicate record %q", r.UUID)
				}
				seen[r.UUID] = true
			}
		}
	}()

	wg.Wait()

	// Drain any records written after the reader goroutine's last poll.
	recs, _, err := tl.Poll()
	if err != nil {
		t.Fatalf("final Poll: %v", err)
	}
	for _, r := range recs {
		seen[r.UUID] = true
	}

	if len(seen) != total {
		t.Errorf("collected %d unique records, want %d", len(seen), total)
	}
}
