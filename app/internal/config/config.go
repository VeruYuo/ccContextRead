// Package config persists CCContextRead's user-configurable settings
// (PLAN.md 4.3 filter switches, 4.4 output directory, T1.11 fallback
// state) to disk as JSON. It has zero GUI dependencies and must be fully
// testable via `go test`.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"ccContextRead/internal/model"
	"ccContextRead/internal/render"
)

// Config is the full persisted application configuration.
type Config struct {
	Filter    model.FilterConfig `json:"filter"`
	ImageMode render.ImageMode   `json:"imageMode"`

	// OutputDirOverride is the user's explicit choice; empty means
	// "auto-resolve" (see EffectiveOutputDir / ResolveOutputDir).
	OutputDirOverride string `json:"outputDirOverride"`
	// FallbackApplied and ResolvedOutputDir record the outcome of the
	// last auto-resolution, so the UI can show the *actual* output path
	// even after the exe-adjacent directory turned out to be read-only
	// (PLAN.md T1.11 "只读安装目录的回落策略").
	FallbackApplied   bool   `json:"fallbackApplied"`
	ResolvedOutputDir string `json:"resolvedOutputDir"`
}

// Default returns the task brief's defaults: only real user input and
// the assistant's final reply are shown, images render as placeholders,
// and the output directory auto-resolves to the exe-adjacent directory.
func Default() Config {
	return Config{
		Filter:    model.DefaultFilterConfig(),
		ImageMode: render.ImagePlaceholder,
	}
}

// Load reads and parses the config file at path. A missing file, an
// unreadable one, or corrupt JSON all fall back to Default() — the
// config file is not load-bearing for correctness, and a broken one
// must never crash the app (PLAN.md T1.11).
func Load(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return Default()
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default()
	}
	return cfg
}

// Save writes cfg to path as indented JSON, creating parent directories
// as needed.
func Save(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
