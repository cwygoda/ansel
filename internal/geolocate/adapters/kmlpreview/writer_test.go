package kmlpreview

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

func TestKMLDocumentEscapesValuesAndIncludesThumbnail(t *testing.T) {
	at := time.Date(2026, 8, 9, 12, 34, 56, 0, time.UTC)
	plan := domain.WritePlan{
		PhotoPath: `/shoot/A&B "one".jpg`,
		Fix: domain.Fix{
			Position: domain.TrackPoint{
				Time:         at,
				Latitude:     52.123456789,
				Longitude:    13.987654321,
				Elevation:    42.42,
				HasElevation: true,
			},
			Method: domain.MethodInterpolated,
			Source: `/tracks/ride & run.fit.xz`,
			Gap:    5 * time.Second,
		},
	}

	doc := kmlDocument([]placemark{{plan: plan, thumbHref: `geolocate-preview-thumbnails/a&b.jpg`}})

	checks := []string{
		`<name>A&amp;B &#34;one&#34;.jpg</name>`,
		`&lt;img src=&#34;geolocate-preview-thumbnails/a&amp;amp;b.jpg&#34; width=&#34;320&#34;/&gt;`,
		`13.98765432,52.12345679,42.42`,
		`ride &amp;amp; run.fit.xz`,
		`2026-08-09 12:34:56 UTC`,
	}
	for _, check := range checks {
		if !strings.Contains(doc, check) {
			t.Errorf("KML document missing %q:\n%s", check, doc)
		}
	}
}

func TestThumbnailHrefIsStableAndSafe(t *testing.T) {
	path := `/shoot/Ä Weird Name!.NEF`

	first := thumbnailHref(path)
	second := thumbnailHref(path)

	if first != second {
		t.Fatalf("thumbnailHref is not stable: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, "geolocate-preview-thumbnails/weird-name-") {
		t.Errorf("thumbnailHref prefix = %q", first)
	}
	if !strings.HasSuffix(first, ".jpg") {
		t.Errorf("thumbnailHref suffix = %q", first)
	}
}

func TestWriteKMZIncludesDocAndThumbnails(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "preview.kmz")
	plan := domain.WritePlan{
		PhotoPath: `/shoot/photo.jpg`,
		Fix:       domain.Fix{Position: domain.TrackPoint{Latitude: 1, Longitude: 2}},
	}
	thumb := thumbnail{href: "geolocate-preview-thumbnails/photo.jpg", data: []byte("jpeg")}

	if err := writeKMZ(path, []placemark{{plan: plan, thumbHref: thumb.href}}, []thumbnail{thumb}); err != nil {
		t.Fatalf("writeKMZ failed: %v", err)
	}

	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open KMZ: %v", err)
	}
	defer archive.Close()

	entries := map[string]bool{}
	for _, file := range archive.File {
		entries[file.Name] = true
	}
	for _, name := range []string{"doc.kml", "geolocate-preview-thumbnails/photo.jpg"} {
		if !entries[name] {
			t.Errorf("KMZ missing %s; entries: %v", name, entries)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat KMZ: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("KMZ is empty")
	}
}
