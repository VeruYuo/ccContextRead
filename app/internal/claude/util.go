package claude

import (
	"strings"
	"time"
)

// parseISO8601 parses a Claude Code "timestamp" field value, which is
// always an ISO8601/RFC3339 string. Returns false if v is not a string or
// fails to parse, so callers can fall back gracefully.
func parseISO8601(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// pathBasename returns the last path component of p, tolerating both
// forward and backward slashes regardless of the host OS (a session's cwd
// may have been recorded on a different platform than we're running on).
func pathBasename(p string) string {
	p = strings.TrimRight(p, "/\\")
	if p == "" {
		return ""
	}
	idx := strings.LastIndexAny(p, "/\\")
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}

// truncateRunes truncates s to at most maxRunes runes, appending "..." if
// truncation occurred. Truncation is rune-based (not byte-based) so
// multi-byte characters (e.g. CJK text) are never split mid-character.
func truncateRunes(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}
