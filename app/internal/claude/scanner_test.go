package claude

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"testing/fstest"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", full, err)
		}
	}
}

func TestListSessionFiles_RecursiveEnumerationAndExclusions(t *testing.T) {
	configDir := t.TempDir()
	writeFiles(t, filepath.Join(configDir, "projects"), map[string]string{
		"D--FixtureProject/11111111-1111-4111-8111-111111111111.jsonl":                          "{}",
		"D--FixtureProject/22222222-2222-4222-8222-222222222222.jsonl":                          "{}",
		"D--FixtureProject/11111111-1111-4111-8111-111111111111/subagents/agent-aaaa.jsonl":     "{}",
		"D--FixtureProject/11111111-1111-4111-8111-111111111111/subagents/agent-aaaa.meta.json": "{}",
		"E--OtherProject/33333333-3333-4333-8333-333333333333.jsonl":                             "{}",
		"E--OtherProject/notes.txt":                                                              "not jsonl",
		"E--OtherProject/agent-orphan.jsonl":                                                      "{}",
	})

	got, err := ListSessionFiles(configDir)
	if err != nil {
		t.Fatalf("ListSessionFiles() error = %v", err)
	}

	want := []string{
		filepath.Join(configDir, "projects", "D--FixtureProject", "11111111-1111-4111-8111-111111111111.jsonl"),
		filepath.Join(configDir, "projects", "D--FixtureProject", "22222222-2222-4222-8222-222222222222.jsonl"),
		filepath.Join(configDir, "projects", "E--OtherProject", "33333333-3333-4333-8333-333333333333.jsonl"),
	}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("ListSessionFiles() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListSessionFiles()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestListSessionFiles_EmptyProjectsDir(t *testing.T) {
	configDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configDir, "projects"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	got, err := ListSessionFiles(configDir)
	if err != nil {
		t.Fatalf("ListSessionFiles() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListSessionFiles() = %v, want empty", got)
	}
}

func TestListSessionFiles_ProjectsDirMissing(t *testing.T) {
	configDir := t.TempDir()

	_, err := ListSessionFiles(configDir)
	if err == nil {
		t.Fatal("ListSessionFiles() error = nil, want error for missing projects dir")
	}
	if !errors.Is(err, ErrProjectsDirNotFound) {
		t.Errorf("ListSessionFiles() error = %v, want wrapping ErrProjectsDirNotFound", err)
	}
}

// permDeniedFS wraps fstest.MapFS and forces a permission error when reading
// one specific directory. Real ACL-based permission denial is unreliable to
// set up portably (especially on Windows), so the error path is exercised
// against a fake fs.FS instead.
type permDeniedFS struct {
	fstest.MapFS
	denyDir string
}

func (f permDeniedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == f.denyDir {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrPermission}
	}
	return f.MapFS.ReadDir(name)
}

func TestWalkSessionFiles_SkipsPermissionDeniedDir(t *testing.T) {
	fsys := permDeniedFS{
		MapFS: fstest.MapFS{
			"readable/session1.jsonl": &fstest.MapFile{Data: []byte("{}")},
			"noperm/session2.jsonl":   &fstest.MapFile{Data: []byte("{}")},
			"another/session3.jsonl":  &fstest.MapFile{Data: []byte("{}")},
		},
		denyDir: "noperm",
	}

	got, err := walkSessionFiles(fsys, ".")
	if err != nil {
		t.Fatalf("walkSessionFiles() error = %v, want nil (permission errors should be skipped)", err)
	}

	want := []string{"another/session3.jsonl", "readable/session1.jsonl"}
	sort.Strings(want)
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("walkSessionFiles() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("walkSessionFiles()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
