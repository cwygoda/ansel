package ports

import (
	"context"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// MetadataReader is a secondary port for reading capture metadata. Reads are
// batched because the underlying tool is far cheaper to drive as one
// long-lived process than as one process per photograph.
type MetadataReader interface {
	Read(ctx context.Context, paths []string) (map[string]domain.Metadata, error)
}

// PreviewSource is a secondary port yielding the bytes of an analysis image:
// the largest embedded preview for a RAW file, or the file itself for a JPEG.
type PreviewSource interface {
	Preview(ctx context.Context, path string) ([]byte, error)
}

// PreviewDecoder is a secondary port that turns encoded preview bytes into a
// downscaled grayscale buffer. Downscaling happens here so no caller ever
// holds a full-resolution decode.
type PreviewDecoder interface {
	DecodeLuma(data []byte, maxEdge int) (*domain.Luma, error)
}

// Store is a secondary port for the feature store. It exists so a run can
// skip work that is already recorded, and so tags can be recomputed without
// re-analyzing pixels.
type Store interface {
	// CachedAnalysis returns stored observations for an image when both its
	// content fingerprint and the analyzer set are unchanged. The boolean
	// reports whether the cache was usable.
	CachedAnalysis(imageID, fingerprint, analysisVersion string) (domain.Observations, uint64, bool, error)

	// SaveAnalysis records an image and its observations.
	SaveAnalysis(img domain.Image, analysisVersion string, observations domain.Observations) error

	// SaveGrouping records similarity groups and each member's rank.
	SaveGrouping(groups []domain.SimilarityGroup, ranks map[string]domain.RankResult) error

	// SaveTags replaces the tags contributed by one source. The source is
	// explicit so an image whose tags have all been withdrawn is cleared
	// correctly, and so tags from elsewhere are never touched.
	SaveTags(source string, tags map[string][]domain.Tag) error
}

// SidecarWriter is a secondary port for interoperable output. Implementations
// must never modify the photograph itself.
type SidecarWriter interface {
	Write(plan domain.SidecarPlan) error
}
