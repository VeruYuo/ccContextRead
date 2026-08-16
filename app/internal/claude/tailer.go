package claude

import (
	"io"
	"os"
	"sync"
	"time"
)

// TailerState is the persistable cursor for a Tailer: how much of the file
// has already been consumed, plus enough file identity to detect a
// truncation/replacement across process restarts.
type TailerState struct {
	Offset  int64
	LineNo  int
	Size    int64
	ModTime time.Time
}

// Tailer performs incremental re-reads of a single growing JSONL file,
// tracking a byte offset so repeated Poll calls only return newly appended
// records. It detects truncation or file replacement (size shrinking, or
// mtime moving backwards) and signals the caller to fall back to a full
// re-parse per 4.5.
//
// A Tailer is safe for concurrent use: Poll and State/ForceFullRescan may be
// called from different goroutines while a writer appends to the same file.
type Tailer struct {
	path string

	mu     sync.Mutex
	state  TailerState
	forced bool
}

// NewTailer creates a Tailer starting from the beginning of the file at
// path. Its first Poll reads every record currently in the file.
func NewTailer(path string) *Tailer {
	return &Tailer{path: path}
}

// NewTailerWithState creates a Tailer that resumes from a previously
// persisted state (e.g. loaded from disk across an app restart), skipping
// records already consumed in an earlier run.
func NewTailerWithState(path string, state TailerState) *Tailer {
	return &Tailer{path: path, state: state}
}

// State returns a snapshot of the Tailer's current cursor, suitable for
// persisting and later restoring via NewTailerWithState.
func (t *Tailer) State() TailerState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// ForceFullRescan clears the Tailer's cursor so the next Poll re-parses the
// file from the start, regardless of size/mtime. Used when the filter
// configuration changes and a full rewrite of derived output is required
// (4.5, edge case 2).
func (t *Tailer) ForceFullRescan() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = TailerState{}
	t.forced = true
}

// Poll checks the file for new data and returns any newly-parsed records.
// The second return value reports whether a full rescan occurred (file
// truncated, replaced, or a rescan was forced): in that case the returned
// records are the complete parse of the file, not just an increment, and
// callers should treat them as a full replacement rather than an append.
func (t *Tailer) Poll() ([]Record, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	f, err := os.Open(t.path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}

	fullRescan := t.forced || t.needsFullRescan(info)
	if fullRescan {
		t.state = TailerState{}
		t.forced = false
	}

	if info.Size() == t.state.Offset {
		t.state.Size = info.Size()
		t.state.ModTime = info.ModTime()
		return nil, fullRescan, nil
	}

	if _, err := f.Seek(t.state.Offset, io.SeekStart); err != nil {
		return nil, false, err
	}

	records, consumed, lastLineNo, err := parseRecordsIncremental(f, t.state.LineNo, t.state.Offset)
	if err != nil {
		return nil, false, err
	}

	t.state.Offset += consumed
	t.state.LineNo = lastLineNo
	t.state.Size = info.Size()
	t.state.ModTime = info.ModTime()

	return records, fullRescan, nil
}

// needsFullRescan reports whether the file at info looks like it was
// truncated or replaced since the last poll, per 4.5's size/mtime checks.
// Must be called with t.mu held.
func (t *Tailer) needsFullRescan(info os.FileInfo) bool {
	if t.state.Offset == 0 {
		return false
	}
	if info.Size() < t.state.Offset {
		return true
	}
	if !t.state.ModTime.IsZero() && info.ModTime().Before(t.state.ModTime) {
		return true
	}
	return false
}
