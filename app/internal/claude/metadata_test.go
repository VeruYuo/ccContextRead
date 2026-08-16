package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fixtureProjectDir = "testdata/projects/D--FixtureProject"

func TestExtractMetadata_BasicSession_CustomTitleWinsOverAITitle(t *testing.T) {
	path := filepath.Join(fixtureProjectDir, "11111111-1111-4111-8111-111111111111.jsonl")

	meta, err := ExtractMetadata(path)
	if err != nil {
		t.Fatalf("ExtractMetadata(%q) error = %v", path, err)
	}

	if meta.SessionID != "11111111-1111-4111-8111-111111111111" {
		t.Errorf("SessionID = %q, want %q", meta.SessionID, "11111111-1111-4111-8111-111111111111")
	}
	if meta.ProjectDir != `D:\FixtureProject` {
		t.Errorf("ProjectDir = %q, want %q", meta.ProjectDir, `D:\FixtureProject`)
	}
	// custom-title (line 19) must win over ai-title (line 17).
	if meta.Title != "Add 函数 bug 排查" {
		t.Errorf("Title = %q, want %q", meta.Title, "Add 函数 bug 排查")
	}
	wantCreated := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	if !meta.CreatedAt.Equal(wantCreated) {
		t.Errorf("CreatedAt = %v, want %v", meta.CreatedAt, wantCreated)
	}
	wantLastActive := time.Date(2026, 8, 10, 9, 3, 50, 0, time.UTC)
	if !meta.LastActiveAt.Equal(wantLastActive) {
		t.Errorf("LastActiveAt = %v, want %v", meta.LastActiveAt, wantLastActive)
	}
}

func TestExtractMetadata_AttachmentsOnlySession_FallsBackToCwdBasename(t *testing.T) {
	path := filepath.Join(fixtureProjectDir, "22222222-2222-4222-8222-222222222222.jsonl")

	meta, err := ExtractMetadata(path)
	if err != nil {
		t.Fatalf("ExtractMetadata(%q) error = %v", path, err)
	}

	// No user record, no custom-title, no ai-title anywhere in this fixture
	// -> title must fall back to the cwd basename.
	if meta.Title != "FixtureProject" {
		t.Errorf("Title = %q, want %q", meta.Title, "FixtureProject")
	}
	if meta.SessionID != "22222222-2222-4222-8222-222222222222" {
		t.Errorf("SessionID = %q, want %q", meta.SessionID, "22222222-2222-4222-8222-222222222222")
	}
}

func TestExtractMetadata_CompactSession_UsesFirstRealUserMessageAsTitle(t *testing.T) {
	path := filepath.Join(fixtureProjectDir, "33333333-3333-4333-8333-333333333333.jsonl")

	meta, err := ExtractMetadata(path)
	if err != nil {
		t.Fatalf("ExtractMetadata(%q) error = %v", path, err)
	}

	// No custom-title/ai-title in this fixture; the first record's content
	// "/compact" does not start with <command-name> or contain
	// <local-command-caveat>, so it is a valid title candidate.
	if meta.Title != "/compact" {
		t.Errorf("Title = %q, want %q", meta.Title, "/compact")
	}
}

func TestExtractMetadata_SkipsPseudoUserMessages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	lines := []map[string]any{
		{
			"type":      "user",
			"sessionId": "pseudo-session",
			"cwd":       "/home/user/project",
			"timestamp": "2026-01-01T00:00:00Z",
			"message":   map[string]any{"role": "user", "content": "<local-command-caveat>ignored</local-command-caveat>"},
		},
		{
			"type":      "user",
			"timestamp": "2026-01-01T00:00:01Z",
			"message":   map[string]any{"role": "user", "content": "<command-name>/clear</command-name>"},
		},
		{
			"type":      "user",
			"timestamp": "2026-01-01T00:00:02Z",
			"message":   map[string]any{"role": "user", "content": "这是真正的用户输入"},
		},
	}
	writeJSONLFile(t, path, lines)

	meta, err := ExtractMetadata(path)
	if err != nil {
		t.Fatalf("ExtractMetadata(%q) error = %v", path, err)
	}
	if meta.Title != "这是真正的用户输入" {
		t.Errorf("Title = %q, want %q", meta.Title, "这是真正的用户输入")
	}
}

