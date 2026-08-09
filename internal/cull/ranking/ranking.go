package ranking

import (
	"fmt"
	"sort"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// Weights are the relative contributions of each normalized score. They are
// configuration, not code: the point of rule-based ranking is that these can
// be tuned without touching the analyzers.
type Weights struct {
	Sharpness float64
	Exposure  float64
}

// Penalties are subtracted after the weighted sum, for defects severe enough
// that no amount of strength elsewhere should compensate.
type Penalties struct {
	SevereBlur              float64
	SevereHighlightClipping float64
	SevereShadowClipping    float64
}

// Thresholds decide when a penalty applies.
type Thresholds struct {
	BlurBelow          float64
	HighlightClipAbove float64
	ShadowClipAbove    float64
}

// Options is the complete ranking policy.
type Options struct {
	Weights    Weights
	Penalties  Penalties
	Thresholds Thresholds
}

// Rank orders the members of every group and explains each placement.
//
// Ranking is deliberately group-relative: the useful question is "which of
// these six frames is strongest", not "is this an objectively good
// photograph".
func Rank(groups []domain.SimilarityGroup, features map[string]domain.Features, opts Options) map[string]domain.RankResult {
	results := make(map[string]domain.RankResult)

	for _, group := range groups {
		scored := scoreMembers(group, features, opts)
		sort.SliceStable(scored, func(a, b int) bool { return scored[a].score > scored[b].score })

		for position, member := range scored {
			results[member.id] = domain.RankResult{
				GroupID:  group.ID,
				Score:    member.score,
				Position: position + 1,
				OutOf:    len(scored),
				Reasons:  reasonsFor(features[member.id], group, features, opts),
			}
		}
	}
	return results
}

type scoredMember struct {
	id    string
	score float64
}

func scoreMembers(group domain.SimilarityGroup, features map[string]domain.Features, opts Options) []scoredMember {
	scored := make([]scoredMember, 0, len(group.Members))
	for _, id := range group.Members {
		scored = append(scored, scoredMember{id: id, score: Score(features[id], opts)})
	}
	return scored
}

// Score combines the available normalized terms and applies penalties.
//
// Weights are renormalized over the terms actually present, so a photograph
// with no detected face is not silently scored out of a smaller maximum than
// one with a face. In this phase only sharpness and exposure exist; face and
// aesthetic terms will slot in without changing this arithmetic.
func Score(feature domain.Features, opts Options) float64 {
	if !feature.Analyzed {
		return 0
	}

	var weighted, totalWeight float64
	for _, term := range []struct{ weight, value float64 }{
		{opts.Weights.Sharpness, feature.Sharpness},
		{opts.Weights.Exposure, feature.ExposureQuality},
	} {
		if term.weight <= 0 {
			continue
		}
		weighted += term.weight * term.value
		totalWeight += term.weight
	}
	if totalWeight == 0 {
		return 0
	}

	return clamp(weighted/totalWeight - penaltyFor(feature, opts))
}

func penaltyFor(feature domain.Features, opts Options) float64 {
	var penalty float64
	if feature.Sharpness < opts.Thresholds.BlurBelow {
		penalty += opts.Penalties.SevereBlur
	}
	if feature.HighlightClipping > opts.Thresholds.HighlightClipAbove {
		penalty += opts.Penalties.SevereHighlightClipping
	}
	if feature.ShadowClipping > opts.Thresholds.ShadowClipAbove {
		penalty += opts.Penalties.SevereShadowClipping
	}
	return penalty
}

// reasonsFor explains a placement in the photographer's terms, so a
// recommendation can be argued with rather than merely accepted.
func reasonsFor(feature domain.Features, group domain.SimilarityGroup, features map[string]domain.Features, opts Options) []string {
	var reasons []string
	if !feature.Analyzed {
		return []string{"analysis unavailable"}
	}

	if len(group.Members) > 1 {
		if rank := sharpnessRank(feature, group, features); rank == 1 {
			reasons = append(reasons, fmt.Sprintf("sharpest of %d frames", len(group.Members)))
		} else {
			reasons = append(reasons, fmt.Sprintf("sharpness ranked %d of %d", rank, len(group.Members)))
		}
	}

	if feature.Sharpness < opts.Thresholds.BlurBelow {
		reasons = append(reasons, "severely soft")
	}
	if feature.HighlightClipping > opts.Thresholds.HighlightClipAbove {
		reasons = append(reasons, fmt.Sprintf("%.1f%% highlights clipped", feature.HighlightClipping*100))
	}
	if feature.ShadowClipping > opts.Thresholds.ShadowClipAbove {
		reasons = append(reasons, fmt.Sprintf("%.1f%% shadows clipped", feature.ShadowClipping*100))
	}
	if len(reasons) == 0 || allClean(feature, opts) {
		reasons = append(reasons, "no severe clipping")
	}
	return reasons
}

func sharpnessRank(feature domain.Features, group domain.SimilarityGroup, features map[string]domain.Features) int {
	rank := 1
	for _, id := range group.Members {
		if features[id].Sharpness > feature.Sharpness {
			rank++
		}
	}
	return rank
}

func allClean(feature domain.Features, opts Options) bool {
	return feature.HighlightClipping <= opts.Thresholds.HighlightClipAbove &&
		feature.ShadowClipping <= opts.Thresholds.ShadowClipAbove
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
