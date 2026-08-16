package claude

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	titleMaxChars  = 60
	headTailChunk  = 16 * 1024
	maxLineBufSize = 64 * 1024 * 1024 // generous ceiling for a single JSONL line
)

// SessionMeta is the lightweight metadata extracted from a session file
// without a full parse: just enough to list and label sessions in the UI.
type SessionMeta struct {
	SessionID    string
	ProjectDir   string
	Title        string
	CreatedAt    time.Time
	LastActiveAt time.Time
	Path         string
}

// ExtractMetadata reads only the head and tail of the session JSONL file at
// path to derive its metadata, so it stays fast regardless of file size.
func ExtractMetadata(path string) (SessionMeta, error) {
	head, tail, err := readHeadTailLines(path, 10, 30)
	if err != nil {
		return SessionMeta{}, err
	}

	var sessionID, projectDir, firstUserMessage string
	var createdAt time.Time

	for _, line := range head {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			continue
		}
		if sessionID == "" {
			sessionID = stringField(v, "sessionId")
		}
		if sessionID == "" {
			sessionID = stringField(v, "session_id")
		}
		if projectDir == "" {
			projectDir = stringField(v, "cwd")
		}
		if createdAt.IsZero() {
			if ts, ok := parseISO8601(v["timestamp"]); ok {
				createdAt = ts
			}
		}
		if firstUserMessage == "" && isRealUserRecord(v) {
			if msg, ok := v["message"].(map[string]any); ok {
				text := strings.TrimSpace(extractText(msg["content"]))
				if text != "" &&
					!strings.Contains(text, "<local-command-caveat>") &&
					!strings.HasPrefix(text, "<command-name>") {
					firstUserMessage = text
				}
			}
		}
		if sessionID != "" && projectDir != "" && !createdAt.IsZero() && firstUserMessage != "" {
			break
		}
	}

	var lastActiveAt time.Time
	var customTitle, aiTitle string

	for _, line := range slices.Backward(tail) {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			continue
		}
		if lastActiveAt.IsZero() {
			if ts, ok := parseISO8601(v["timestamp"]); ok {
				lastActiveAt = ts
			}
		}
		if customTitle == "" && stringField(v, "type") == "custom-title" {
			customTitle = strings.TrimSpace(stringField(v, "customTitle"))
		}
		if aiTitle == "" && stringField(v, "type") == "ai-title" {
			aiTitle = strings.TrimSpace(stringField(v, "aiTitle"))
		}
		if !lastActiveAt.IsZero() && customTitle != "" && aiTitle != "" {
			break
		}
	}

	if sessionID == "" {
		sessionID = inferSessionIDFromFilename(path)
	}

	var title string
	switch {
	case customTitle != "":
		title = truncateRunes(customTitle, titleMaxChars)
	case aiTitle != "":
		title = truncateRunes(aiTitle, titleMaxChars)
	case firstUserMessage != "":
		title = truncateRunes(firstUserMessage, titleMaxChars)
	default:
		title = pathBasename(projectDir)
	}

	return SessionMeta{
		SessionID:    sessionID,
		ProjectDir:   projectDir,
		Title:        title,
		CreatedAt:    createdAt,
		LastActiveAt: lastActiveAt,
		Path:         path,
	}, nil
}

func stringField(v map[string]any, key string) string {
	s, _ := v[key].(string)
	return s
}

// isRealUserRecord reports whether v looks like a user-authored record,
// mirroring Claude Code's own role/type conventions.
func isRealUserRecord(v map[string]any) bool {
	if stringField(v, "type") == "user" {
		return true
	}
	if msg, ok := v["message"].(map[string]any); ok {
		if stringField(msg, "role") == "user" {
			return true
		}
	}
	return false
}

// extractText renders a message.content value (string, block array, or a
// single block object) down to plain text for title/summary purposes.
func extractText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, item := range c {
			if t := extractTextFromItem(item); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		return stringField(c, "text")
	default:
		return ""
	}
}

func extractTextFromItem(item any) string {
	m, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	switch stringField(m, "type") {
	case "tool_use":
		name := stringField(m, "name")
		if name == "" {
			name = "unknown"
		}
		return "[Tool: " + name + "]"
	case "tool_result":
		return extractText(m["content"])
	}
	if s := stringField(m, "text"); s != "" {
		return s
	}
	if content, ok := m["content"]; ok {
		return extractText(content)
	}
	return ""
}

func inferSessionIDFromFilename(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// readHeadTailLines reads the first headN and last tailN lines of the file
// at path without loading the whole file into memory for large sessions.
func readHeadTailLines(path string, headN, tailN int) (head, tail []string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	head, err = readHeadLines(f, headN)
	if err != nil {
		return nil, nil, err
	}
	tail, err = readTailLines(f, tailN)
	if err != nil {
		return nil, nil, err
	}
	return head, tail, nil
}

func readHeadLines(f *os.File, n int) ([]string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBufSize)
	var lines []string
	for len(lines) < n && scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return lines, err
	}
	return lines, nil
}

// readTailLines returns the last n lines of f, growing its read window from
// the end of the file until it has captured enough complete lines (or has
// reached the start of the file).
func readTailLines(f *os.File, n int) ([]string, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()

	chunk := int64(headTailChunk)
	for {
		start := max(size-chunk, 0)
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, size-start))
		if err != nil {
			return nil, err
		}
		text := string(data)
		if start > 0 {
			// We likely seeked into the middle of a line; drop the partial
			// prefix up to (and including) the next newline.
			if idx := strings.IndexByte(text, '\n'); idx >= 0 {
				text = text[idx+1:]
			} else {
				text = ""
			}
		}
		lines := splitLines(text)
		if len(lines) >= n || start == 0 {
			if len(lines) > n {
				lines = lines[len(lines)-n:]
			}
			return lines, nil
		}
		chunk *= 2
	}
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	raw := strings.Split(text, "\n")
	// Drop a trailing empty element produced by a final newline.
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	lines := make([]string, len(raw))
	for i, l := range raw {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}
