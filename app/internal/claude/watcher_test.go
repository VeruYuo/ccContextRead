package claude

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// waitForPath blocks until ch delivers want, or fails the test after timeout.
func waitForPath(t *testing.T, ch <-chan string, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case got := <-ch:
			if filepath.Clean(got) == filepath.Clean(want) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out after %v waiting for event on %q", timeout, want)
		}
	}
}

func TestWatcher_FsnotifyEventTriggersCallback(t *testing.T) {
	dir := t.TempDir()
	events := make(chan string, 16)

	// A very long poll interval isolates this test to the fsnotify path:
	// if the event arrives within the short timeout below, polling cannot
	// be the source.
	w, err := NewWatcher(dir, func(path string) { events <- path }, WithPollInterval(10*time.Second))
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	target := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, target, `{"type":"user"}`+"\n")

	waitForPath(t, events, target, 3*time.Second)
}

func TestWatcher_PollFallbackDiscoversChangeWithinTwoSeconds(t *testing.T) {
	dir := t.TempDir()
	events := make(chan string, 16)

	// Poll-only watcher simulates "fsnotify 静默": real fsnotify silence
	// can't be induced deterministically across platforms, so this
	// exercises the polling path directly per 4.5/10.
	w := newPollOnlyWatcherForTest(dir, func(path string) { events <- path }, 200*time.Millisecond)
	defer w.Close()

	target := filepath.Join(dir, "session.jsonl")
	mustWriteFile(t, target, `{"type":"user"}`+"\n")

	waitForPath(t, events, target, 2*time.Second)
}

func TestWatcher_NewFileInWatchedDirectoryDiscovered(t *testing.T) {
	dir := t.TempDir()
	events := make(chan string, 16)

	w, err := NewWatcher(dir, func(path string) { events <- path }, WithPollInterval(200*time.Millisecond))
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	target := filepath.Join(dir, "new-session.jsonl")
	mustWriteFile(t, target, `{"type":"user"}`+"\n")

	waitForPath(t, events, target, 3*time.Second)
}

func TestWatcher_NewNestedDirectoryAndFileDiscovered(t *testing.T) {
	dir := t.TempDir()
	events := make(chan string, 16)

	w, err := NewWatcher(dir, func(path string) { events <- path }, WithPollInterval(200*time.Millisecond))
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	sub := filepath.Join(dir, "D--NewProject")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	target := filepath.Join(sub, "session.jsonl")
	mustWriteFile(t, target, `{"type":"user"}`+"\n")

	waitForPath(t, events, target, 3*time.Second)
}

func TestWatcher_NewWatcherMissingRootReturnsError(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")

	if _, err := NewWatcher(missing, func(string) {}); err == nil {
		t.Fatal("NewWatcher(missing root) = nil error, want non-nil")
	}
}

func TestWatcher_CloseLeavesNoGoroutineLeak(t *testing.T) {
	dir := t.TempDir()

	runtime.GC()
	baseline := runtime.NumGoroutine()

	w, err := NewWatcher(dir, func(string) {}, WithPollInterval(50*time.Millisecond))
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // let both internal goroutines actually start
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		runtime.GC()
		if n := runtime.NumGoroutine(); n <= baseline {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutine count did not return to baseline %d within timeout, last=%d", baseline, runtime.NumGoroutine())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
