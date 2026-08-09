package analysis

import (
	"math/bits"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// dHash compares each pixel with its right-hand neighbour on a 9x8 reduction,
// yielding 64 bits. Because it encodes gradient direction rather than absolute
// brightness, it is stable across exposure tweaks and rendering differences —
// which is exactly what near-duplicate detection needs.
const (
	hashWidth  = 8
	hashHeight = 8
)

// PerceptualHash returns a 64-bit dHash of the analysis preview.
//
// This is not an Analyzer: a hash is an identity fingerprint rather than a
// score, and a 64-bit value cannot survive the float64 observation store
// without losing its low bits.
func PerceptualHash(luma *domain.Luma) uint64 {
	if luma.Empty() {
		return 0
	}
	reduced := boxDownsample(luma, hashWidth+1, hashHeight)

	var hash uint64
	bit := 0
	for y := 0; y < hashHeight; y++ {
		for x := 0; x < hashWidth; x++ {
			if reduced.At(x, y) > reduced.At(x+1, y) {
				hash |= 1 << uint(bit)
			}
			bit++
		}
	}
	return hash
}

// HammingDistance counts differing bits between two hashes. 0 means visually
// identical; 64 means maximally different.
func HammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// boxDownsample averages source pixels into each destination cell. Averaging
// rather than point-sampling matters here: a point sample of a noisy frame
// produces an unstable hash, and burst frames would stop matching each other.
func boxDownsample(src *domain.Luma, width, height int) *domain.Luma {
	dst := domain.NewLuma(width, height)

	for y := 0; y < height; y++ {
		yStart := y * src.Height / height
		yEnd := max(yStart+1, (y+1)*src.Height/height)

		for x := 0; x < width; x++ {
			xStart := x * src.Width / width
			xEnd := max(xStart+1, (x+1)*src.Width/width)
			dst.Set(x, y, boxMean(src, xStart, xEnd, yStart, yEnd))
		}
	}
	return dst
}

func boxMean(src *domain.Luma, xStart, xEnd, yStart, yEnd int) float64 {
	var sum float64
	count := 0
	for y := yStart; y < yEnd; y++ {
		for x := xStart; x < xEnd; x++ {
			sum += src.At(x, y)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
