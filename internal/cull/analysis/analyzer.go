package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// Input is what every analyzer receives. Preview is the shared grayscale
// buffer; analyzers must treat it as read-only.
type Input struct {
	ImageID  string
	Metadata domain.Metadata
	Preview  *domain.Luma
}

// Analyzer produces independent observations about one photograph. No
// analyzer owns the definition of a good photograph; each contributes facts
// that policy code later interprets.
type Analyzer interface {
	Name() string
	Version() string
	Analyze(ctx context.Context, in Input) ([]domain.Observation, error)
}

// Default returns the Phase 1 analyzer set, cheap and deterministic, in the
// order they should run.
func Default() []Analyzer {
	return []Analyzer{
		Sharpness{},
		Exposure{},
		Clipping{},
	}
}

// SetVersion fingerprints an analyzer set so cached results can be invalidated
// when an analyzer is added, removed, or revised. Changing a policy threshold
// deliberately does not affect this value.
func SetVersion(analyzers []Analyzer) string {
	parts := make([]string, 0, len(analyzers))
	for _, analyzer := range analyzers {
		parts = append(parts, analyzer.Name()+":"+analyzer.Version())
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:8])
}

// observe is a small helper so analyzers stay free of repetitive struct
// literals.
func observe(analyzer Analyzer, key string, value float64) domain.Observation {
	return domain.Observation{
		Key:      key,
		Value:    value,
		Analyzer: analyzer.Name(),
		Version:  analyzer.Version(),
	}
}
