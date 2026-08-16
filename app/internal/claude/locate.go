// Package claude reads Claude Code's on-disk session data (~/.claude).
// It has zero GUI dependencies and must be fully testable via `go test`.
package claude

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigDir resolves the Claude Code configuration directory. It honors
// CLAUDE_CONFIG_DIR when set; otherwise it falls back to the user's home
// directory (%USERPROFILE%\.claude on Windows, $HOME/.claude elsewhere).
func ConfigDir() (string, error) {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}
