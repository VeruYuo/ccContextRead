package claude

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseRecords_AllKnownTypesFromFixture(t *testing.T) {
	f, err := os.Open("testdata/projects/D--FixtureProject/11111111-1111-4111-8111-111111111111.jsonl")
	if err != nil {
		t.Fatalf("Open fixture: %v", err)
	}
	defer f.Close()

	records, err := ParseRecords(f)
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}

	// The full 2.2 type table (14 known values).
	want := []string{
		"user", "assistant", "attachment", "system",
		"file-history-snapshot", "file-history-delta",
		"mode", "permission-mode", "ai-title", "agent-name",
		"custom-title", "last-prompt", "queue-operation", "agent-setting",
	}
	seen := map[string]bool{}
	for _, r := range records {
		seen[r.Type] = true
	}
	for _, wantType := range want {
		if !seen[wantType] {
			t.Errorf("type %q not found among parsed records", wantType)
		}
	}

	if len(records) != 24 {
		t.Errorf("len(records) = %d, want 24", len(records))
	}

	first := records[0]
	if first.Type != "user" {
		t.Errorf("records[0].Type = %q, want %q", first.Type, "user")
	}
	if first.UUID != "11111111-1111-4111-8111-000000000001" {
		t.Errorf("records[0].UUID = %q, want %q", first.UUID, "11111111-1111-4111-8111-000000000001")
	}
	if first.ParentUUID != "" {
		t.Errorf("records[0].ParentUUID = %q, want empty (null in source)", first.ParentUUID)
	}
	if first.SessionID != "11111111-1111-4111-8111-111111111111" {
		t.Errorf("records[0].SessionID = %q, want fixture session id", first.SessionID)
	}
	if first.CWD != `D:\FixtureProject` {
		t.Errorf("records[0].CWD = %q, want %q", first.CWD, `D:\FixtureProject`)
	}
	wantTS := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	if !first.Timestamp.Equal(wantTS) {
		t.Errorf("records[0].Timestamp = %v, want %v", first.Timestamp, wantTS)
	}
	if first.LineNo != 1 {
		t.Errorf("records[0].LineNo = %d, want 1", first.LineNo)
	}
	if first.ByteOffset != 0 {
		t.Errorf("records[0].ByteOffset = %d, want 0", first.ByteOffset)
	}
}

func TestParseRecords_SkipsInvalidJSONWithoutAborting(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"user","uuid":"a"}`,
		`not-valid-json{{{`,
		`{"type":"assistant","uuid":"b"}`,
	}, "\n") + "\n"

	records, err := ParseRecords(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0].UUID != "a" || records[1].UUID != "b" {
		t.Errorf("records = %+v, want uuids a, b", records)
	}
	// The invalid line still occupies physical line 2, so the next valid
	// record must report line 3, not 2.
	if records[1].LineNo != 3 {
		t.Errorf("records[1].LineNo = %d, want 3", records[1].LineNo)
	}
}

func TestParseRecords_BOM_CRLF_BlankLines_NoTrailingNewline(t *testing.T) {
	bom := "\xEF\xBB\xBF"
	input := bom +
		`{"type":"user","uuid":"a"}` + "\r\n" +
		"\r\n" + // blank line (CRLF)
		`{"type":"assistant","uuid":"b"}` // no trailing newline at all

	records, err := ParseRecords(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2: %+v", len(records), records)
	}
	if records[0].UUID != "a" {
		t.Errorf("records[0].UUID = %q, want %q", records[0].UUID, "a")
	}
	if string(records[0].Raw) != `{"type":"user","uuid":"a"}` {
		t.Errorf("records[0].Raw = %q, want no BOM/CR bytes", records[0].Raw)
	}
	if records[1].UUID != "b" {
		t.Errorf("records[1].UUID = %q, want %q", records[1].UUID, "b")
	}
	if string(records[1].Raw) != `{"type":"assistant","uuid":"b"}` {
		t.Errorf("records[1].Raw = %q, want exact trailing line with no newline", records[1].Raw)
	}
	// Line numbers: 1 = user record, 2 = blank line (skipped), 3 = assistant record.
	if records[1].LineNo != 3 {
		t.Errorf("records[1].LineNo = %d, want 3", records[1].LineNo)
	}
}

func TestParseRecords_TimestampMissingOrMalformed(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"user","uuid":"no-ts"}`,
		`{"type":"user","uuid":"bad-ts","timestamp":"not-a-timestamp"}`,
	}, "\n") + "\n"

	records, err := ParseRecords(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2 (records must survive bad/missing timestamp)", len(records))
	}
	for _, r := range records {
		if !r.Timestamp.IsZero() {
			t.Errorf("record %q Timestamp = %v, want zero value", r.UUID, r.Timestamp)
		}
	}
}

func TestParseRecords_RawIsByteExactWithSourceLine(t *testing.T) {
	line := `{"type":"user",   "extra":1,"nested":{"a":  [1,2,3]}}`
	input := line + "\n"

	records, err := ParseRecords(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if string(records[0].Raw) != line {
		t.Errorf("Raw = %q, want byte-exact %q", records[0].Raw, line)
	}
}

func TestParseRecords_SessionIDFallsBackToSnakeCase(t *testing.T) {
	input := `{"type":"user","session_id":"snake-only"}` + "\n"

	records, err := ParseRecords(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].SessionID != "snake-only" {
		t.Errorf("SessionID = %q, want %q", records[0].SessionID, "snake-only")
	}
}

func TestParseRecords_EmptyInput(t *testing.T) {
	records, err := ParseRecords(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseRecords() error = %v", err)
	}
	if len(records) != 0 {
		t.Errorf("len(records) = %d, want 0", len(records))
	}
}
