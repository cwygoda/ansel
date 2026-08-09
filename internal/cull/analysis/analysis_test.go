package analysis

import (
	"context"
	"testing"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// checkerboard produces a maximally detailed image: every cell boundary is a
// hard edge, so it stands in for a sharply focused frame.
func checkerboard(size, cell int) *domain.Luma {
	luma := domain.NewLuma(size, size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if (x/cell+y/cell)%2 == 0 {
				luma.Set(x, y, 1)
			}
		}
	}
	return luma
}

// blur applies a box blur, standing in for a frame that missed focus.
func blur(src *domain.Luma, radius int) *domain.Luma {
	dst := domain.NewLuma(src.Width, src.Height)
	for y := 0; y < src.Height; y++ {
		for x := 0; x < src.Width; x++ {
			var sum float64
			count := 0
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					sum += src.At(x+dx, y+dy)
					count++
				}
			}
			dst.Set(x, y, sum/float64(count))
		}
	}
	return dst
}

func flat(size int, value float64) *domain.Luma {
	luma := domain.NewLuma(size, size)
	for i := range luma.Pix {
		luma.Pix[i] = value
	}
	return luma
}

func observationValue(t *testing.T, observations []domain.Observation, key string) float64 {
	t.Helper()
	for _, observation := range observations {
		if observation.Key == key {
			return observation.Value
		}
	}
	t.Fatalf("observation %q not found", key)
	return 0
}

func TestSharpnessRanksFocusedAboveBlurred(t *testing.T) {
	sharp := checkerboard(64, 4)
	soft := blur(sharp, 3)

	sharpObs, err := Sharpness{}.Analyze(context.Background(), Input{Preview: sharp})
	if err != nil {
		t.Fatalf("unexpected error analyzing sharp image: %v", err)
	}
	softObs, err := Sharpness{}.Analyze(context.Background(), Input{Preview: soft})
	if err != nil {
		t.Fatalf("unexpected error analyzing blurred image: %v", err)
	}

	for _, key := range []string{domain.KeySharpnessLaplacian, domain.KeySharpnessTenengrad} {
		t.Run(key, func(t *testing.T) {
			sharpValue := observationValue(t, sharpObs, key)
			softValue := observationValue(t, softObs, key)
			if sharpValue <= softValue {
				t.Errorf("%s = %.6f for sharp, %.6f for blurred; expected sharp to be higher",
					key, sharpValue, softValue)
			}
		})
	}
}

func TestSharpnessRejectsEmptyPreview(t *testing.T) {
	if _, err := (Sharpness{}).Analyze(context.Background(), Input{Preview: &domain.Luma{}}); err == nil {
		t.Error("Analyze on an empty preview expected error, got nil")
	}
}

func TestExposureReportsDistribution(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		minMedian float64
		maxMedian float64
	}{
		{"dark", 0.05, 0, 0.10},
		{"mid", 0.50, 0.45, 0.55},
		{"bright", 0.95, 0.90, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			observations, err := Exposure{}.Analyze(context.Background(), Input{Preview: flat(16, tc.value)})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			median := observationValue(t, observations, domain.KeyLuminanceMedian)
			if median < tc.minMedian || median > tc.maxMedian {
				t.Errorf("median = %.3f, expected within [%.2f, %.2f]", median, tc.minMedian, tc.maxMedian)
			}
		})
	}
}

func TestClippingSeparatesShadowsFromHighlights(t *testing.T) {
	tests := []struct {
		name          string
		value         float64
		wantShadow    float64
		wantHighlight float64
	}{
		{"all black", 0, 1, 0},
		{"all white", 1, 0, 1},
		{"mid grey", 0.5, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			observations, err := Clipping{}.Analyze(context.Background(), Input{Preview: flat(16, tc.value)})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			shadow := observationValue(t, observations, domain.KeyShadowClipping)
			highlight := observationValue(t, observations, domain.KeyHighlightClipping)

			if shadow != tc.wantShadow {
				t.Errorf("shadow clipping = %.3f, expected %.3f", shadow, tc.wantShadow)
			}
			if highlight != tc.wantHighlight {
				t.Errorf("highlight clipping = %.3f, expected %.3f", highlight, tc.wantHighlight)
			}
		})
	}
}

func TestPerceptualHashMatchesNearIdenticalFrames(t *testing.T) {
	base := checkerboard(64, 8)
	// A slight blur stands in for the next frame of a burst: the same scene,
	// marginally different pixels. Its hash must stay close.
	nearlyIdentical := blur(base, 1)
	different := flat(64, 0.5)

	baseHash := PerceptualHash(base)
	if distance := HammingDistance(baseHash, PerceptualHash(base)); distance != 0 {
		t.Errorf("identical images differ by %d bits, expected 0", distance)
	}

	near := HammingDistance(baseHash, PerceptualHash(nearlyIdentical))
	if near > 10 {
		t.Errorf("near-identical images differ by %d bits, expected at most 10", near)
	}

	far := HammingDistance(baseHash, PerceptualHash(different))
	if far <= near {
		t.Errorf("unrelated image differs by %d bits, near-identical by %d; expected unrelated to be larger", far, near)
	}
}

func TestPercentileRank(t *testing.T) {
	population := []float64{1, 2, 3, 4, 5}

	tests := []struct {
		name    string
		value   float64
		wantMin float64
		wantMax float64
	}{
		{"lowest", 1, 0, 0.2},
		{"middle", 3, 0.4, 0.6},
		{"highest", 5, 0.8, 1},
		{"above range", 99, 1, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rank := PercentileRank(population, tc.value)
			if rank < tc.wantMin || rank > tc.wantMax {
				t.Errorf("PercentileRank(%v) = %.3f, expected within [%.2f, %.2f]",
					tc.value, rank, tc.wantMin, tc.wantMax)
			}
		})
	}
}

// A single-image run has no scale to rank against; a neutral 0.5 keeps it from
// being treated as either the best or the worst frame of a shoot.
func TestPercentileRankWithoutPopulation(t *testing.T) {
	if rank := PercentileRank([]float64{7}, 7); rank != 0.5 {
		t.Errorf("PercentileRank of a single-value population = %.3f, expected 0.5", rank)
	}
	if rank := PercentileRank(nil, 7); rank != 0 {
		t.Errorf("PercentileRank of an empty population = %.3f, expected 0", rank)
	}
}

func TestSetVersionChangesWithAnalyzerSet(t *testing.T) {
	base := SetVersion([]Analyzer{Sharpness{}, Exposure{}})

	if same := SetVersion([]Analyzer{Sharpness{}, Exposure{}}); same != base {
		t.Errorf("SetVersion is not stable: %q then %q", base, same)
	}
	if fewer := SetVersion([]Analyzer{Sharpness{}}); fewer == base {
		t.Error("SetVersion did not change when an analyzer was removed")
	}
}
