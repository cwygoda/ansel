package application

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("failed to write fixture %s: %v", name, err)
	}
	return path
}

func fingerprintOf(t *testing.T, path string) string {
	t.Helper()
	fingerprint, _, _, err := Fingerprint(path)
	if err != nil {
		t.Fatalf("Fingerprint(%s) unexpected error: %v", path, err)
	}
	return fingerprint
}

func TestFingerprintIsStable(t *testing.T) {
	path := writeFile(t, t.TempDir(), "photo.jpg", []byte("some photograph bytes"))

	if first, second := fingerprintOf(t, path), fingerprintOf(t, path); first != second {
		t.Errorf("Fingerprint is not stable: %q then %q", first, second)
	}
}

func TestFingerprintChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	original := writeFile(t, dir, "a.jpg", []byte("original content"))
	edited := writeFile(t, dir, "b.jpg", []byte("edited content!"))

	if fingerprintOf(t, original) == fingerprintOf(t, edited) {
		t.Error("different content produced the same fingerprint")
	}
}

// Only the head and tail are read, so a change in the middle of a large file
// must still be caught through the size component or the tail window.
func TestFingerprintDetectsTruncation(t *testing.T) {
	dir := t.TempDir()
	full := writeFile(t, dir, "full.jpg", bytes.Repeat([]byte("x"), 200*1024))
	truncated := writeFile(t, dir, "short.jpg", bytes.Repeat([]byte("x"), 199*1024))

	if fingerprintOf(t, full) == fingerprintOf(t, truncated) {
		t.Error("files of different sizes produced the same fingerprint")
	}
}

func TestFingerprintReportsSizeAndModificationTime(t *testing.T) {
	content := []byte("photograph")
	path := writeFile(t, t.TempDir(), "photo.jpg", content)

	_, size, mtimeNs, err := Fingerprint(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, expected %d", size, len(content))
	}
	if mtimeNs == 0 {
		t.Error("modification time was not reported")
	}
}

func TestFingerprintFailsOnMissingFile(t *testing.T) {
	if _, _, _, err := Fingerprint(filepath.Join(t.TempDir(), "absent.jpg")); err == nil {
		t.Error("Fingerprint on a missing file expected error, got nil")
	}
}

func TestDiscoverFiltersByExtension(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "keep.jpg", []byte("a"))
	writeFile(t, dir, "keep.NEF", []byte("b"))
	writeFile(t, dir, "skip.txt", []byte("c"))
	writeFile(t, dir, "skip.mov", []byte("d"))

	images, err := Discover(dir, []string{".nef", ".jpg", ".jpeg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(images) != 2 {
		t.Fatalf("Discover found %d images, expected 2", len(images))
	}
	for _, img := range images {
		if filepath.Ext(img.Path) == ".txt" || filepath.Ext(img.Path) == ".mov" {
			t.Errorf("Discover included an unsupported file: %s", img.Path)
		}
		if img.ID == "" || img.Fingerprint == "" {
			t.Errorf("Discover left %s without an identity", img.Path)
		}
	}
}

func TestDiscoverSkipsHiddenDirectories(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "visible.jpg", []byte("a"))
	hidden := filepath.Join(dir, ".cache")
	if err := os.Mkdir(hidden, 0755); err != nil {
		t.Fatalf("failed to create hidden dir: %v", err)
	}
	writeFile(t, hidden, "cached.jpg", []byte("b"))

	images, err := Discover(dir, []string{".jpg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(images) != 1 {
		t.Fatalf("Discover found %d images, expected 1", len(images))
	}
}

func TestSidecarPathFor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/shoot/DSC_1234.NEF", "/shoot/DSC_1234.xmp"},
		{"/shoot/photo.jpg", "/shoot/photo.xmp"},
		{"/shoot/no-extension", "/shoot/no-extension.xmp"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := SidecarPathFor(tc.input); got != tc.want {
				t.Errorf("SidecarPathFor(%q) = %q, expected %q", tc.input, got, tc.want)
			}
		})
	}
}
