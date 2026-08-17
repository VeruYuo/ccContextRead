package config

import (
	"os"
	"path/filepath"
	"testing"

	"ccContextRead/internal/model"
	"ccContextRead/internal/render"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	want := Config{
		Filter: model.FilterConfig{
			UserPrompt:    true,
			AssistantText: true,
			ToolUse:       true,
			FileChange:    model.FileChangeSummary,
			TruncateChars: 500,
		},
		ImageMode:         render.ImageAttachment,
		OutputDirOverride: `D:\out`,
		FallbackApplied:   true,
		ResolvedOutputDir: `D:\out`,
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := Load(path)
	if got != want {
		t.Fatalf("round trip mismatch:\n got  %+v\n want %+v", got, want)
	}
}

func TestLoadMissingFileFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	got := Load(filepath.Join(dir, "does-not-exist.json"))
	want := Default()
	if got != want {
		t.Fatalf("Load of missing file = %+v, want Default() = %+v", got, want)
	}
}

func TestLoadCorruptFileFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	want := Default()
	if got != want {
		t.Fatalf("Load of corrupt file = %+v, want Default() = %+v", got, want)
	}
}

func TestLoadEmptyFileFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	want := Default()
	if got != want {
		t.Fatalf("Load of empty file = %+v, want Default() = %+v", got, want)
	}
}

func TestDefaultMatchesTaskBriefDefaults(t *testing.T) {
	got := Default()
	if !got.Filter.UserPrompt || !got.Filter.AssistantText {
		t.Fatalf("Default() filter must keep UserPrompt+AssistantText on: %+v", got.Filter)
	}
	if got.Filter.ToolUse || got.Filter.Thinking {
		t.Fatalf("Default() filter must keep every other switch off: %+v", got.Filter)
	}
	if got.ImageMode != render.ImagePlaceholder {
		t.Fatalf("Default() ImageMode = %v, want ImagePlaceholder", got.ImageMode)
	}
	if got.OutputDirOverride != "" {
		t.Fatalf("Default() OutputDirOverride = %q, want empty (auto-resolve)", got.OutputDirOverride)
	}
}

func TestSaveCreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sub", "config.json")
	if err := Save(path, Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}
