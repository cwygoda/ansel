package analysis

import (
	"context"
	"errors"
	"math"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// ErrNoPreview is returned when an analyzer is handed an empty buffer.
var ErrNoPreview = errors.New("no analysis preview available")

// Sharpness measures high-frequency energy two independent ways. Neither is
// sufficient alone: variance of Laplacian is sensitive to noise, Tenengrad is
// sensitive to strong edges. Both are reported raw and normalized later,
// because absolute values are scene-dependent and no fixed threshold is
// defensible.
type Sharpness struct{}

func (Sharpness) Name() string    { return "sharpness" }
func (Sharpness) Version() string { return "1" }

func (s Sharpness) Analyze(_ context.Context, in Input) ([]domain.Observation, error) {
	if in.Preview.Empty() {
		return nil, ErrNoPreview
	}
	return []domain.Observation{
		observe(s, domain.KeySharpnessLaplacian, varianceOfLaplacian(in.Preview)),
		observe(s, domain.KeySharpnessTenengrad, tenengrad(in.Preview)),
	}, nil
}

// varianceOfLaplacian convolves with the 4-neighbour Laplacian and returns the
// variance of the response. A blurred frame has little high-frequency energy,
// so its response clusters near zero.
func varianceOfLaplacian(luma *domain.Luma) float64 {
	var sum, sumSquares float64
	count := float64(luma.Width * luma.Height)

	for y := 0; y < luma.Height; y++ {
		for x := 0; x < luma.Width; x++ {
			response := luma.At(x, y-1) + luma.At(x-1, y) +
				luma.At(x+1, y) + luma.At(x, y+1) - 4*luma.At(x, y)
			sum += response
			sumSquares += response * response
		}
	}

	mean := sum / count
	return sumSquares/count - mean*mean
}

// tenengrad returns mean squared Sobel gradient magnitude.
func tenengrad(luma *domain.Luma) float64 {
	var total float64
	count := float64(luma.Width * luma.Height)

	for y := 0; y < luma.Height; y++ {
		for x := 0; x < luma.Width; x++ {
			gx, gy := sobelAt(luma, x, y)
			total += gx*gx + gy*gy
		}
	}
	return total / count
}

// sobelAt returns the horizontal and vertical Sobel responses at x,y.
func sobelAt(luma *domain.Luma, x, y int) (gx, gy float64) {
	topLeft, top, topRight := luma.At(x-1, y-1), luma.At(x, y-1), luma.At(x+1, y-1)
	left, right := luma.At(x-1, y), luma.At(x+1, y)
	bottomLeft, bottom, bottomRight := luma.At(x-1, y+1), luma.At(x, y+1), luma.At(x+1, y+1)

	gx = (topRight + 2*right + bottomRight) - (topLeft + 2*left + bottomLeft)
	gy = (bottomLeft + 2*bottom + bottomRight) - (topLeft + 2*top + topRight)
	return gx, gy
}

// PercentileRank returns where value sits within population, in 0..1. This is
// how raw sharpness becomes a comparable score: the shoot itself supplies the
// scale, so a landscape set and a portrait set are each judged on their own
// terms rather than against a hardcoded constant.
func PercentileRank(sorted []float64, value float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return 0.5
	}
	below, equal := 0, 0
	for _, candidate := range sorted {
		switch {
		case candidate < value:
			below++
		case candidate == value:
			equal++
		}
	}
	rank := (float64(below) + 0.5*float64(equal)) / float64(len(sorted))
	return math.Min(1, math.Max(0, rank))
}
