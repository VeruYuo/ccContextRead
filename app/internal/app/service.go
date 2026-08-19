package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"ccContextRead/internal/claude"
	"ccContextRead/internal/config"
	"ccContextRead/internal/render"
)

// SessionSummary is the lightweight, UI-facing view of a session: enough
// to list and label it without a full parse (PLAN.md T1.12b session list).
type SessionSummary struct {
	SessionID    string
	ProjectDir   string
	ProjectName  string
	Title        string
	Path         string
	CreatedAt    time.Time
	LastActiveAt time.Time
	IsActive     bool
}

// StatusInfo is the current watch status, used both for the first paint
// (before any "session:updated" event has arrived) and to answer a direct
// status query from the frontend.
type StatusInfo struct {
	Watching      bool
	SessionID     string
	OutputPath    string
	EventCount    int
	LastUpdatedAt time.Time
}

// Service is the Wails-independent GUI backing layer: it turns the
// already-tested Orchestrator (T1.12a) into session listing, config
// persistence, and watch lifecycle operations the Wails-facing App struct
// can forward to almost verbatim. It has zero Wails dependency and is
// fully testable via `go test` (PLAN.md layering rule 1).
type Service struct {
	emitter   Emitter
	configDir string
	exeDir    string

	mu     sync.Mutex
	cfg    config.Config
	orch   *Orchestrator
	status StatusInfo

	followMu       sync.Mutex
	followStop     func()
	followInterval time.Duration
}

// ServiceOption configures a Service.
type ServiceOption func(*Service)

// WithFollowInterval overrides how often SetFollowActive polls for a
// change in which session is live (default 3s in production; tests use a
// much shorter one).
func WithFollowInterval(d time.Duration) ServiceOption {
	return func(s *Service) { s.followInterval = d }
}

