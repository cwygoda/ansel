package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"
)

// Image is a discovered photograph. Originals are never modified; every value
// here is derived by reading the file.
type Image struct {
	ID          string
	Path        string
	FileSize    int64
	MTimeNs     int64
	Fingerprint string
	Metadata    Metadata

	// PerceptualHash is a 64-bit dHash of the analysis preview. It is stored
	// separately from Observation because a 64-bit hash cannot survive a
	// float64 round trip, and because it is an identity fingerprint rather
	// than a score.
	PerceptualHash uint64
}

// Preview classes. A RAW file is analyzed through the camera's own embedded
// preview, which carries almost no output sharpening; a delivered JPEG has
// usually been sharpened hard. The same photograph can differ twentyfold in
// high-frequency energy between the two, so they are never ranked against
// each other.
const (
	PreviewRAW      = "raw"
	PreviewRendered = "rendered"
)

// PreviewClass reports which population this photograph's sharpness may be
// compared against.
func (i Image) PreviewClass() string {
	switch strings.ToLower(filepath.Ext(i.Path)) {
	case ".jpg", ".jpeg":
		return PreviewRendered
	default:
		return PreviewRAW
	}
}

// NewImageID derives a stable identifier from the canonical path. It is
// deliberately independent of content so that re-editing a file keeps its
// identity; content changes are detected through Fingerprint instead.
func NewImageID(canonicalPath string) string {
	sum := sha256.Sum256([]byte(canonicalPath))
	return hex.EncodeToString(sum[:8])
}

// Metadata is the capture information extracted before any visual analysis.
type Metadata struct {
	CaptureTime    time.Time
	Camera         string
	Lens           string
	FocalLength    float64
	Aperture       float64
	ShutterSeconds float64
	ISO            int
	Orientation    int
	Width          int
	Height         int
}

// Region references a sub-area of an image, so an observation can describe a
// face or subject rather than the whole frame.
type Region struct {
	ID string
	X  int
	Y  int
	W  int
	H  int
}

// Observation is a single fact or probabilistic score produced by one
// analyzer. Values are stored raw; normalization happens in policy code so
// thresholds can change without re-running analysis.
type Observation struct {
	Key        string
	Value      float64
	Confidence *float64
	Region     *Region
	Analyzer   string
	Version    string
}

// Observation keys produced by the Phase 1 analyzers.
const (
	KeySharpnessLaplacian = "sharpness_laplacian"
	KeySharpnessTenengrad = "sharpness_tenengrad"
	KeyLuminanceMean      = "luminance_mean"
	KeyLuminanceMedian    = "luminance_median"
	KeyLuminanceP05       = "luminance_p05"
	KeyLuminanceP95       = "luminance_p95"
	KeyShadowOccupancy    = "shadow_occupancy"
	KeyHighlightOccupancy = "highlight_occupancy"
	KeyShadowClipping     = "shadow_clipping"
	KeyHighlightClipping  = "highlight_clipping"
)

// Failure records that one analyzer or stage did not complete for one image.
// Analysis is best-effort: a failure never removes an image from the run.
type Failure struct {
	ImageID  string
	Path     string
	Stage    string
	Analyzer string
	Err      string
}

// Observations is a set of observations for a single image.
type Observations []Observation

// Value returns the value recorded for key, and whether it was present.
// Only whole-frame observations (those without a region) are considered.
func (o Observations) Value(key string) (float64, bool) {
	for _, obs := range o {
		if obs.Key == key && obs.Region == nil {
			return obs.Value, true
		}
	}
	return 0, false
}

// ValueOr returns the value recorded for key, or fallback when absent.
func (o Observations) ValueOr(key string, fallback float64) float64 {
	if value, ok := o.Value(key); ok {
		return value
	}
	return fallback
}
