package claude

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrProjectsDirNotFound is returned by ListSessionFiles when
// <configDir>/projects does not exist or is not a directory.
var ErrProjectsDirNotFound = errors.New("claude: projects directory not found")

// ListSessionFiles recursively enumerates main session JSONL files under
// <configDir>/projects. Subagent sidecar files (agent-*.jsonl, wherever they
// appear) and anything under a subagents/ subdirectory are excluded.
// Directories that cannot be read due to a permission error are skipped
// rather than aborting the whole scan. Returned paths are absolute and
// sorted.
func ListSessionFiles(configDir string) ([]string, error) {
	projectsDir := filepath.Join(configDir, "projects")

	info, err := os.Stat(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrProjectsDirNotFound, projectsDir)
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrProjectsDirNotFound, projectsDir)
	}

	rel, err := walkSessionFiles(os.DirFS(projectsDir), ".")
	if err != nil {
		return nil, err
	}

	results := make([]string, len(rel))
	for i, r := range rel {
		results[i] = filepath.Join(projectsDir, filepath.FromSlash(r))
	}
	sort.Strings(results)
	return results, nil
}

// walkSessionFiles walks fsys starting at root and returns the slash-
// separated relative paths of qualifying session files.
func walkSessionFiles(fsys fs.FS, root string) ([]string, error) {
	var results []string
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			return err
		}
		if d.IsDir() {
			if d.Name() == "subagents" {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, "agent-") {
			return nil
		}
		if strings.EqualFold(filepath.Ext(name), ".jsonl") {
			results = append(results, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(results)
	return results, nil
}
