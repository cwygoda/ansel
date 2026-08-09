package application

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// discoverPhotos walks a directory for photographs of the configured types.
// Paths come back sorted and absolute so a run is reproducible and so the
// sidecar written beside a photograph lands where the operator expects.
func discoverPhotos(root string, extensions []string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return skipHidden(path, entry)
		}
		if !includes(path, extensions) {
			return nil
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		paths = append(paths, absolute)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk %s: %w", root, err)
	}

	sort.Strings(paths)
	return paths, nil
}

// skipHidden keeps the walk out of dot-directories, which hold caches and
// version-control data rather than photographs.
func skipHidden(path string, entry fs.DirEntry) error {
	if entry.Name() != "." && strings.HasPrefix(entry.Name(), ".") {
		return filepath.SkipDir
	}
	return nil
}

// includes reports whether a path carries one of the wanted extensions.
// Comparison is case-insensitive, since cameras disagree about whether raw
// files end in .NEF or .nef.
func includes(path string, extensions []string) bool {
	actual := strings.ToLower(filepath.Ext(path))
	if actual == "" {
		return false
	}
	for _, allowed := range extensions {
		if actual == strings.ToLower(allowed) {
			return true
		}
	}
	return false
}
