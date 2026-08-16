package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// staleAfter is how long a session registry entry may go without an
// updatedAt refresh before it's considered stale (2.6: "建议超过 5 分钟未更新
// 即视为失效").
const staleAfter = 5 * time.Minute

// LiveSession is a parsed, liveness-filtered entry from
// ~/.claude/sessions/<pid>.json.
type LiveSession struct {
	PID        int
	SessionID  string
	Cwd        string
	Kind       string // "interactive" or "bg"
	Status     string // "busy" or "idle"
	Entrypoint string
	Name       string
	NameSource string
	JobID      string // only set when Kind == "bg"
	StartedAt  time.Time
	UpdatedAt  time.Time
}

// ListLiveSessions reads every <pid>.json file in sessionsDir and returns
// the ones that are actually live: the pid must correspond to a running
// process, and updatedAt must be within staleAfter of now (a crashed CC
// process can leave its registry file behind, per 2.6). When the same
// sessionId appears in more than one file, only the entry with the latest
// updatedAt is kept.
//
// A missing sessionsDir is not an error: it just means there are no live
// sessions to report.
func ListLiveSessions(sessionsDir string) ([]LiveSession, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	bySessionID := make(map[string]LiveSession)
	var order []string

	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		sess, ok := parseLiveSessionFile(filepath.Join(sessionsDir, e.Name()))
		if !ok {
			continue
		}
		if !isProcessAlive(sess.PID) {
			continue
		}
		if time.Since(sess.UpdatedAt) > staleAfter {
			continue
		}

		existing, seen := bySessionID[sess.SessionID]
		if !seen {
			order = append(order, sess.SessionID)
			bySessionID[sess.SessionID] = sess
		} else if sess.UpdatedAt.After(existing.UpdatedAt) {
			bySessionID[sess.SessionID] = sess
		}
	}

	sessions := make([]LiveSession, 0, len(order))
	for _, id := range order {
		sessions = append(sessions, bySessionID[id])
	}
	return sessions, nil
}

func parseLiveSessionFile(path string) (LiveSession, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LiveSession{}, false
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return LiveSession{}, false
	}

	pid, ok := intField(v, "pid")
	if !ok {
		return LiveSession{}, false
	}
	sessionID := stringField(v, "sessionId")
	if sessionID == "" {
		return LiveSession{}, false
	}
	updatedAt, ok := millisField(v, "updatedAt")
	if !ok {
		return LiveSession{}, false
	}

	startedAt, _ := millisField(v, "startedAt")

	return LiveSession{
		PID:        pid,
		SessionID:  sessionID,
		Cwd:        stringField(v, "cwd"),
		Kind:       stringField(v, "kind"),
		Status:     stringField(v, "status"),
		Entrypoint: stringField(v, "entrypoint"),
		Name:       stringField(v, "name"),
		NameSource: stringField(v, "nameSource"),
		JobID:      stringField(v, "jobId"),
		StartedAt:  startedAt,
		UpdatedAt:  updatedAt,
	}, true
}

func intField(v map[string]any, key string) (int, bool) {
	n, ok := v[key].(float64)
	if !ok {
		return 0, false
	}
	return int(n), true
}

func millisField(v map[string]any, key string) (time.Time, bool) {
	n, ok := v[key].(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(n)), true
}

// isProcessAlive reports whether pid identifies a currently-running
// process. Unlike Unix, os.FindProcess on Windows actually opens a handle
// via OpenProcess and fails immediately if the pid doesn't correspond to a
// live process, so its error is a sufficient liveness check here (this
// project targets Windows only; see 2.6 on crashed processes leaving stale
// registry files behind).
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.FindProcess(pid)
	return err == nil
}
