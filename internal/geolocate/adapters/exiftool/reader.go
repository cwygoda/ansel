// Package exiftool adapts the exiftool executable to the geolocate ports.
// Nothing outside this package knows that metadata comes from a subprocess.
//
// The cull feature drives exiftool too, and both now share the subprocess
// protocol in internal/exiftool. What is not shared is the reading itself.
// Cull resolves a camera's unzoned timestamp against the machine's own local
// zone, which is harmless when grouping bursts by their spacing and quietly
// wrong when the answer is a place on the Earth: culling a Tokyo shoot at a
// desk in Berlin would shift every frame eight hours along the track. This
// reader keeps the reading and its zone apart and lets the application decide.
package exiftool

import (
	"context"
	"os"

	"github.com/cwygoda/ansel/internal/exiftool"
	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

// Reader reads capture metadata for geolocation.
type Reader struct {
	session *exiftool.Session
}

// New returns a Reader driving the given session.
func New(session *exiftool.Session) *Reader {
	return &Reader{session: session}
}

// Read returns capture clocks keyed by the path they were requested for.
func (r *Reader) Read(ctx context.Context, paths []string) (map[string]domain.Photo, error) {
	entries, err := r.session.Query(ctx, captureTags(), paths)
	if err != nil {
		return nil, err
	}

	photos := make(map[string]domain.Photo, len(entries))
	for _, entry := range entries {
		source := exiftool.AsString(entry["SourceFile"])
		if source == "" {
			continue
		}
		photos[source] = domain.Photo{Path: source, Clock: clockFrom(entry)}
	}
	return photos, nil
}

// HasCoordinates reports which targets already carry a position. Targets that
// do not exist are skipped rather than queried, since a sidecar that has yet
// to be created plainly holds no coordinates and exiftool would only complain.
func (r *Reader) HasCoordinates(ctx context.Context, paths []string) (map[string]bool, error) {
	entries, err := r.session.Query(ctx, positionTags(), existing(paths))
	if err != nil {
		return nil, err
	}

	located := make(map[string]bool, len(entries))
	for _, entry := range entries {
		source := exiftool.AsString(entry["SourceFile"])
		if source == "" {
			continue
		}
		located[source] = entry["GPSLatitude"] != nil && entry["GPSLongitude"] != nil
	}
	return located, nil
}

// captureTags lists exactly the tags geolocation needs.
func captureTags() []string {
	return []string{
		"-SourceFile",
		"-DateTimeOriginal",
		"-CreateDate",
		// The zone the camera was set to, when it bothered to record one.
		// Newer bodies write it; most do not.
		"-OffsetTimeOriginal",
		"-OffsetTime",
		"-GPSLatitude",
		"-GPSLongitude",
	}
}

func positionTags() []string {
	return []string{"-SourceFile", "-GPSLatitude", "-GPSLongitude"}
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
