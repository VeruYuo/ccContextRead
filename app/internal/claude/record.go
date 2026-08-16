package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"time"
)

// Record is the generic shell of one JSONL line. Raw preserves the input
// bytes untouched (byte-exact, no re-serialization) so the Trajectory
// inspector can always show the source of truth.
type Record struct {
	Type        string
	UUID        string
	ParentUUID  string
	Timestamp   time.Time
	SessionID   string
	CWD         string
	GitBranch   string
	Version     string
	IsSidechain bool
	IsMeta      bool
	AgentID     string // old-format sidechain grouping key (2.2); empty outside isSidechain records
	Raw         json.RawMessage
	LineNo      int   // 1-based physical line number in the source file
	ByteOffset  int64 // byte offset of the line's first byte in the source file
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ParseRecords reads newline-delimited JSON records from r, one per
// physical line. It tolerates a leading UTF-8 BOM, CRLF line endings, blank
// lines, and a final line with no trailing newline. Lines that are empty or
// fail to parse as JSON are skipped without aborting the scan; only genuine
// I/O errors are returned. Fields with an unexpected value type (e.g. an
// unknown record's timestamp) fall back to their zero value rather than
// failing the record.
func ParseRecords(r io.Reader) ([]Record, error) {
	br := bufio.NewReader(r)

	var offset int64
	if bomBuf, err := br.Peek(3); err == nil && bytes.Equal(bomBuf, utf8BOM) {
		if _, err := br.Discard(3); err != nil {
			return nil, err
		}
		offset = 3
	}

	var records []Record
	lineNo := 0

	for {
		lineBytes, readErr := br.ReadBytes('\n')
		if len(lineBytes) == 0 && readErr != nil {
			break
		}
		lineNo++
		lineStart := offset
		offset += int64(len(lineBytes))

		line := bytes.TrimSuffix(lineBytes, []byte("\n"))
		line = bytes.TrimSuffix(line, []byte("\r"))

		if len(bytes.TrimSpace(line)) > 0 {
			if rec, ok := parseRecordLine(line, lineNo, lineStart); ok {
				records = append(records, rec)
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return records, readErr
		}
	}

	return records, nil
}

func parseRecordLine(line []byte, lineNo int, byteOffset int64) (Record, bool) {
	var v map[string]any
	if err := json.Unmarshal(line, &v); err != nil {
		return Record{}, false
	}

	raw := make(json.RawMessage, len(line))
	copy(raw, line)

	rec := Record{
		Type:        stringField(v, "type"),
		UUID:        stringField(v, "uuid"),
		ParentUUID:  stringField(v, "parentUuid"),
		SessionID:   stringField(v, "sessionId"),
		CWD:         stringField(v, "cwd"),
		GitBranch:   stringField(v, "gitBranch"),
		Version:     stringField(v, "version"),
		IsSidechain: boolField(v, "isSidechain"),
		IsMeta:      boolField(v, "isMeta"),
		AgentID:     stringField(v, "agentId"),
		Raw:         raw,
		LineNo:      lineNo,
		ByteOffset:  byteOffset,
	}
	if rec.SessionID == "" {
		rec.SessionID = stringField(v, "session_id")
	}
	if ts, ok := parseISO8601(v["timestamp"]); ok {
		rec.Timestamp = ts
	}

	return rec, true
}

func boolField(v map[string]any, key string) bool {
	b, _ := v[key].(bool)
	return b
}

// parseRecordsIncremental reads newline-delimited JSON records from r
// starting at startLineNo/startByteOffset, mirroring ParseRecords' line
// handling (BOM stripped only when startByteOffset is 0, CRLF and blank
// lines tolerated). Unlike ParseRecords, a final line with no trailing
// newline is NOT consumed — it may be a half-written line (4.5, edge case
// 1), so it is left for a later call once completed. It returns the parsed
// records, the number of bytes actually consumed (i.e. up to the last
// complete newline), and the physical line number of the last line
// consumed (whether or not it parsed as a valid record).
func parseRecordsIncremental(r io.Reader, startLineNo int, startByteOffset int64) (records []Record, consumed int64, lastLineNo int, err error) {
	br := bufio.NewReader(r)

	if startByteOffset == 0 {
		if bomBuf, peekErr := br.Peek(3); peekErr == nil && bytes.Equal(bomBuf, utf8BOM) {
			if _, discardErr := br.Discard(3); discardErr != nil {
				return nil, 0, startLineNo, discardErr
			}
			consumed += 3
		}
	}

	lineNo := startLineNo

	for {
		lineBytes, readErr := br.ReadBytes('\n')
		if readErr != nil {
			// No trailing newline: either EOF with nothing left, or a
			// half-written final line. Either way, don't consume it.
			if readErr == io.EOF {
				break
			}
			return records, consumed, lineNo, readErr
		}

		lineNo++
		consumed += int64(len(lineBytes))

		line := bytes.TrimSuffix(lineBytes, []byte("\n"))
		line = bytes.TrimSuffix(line, []byte("\r"))

		if len(bytes.TrimSpace(line)) > 0 {
			if rec, ok := parseRecordLine(line, lineNo, startByteOffset+consumed-int64(len(lineBytes))); ok {
				records = append(records, rec)
			}
		}
	}

	return records, consumed, lineNo, nil
}