func TestExtractMetadata_SessionIDFallsBackToSnakeCaseThenFilename(t *testing.T) {
	dir := t.TempDir()

	t.Run("falls back to session_id", func(t *testing.T) {
		path := filepath.Join(dir, "snake.jsonl")
		writeJSONLFile(t, path, []map[string]any{
			{"type": "user", "session_id": "snake-case-id", "timestamp": "2026-01-01T00:00:00Z", "message": map[string]any{"role": "user", "content": "hi"}},
		})
		meta, err := ExtractMetadata(path)
		if err != nil {
			t.Fatalf("ExtractMetadata error = %v", err)
		}
		if meta.SessionID != "snake-case-id" {
			t.Errorf("SessionID = %q, want %q", meta.SessionID, "snake-case-id")
		}
	})

	t.Run("falls back to filename when both are missing", func(t *testing.T) {
		path := filepath.Join(dir, "no-id-at-all.jsonl")
		writeJSONLFile(t, path, []map[string]any{
			{"type": "user", "timestamp": "2026-01-01T00:00:00Z", "message": map[string]any{"role": "user", "content": "hi"}},
		})
		meta, err := ExtractMetadata(path)
		if err != nil {
			t.Fatalf("ExtractMetadata error = %v", err)
		}
		if meta.SessionID != "no-id-at-all" {
			t.Errorf("SessionID = %q, want %q", meta.SessionID, "no-id-at-all")
		}
	})
}

func TestExtractMetadata_TitleTruncatedByRuneNotByte(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long-title.jsonl")
	longTitle := strings.Repeat("标", 80) // 80 multi-byte runes
	writeJSONLFile(t, path, []map[string]any{
		{"type": "user", "sessionId": "s1", "timestamp": "2026-01-01T00:00:00Z", "message": map[string]any{"role": "user", "content": "hi"}},
		{"type": "custom-title", "timestamp": "2026-01-01T00:00:01Z", "customTitle": longTitle},
	})

	meta, err := ExtractMetadata(path)
	if err != nil {
		t.Fatalf("ExtractMetadata error = %v", err)
	}

	wantRunes := []rune(strings.Repeat("标", 60))
	want := string(wantRunes) + "..."
	if meta.Title != want {
		t.Errorf("Title = %q (rune len %d), want %q (rune len %d)", meta.Title, len([]rune(meta.Title)), want, len(wantRunes)+3)
	}
	// Sanity check the source was not accidentally shorter than the limit.
	if len([]rune(longTitle)) <= 60 {
		t.Fatalf("test setup bug: longTitle has only %d runes", len([]rune(longTitle)))
	}
}

func TestExtractMetadata_PerformanceHeadTailOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in -short mode")
	}
	sizes := map[string]int{
		"10KB": 10 * 1024,
		"6MB":  6 * 1024 * 1024,
	}
	for name, size := range sizes {
		t.Run(name, func(t *testing.T) {
			path := writeSyntheticSession(t, size)

			start := time.Now()
			meta, err := ExtractMetadata(path)
			elapsed := time.Since(start)

			if err != nil {
				t.Fatalf("ExtractMetadata error = %v", err)
			}
			if meta.SessionID != "synthetic-session" {
				t.Errorf("SessionID = %q, want %q", meta.SessionID, "synthetic-session")
			}
			if elapsed > 20*time.Millisecond {
				t.Errorf("ExtractMetadata took %v for a %s file, want < 20ms", elapsed, name)
			}
		})
	}
}

func writeJSONLFile(t *testing.T, path string, lines []map[string]any) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create(%q): %v", path, err)
	}
	defer f.Close()
	for _, line := range lines {
		b, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
}

// writeSyntheticSession builds a JSONL file of approximately targetBytes,
// with a valid head (sessionId/cwd/timestamp) and a custom-title tail line,
// used to exercise the head/tail-only performance characteristics of
// ExtractMetadata without committing a multi-megabyte fixture to the repo.
func writeSyntheticSession(t *testing.T, targetBytes int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()

	writeLine := func(v map[string]any) int {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		b = append(b, '\n')
		if _, err := f.Write(b); err != nil {
			t.Fatalf("Write: %v", err)
		}
		return len(b)
	}

	written := writeLine(map[string]any{
		"type":      "user",
		"sessionId": "synthetic-session",
		"cwd":       "/home/user/synthetic-project",
		"timestamp": "2026-01-01T00:00:00Z",
		"message":   map[string]any{"role": "user", "content": "hello from synthetic fixture"},
	})

	filler := strings.Repeat("x", 500)
	for written < targetBytes {
		written += writeLine(map[string]any{
			"type":      "assistant",
			"timestamp": "2026-01-01T00:00:01Z",
			"message":   map[string]any{"role": "assistant", "content": filler},
		})
	}

	writeLine(map[string]any{
		"type":        "custom-title",
		"timestamp":   "2026-01-01T00:10:00Z",
		"customTitle": "Synthetic Session Title",
	})

	return path
}
