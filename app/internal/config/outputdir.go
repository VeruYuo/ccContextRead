package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// illegalDirChars are the characters Windows forbids in a path component
// (PLAN.md 4.4).
const illegalDirChars = `<>:"/\|?*`

// maxDirNameLen caps a single sanitized path component (by rune) so the
// full output path — <exeDir>\<projectName>-<sessionId8>\<sessionId>.md —
// stays well clear of Windows' historical MAX_PATH even for pathological
// project names.
const maxDirNameLen = 120

// reservedNames are the Windows device names that can't be used as a
// path component regardless of case or trailing extension.
var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// SanitizeDirName makes name safe to use as a single Windows path
// component: illegal characters and control characters become "_", a
// reserved device name (checked case-insensitively against the part
// before the first '.') gets a trailing "_", and the result is
// truncated by rune — never by byte, which would corrupt multi-byte
// text such as CJK — to maxDirNameLen.
func SanitizeDirName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || strings.ContainsRune(illegalDirChars, r) {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimRight(b.String(), " .")
	if out == "" {
		out = "_"
	}

	base := out
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if reservedNames[strings.ToUpper(base)] {
		out += "_"
	}

	if runes := []rune(out); len(runes) > maxDirNameLen {
		out = string(runes[:maxDirNameLen])
	}
	return out
}

// SessionOutputDirName builds the "<项目名>-<sessionId 前 8 位>" directory
// name from PLAN.md 4.4. projectName is sanitized; sessionID is used
// verbatim (session IDs are already filesystem-safe UUIDs) and taken by
// rune, not byte.
func SessionOutputDirName(projectName, sessionID string) string {
	short := sessionID
	if r := []rune(sessionID); len(r) > 8 {
		short = string(r[:8])
	}
	return SanitizeDirName(projectName) + "-" + short
}

// probeWritable reports whether dir can be created and written to. It is
// a package variable so tests can stub it: on Windows os.Chmod does not
// change real ACLs, so the only reliable way to test writability is to
// actually attempt it, and the only reliable way to test the *fallback
// chain* without touching the real filesystem's permissions is to
// replace this function (same approach as claude.permDeniedFS, see
// PLAN.md 12.3).
var probeWritable = defaultProbeWritable

func defaultProbeWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".cccontextread-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// ResolveOutputDir implements the T1.11 fallback chain: prefer exeDir
// (the exe-adjacent directory). If it isn't writable — e.g. the exe
// lives under Program Files — fall back to %APPDATA%\CCContextRead\output.
// If that's unavailable too, return an error rather than silently
// writing to a temp directory the user would never find.
func ResolveOutputDir(exeDir string) (dir string, fellBack bool, err error) {
	if probeErr := probeWritable(exeDir); probeErr == nil {
		return exeDir, false, nil
	}

	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", false, fmt.Errorf("config: exe directory %q is not writable and %%APPDATA%% is not set", exeDir)
	}
	fallback := filepath.Join(appData, "CCContextRead", "output")
	if probeErr := probeWritable(fallback); probeErr == nil {
		return fallback, true, nil
	}
	return "", false, fmt.Errorf("config: neither exe directory %q nor fallback %q is writable", exeDir, fallback)
}

// EffectiveOutputDir returns the directory md files should be written
// to: cfg.OutputDirOverride verbatim if the user set one (never probed —
// an explicit choice is trusted as-is), otherwise the auto-resolved
// exe-adjacent directory per ResolveOutputDir.
func EffectiveOutputDir(cfg Config, exeDir string) (dir string, fellBack bool, err error) {
	if cfg.OutputDirOverride != "" {
		return cfg.OutputDirOverride, false, nil
	}
	return ResolveOutputDir(exeDir)
}
