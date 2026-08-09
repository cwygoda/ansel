package application

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// Discover walks a shoot directory and returns the photographs to analyze,
// ordered by path so a run is reproducible.
func Discover(root string, extensions []string) ([]domain.Image, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return skipHidden(entry)
		}
		if includes(extensions, path) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk %s: %w", root, err)
	}

	sort.Strings(paths)
	return describeAll(paths), nil
}

// skipHidden keeps the walk out of dot-directories, which hold caches and
// application state rather than photographs.
func skipHidden(entry fs.DirEntry) error {
	if strings.HasPrefix(entry.Name(), ".") && entry.Name() != "." {
		return filepath.SkipDir
	}
	return nil
}

func describeAll(paths []string) []domain.Image {
	images := make([]domain.Image, 0, len(paths))
	for _, path := range paths {
		img, err := describe(path)
		if err != nil {
			// A file that cannot be stat'd is not analyzable; the walk itself
			// already succeeded, so skip it rather than failing the run.
			continue
		}
		images = append(images, img)
	}
	return images
}

func describe(path string) (domain.Image, error) {
	canonical, err := filepath.Abs(path)
	if err != nil {
		return domain.Image{}, err
	}
	fingerprint, size, mtimeNs, err := Fingerprint(canonical)
	if err != nil {
		return domain.Image{}, err
	}
	return domain.Image{
		ID:          domain.NewImageID(canonical),
		Path:        canonical,
		FileSize:    size,
		MTimeNs:     mtimeNs,
		Fingerprint: fingerprint,
	}, nil
}

func includes(extensions []string, path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	for _, allowed := range extensions {
		if ext == strings.ToLower(allowed) {
			return true
		}
	}
	return false
}
