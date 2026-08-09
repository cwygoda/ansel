package policy

import (
	"math"
	"sort"

	"github.com/cwygoda/ansel/internal/cull/analysis"
	"github.com/cwygoda/ansel/internal/cull/domain"
)

// Normalize turns raw observations into comparable 0..1 features.
//
// This is policy, not analysis, and that separation is the point: thresholds
// and scales can change here and tags can be recomputed without re-running a
// single expensive measurement.
func Normalize(images []domain.Image, observations map[string]domain.Observations, opts Options) map[string]domain.Features {
	scales := sharpnessScales(images, observations)
	features := make(map[string]domain.Features, len(images))

	for _, img := range images {
		obs, analyzed := observations[img.ID]
		if !analyzed || len(obs) == 0 {
			features[img.ID] = domain.Features{ImageID: img.ID}
			continue
		}
		features[img.ID] = featuresFor(img, obs, scales[img.PreviewClass()], opts)
	}
	return features
}

func featuresFor(img domain.Image, obs domain.Observations, scale scale, opts Options) domain.Features {
	highlightClipping := obs.ValueOr(domain.KeyHighlightClipping, 0)
	shadowClipping := obs.ValueOr(domain.KeyShadowClipping, 0)
	rawSharpness := obs.ValueOr(domain.KeySharpnessLaplacian, 0)

	return domain.Features{
		ImageID:           img.ID,
		Sharpness:         analysis.PercentileRank(scale.values, rawSharpness),
		SharpnessRelative: scale.relative(rawSharpness),
		ExposureQuality:   exposureQuality(highlightClipping, shadowClipping, opts),
		HighlightClipping: highlightClipping,
		ShadowClipping:    shadowClipping,
		LuminanceMedian:   obs.ValueOr(domain.KeyLuminanceMedian, 0),
		Analyzed:          true,
	}
}

// exposureQuality is derived from clipping rather than average brightness.
// A deliberately dark frame is not a mistake; a frame whose shadows have gone
// to solid black has lost information that cannot be recovered.
func exposureQuality(highlightClipping, shadowClipping float64, opts Options) float64 {
	highlight := ratio(highlightClipping, opts.HighlightClipWarning)
	shadow := ratio(shadowClipping, opts.ShadowClipWarning)
	return math.Max(0, 1-(highlight+shadow)/2)
}

// ratio scores a measurement against its warning threshold, saturating at 1 so
// a catastrophically clipped frame cannot drag the score arbitrarily negative.
func ratio(value, warning float64) float64 {
	if warning <= 0 {
		return 0
	}
	return math.Min(1, value/warning)
}

// scale is the sharpness distribution of one comparable population.
type scale struct {
	values []float64
	median float64
}

// relative expresses a raw measurement as a multiple of the typical frame.
func (s scale) relative(value float64) float64 {
	if s.median <= 0 {
		return 1
	}
	return value / s.median
}

// sharpnessScales builds one distribution per preview class.
//
// Ranking a camera's embedded RAW preview against a delivered JPEG measures
// the sharpening applied on export, not the photograph: the same frame can
// differ by more than an order of magnitude between the two. Keeping the
// populations apart is what stops every RAW file in a mixed directory from
// being reported as soft.
func sharpnessScales(images []domain.Image, observations map[string]domain.Observations) map[string]scale {
	grouped := make(map[string][]float64)
	for _, img := range images {
		obs, ok := observations[img.ID]
		if !ok {
			continue
		}
		if value, present := obs.Value(domain.KeySharpnessLaplacian); present {
			class := img.PreviewClass()
			grouped[class] = append(grouped[class], value)
		}
	}

	scales := make(map[string]scale, len(grouped))
	for class, values := range grouped {
		sort.Float64s(values)
		scales[class] = scale{values: values, median: medianOf(values)}
	}
	return scales
}

func medianOf(sorted []float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}
