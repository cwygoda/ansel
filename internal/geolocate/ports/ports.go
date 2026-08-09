// Package ports declares the secondary interfaces the geolocate application
// depends on. It imports the domain and nothing else: no file format, no
// subprocess and no third-party client type may appear here.
package ports

import (
	"context"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

// TrackDecoder reads one file of geospatial line data.
//
// This is the seam that keeps the feature open to GPX, TCX and KML. Each
// format arrives as its own adapter implementing this interface, and only the
// composition root in cmd/ ever learns which formats exist.
//
// Supports belongs to the decoder rather than to a lookup table in the
// application because a format's identity is its own business: FIT arrives
// here xz-compressed under the double extension ".fit.xz", and other formats
// will bring their own conventions.
type TrackDecoder interface {
	Supports(path string) bool
	Decode(ctx context.Context, path string) (domain.Track, error)
}

// MetadataReader is a secondary port for reading capture metadata. Reads are
// batched because the underlying tool is far cheaper to drive as one
// long-lived process than as one process per photograph.
//
// Implementations must return the camera's wall-clock reading and its
// recorded UTC offset separately, in domain.CaptureClock. Resolving an unzoned
// reading against the machine's own zone is the application's decision to
// make, never the adapter's.
type MetadataReader interface {
	Read(ctx context.Context, paths []string) (map[string]domain.Photo, error)

	// HasCoordinates reports which of the given paths already carry a
	// position. It is asked about write targets rather than photographs,
	// since under the default mode the target is a sidecar. Paths that do not
	// exist yet are simply absent from the result.
	HasCoordinates(ctx context.Context, paths []string) (map[string]bool, error)
}

// MetadataWriter is a secondary port for recording positions.
//
// Each plan names its own target, so this interface is unaware of the choice
// between an XMP sidecar and the photograph itself. Implementations must
// preserve properties they were not asked to change: a sidecar commonly
// already holds ratings and labels written by `ansel cull`.
//
// Plans are updated in place. A plan already carrying a Skipped reason must be
// left untouched, and one that cannot be written records why on itself rather
// than aborting the batch. The error return is for a failure of the run as a
// whole, such as a cancelled context.
type MetadataWriter interface {
	Write(ctx context.Context, plans []domain.WritePlan) error
}
