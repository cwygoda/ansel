// Package exiftool adapts the exiftool executable to the cull ports. It is an
// adapter, not part of the domain: nothing outside this package knows that
// metadata comes from a subprocess.
//
// The subprocess protocol itself lives in internal/exiftool, which the
// geolocate adapter drives too. What stays here is what is genuinely this
// feature's own: the tags a cull run needs, and how they map onto its domain.
package exiftool

import (
	"context"
	"os"

	"github.com/cwygoda/ansel/internal/cull/domain"
	"github.com/cwygoda/ansel/internal/exiftool"
)

// Reader reads capture metadata and embedded previews.
type Reader struct {
	session *exiftool.Session
}

// New returns a Reader driving the given session.
func New(session *exiftool.Session) *Reader {
	return &Reader{session: session}
}

// Read returns metadata keyed by the path it was requested for.
func (r *Reader) Read(ctx context.Context, paths []string) (map[string]domain.Metadata, error) {
	entries, err := r.session.Query(ctx, captureTags(), paths)
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]domain.Metadata, len(entries))
	for _, entry := range entries {
		source := exiftool.AsString(entry["SourceFile"])
		if source == "" {
			continue
		}
		metadata[source] = metadataFrom(entry)
	}
	return metadata, nil
}

// HasRating reports which sidecars already carry a rating. Files that do not
// exist are skipped rather than queried, since a sidecar yet to be created
// plainly holds no rating and exiftool would only complain.
//
// A rating of zero counts as absent. Zero stars means "not yet judged" in
// every application that reads these files, which is exactly what a sidecar
// holding only coordinates should look like.
func (r *Reader) HasRating(ctx context.Context, paths []string) (map[string]bool, error) {
	entries, err := r.session.Query(ctx, ratingTags(), existing(paths))
	if err != nil {
		return nil, err
	}

	rated := make(map[string]bool, len(entries))
	for _, entry := range entries {
		source := exiftool.AsString(entry["SourceFile"])
		if source == "" {
			continue
		}
		rated[source] = exiftool.AsInt(entry["Rating"]) > 0
	}
	return rated, nil
}

// captureTags lists exactly the tags the pipeline needs.
func captureTags() []string {
	return []string{
		"-SourceFile",
		"-DateTimeOriginal",
		"-CreateDate",
		"-Make",
		"-Model",
		"-LensModel",
		"-FocalLength",
		"-FNumber",
		"-ExposureTime",
		"-ISO",
		"-Orientation",
		"-ImageWidth",
		"-ImageHeight",
	}
}

func ratingTags() []string {
	return []string{"-SourceFile", "-XMP:Rating"}
}

func existing(paths []string) []string {
	present := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			present = append(present, path)
		}
	}
	return present
}
