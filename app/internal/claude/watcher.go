// Package claude: file-system watcher layering fsnotify over a polling
// fallback, since fsnotify misses some write patterns on Windows (10's risk
// table: "Windows fsnotify 不触发").
package claude

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// defaultPollInterval is the production polling fallback cadence mandated
// by 10 ("轮询兜底强制启用，间隔 1 s").
const defaultPollInterval = 1 * time.Second

// WatcherOption configures optional Watcher behavior.
type WatcherOption func(*watcherConfig)

type watcherConfig struct {
	pollInterval time.Duration
}

// WithPollInterval overrides the default polling fallback interval. Mainly
// useful for tests that need a faster (or explicitly very slow, to isolate
// the fsnotify path) cadence than production's 1s default.
func WithPollInterval(d time.Duration) WatcherOption {
	return func(c *watcherConfig) { c.pollInterval = d }
}

// Watcher observes a directory tree (typically <configDir>/projects) for
// created or modified files, invoking a callback with each changed path.
// It combines fsnotify, for low-latency notification, with a polling
// fallback that guarantees changes are discovered even when fsnotify stays
// silent (a documented risk on Windows for some write patterns).
type Watcher struct {
	root     string
	callback func(path string)

	fsw *fsnotify.Watcher // nil in poll-only mode (tests only)

	pollInterval time.Duration
	mtimes       map[string]time.Time
	mtimesMu     sync.Mutex

	stopCh    chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewWatcher creates a Watcher rooted at root, invoking callback whenever a
// file under root is created or modified. Subdirectories present at
// creation time, and any created afterwards, are watched too, so a new
// session's directory is picked up automatically.
func NewWatcher(root string, callback func(path string), opts ...WatcherOption) (*Watcher, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("claude: watch root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("claude: watch root %q is not a directory", root)
	}

	cfg := watcherConfig{pollInterval: defaultPollInterval}
	for _, opt := range opts {
		opt(&cfg)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("claude: create fsnotify watcher: %w", err)
	}

	w := &Watcher{
		root:         root,
		callback:     callback,
		fsw:          fsw,
		pollInterval: cfg.pollInterval,
		mtimes:       make(map[string]time.Time),
		stopCh:       make(chan struct{}),
	}

	if err := w.addTreeToFsnotify(root); err != nil {
		fsw.Close()
		return nil, err
	}
	w.snapshotMtimes()

	w.wg.Add(2)
	go w.runFsnotifyLoop()
	go w.runPollLoop()

	return w, nil
}

// newPollOnlyWatcherForTest builds a Watcher with fsnotify disabled, so
// tests can exercise the polling fallback path in isolation from fsnotify.
func newPollOnlyWatcherForTest(root string, callback func(string), pollInterval time.Duration) *Watcher {
	w := &Watcher{
		root:         root,
		callback:     callback,
		pollInterval: pollInterval,
		mtimes:       make(map[string]time.Time),
		stopCh:       make(chan struct{}),
	}
	w.snapshotMtimes()
	w.wg.Add(1)
	go w.runPollLoop()
	return w
}

// Close stops the Watcher's background goroutines and releases its
// fsnotify handle. Safe to call more than once; only the first call has
// effect.
func (w *Watcher) Close() error {
	var err error
	w.closeOnce.Do(func() {
		close(w.stopCh)
		if w.fsw != nil {
			err = w.fsw.Close()
		}
		w.wg.Wait()
	})
	return err
}

func (w *Watcher) addTreeToFsnotify(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return w.fsw.Add(path)
		}
		return nil
	})
}

func (w *Watcher) runFsnotifyLoop() {
	defer w.wg.Done()
	for {
		select {
		case <-w.stopCh:
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handleFsnotifyEvent(event)
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			// Non-fatal: the polling fallback still covers us even if
			// fsnotify itself misbehaves.
		}
	}
}

func (w *Watcher) handleFsnotifyEvent(event fsnotify.Event) {
	if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
		return
	}
	info, err := os.Stat(event.Name)
	if err != nil {
		return
	}
	if info.IsDir() {
		if event.Op&fsnotify.Create != 0 {
			// A newly created subdirectory (e.g. a new project) needs to
			// be watched itself, and may already contain files created
			// before we could Add() it.
			_ = w.addTreeToFsnotify(event.Name)
			w.notifyExistingFilesUnder(event.Name)
		}
		return
	}
	w.markSeen(event.Name, info.ModTime())
	w.callback(event.Name)
}

func (w *Watcher) notifyExistingFilesUnder(dir string) {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		w.markSeen(path, info.ModTime())
		w.callback(path)
		return nil
	})
}

func (w *Watcher) runPollLoop() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.pollOnce()
		}
	}
}

func (w *Watcher) pollOnce() {
	_ = filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mtime := info.ModTime()

		w.mtimesMu.Lock()
		prev, known := w.mtimes[path]
		changed := !known || mtime.After(prev)
		if changed {
			w.mtimes[path] = mtime
		}
		w.mtimesMu.Unlock()

		if changed {
			w.callback(path)
		}
		return nil
	})
}

func (w *Watcher) markSeen(path string, mtime time.Time) {
	w.mtimesMu.Lock()
	w.mtimes[path] = mtime
	w.mtimesMu.Unlock()
}

// snapshotMtimes records the mtimes of files already present at start-up so
// the first poll only reports genuinely new changes, not pre-existing
// files.
func (w *Watcher) snapshotMtimes() {
	_ = filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		w.mtimes[path] = info.ModTime()
		return nil
	})
}
