package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	ccapp "ccContextRead/internal/app"
	"ccContextRead/internal/claude"
	"ccContextRead/internal/config"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-facing binding shell: every exported method here is a
// thin forward to ccapp.Service. Business logic lives in ccapp.Service so
// it stays testable via `go test` without a running Wails runtime
// (PLAN.md layering rule 1 / T1.12a's "GUI is just a thin shell"
// principle applied to T1.12b).
//
// Wails binds every exported method of a bound struct, so App
// deliberately does *not* implement ccapp.Emitter itself (that would
// expose Emit as a callable frontend method); wailsEmitter below is a
// separate, unbound type for that.
type App struct {
	ctx     context.Context
	svc     *ccapp.Service
	emitter *wailsEmitter
}

// wailsEmitter adapts ccapp.Emitter to runtime.EventsEmit and, for
// "session:updated" payloads, also feeds the Service's status cache so
// GetStatus() has something to return before the frontend's first event
// subscription fires.
type wailsEmitter struct {
	ctx context.Context
	svc *ccapp.Service
}

func (e *wailsEmitter) Emit(event string, data any) {
	if event == "session:updated" {
		if ev, ok := data.(ccapp.UpdateEvent); ok {
			e.svc.RecordUpdate(ev.SessionID, ev.OutputPath, ev.EventCount)
		}
	}
	runtime.EventsEmit(e.ctx, event, data)
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved so we can
// call the runtime methods, and it's when the Service (and its persisted
// config) is wired up.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	configDir, err := claude.ConfigDir()
	if err != nil {
		log.Printf("ccContextRead: resolve Claude config dir: %v", err)
	}
	a.emitter = &wailsEmitter{ctx: ctx}
	a.svc = ccapp.NewService(a.emitter, configDir, exeDir())
	a.emitter.svc = a.svc
	a.svc.LoadConfig()
}

// shutdown stops any active watch so the Watcher/Tailer goroutines and the
// follow-mode ticker don't outlive the window.
func (a *App) shutdown(_ context.Context) {
	a.svc.SetFollowActive(false, nil)
	a.svc.StopWatching()
}

// exeDir returns the directory the running executable lives in, falling
// back to the working directory if it can't be determined (PLAN.md T1.11).
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return filepath.Dir(resolved)
}

// ListSessions lists every session Claude Code has recorded, most
// recently active first.
func (a *App) ListSessions() ([]ccapp.SessionSummary, error) {
	return a.svc.ListSessions()
}

// GetConfig returns the current persisted configuration.
func (a *App) GetConfig() config.Config {
	return a.svc.GetConfig()
}

// SaveConfig persists cfg and, if a session is currently being watched,
// forces an immediate full rewrite.
func (a *App) SaveConfig(cfg config.Config) error {
	return a.svc.SaveConfig(cfg)
}

// OutputDirInfo is EffectiveOutputDir's return value (a struct instead of
// multiple return values so the generated TS binding stays a single
// object rather than a tuple).
type OutputDirInfo struct {
	Dir      string
	FellBack bool
}

// EffectiveOutputDir reports where md files are currently being written.
func (a *App) EffectiveOutputDir() (OutputDirInfo, error) {
	dir, fellBack, err := a.svc.EffectiveOutputDir()
	return OutputDirInfo{Dir: dir, FellBack: fellBack}, err
}

// ChooseOutputDir opens a native folder picker and returns the chosen
// path, or "" if the user cancelled. It's the only place in App that
// calls a Wails dialog directly.
func (a *App) ChooseOutputDir() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择输出目录",
	})
}

// StartWatching starts watching sessionID, stopping any previously
// watched session first.
func (a *App) StartWatching(sessionID string) error {
	return a.svc.StartWatching(sessionID)
}

// StopWatching stops the active watch, if any.
func (a *App) StopWatching() {
	a.svc.StopWatching()
}

// GetStatus returns the current watch status for the frontend's first
// paint, before any "session:updated" event has arrived.
func (a *App) GetStatus() ccapp.StatusInfo {
	return a.svc.Status()
}

// CurrentDocumentResult wraps Service.CurrentDocument's (UpdateEvent, bool)
// pair into a single struct: Wails' generated binding only treats a second
// return value as meaningful when it's an `error` (internal/binding's
// BoundMethod.Call silently drops any other second value), so a plain
// (UpdateEvent, bool) signature would lose Ok on the wire. Same fix as
// OutputDirInfo above.
type CurrentDocumentResult struct {
	Event ccapp.UpdateEvent
	Ok    bool
}

// GetCurrentDocument returns the most-recently rendered document for the
// actively watched session, if any (PLAN.md T1.13): it lets the frontend
// fetch a snapshot on demand — after a tab switch, after selecting a
// session, or after a follow-mode auto-switch — instead of only relying on
// the next "session:updated" event, which may already have fired before the
// frontend was ready to receive it.
func (a *App) GetCurrentDocument() CurrentDocumentResult {
	ev, ok := a.svc.CurrentDocument()
	return CurrentDocumentResult{Event: ev, Ok: ok}
}

// ListLiveSessions reports every currently-live Claude Code session.
func (a *App) ListLiveSessions() ([]claude.LiveSession, error) {
	return a.svc.ListLiveSessions()
}

// SetFollowActive turns "follow the active session" on or off. Switches
// are reported to the frontend via a "follow:changed" event rather than a
// JS callback, since Wails bindings can't carry function values.
func (a *App) SetFollowActive(enabled bool) {
	a.svc.SetFollowActive(enabled, func(sessionID string) {
		a.emitter.Emit("follow:changed", sessionID)
	})
}
