package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ccContextRead/internal/claude"
)

func sessionRecordLine(sessionID, cwd, timestamp, text string) string {
	return `{"type":"user","uuid":"u1","parentUuid":null,"timestamp":"` + timestamp +
		`","sessionId":"` + sessionID + `","cwd":"` + cwd +
		`","message":{"role":"user","content":"` + text + `"}}` + "\n"
}

func writeSessionFile(t *testing.T, configDir, projectSubdir, sessionID, cwd, timestamp, text string) string {
	t.Helper()
	dir := filepath.Join(configDir, "projects", projectSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(sessionRecordLine(sessionID, cwd, timestamp, text)), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func writeLiveRegistry(t *testing.T, configDir string, pid int, sessionID string, updatedAt time.Time) {
	t.Helper()
	dir := filepath.Join(configDir, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := `{"pid":` + itoa(pid) + `,"sessionId":"` + sessionID + `","cwd":"D:/x","kind":"interactive",` +
		`"status":"idle","startedAt":` + itoa64(updatedAt.UnixMilli()) + `,"updatedAt":` + itoa64(updatedAt.UnixMilli()) + `}`
	path := filepath.Join(dir, itoa(pid)+".json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
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

func itoa64(n int64) string {
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

func TestService_ListSessions_SortsByLastActiveDescAndFlagsActive(t *testing.T) {
	configDir := t.TempDir()
	writeSessionFile(t, configDir, "p1", "11111111-1111-4111-8111-111111111111", "D:/ProjOne",
		"2026-08-10T09:00:00.000Z", "older session")
	writeSessionFile(t, configDir, "p2", "22222222-2222-4222-8222-222222222222", "D:/ProjTwo",
		"2026-08-12T09:00:00.000Z", "newer session")
	writeLiveRegistry(t, configDir, os.Getpid(), "22222222-2222-4222-8222-222222222222", time.Now())

	svc := NewService(newFakeEmitter(), configDir, t.TempDir())
	got, err := svc.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListSessions() returned %d sessions, want 2", len(got))
	}
	if got[0].SessionID != "22222222-2222-4222-8222-222222222222" {
		t.Errorf("got[0].SessionID = %q, want the newer session first", got[0].SessionID)
	}
	if !got[0].IsActive {
		t.Errorf("got[0].IsActive = false, want true (matches live registry)")
	}
	if got[1].IsActive {
		t.Errorf("got[1].IsActive = true, want false")
	}
	if got[0].ProjectName != "ProjTwo" {
		t.Errorf("got[0].ProjectName = %q, want %q", got[0].ProjectName, "ProjTwo")
	}
}

func TestService_ListSessions_MissingProjectsDir_ReturnsEmpty(t *testing.T) {
	svc := NewService(newFakeEmitter(), t.TempDir(), t.TempDir())
	got, err := svc.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error = %v, want nil for a missing projects dir", err)
	}
	if len(got) != 0 {
		t.Errorf("ListSessions() = %v, want empty", got)
	}
}

func TestBuildSessionSummaries_SkipsUnreadableFile(t *testing.T) {
	configDir := t.TempDir()
	good := writeSessionFile(t, configDir, "p1", "11111111-1111-4111-8111-111111111111", "D:/ProjOne",
		"2026-08-10T09:00:00.000Z", "fine")
	ghost := filepath.Join(configDir, "projects", "p1", "does-not-exist.jsonl")

	got := buildSessionSummaries([]string{good, ghost}, nil)
	if len(got) != 1 {
		t.Fatalf("buildSessionSummaries() returned %d entries, want 1 (the unreadable file must be skipped)", len(got))
	}
	if got[0].SessionID != "11111111-1111-4111-8111-111111111111" {
		t.Errorf("got[0].SessionID = %q, want the readable session", got[0].SessionID)
	}
}

func TestPickFollowTarget_ChoosesMostRecentlyUpdatedKnownSession(t *testing.T) {
	now := time.Now()
	live := []claude.LiveSession{
		{SessionID: "old", UpdatedAt: now.Add(-1 * time.Minute)},
		{SessionID: "new", UpdatedAt: now},
	}
	sessions := []SessionSummary{{SessionID: "old"}, {SessionID: "new"}}

	got, ok := pickFollowTarget(live, sessions)
	if !ok {
		t.Fatalf("pickFollowTarget() ok = false, want true")
	}
	if got != "new" {
		t.Errorf("pickFollowTarget() = %q, want %q", got, "new")
	}
}

func TestPickFollowTarget_NoLiveSessions(t *testing.T) {
	_, ok := pickFollowTarget(nil, []SessionSummary{{SessionID: "a"}})
	if ok {
		t.Errorf("pickFollowTarget() ok = true, want false when there are no live sessions")
	}
}

func TestPickFollowTarget_LiveSessionHasNoMatchingFile(t *testing.T) {
	live := []claude.LiveSession{{SessionID: "orphan", UpdatedAt: time.Now()}}
	_, ok := pickFollowTarget(live, []SessionSummary{{SessionID: "other"}})
	if ok {
		t.Errorf("pickFollowTarget() ok = true, want false when the live session has no matching on-disk file")
	}
}

func TestService_StartWatching_BuildsSanitizedOutputPath(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeSessionFile(t, configDir, "p1", sessionID, "D:/Fixture<Proj>",
		"2026-08-10T09:00:00.000Z", "hello")

	svc := NewService(newFakeEmitter(), configDir, exeDir)
	if err := svc.StartWatching(sessionID); err != nil {
		t.Fatalf("StartWatching() error = %v", err)
	}
	defer svc.StopWatching()

	wantDir := filepath.Join(exeDir, "Fixture_Proj_-11111111")
	wantPath := filepath.Join(wantDir, sessionID+".md")
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("expected output file at %q, stat error: %v", wantPath, err)
	}
}

func TestService_StartWatching_UnknownSessionID_ReturnsError(t *testing.T) {
	svc := NewService(newFakeEmitter(), t.TempDir(), t.TempDir())
	if err := svc.StartWatching("does-not-exist"); err == nil {
		t.Fatal("StartWatching() error = nil, want error for an unknown session id")
	}
}

func TestService_SaveConfig_PersistsAndRoundTripsThroughLoadConfig(t *testing.T) {
	exeDir := t.TempDir()
	svc := NewService(newFakeEmitter(), t.TempDir(), exeDir)

	cfg := svc.GetConfig()
	cfg.ImageMode = 1 // render.ImageAttachment
	cfg.Filter.ToolUse = true
	if err := svc.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	fresh := NewService(newFakeEmitter(), t.TempDir(), exeDir)
	got := fresh.LoadConfig()
	if got.ImageMode != 1 {
		t.Errorf("LoadConfig().ImageMode = %v, want 1", got.ImageMode)
	}
	if !got.Filter.ToolUse {
		t.Errorf("LoadConfig().Filter.ToolUse = false, want true")
	}
}

func TestService_SaveConfig_WhileWatching_ForcesFullRewrite(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeSessionFile(t, configDir, "p1", sessionID, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "hello")

	emitter := newFakeEmitter()
	svc := NewService(emitter, configDir, exeDir)
	if err := svc.StartWatching(sessionID); err != nil {
		t.Fatalf("StartWatching() error = %v", err)
	}
	defer svc.StopWatching()
	<-emitter.updates // drain the initial synchronous pass

	cfg := svc.GetConfig()
	cfg.Filter.ToolUse = true
	if err := svc.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	select {
	case ev := <-emitter.updates:
		if !ev.Full {
			t.Errorf("Full = false, want true (a filter change must force a full rewrite)")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the update triggered by SaveConfig")
	}
}

func TestService_EffectiveOutputDir_ReflectsOverride(t *testing.T) {
	exeDir := t.TempDir()
	override := t.TempDir()
	svc := NewService(newFakeEmitter(), t.TempDir(), exeDir)

	cfg := svc.GetConfig()
	cfg.OutputDirOverride = override
	if err := svc.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	dir, fellBack, err := svc.EffectiveOutputDir()
	if err != nil {
		t.Fatalf("EffectiveOutputDir() error = %v", err)
	}
	if dir != override {
		t.Errorf("EffectiveOutputDir() dir = %q, want %q", dir, override)
	}
	if fellBack {
		t.Errorf("EffectiveOutputDir() fellBack = true, want false for an explicit override")
	}
}

func TestService_Status_ReflectsRecordUpdateAndResetsOnStop(t *testing.T) {
	svc := NewService(newFakeEmitter(), t.TempDir(), t.TempDir())
	svc.RecordUpdate("sess1", "out.md", 3)

	got := svc.Status()
	if !got.Watching || got.SessionID != "sess1" || got.OutputPath != "out.md" || got.EventCount != 3 {
		t.Errorf("Status() = %+v, want Watching=true SessionID=sess1 OutputPath=out.md EventCount=3", got)
	}

	svc.StopWatching()
	if got := svc.Status(); got.Watching || got.SessionID != "" {
		t.Errorf("Status() after StopWatching() = %+v, want the zero value", got)
	}
}

func TestService_ListLiveSessions_EmptyWhenRegistryMissing(t *testing.T) {
	svc := NewService(newFakeEmitter(), t.TempDir(), t.TempDir())
	got, err := svc.ListLiveSessions()
	if err != nil {
		t.Fatalf("ListLiveSessions() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListLiveSessions() = %v, want empty", got)
	}
}

func TestService_SetFollowActive_SwitchesToMostRecentLiveKnownSession(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeSessionFile(t, configDir, "p1", sessionID, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "hello")
	writeLiveRegistry(t, configDir, os.Getpid(), sessionID, time.Now())

	svc := NewService(newFakeEmitter(), configDir, exeDir, WithFollowInterval(30*time.Millisecond))
	defer svc.StopWatching()

	switched := make(chan string, 8)
	svc.SetFollowActive(true, func(id string) { switched <- id })
	defer svc.SetFollowActive(false, nil)

	select {
	case id := <-switched:
		if id != sessionID {
			t.Errorf("onSwitch id = %q, want %q", id, sessionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for follow mode to switch to the live session")
	}
	if got := svc.Status().SessionID; got != sessionID {
		t.Errorf("Status().SessionID = %q, want %q", got, sessionID)
	}
}

func TestService_CurrentDocument_OkFalseBeforeWatching(t *testing.T) {
	svc := NewService(newFakeEmitter(), t.TempDir(), t.TempDir())
	_, ok := svc.CurrentDocument("11111111-1111-4111-8111-111111111111")
	if ok {
		t.Error("CurrentDocument() ok = true before StartWatching, want false")
	}
}

func TestService_CurrentDocument_OkFalseAfterStopWatching(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeSessionFile(t, configDir, "p1", sessionID, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "hello")

	emitter := newFakeEmitter()
	svc := NewService(emitter, configDir, exeDir)
	if err := svc.StartWatching(sessionID); err != nil {
		t.Fatalf("StartWatching() error = %v", err)
	}
	<-emitter.updates
	svc.StopWatching()

	_, ok := svc.CurrentDocument(sessionID)
	if ok {
		t.Error("CurrentDocument() ok = true after StopWatching, want false")
	}
}

func TestService_CurrentDocument_ReturnsMostRecentMarkdown(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeSessionFile(t, configDir, "p1", sessionID, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "current document content")

	emitter := newFakeEmitter()
	svc := NewService(emitter, configDir, exeDir)
	if err := svc.StartWatching(sessionID); err != nil {
		t.Fatalf("StartWatching() error = %v", err)
	}
	defer svc.StopWatching()
	<-emitter.updates

	ev, ok := svc.CurrentDocument(sessionID)
	if !ok {
		t.Fatal("CurrentDocument() ok = false after StartWatching, want true")
	}
	if ev.SessionID != sessionID {
		t.Errorf("CurrentDocument().SessionID = %q, want %q", ev.SessionID, sessionID)
	}
	if ev.Markdown == "" {
		t.Error("CurrentDocument().Markdown is empty")
	}
}

func TestService_CurrentDocument_SwitchSession_ReturnsNewSessionDoc(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sid1 := "11111111-1111-4111-8111-111111111111"
	sid2 := "22222222-2222-4222-8222-222222222222"
	writeSessionFile(t, configDir, "p1", sid1, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "session one content")
	writeSessionFile(t, configDir, "p2", sid2, "D:/ProjTwo", "2026-08-11T09:00:00.000Z", "session two content")

	emitter := newFakeEmitter()
	svc := NewService(emitter, configDir, exeDir)

	if err := svc.StartWatching(sid1); err != nil {
		t.Fatalf("StartWatching(sid1) error = %v", err)
	}
	<-emitter.updates

	if err := svc.StartWatching(sid2); err != nil {
		t.Fatalf("StartWatching(sid2) error = %v", err)
	}
	defer svc.StopWatching()
	<-emitter.updates

	ev, ok := svc.CurrentDocument(sid2)
	if !ok {
		t.Fatal("CurrentDocument() ok = false after switching sessions, want true")
	}
	if ev.SessionID != sid2 {
		t.Errorf("CurrentDocument().SessionID = %q, want %q (new session)", ev.SessionID, sid2)
	}
}

// TestService_CurrentDocument_OkFalseOnSessionIDMismatch covers PLAN.md
// 12.2.1 问题③ fix 4: a caller asking for a session other than the one
// actually being watched must get ok=false, not whatever document happens
// to be loaded. This is what lets the frontend tell "the backend hasn't
// caught up to my StartWatching yet" apart from "here's your data".
func TestService_CurrentDocument_OkFalseOnSessionIDMismatch(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sid1 := "11111111-1111-4111-8111-111111111111"
	sid2 := "22222222-2222-4222-8222-222222222222"
	writeSessionFile(t, configDir, "p1", sid1, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "session one content")

	emitter := newFakeEmitter()
	svc := NewService(emitter, configDir, exeDir)
	if err := svc.StartWatching(sid1); err != nil {
		t.Fatalf("StartWatching() error = %v", err)
	}
	defer svc.StopWatching()
	<-emitter.updates

	// sid2 was never watched, so asking for it while sid1 is active must
	// not return sid1's document.
	if _, ok := svc.CurrentDocument(sid2); ok {
		t.Error("CurrentDocument(sid2) ok = true while watching sid1, want false")
	}
	// sid1 itself must still work — the check rejects mismatches, not every
	// request.
	if ev, ok := svc.CurrentDocument(sid1); !ok || ev.SessionID != sid1 {
		t.Errorf("CurrentDocument(sid1) = %+v, %v, want matching sid1 doc, true", ev, ok)
	}
}

func TestService_SessionsChanged_EmittedOnStartWatching(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeSessionFile(t, configDir, "p1", sessionID, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "hello")

	emitter := newFakeEmitter()
	svc := NewService(emitter, configDir, exeDir)
	if err := svc.StartWatching(sessionID); err != nil {
		t.Fatalf("StartWatching() error = %v", err)
	}
	defer svc.StopWatching()

	select {
	case <-emitter.sessionsChanged:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sessions:changed after StartWatching")
	}
}

func TestService_SessionsChanged_EmittedOnStopWatching(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeSessionFile(t, configDir, "p1", sessionID, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "hello")

	emitter := newFakeEmitter()
	svc := NewService(emitter, configDir, exeDir)
	if err := svc.StartWatching(sessionID); err != nil {
		t.Fatalf("StartWatching() error = %v", err)
	}
	// drain the StartWatching event
	select {
	case <-emitter.sessionsChanged:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sessions:changed after StartWatching")
	}

	svc.StopWatching()

	select {
	case <-emitter.sessionsChanged:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sessions:changed after StopWatching")
	}
}

func TestService_SessionsChanged_EmittedOnFollowSwitch(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeSessionFile(t, configDir, "p1", sessionID, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "hello")
	writeLiveRegistry(t, configDir, os.Getpid(), sessionID, time.Now())

	emitter := newFakeEmitter()
	svc := NewService(emitter, configDir, exeDir, WithFollowInterval(30*time.Millisecond))
	defer svc.StopWatching()

	svc.SetFollowActive(true, nil)
	defer svc.SetFollowActive(false, nil)

	select {
	case <-emitter.sessionsChanged:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sessions:changed after follow auto-switch")
	}
}

func TestService_CurrentDocument_ConcurrentStartWatching_NoDeadlock(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeSessionFile(t, configDir, "p1", sessionID, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "hello")

	emitter := newFakeEmitter()
	svc := NewService(emitter, configDir, exeDir)
	if err := svc.StartWatching(sessionID); err != nil {
		t.Fatalf("StartWatching() error = %v", err)
	}
	defer svc.StopWatching()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			svc.CurrentDocument(sessionID)
		}
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock detected: CurrentDocument did not return within 3s under concurrent StartWatching")
	}
}

func TestService_SetFollowActive_DisablingStopsFurtherSwitches(t *testing.T) {
	configDir := t.TempDir()
	exeDir := t.TempDir()
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeSessionFile(t, configDir, "p1", sessionID, "D:/ProjOne", "2026-08-10T09:00:00.000Z", "hello")
	writeLiveRegistry(t, configDir, os.Getpid(), sessionID, time.Now())

	svc := NewService(newFakeEmitter(), configDir, exeDir, WithFollowInterval(30*time.Millisecond))
	defer svc.StopWatching()

	switched := make(chan string, 8)
	svc.SetFollowActive(true, func(id string) { switched <- id })
	select {
	case <-switched:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first switch")
	}
	svc.SetFollowActive(false, nil)

	// Drain anything already in flight, then make sure nothing new shows up.
	drain := true
	for drain {
		select {
		case <-switched:
		default:
			drain = false
		}
	}
	select {
	case id := <-switched:
		t.Errorf("received an onSwitch(%q) call after disabling follow mode", id)
	case <-time.After(200 * time.Millisecond):
	}
}
