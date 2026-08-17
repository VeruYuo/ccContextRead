// Package app hosts the Orchestrator that wires Scanner + Watcher + Tailer
// + Filter + Render into one live-updating pipeline. This file must not
// import Wails: it needs to be fully testable via `go test` without a
// running Wails runtime (PLAN.md T1.12a). The Wails-facing shell lives
// elsewhere and only supplies an Emitter implementation.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ccContextRead/internal/claude"
	"ccContextRead/internal/model"
	"ccContextRead/internal/render"
)

// Emitter abstracts however the app layer notifies the outside world of
// progress, decoupling the Orchestrator from Wails' runtime.EventsEmit.
type Emitter interface {
	Emit(event string, data any)
}

// UpdateEvent is the payload of a "session:updated" Emit.
type UpdateEvent struct {
	SessionID  string
	Markdown   string
	OutputPath string
	EventCount int
	// Full reports whether this write was a full rewrite (initial load or
	// a forced rescan after a filter change) rather than an incremental
	// append (PLAN.md M1 acceptance item 4).
	Full bool
}

// ErrorEvent is the payload of an "error" Emit.
type ErrorEvent struct {
	SessionID string
	Message   string
}

// Option configures an Orchestrator.
type Option func(*orchestratorConfig)

type orchestratorConfig struct {
	pollInterval     time.Duration
	debounceInterval time.Duration
}

// WithPollInterval overrides the Watcher's polling fallback interval
// (default 1s in production; tests use a much shorter one).
func WithPollInterval(d time.Duration) Option {
	return func(c *orchestratorConfig) { c.pollInterval = d }
}

// WithDebounceInterval overrides the write-coalescing delay (PLAN.md 4.5
// point 5: "写入节流合并", default 300ms in production).
func WithDebounceInterval(d time.Duration) Option {
	return func(c *orchestratorConfig) { c.debounceInterval = d }
}

// Orchestrator watches a single Claude Code session file and keeps a
// filtered, rendered Markdown copy of it up to date on disk, emitting a
// "session:updated" event after each write and an "error" event (which
// also stops the watch) if a write ever fails.
type Orchestrator struct {
	emitter Emitter

	pollInterval     time.Duration
	debounceInterval time.Duration

	mu            sync.Mutex
	sessionID     string
	sessionPath   string
	outputPath    string
	filter        model.FilterConfig
	renderOpts    render.Options
	tailer        *claude.Tailer
	records       []claude.Record
	watcher       *claude.Watcher
	debounceTimer *time.Timer
	stopped       bool
}

// NewOrchestrator creates an Orchestrator that reports progress via
// emitter.
func NewOrchestrator(emitter Emitter, opts ...Option) *Orchestrator {
	cfg := orchestratorConfig{
		pollInterval:     1 * time.Second,
		debounceInterval: 300 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Orchestrator{
		emitter:          emitter,
		pollInterval:     cfg.pollInterval,
		debounceInterval: cfg.debounceInterval,
	}
}

// Start begins watching sessionPath and writes the filtered, rendered
// Markdown to outputPath, performing an initial synchronous read/write
// before returning so callers see output immediately rather than waiting
// for the first file-change event.
func (o *Orchestrator) Start(sessionPath, outputPath, sessionID string, filter model.FilterConfig, renderOpts render.Options) error {
	o.mu.Lock()
	o.sessionID = sessionID
	o.sessionPath = sessionPath
	o.outputPath = outputPath
	o.filter = filter
	o.renderOpts = renderOpts
	o.tailer = claude.NewTailer(sessionPath)
	o.records = nil
	o.stopped = false
	o.mu.Unlock()

	if err := o.runPass(); err != nil {
		return fmt.Errorf("orchestrator: initial read of %q: %w", sessionPath, err)
	}

	watcher, err := claude.NewWatcher(filepath.Dir(sessionPath), o.onFileChanged, claude.WithPollInterval(o.pollInterval))
	if err != nil {
		return fmt.Errorf("orchestrator: start watcher: %w", err)
	}

	o.mu.Lock()
	o.watcher = watcher
	o.mu.Unlock()
	return nil
}

// Stop stops watching and releases the Watcher. Safe to call more than
// once, and safe to call even if Start failed partway through.
func (o *Orchestrator) Stop() {
	o.mu.Lock()
	o.stopped = true
	w := o.watcher
	t := o.debounceTimer
	o.watcher = nil
	o.debounceTimer = nil
	o.mu.Unlock()

	if t != nil {
		t.Stop()
	}
	if w != nil {
		w.Close()
	}
}

// UpdateFilter changes the active filter/render options and forces a full
// rewrite (PLAN.md M1 acceptance item 4: toggling a switch must not wait
// for the next file change, and must re-derive the whole document rather
// than just append). Errors are both returned and routed through the
// Emitter's "error" event, consistent with the watch-driven error path.
func (o *Orchestrator) UpdateFilter(filter model.FilterConfig, renderOpts render.Options) error {
	o.mu.Lock()
	o.filter = filter
	o.renderOpts = renderOpts
	if o.tailer != nil {
		o.tailer.ForceFullRescan()
	}
	o.mu.Unlock()

	if err := o.runPass(); err != nil {
		o.handleFatalError(err)
		return err
	}
	return nil
}

func (o *Orchestrator) onFileChanged(path string) {
	o.mu.Lock()
	target := o.sessionPath
	stopped := o.stopped
	o.mu.Unlock()
	if stopped || filepath.Clean(path) != filepath.Clean(target) {
		return
	}
	o.scheduleProcess()
}

// scheduleProcess coalesces bursts of file-change notifications into a
// single processing pass at most once per debounceInterval (PLAN.md 4.5
// point 5).
func (o *Orchestrator) scheduleProcess() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stopped {
		return
	}
	if o.debounceTimer != nil {
		o.debounceTimer.Stop()
	}
	o.debounceTimer = time.AfterFunc(o.debounceInterval, o.process)
}

