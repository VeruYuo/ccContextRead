package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSessionRegistryFile(t *testing.T, dir string, pid int, fields map[string]any) string {
	t.Helper()
	m := map[string]any{"pid": pid}
	for k, v := range fields {
		m[k] = v
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	path := filepath.Join(dir, itoa(pid)+".json")
	mustWriteFile(t, path, string(data))
	return path
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestLiveSessions_ParsesRegistryFile(t *testing.T) {
	dir := t.TempDir()
	self := os.Getpid()
	updatedAt := time.Now().UnixMilli()
	writeSessionRegistryFile(t, dir, self, map[string]any{
		"sessionId":  "ca44ff06-live",
		"cwd":        `D:\ccContextRead`,
		"startedAt":  updatedAt - 60000,
		"kind":       "interactive",
		"entrypoint": "cli",
		"name":       "cccontextread-0f",
		"nameSource": "derived",
		"status":     "busy",
		"updatedAt":  updatedAt,
	})

	sessions, err := ListLiveSessions(dir)
	if err != nil {
		t.Fatalf("ListLiveSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1: %+v", len(sessions), sessions)
	}
	got := sessions[0]
	if got.SessionID != "ca44ff06-live" {
		t.Errorf("SessionID = %q, want ca44ff06-live", got.SessionID)
	}
	if got.PID != self {
		t.Errorf("PID = %d, want %d", got.PID, self)
	}
	if got.Kind != "interactive" {
		t.Errorf("Kind = %q, want interactive", got.Kind)
	}
	if got.Status != "busy" {
		t.Errorf("Status = %q, want busy", got.Status)
	}
}

func TestLiveSessions_ZombiePidFileIgnored(t *testing.T) {
	dir := t.TempDir()
	// A pid vanishingly unlikely to be alive; Windows also rejects
	// out-of-range pids outright, which we want to treat the same as "not
	// alive" rather than erroring out the whole scan.
	deadPid := 999999
	writeSessionRegistryFile(t, dir, deadPid, map[string]any{
		"sessionId": "ghost-session",
		"cwd":       `D:\ccContextRead`,
		"kind":      "interactive",
		"status":    "idle",
		"updatedAt": time.Now().UnixMilli(),
	})

	sessions, err := ListLiveSessions(dir)
	if err != nil {
		t.Fatalf("ListLiveSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d, want 0 (zombie pid should be ignored): %+v", len(sessions), sessions)
	}
}

func TestLiveSessions_StaleUpdatedAtExcluded(t *testing.T) {
	dir := t.TempDir()
	self := os.Getpid()
	staleUpdatedAt := time.Now().Add(-10 * time.Minute).UnixMilli()
	writeSessionRegistryFile(t, dir, self, map[string]any{
		"sessionId": "stale-session",
		"cwd":       `D:\ccContextRead`,
		"kind":      "interactive",
		"status":    "idle",
		"updatedAt": staleUpdatedAt,
	})

	sessions, err := ListLiveSessions(dir)
	if err != nil {
		t.Fatalf("ListLiveSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d, want 0 (updatedAt >5min old should be excluded): %+v", len(sessions), sessions)
	}
}

func TestLiveSessions_FreshUpdatedAtWithinFiveMinutesIncluded(t *testing.T) {
	dir := t.TempDir()
	self := os.Getpid()
	freshUpdatedAt := time.Now().Add(-4 * time.Minute).UnixMilli()
	writeSessionRegistryFile(t, dir, self, map[string]any{
		"sessionId": "fresh-session",
		"cwd":       `D:\ccContextRead`,
		"kind":      "interactive",
		"status":    "idle",
		"updatedAt": freshUpdatedAt,
	})

	sessions, err := ListLiveSessions(dir)
	if err != nil {
		t.Fatalf("ListLiveSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1 (updatedAt <5min old should be included): %+v", len(sessions), sessions)
	}
}

func TestLiveSessions_SameSessionIDMultiplePidsTakesLatest(t *testing.T) {
	dir := t.TempDir()
	self := os.Getpid()
	older := time.Now().Add(-2 * time.Minute).UnixMilli()
	newer := time.Now().UnixMilli()

	// Both entries use the current process's pid since only a live pid
	// survives filtering; two distinct on-disk files (by construction,
	// registry files are named by pid) can't both hold this process's
	// pid, so we simulate the "same sessionId, two pids" scenario using a
	// second live pid: this process's parent, which is also guaranteed
	// alive for the duration of the test.
	selfEntry := map[string]any{
		"sessionId": "dup-session",
		"cwd":       `D:\ccContextRead`,
		"kind":      "interactive",
		"status":    "idle",
		"updatedAt": older,
	}
	writeSessionRegistryFile(t, dir, self, selfEntry)

	parentPid := os.Getppid()
	if parentPid > 0 && parentPid != self {
		writeSessionRegistryFile(t, dir, parentPid, map[string]any{
			"sessionId": "dup-session",
			"cwd":       `D:\ccContextRead`,
			"kind":      "interactive",
			"status":    "busy",
			"updatedAt": newer,
		})
	} else {
		t.Skip("no distinct live parent pid available to simulate duplicate sessionId")
	}

	sessions, err := ListLiveSessions(dir)
	if err != nil {
		t.Fatalf("ListLiveSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1 (duplicate sessionId should collapse to latest): %+v", len(sessions), sessions)
	}
	if sessions[0].UpdatedAt.UnixMilli() != newer {
		t.Errorf("UpdatedAt = %v, want the newer entry (updatedAt=%d)", sessions[0].UpdatedAt, newer)
	}
	if sessions[0].Status != "busy" {
		t.Errorf("Status = %q, want busy (from the newer entry)", sessions[0].Status)
	}
}

func TestLiveSessions_MissingDirectoryReturnsEmptyNotError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")

	sessions, err := ListLiveSessions(dir)
	if err != nil {
		t.Fatalf("ListLiveSessions(missing dir) error = %v, want nil", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d, want 0", len(sessions))
	}
}

func TestLiveSessions_MalformedJSONFileSkipped(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "12345.json"), "{not valid json")

	self := os.Getpid()
	writeSessionRegistryFile(t, dir, self, map[string]any{
		"sessionId": "good-session",
		"cwd":       `D:\ccContextRead`,
		"kind":      "interactive",
		"status":    "idle",
		"updatedAt": time.Now().UnixMilli(),
	})

	sessions, err := ListLiveSessions(dir)
	if err != nil {
		t.Fatalf("ListLiveSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "good-session" {
		t.Fatalf("sessions = %+v, want exactly the well-formed entry", sessions)
	}
}

func TestLiveSessions_BackgroundKindKeepsJobID(t *testing.T) {
	dir := t.TempDir()
	self := os.Getpid()
	writeSessionRegistryFile(t, dir, self, map[string]any{
		"sessionId": "bg-session",
		"cwd":       `D:\ccContextRead`,
		"kind":      "bg",
		"jobId":     "job-42",
		"status":    "busy",
		"updatedAt": time.Now().UnixMilli(),
	})

	sessions, err := ListLiveSessions(dir)
	if err != nil {
		t.Fatalf("ListLiveSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	if sessions[0].Kind != "bg" {
		t.Errorf("Kind = %q, want bg", sessions[0].Kind)
	}
	if sessions[0].JobID != "job-42" {
		t.Errorf("JobID = %q, want job-42", sessions[0].JobID)
	}
}
