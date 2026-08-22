package massstorage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/camera/domain"
)

func TestDetect(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "NIKON Z6_3 ", "DCIM"))
	mustMkdir(t, filepath.Join(root, "NO_CARD"))

	client := New([]string{root})
	got, err := client.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	want := []domain.Camera{{Model: "NIKON Z6_3", Port: filepath.Join(root, "NIKON Z6_3 ")}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Detect() = %#v, want %#v", got, want)
	}
	if got[0].KnownName() != "Nikon Z6 III" {
		t.Fatalf("KnownName() = %q, want %q", got[0].KnownName(), "Nikon Z6 III")
	}
}

func TestDetectAcceptsCardRoot(t *testing.T) {
	card := filepath.Join(t.TempDir(), "NIKON Z6_3 ")
	mustMkdir(t, filepath.Join(card, "DCIM"))

	client := New([]string{card})
	got, err := client.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	want := []domain.Camera{{Model: "NIKON Z6_3", Port: card}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Detect() = %#v, want %#v", got, want)
	}
}

func TestListFiles(t *testing.T) {
	card := t.TempDir()
	mustWrite(t, filepath.Join(card, "DCIM", "101NZ6_3", "DSC_0002.NEF"), []byte("raw"))
	mustWrite(t, filepath.Join(card, "DCIM", "100NZ6_3", "DSC_0001.JPG"), []byte("jpeg"))
	mustWrite(t, filepath.Join(card, "NIKON", "Z6_3", "settings.bin"), []byte("settings"))

	client := New([]string{filepath.Dir(card)})
	got, err := client.ListFiles(context.Background(), domain.Camera{Port: card})
	if err != nil {
		t.Fatalf("ListFiles() error = %v", err)
	}

	want := []domain.RemoteFile{
		{Folder: "/DCIM/100NZ6_3", Name: "DSC_0001.JPG", SizeBytes: 4},
		{Folder: "/DCIM/101NZ6_3", Name: "DSC_0002.NEF", SizeBytes: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListFiles() = %#v, want %#v", got, want)
	}
}

func TestDownload(t *testing.T) {
	card := t.TempDir()
	source := filepath.Join(card, "DCIM", "100NZ6_3", "DSC_0001.JPG")
	mustWrite(t, source, []byte("jpeg"))
	modTime := time.Date(2025, 8, 22, 12, 34, 56, 0, time.UTC)
	if err := os.Chtimes(source, modTime, modTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	dest := filepath.Join(t.TempDir(), "DSC_0001.JPG")
	client := New(nil)
	err := client.Download(
		context.Background(),
		domain.Camera{Port: card},
		domain.RemoteFile{Folder: "/DCIM/100NZ6_3", Name: "DSC_0001.JPG"},
		dest,
	)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "jpeg" {
		t.Fatalf("downloaded content = %q, want %q", string(data), "jpeg")
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.ModTime().Sub(modTime).Abs() > time.Second {
		t.Fatalf("downloaded mod time = %s, want %s", info.ModTime(), modTime)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