// process is the debounce timer callback: run one pass and, on failure,
// treat it as fatal (emit "error" and stop watching).
func (o *Orchestrator) process() {
	if err := o.runPass(); err != nil {
		o.handleFatalError(err)
	}
}

// runPass polls the Tailer for new records, re-normalizes/filters/renders
// the accumulated record set, and writes the result to outputPath,
// emitting "session:updated" on success. It is a no-op (no write, no
// emit) when Poll finds nothing new — in particular when the file's tail
// is an incomplete line (4.5 edge case 1).
func (o *Orchestrator) runPass() error {
	o.mu.Lock()
	if o.stopped {
		o.mu.Unlock()
		return nil
	}

	recs, fullRescan, err := o.tailer.Poll()
	if err != nil {
		o.mu.Unlock()
		return err
	}
	if len(recs) == 0 && !fullRescan {
		o.mu.Unlock()
		return nil
	}
	if fullRescan {
		o.records = recs
	} else {
		o.records = append(o.records, recs...)
	}

	events := model.Normalize(o.records)
	header := buildHeader(o.sessionID, o.records, events)
	filtered := model.Apply(events, o.filter)
	sessionID := o.sessionID
	outputPath := o.outputPath
	renderOpts := o.renderOpts
	o.mu.Unlock()

	md, err := render.RenderMarkdown(header, filtered, renderOpts)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, []byte(md), 0o644); err != nil {
		return fmt.Errorf("write output %q: %w", outputPath, err)
	}

	o.emitter.Emit("session:updated", UpdateEvent{
		SessionID:  sessionID,
		Markdown:   md,
		OutputPath: outputPath,
		EventCount: len(filtered),
		Full:       fullRescan,
	})
	return nil
}

// handleFatalError stops the watch and reports err via the Emitter's
// "error" event, exactly once even if called concurrently from multiple
// failed passes.
func (o *Orchestrator) handleFatalError(err error) {
	o.mu.Lock()
	alreadyStopped := o.stopped
	sessionID := o.sessionID
	o.stopped = true
	w := o.watcher
	t := o.debounceTimer
	o.watcher = nil
	o.debounceTimer = nil
	o.mu.Unlock()

	if t != nil {
		t.Stop()
	}
	if w != nil {
		w.Close()
	}
	if alreadyStopped {
		return
	}
	o.emitter.Emit("error", ErrorEvent{SessionID: sessionID, Message: err.Error()})
}

// buildHeader derives the document header from the accumulated records
// (project name, session id, CC version, start/end time) and the
// normalized events (model name), independent of the active filter so
// header fields don't disappear just because e.g. tool-use is hidden.
func buildHeader(sessionID string, records []claude.Record, events []model.Event) render.Header {
	h := render.Header{SessionID: sessionID}
	for _, r := range records {
		if h.ProjectName == "" && r.CWD != "" {
			h.ProjectName = filepath.Base(r.CWD)
		}
		if r.Version != "" {
			h.CCVersion = r.Version
		}
		if !r.Timestamp.IsZero() {
			if h.StartedAt.IsZero() || r.Timestamp.Before(h.StartedAt) {
				h.StartedAt = r.Timestamp
			}
			if r.Timestamp.After(h.EndedAt) {
				h.EndedAt = r.Timestamp
			}
		}
	}
	for _, ev := range events {
		if ev.Model != "" {
			h.Model = ev.Model
		}
	}
	return h
}
