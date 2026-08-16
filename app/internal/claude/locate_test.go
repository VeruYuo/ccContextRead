package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir_HonorsEnvOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom-claude-dir")
	t.Setenv("CLAUDE_CONFIG_DIR", want)

	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error = %v", err)
	}
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestConfigDir_FallsBackToUserProfile(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}
	want := filepath.Join(home, ".claude")

	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error = %v", err)
	}
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}
