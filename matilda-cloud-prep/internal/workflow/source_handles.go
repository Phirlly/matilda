package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateCachedSourceHandleFiles verifies that safe docs/ source handles point
// to existing local reference files from a caller-provided project root.
func ValidateCachedSourceHandleFiles(projectRoot string, handles []SourceHandle) error {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}

	copied, err := safeSourceHandles("source handles", handles)
	if err != nil {
		return err
	}

	for _, handle := range copied {
		path := filepath.Join(root, filepath.FromSlash(handle.URI))
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("source handle %q must reference an existing cached docs file", handle.URI)
			}
			return fmt.Errorf("source handle %q cannot be accessed", handle.URI)
		}
		if info.IsDir() {
			return fmt.Errorf("source handle %q must reference a cached docs file, not a directory", handle.URI)
		}
	}

	return nil
}
