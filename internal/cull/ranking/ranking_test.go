package ranking

import (
	"math"
	"testing"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

func defaultOptions() Options {
	return Options{
		Weights:   Weights{Sharpness: 0.30, Exposure: 0.15},
		Penalties: Penalties{SevereBlur: 0.30, SevereHighlightClipping: 0.10, SevereShadowClipping: 0.10},
		Thresholds: Thresholds{
			BlurBelow:          0.15,
			HighlightClipAbove: 0.03,
			ShadowClipAbove:    0.08,
		},
	}
}

func analyzed(sharpness, exposure float64) domain.Features {
	return domain.Features{Sharpness: sharpness, ExposureQuality: exposure, Analyzed: true}
}

// Weights sum to 0.45, not 1. A flawless frame must still score 1.0, or every
// photograph would be capped at the arbitrary total of whichever terms happen
// to exist in this phase.
func TestScoreRenormalizesOverPresentTerms(t *testing.T) {
	if score := Score(analyzed(1, 1), defaultOptions()); math.Abs(score-1) > 1e-9 {
		t.Errorf("Score of a flawless frame = %.6f, expected 1", score)
	}
	if score := Score(analyzed(0, 0), defaultOptions()); score != 0 {
		t.Errorf("Score of a worthless frame = %.6f, expected 0", score)
	}
}

func TestScoreWeightsSharpnessAboveExposure(t *testing.T) {
	sharpOnly := Score(analyzed(1, 0), defaultOptions())
	exposedOnly := Score(analyzed(0, 1), defaultOptions())

	if sharpOnly <= exposedOnly {
		t.Errorf("sharpness-only = %.3f, exposure-only = %.3f; expected sharpness to carry more weight",
			sharpOnly, exposedOnly)
	}
}

// A term with zero weight must drop out entirely rather than count as a zero
// score, which would silently punish every photograph.
func TestScoreIgnoresZeroWeightedTerms(t *testing.T) {
	options := defaultOptions()
	options.Weights.Exposure = 0

	if score := Score(analyzed(1, 0), options); math.Abs(score-1) > 1e-9 {
		t.Errorf("Score = %.6f, expected 1 when the only weighted term is perfect", score)
	}
}

func TestScoreAppliesPenalties(t *testing.T) {
	tests := []struct {
		name     string
		feature  domain.Features
		lessThan float64
	}{
		{
			name:     "severe blur",
			feature:  domain.Features{Sharpness: 0.10, ExposureQuality: 1, Analyzed: true},
			lessThan: 0.40,
		},
		{
			name: "highlight clipping",
			feature: domain.Features{
				Sharpness: 1, ExposureQuality: 1, HighlightClipping: 0.20, Analyzed: true,
			},
			lessThan: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if score := Score(tc.feature, defaultOptions()); score >= tc.lessThan {
				t.Errorf("Score = %.3f, expected below %.3f", score, tc.lessThan)
			}
		})
	}
}

// A frame that could not be measured must never win its group by default.
func TestScoreOfUnanalyzedFrameIsZero(t *testing.T) {
	if score := Score(domain.Features{Sharpness: 1, ExposureQuality: 1}, defaultOptions()); score != 0 {
		t.Errorf("Score of an unanalyzed frame = %.3f, expected 0", score)
	}
}

func TestRankOrdersGroupBySharpness(t *testing.T) {
	group := domain.SimilarityGroup{ID: "g-0001", Members: []string{"soft", "sharp", "middle"}}
	features := map[string]domain.Features{
		"soft":   analyzed(0.2, 1),
		"sharp":  analyzed(0.9, 1),
		"middle": analyzed(0.5, 1),
	}

	ranks := Rank([]domain.SimilarityGroup{group}, features, defaultOptions())

	if ranks["sharp"].Position != 1 {
		t.Errorf("sharpest frame ranked %d, expected 1", ranks["sharp"].Position)
	}
	if !ranks["sharp"].BestInGroup() {
		t.Error("sharpest frame is not marked best in group")
	}
	if ranks["soft"].Position != 3 {
		t.Errorf("softest frame ranked %d, expected 3", ranks["soft"].Position)
	}
	if ranks["sharp"].OutOf != 3 {
		t.Errorf("OutOf = %d, expected 3", ranks["sharp"].OutOf)
	}
	if len(ranks["sharp"].Reasons) == 0 {
		t.Error("ranking produced no explanation")
	}
}

// A lone photograph has no alternative it was chosen over, so calling it
// "best in group" would overstate what the analysis actually established.
func TestRankDoesNotMarkLoneFrameAsBest(t *testing.T) {
	group := domain.SimilarityGroup{ID: "g-0001", Members: []string{"only"}}
	features := map[string]domain.Features{"only": analyzed(0.9, 1)}

	ranks := Rank([]domain.SimilarityGroup{group}, features, defaultOptions())

	if ranks["only"].BestInGroup() {
		t.Error("a single-frame group reported best_in_group")
	}
}
