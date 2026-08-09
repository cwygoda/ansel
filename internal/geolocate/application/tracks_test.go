package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
	"github.com/cwygoda/ansel/internal/geolocate/ports"
)

// activityDir mimics an export directory: every recording sits beside a JSON
// file of the same name, which no track decoder can read.
func activityDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return dir
}

func locatorFor(dir string) *Locator {
	cfg := DefaultConfig()
	cfg.TracksDir = dir
	return &Locator{
		Decoders: []ports.TrackDecoder{fakeDecoder{}},
		Config:   cfg,
	}
}

func photoAt(hour, minute int) domain.Photo {
	return domain.Photo{
		Path:  "DSC_0001.jpg",
		Clock: domain.CaptureClock{Wall: time.Date(2017, 8, 8, hour, minute, 0, 0, time.UTC)},
	}
}

// The bug this guards: sibling metadata files were reported as failures,
// burying a clean run in noise about files nobody asked to read.
func TestDiscoverTracksIgnoresFilesNoDecoderClaims(t *testing.T) {
	dir := activityDir(t,
		"2017-08-08-20-28-54_cycling.fit.xz",
		"2017-08-08-20-28-54_cycling.json",
		"manifest.json",
	)

	paths, err := locatorFor(dir).discoverTracks([]domain.Photo{photoAt(20, 30)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("got %d paths, expected only the track: %v", len(paths), paths)
	}
	if !strings.HasSuffix(paths[0], ".fit.xz") {
		t.Errorf("selected %q, expected the .fit.xz", paths[0])
	}
}

func TestDiscoverTracksNarrowsByFilenameDate(t *testing.T) {
	dir := activityDir(t,
		"2017-08-07-08-00-00_running.fit.xz",
		"2017-08-08-20-28-54_cycling.fit.xz",
		"2017-08-09-08-00-00_running.fit.xz",
		"2011-01-01-08-00-00_ancient.fit.xz",
		"2023-06-01-08-00-00_recent.fit.xz",
	)

	paths, err := locatorFor(dir).discoverTracks([]domain.Photo{photoAt(20, 30)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A day either side of the shoot, and nothing from other years.
	if len(paths) != 3 {
		t.Fatalf("got %d paths, expected the 3 within a day: %v", len(paths), paths)
	}
	for _, path := range paths {
		if strings.Contains(path, "ancient") || strings.Contains(path, "recent") {
			t.Errorf("%q is nowhere near the shoot", path)
		}
	}
}

// A filename that says nothing about its date cannot be ruled out cheaply, so
// it is decoded rather than missed.
func TestDiscoverTracksKeepsUndatedFilenames(t *testing.T) {
	dir := activityDir(t, "morning-ride.fit.xz", "2011-01-01-08-00-00_ancient.fit.xz")

	paths, err := locatorFor(dir).discoverTracks([]domain.Photo{photoAt(20, 30)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(paths) != 1 || !strings.Contains(paths[0], "morning-ride") {
		t.Errorf("got %v, expected only the undated file", paths)
	}
}

func TestDiscoverTracksReportsWhenNothingCovers(t *testing.T) {
	dir := activityDir(t, "2011-01-01-08-00-00_ancient.fit.xz")

	_, err := locatorFor(dir).discoverTracks([]domain.Photo{photoAt(20, 30)})
	if err == nil {
		t.Fatal("expected an error when no track is near the shoot")
	}
	if !strings.Contains(err.Error(), "2017-08") {
		t.Errorf("error %q should name the range it searched", err)
	}
}

// A directory named with --track behaves like discovery: siblings are skipped.
func TestExpandOneSkipsUnsupportedFilesInADirectory(t *testing.T) {
	dir := activityDir(t, "ride.fit.xz", "ride.json")

	paths, err := locatorFor(dir).expandOne(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(paths) != 1 || !strings.HasSuffix(paths[0], ".fit.xz") {
		t.Errorf("got %v, expected only the track file", paths)
	}
}

func TestExpandOneFiltersGlobMatches(t *testing.T) {
	dir := activityDir(t, "ride.fit.xz", "ride.json")

	paths, err := locatorFor(dir).expandOne(filepath.Join(dir, "ride.*"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(paths) != 1 || !strings.HasSuffix(paths[0], ".fit.xz") {
		t.Errorf("got %v, expected the glob to keep only readable tracks", paths)
	}
}

// A file named outright is what the operator meant, so an unreadable one is
// surfaced rather than silently dropped.
func TestExpandOneKeepsAnExplicitlyNamedFile(t *testing.T) {
	dir := activityDir(t, "ride.json")

	paths, err := locatorFor(dir).expandOne(filepath.Join(dir, "ride.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("got %v, expected the named file to be kept for reporting", paths)
	}
}

func TestTrackOffsetTakesTheFirstStated(t *testing.T) {
	tracks := []domain.Track{
		{Source: "a.fit.xz"},
		{Source: "b.fit.xz", UTCOffset: 2 * time.Hour, HasUTCOffset: true},
	}

	offset, ok := trackOffset(tracks)
	if !ok {
		t.Fatal("expected an offset")
	}
	if offset != 2*time.Hour {
		t.Errorf("offset = %s, expected 2h", offset)
	}
}

func TestTrackOffsetIsUnknownWhenNoneStated(t *testing.T) {
	if _, ok := trackOffset([]domain.Track{{Source: "a.fit.xz"}}); ok {
		t.Error("expected no offset when no track states one")
	}
}
