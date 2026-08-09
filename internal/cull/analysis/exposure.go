package analysis

import (
	"context"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

const histogramBins = 256

// Exposure reports the luminance distribution. It deliberately emits
// percentiles and occupancy rather than a verdict: a low-key or high-key
// photograph is a legitimate choice, so mean brightness alone must never be
// treated as a failure.
type Exposure struct{}

func (Exposure) Name() string    { return "exposure" }
func (Exposure) Version() string { return "1" }

func (e Exposure) Analyze(_ context.Context, in Input) ([]domain.Observation, error) {
	if in.Preview.Empty() {
		return nil, ErrNoPreview
	}
	hist, total := histogram(in.Preview)

	return []domain.Observation{
		observe(e, domain.KeyLuminanceMean, mean(in.Preview)),
		observe(e, domain.KeyLuminanceMedian, percentile(hist, total, 0.50)),
		observe(e, domain.KeyLuminanceP05, percentile(hist, total, 0.05)),
		observe(e, domain.KeyLuminanceP95, percentile(hist, total, 0.95)),
		observe(e, domain.KeyShadowOccupancy, fractionBelow(hist, total, 0.05)),
		observe(e, domain.KeyHighlightOccupancy, fractionAbove(hist, total, 0.95)),
	}, nil
}

// histogram bins the buffer into 256 levels, which is enough resolution for
// percentiles and keeps the pass O(n) with no allocation per pixel.
func histogram(luma *domain.Luma) ([histogramBins]int, int) {
	var hist [histogramBins]int
	for _, value := range luma.Pix {
		hist[binOf(value)]++
	}
	return hist, len(luma.Pix)
}

func binOf(value float64) int {
	bin := int(value * (histogramBins - 1))
	if bin < 0 {
		return 0
	}
	if bin >= histogramBins {
		return histogramBins - 1
	}
	return bin
}

func mean(luma *domain.Luma) float64 {
	var sum float64
	for _, value := range luma.Pix {
		sum += value
	}
	return sum / float64(len(luma.Pix))
}

// percentile returns the luminance below which the given fraction of pixels
// fall.
func percentile(hist [histogramBins]int, total int, fraction float64) float64 {
	if total == 0 {
		return 0
	}
	target := int(fraction * float64(total))
	cumulative := 0
	for bin := 0; bin < histogramBins; bin++ {
		cumulative += hist[bin]
		if cumulative >= target {
			return float64(bin) / float64(histogramBins-1)
		}
	}
	return 1
}

func fractionBelow(hist [histogramBins]int, total int, threshold float64) float64 {
	if total == 0 {
		return 0
	}
	limit := binOf(threshold)
	count := 0
	for bin := 0; bin < limit; bin++ {
		count += hist[bin]
	}
	return float64(count) / float64(total)
}

func fractionAbove(hist [histogramBins]int, total int, threshold float64) float64 {
	if total == 0 {
		return 0
	}
	limit := binOf(threshold)
	count := 0
	for bin := limit + 1; bin < histogramBins; bin++ {
		count += hist[bin]
	}
	return float64(count) / float64(total)
}
