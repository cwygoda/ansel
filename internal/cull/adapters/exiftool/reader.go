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
