package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeDirName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"illegal chars replaced", `a<b>c:d"e/f\g|h?i*j`, "a_b_c_d_e_f_g_h_i_j"},
		{"reserved name exact", "CON", "CON_"},
		{"reserved name case-insensitive", "con", "con_"},
		{"reserved name with extension", "NUL.txt", "NUL.txt_"},
		{"non-reserved name kept as-is", "myproject", "myproject"},
		{"trailing dots and spaces trimmed", "abc. ", "abc"},
		{"empty becomes underscore", "", "_"},
		{"only illegal chars becomes underscores not empty", "<<<", "___"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeDirName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeDirName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeDirNameTruncatesLongNames(t *testing.T) {
	long := strings.Repeat("a", maxDirNameLen+50)
	got := SanitizeDirName(long)
	if n := len([]rune(got)); n != maxDirNameLen {
		t.Fatalf("len(SanitizeDirName(long)) = %d, want %d", n, maxDirNameLen)
	}
}

func TestSanitizeDirNameTruncatesByRuneNotByte(t *testing.T) {
	// Each CJK rune is 3 bytes in UTF-8; truncating by byte would split
	// one in half and produce invalid UTF-8 (PLAN.md easy-to-miss point 8).
	long := strings.Repeat("中", maxDirNameLen+50)
	got := SanitizeDirName(long)
	if n := len([]rune(got)); n != maxDirNameLen {
		t.Fatalf("len([]rune(SanitizeDirName(long))) = %d, want %d", n, maxDirNameLen)
	}
	for i, r := range got {
		if r == '\uFFFD' {
			t.Fatalf("SanitizeDirName produced invalid UTF-8 at byte %d: %q", i, got)
		}
	}
}

func TestSessionOutputDirName(t *testing.T) {
	tests := []struct {
		name      string
		project   string
		sessionID string
		want      string
	}{
		{"normal", "ccContextRead", "ca44ff06-1234-5678-9abc-def012345678", "ccContextRead-ca44ff06"},
		{"short session id kept whole", "proj", "abc", "proj-abc"},
		{"illegal chars in project name sanitized", `my<proj>`, "12345678xxxx", "my_proj_-12345678"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SessionOutputDirName(tt.project, tt.sessionID)
			if got != tt.want {
				t.Errorf("SessionOutputDirName(%q, %q) = %q, want %q", tt.project, tt.sessionID, got, tt.want)
			}
		})
	}
}

func withStubProbe(t *testing.T, stub func(dir string) error) {
	t.Helper()
	orig := probeWritable
	probeWritable = stub
	t.Cleanup(func() { probeWritable = orig })
}

func TestResolveOutputDir_ExeDirWritable(t *testing.T) {
	withStubProbe(t, func(dir string) error { return nil })

	dir, fellBack, err := ResolveOutputDir(`D:\app`)
	if err != nil {
		t.Fatalf("ResolveOutputDir: %v", err)
	}
	if fellBack {
		t.Fatalf("fellBack = true, want false")
	}
	if dir != `D:\app` {
		t.Fatalf("dir = %q, want %q", dir, `D:\app`)
	}
}

func TestResolveOutputDir_FallsBackToAppData(t *testing.T) {
	t.Setenv("APPDATA", `C:\Users\test\AppData\Roaming`)
	withStubProbe(t, func(dir string) error {
		if dir == `D:\app` {
			return errors.New("permission denied")
		}
		return nil
	})

	dir, fellBack, err := ResolveOutputDir(`D:\app`)
	if err != nil {
		t.Fatalf("ResolveOutputDir: %v", err)
	}
	if !fellBack {
		t.Fatalf("fellBack = false, want true")
	}
	want := filepath.Join(`C:\Users\test\AppData\Roaming`, "CCContextRead", "output")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}

func TestResolveOutputDir_NoAppDataReturnsError(t *testing.T) {
	t.Setenv("APPDATA", "")
	withStubProbe(t, func(dir string) error { return errors.New("permission denied") })

	_, _, err := ResolveOutputDir(`D:\app`)
	if err == nil {
		t.Fatal("expected error when exe dir unwritable and APPDATA unset, got nil")
	}
}

func TestResolveOutputDir_AppDataAlsoUnwritableReturnsError(t *testing.T) {
	t.Setenv("APPDATA", `C:\Users\test\AppData\Roaming`)
	withStubProbe(t, func(dir string) error { return errors.New("permission denied") })

	dir, fellBack, err := ResolveOutputDir(`D:\app`)
	if err == nil {
		t.Fatalf("expected error when neither location is writable, got dir=%q fellBack=%v", dir, fellBack)
	}
}

func TestEffectiveOutputDir_OverrideWins(t *testing.T) {
	// Even if probing would fail, an explicit override must be used
	// verbatim and never probed (PLAN.md T1.11: "未指定输出目录时"
	// only applies to the auto-resolve path).
	withStubProbe(t, func(dir string) error { return errors.New("should not be called") })

	cfg := Config{OutputDirOverride: `E:\custom\out`}
	dir, fellBack, err := EffectiveOutputDir(cfg, `D:\app`)
	if err != nil {
		t.Fatalf("EffectiveOutputDir: %v", err)
	}
	if fellBack {
		t.Fatalf("fellBack = true, want false")
	}
	if dir != `E:\custom\out` {
		t.Fatalf("dir = %q, want override %q", dir, `E:\custom\out`)
	}
}

func TestEffectiveOutputDir_AutoResolvesWhenUnset(t *testing.T) {
	withStubProbe(t, func(dir string) error { return nil })

	cfg := Config{}
	dir, fellBack, err := EffectiveOutputDir(cfg, `D:\app`)
	if err != nil {
		t.Fatalf("EffectiveOutputDir: %v", err)
	}
	if fellBack {
		t.Fatalf("fellBack = true, want false")
	}
	if dir != `D:\app` {
		t.Fatalf("dir = %q, want exe dir %q", dir, `D:\app`)
	}
}

// defaultProbeWritable is exercised directly (not via the stub) so the
// real filesystem probing logic — not just its interface — is covered.
func TestDefaultProbeWritable_WritableDir(t *testing.T) {
	dir := t.TempDir()
	if err := defaultProbeWritable(dir); err != nil {
		t.Fatalf("defaultProbeWritable(%q) = %v, want nil", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("probe file left behind: %v", entries)
	}
}

func TestDefaultProbeWritable_CreatesMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-yet-created")
	if err := defaultProbeWritable(dir); err != nil {
		t.Fatalf("defaultProbeWritable(%q) = %v, want nil", dir, err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("expected dir to be created: %v", err)
	}
}

func TestDefaultProbeWritable_ParentIsFileReturnsError(t *testing.T) {
	// MkdirAll on a path whose parent component is a regular file must
	// fail rather than silently succeeding.
	base := t.TempDir()
	filePath := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filePath, "child")
	if err := defaultProbeWritable(target); err == nil {
		t.Fatalf("defaultProbeWritable(%q) = nil, want error (parent is a file)", target)
	}
}

// End-to-end: ResolveOutputDir wired to the real defaultProbeWritable,
// no stub, against an actually-writable temp directory.
func TestResolveOutputDir_RealFilesystemExeDirWritable(t *testing.T) {
	dir := t.TempDir()
	got, fellBack, err := ResolveOutputDir(dir)
	if err != nil {
		t.Fatalf("ResolveOutputDir: %v", err)
	}
	if fellBack {
		t.Fatalf("fellBack = true, want false")
	}
	if got != dir {
		t.Fatalf("dir = %q, want %q", got, dir)
	}
}