// NewService creates a Service backed by configDir (Claude Code's config
// directory, typically claude.ConfigDir()) and exeDir (the directory the
// running exe lives in, used for T1.11's output-dir fallback chain). It
// starts with default config; call LoadConfig to read any persisted
// settings from disk.
func NewService(emitter Emitter, configDir, exeDir string, opts ...ServiceOption) *Service {
	s := &Service{emitter: emitter, configDir: configDir, exeDir: exeDir, cfg: config.Default(), followInterval: 3 * time.Second}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// settingsFilePath resolves where the app's own settings.json lives: next
// to the session output (same writable base directory ResolveOutputDir
// picks for md files), independent of any per-session OutputDirOverride.
func (s *Service) settingsFilePath() (string, error) {
	base, _, err := config.ResolveOutputDir(s.exeDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "settings.json"), nil
}

// LoadConfig reads persisted settings from disk (falling back to defaults
// per config.Load's contract) and returns the resulting config.
func (s *Service) LoadConfig() config.Config {
	if path, err := s.settingsFilePath(); err == nil {
		s.mu.Lock()
		s.cfg = config.Load(path)
		s.mu.Unlock()
	}
	return s.GetConfig()
}

// GetConfig returns the current in-memory config.
func (s *Service) GetConfig() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// SaveConfig persists cfg and, if a session is currently being watched,
// forces an immediate full rewrite so a filter change is visible right
// away (PLAN.md M1 acceptance item 4) rather than waiting for the next
// file change.
func (s *Service) SaveConfig(cfg config.Config) error {
	path, err := s.settingsFilePath()
	if err != nil {
		return err
	}
	if err := config.Save(path, cfg); err != nil {
		return err
	}

	s.mu.Lock()
	s.cfg = cfg
	orch := s.orch
	s.mu.Unlock()

	if orch != nil {
		return orch.UpdateFilter(cfg.Filter, renderOptsFor(cfg))
	}
	return nil
}

// EffectiveOutputDir reports where md files are currently being written
// (or would be written), including whether T1.11's fallback kicked in.
func (s *Service) EffectiveOutputDir() (dir string, fellBack bool, err error) {
	return config.EffectiveOutputDir(s.GetConfig(), s.exeDir)
}

// ChooseOutputDir opens a native folder picker. It is implemented by the
// Wails-facing App (the only place allowed to touch runtime.*); Service
// itself has no dialog of its own to offer, hence no method here.

// ListSessions enumerates every session under configDir/projects and
// cross-references ~/.claude/sessions for liveness, sorted by most
// recently active first. A missing projects directory (fresh machine, or
// CLAUDE_CONFIG_DIR pointed elsewhere) is not an error: it just means
// there is nothing to list yet.
func (s *Service) ListSessions() ([]SessionSummary, error) {
	files, err := claude.ListSessionFiles(s.configDir)
	if err != nil {
		if errors.Is(err, claude.ErrProjectsDirNotFound) {
			return nil, nil
		}
		return nil, err
	}

	live, _ := claude.ListLiveSessions(filepath.Join(s.configDir, "sessions"))
	return buildSessionSummaries(files, live), nil
}

// buildSessionSummaries turns session file paths into SessionSummary DTOs,
// flagging liveness from live and sorting most-recently-active first. A
// file that can no longer be read (removed or corrupted between
// enumeration and this call) is skipped rather than failing the whole
// list.
func buildSessionSummaries(files []string, live []claude.LiveSession) []SessionSummary {
	active := make(map[string]bool, len(live))
	for _, l := range live {
		active[l.SessionID] = true
	}

	out := make([]SessionSummary, 0, len(files))
	for _, f := range files {
		meta, err := claude.ExtractMetadata(f)
		if err != nil {
			continue
		}
		out = append(out, SessionSummary{
			SessionID:    meta.SessionID,
			ProjectDir:   meta.ProjectDir,
			ProjectName:  filepath.Base(meta.ProjectDir),
			Title:        meta.Title,
			Path:         meta.Path,
			CreatedAt:    meta.CreatedAt,
			LastActiveAt: meta.LastActiveAt,
			IsActive:     active[meta.SessionID],
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].LastActiveAt.After(out[j].LastActiveAt)
	})
	return out
}

// StartWatching resolves sessionID to its on-disk file, computes the
// sanitized output path per PLAN.md 4.4, and starts watching it, stopping
// any previously watched session first (Service watches at most one
// session at a time, matching the Orchestrator's single-session design).
func (s *Service) StartWatching(sessionID string) error {
	sessions, err := s.ListSessions()
	if err != nil {
		return err
	}
	var target *SessionSummary
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			target = &sessions[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("app: unknown session %q", sessionID)
	}

	cfg := s.GetConfig()
	baseDir, _, err := config.EffectiveOutputDir(cfg, s.exeDir)
	if err != nil {
		return err
	}
	outDir := filepath.Join(baseDir, config.SessionOutputDirName(target.ProjectName, sessionID))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	outputPath := filepath.Join(outDir, sessionID+".md")

	s.mu.Lock()
	prev := s.orch
	s.orch = nil
	s.mu.Unlock()
	if prev != nil {
		prev.Stop()
	}

	orch := NewOrchestrator(s.emitter)
	if err := orch.Start(target.Path, outputPath, sessionID, cfg.Filter, renderOptsFor(cfg)); err != nil {
		return err
	}

	s.mu.Lock()
	s.orch = orch
	s.status = StatusInfo{Watching: true, SessionID: sessionID, OutputPath: outputPath}
	s.mu.Unlock()
	s.emitter.Emit("sessions:changed", nil)
	return nil
}

// StopWatching stops the active watch, if any. Safe to call even when
// nothing is being watched.
func (s *Service) StopWatching() {
	s.mu.Lock()
	orch := s.orch
	s.orch = nil
	s.status = StatusInfo{}
	s.mu.Unlock()
	if orch != nil {
		orch.Stop()
	}
	s.emitter.Emit("sessions:changed", nil)
}

// CurrentDocument returns the most-recently rendered document for the
// actively watched session, if any. ok=false means either nothing is being
// watched (StartWatching not yet called, or StopWatching was called) or the
// active watch has not completed a single render pass.
func (s *Service) CurrentDocument() (UpdateEvent, bool) {
	s.mu.Lock()
	orch := s.orch
	s.mu.Unlock()
	if orch == nil {
		return UpdateEvent{}, false
	}
	return orch.LastUpdate()
}

// Status returns the last known watch status, for the frontend's first
// paint before any "session:updated" event has arrived.
func (s *Service) Status() StatusInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// RecordUpdate lets the Wails-facing App feed Orchestrator "session:updated"
// payloads back into Status() without Service depending on the Orchestrator
// event payload type directly (it already doesn't import Wails, but this
// keeps event plumbing entirely in the App shell).
func (s *Service) RecordUpdate(sessionID, outputPath string, eventCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = StatusInfo{
		Watching:      true,
		SessionID:     sessionID,
		OutputPath:    outputPath,
		EventCount:    eventCount,
		LastUpdatedAt: time.Now(),
	}
}

// ListLiveSessions reports every currently-live Claude Code session
// (PLAN.md 2.6), for the "follow active session" feature.
func (s *Service) ListLiveSessions() ([]claude.LiveSession, error) {
	return claude.ListLiveSessions(filepath.Join(s.configDir, "sessions"))
}

// SetFollowActive turns "follow the active session" on or off. While on,
// it polls live sessions every followInterval and switches the watch to
// whichever known session was most recently updated, invoking onSwitch
// with the new session id whenever it actually switches.
func (s *Service) SetFollowActive(enabled bool, onSwitch func(sessionID string)) {
	s.followMu.Lock()
	defer s.followMu.Unlock()

	if s.followStop != nil {
		s.followStop()
		s.followStop = nil
	}
	if !enabled {
		return
	}

	stop := make(chan struct{})
	s.followStop = sync.OnceFunc(func() { close(stop) })

	go func() {
		ticker := time.NewTicker(s.followInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s.tickFollow(onSwitch)
			}
		}
	}()
}

func (s *Service) tickFollow(onSwitch func(sessionID string)) {
	live, err := s.ListLiveSessions()
	if err != nil || len(live) == 0 {
		return
	}
	sessions, err := s.ListSessions()
	if err != nil {
		return
	}
	target, ok := pickFollowTarget(live, sessions)
	if !ok {
		return
	}
	if s.Status().SessionID == target {
		return
	}
	if err := s.StartWatching(target); err != nil {
		return
	}
	if onSwitch != nil {
		onSwitch(target)
	}
}

// pickFollowTarget picks which known session (one that actually has an
// on-disk file, i.e. appears in sessions) the "follow active session"
// feature should switch to: the live session with the most recent
// UpdatedAt among those with a matching file. Returns ok=false if there's
// no such candidate — either no live sessions at all, or none of them
// correspond to a file ListSessions can see.
func pickFollowTarget(live []claude.LiveSession, sessions []SessionSummary) (string, bool) {
	known := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		known[s.SessionID] = true
	}

	var best claude.LiveSession
	found := false
	for _, l := range live {
		if !known[l.SessionID] {
			continue
		}
		if !found || l.UpdatedAt.After(best.UpdatedAt) {
			best = l
			found = true
		}
	}
	if !found {
		return "", false
	}
	return best.SessionID, true
}

func renderOptsFor(cfg config.Config) render.Options {
	return render.Options{ImageMode: cfg.ImageMode, FileChange: cfg.Filter.FileChange}
}
