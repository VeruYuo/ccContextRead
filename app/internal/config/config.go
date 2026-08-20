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

// Theme is the user's manual light/dark override (PLAN.md 12.2.1 ⑤ /
// T1.17). ThemeSystem means "follow the OS setting" — the frontend resolves
// it, this package only persists the choice.
type Theme string

const (
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
	ThemeSystem Theme = "system"
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
	Theme             Theme  `json:"theme"`
}

// Default returns the task brief's defaults: only real user input and
// the assistant's final reply are shown, images render as placeholders,
// the output directory auto-resolves to the exe-adjacent directory, and
// the UI follows the OS light/dark setting until the user picks one.
func Default() Config {
	return Config{
		Filter:    model.DefaultFilterConfig(),
		ImageMode: render.ImagePlaceholder,
		Theme:     ThemeSystem,
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
	if cfg.Theme == "" {
		// A config file written before T1.17 has no "theme" key; the zero
		// value must not silently mean "always light".
		cfg.Theme = ThemeSystem
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
