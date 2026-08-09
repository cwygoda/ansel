package policy

import (
	"github.com/cwygoda/ansel/internal/cull/domain"
)

// Source names this package as the origin of a tag, so machine-derived labels
// stay distinguishable from anything a person applied.
const Source = "policy"

// Options are the tagging thresholds. All of them are configuration; changing
// one recomputes tags without re-running analysis.
type Options struct {
	SharpAbove float64
	SoftBelow  float64

	// SoftRelativeBelow is how far below a typical frame a photograph must
	// fall before it is called soft, as a multiple of the median.
	//
	// Rank alone is not evidence of a defect: a quarter of any shoot sits in
	// the bottom quarter, however sharp every frame is. Requiring measurably
	// less detail as well means a uniformly sharp shoot reports nothing soft,
	// which is the honest answer.
	SoftRelativeBelow float64

	HighlightClipWarning float64
	ShadowClipWarning    float64
	UnderexposedMedian   float64
	OverexposedMedian    float64
}

// Tags converts scores into human-readable labels. The raw score behind every
// tag remains in the store, so a tag is always arguable.
func Tags(
	features map[string]domain.Features,
	groups []domain.SimilarityGroup,
	ranks map[string]domain.RankResult,
	opts Options,
) map[string][]domain.Tag {
	byImage := make(map[string][]domain.Tag, len(features))
	kinds := groupKinds(groups)

	for imageID, feature := range features {
		var tags []domain.Tag
		tags = append(tags, technicalTags(feature, opts)...)
		tags = append(tags, groupTags(imageID, kinds, ranks)...)
		if hasWarning(feature, opts) {
			tags = append(tags, tag(domain.TagTechnicalWarning))
		}
		byImage[imageID] = tags
	}
	return byImage
}

func technicalTags(feature domain.Features, opts Options) []domain.Tag {
	if !feature.Analyzed {
		return nil
	}
	var tags []domain.Tag

	switch {
	case feature.Sharpness >= opts.SharpAbove:
		tags = append(tags, tag(domain.TagSharp))
	case isSoft(feature, opts):
		tags = append(tags, tag(domain.TagSoft))
	}

	if feature.HighlightClipping > opts.HighlightClipWarning {
		tags = append(tags, tag(domain.TagHighlightsClipped))
	}
	if feature.ShadowClipping > opts.ShadowClipWarning {
		tags = append(tags, tag(domain.TagShadowsClipped))
	}
	return append(tags, exposureTags(feature, opts)...)
}

// exposureTags require both an extreme median and actual clipping. Brightness
// alone is a choice; brightness plus lost detail is a problem.
func exposureTags(feature domain.Features, opts Options) []domain.Tag {
	var tags []domain.Tag
	if feature.LuminanceMedian < opts.UnderexposedMedian && feature.ShadowClipping > opts.ShadowClipWarning {
		tags = append(tags, tag(domain.TagUnderexposed))
	}
	if feature.LuminanceMedian > opts.OverexposedMedian && feature.HighlightClipping > opts.HighlightClipWarning {
		tags = append(tags, tag(domain.TagOverexposed))
	}
	return tags
}

func groupTags(imageID string, kinds map[string]domain.GroupKind, ranks map[string]domain.RankResult) []domain.Tag {
	rank, ok := ranks[imageID]
	if !ok {
		return nil
	}
	var tags []domain.Tag
	if rank.OutOf > 1 {
		tags = append(tags, tag(domain.TagSimilarGroup))
	}
	if kinds[rank.GroupID] == domain.GroupNearDuplicate {
		tags = append(tags, tag(domain.TagNearDuplicate))
	}
	if rank.BestInGroup() {
		tags = append(tags, tag(domain.TagBestInGroup))
	}
	return tags
}

// isSoft requires both a low rank and measurably less detail than a typical
// frame, so a shoot in which everything is sharp reports nothing soft.
func isSoft(feature domain.Features, opts Options) bool {
	return feature.Sharpness <= opts.SoftBelow &&
		feature.SharpnessRelative < opts.SoftRelativeBelow
}

func hasWarning(feature domain.Features, opts Options) bool {
	if !feature.Analyzed {
		return true
	}
	return isSoft(feature, opts) ||
		feature.HighlightClipping > opts.HighlightClipWarning ||
		feature.ShadowClipping > opts.ShadowClipWarning
}

func groupKinds(groups []domain.SimilarityGroup) map[string]domain.GroupKind {
	kinds := make(map[string]domain.GroupKind, len(groups))
	for _, group := range groups {
		kinds[group.ID] = group.Kind
	}
	return kinds
}

func tag(name string) domain.Tag {
	return domain.Tag{Name: name, Source: Source}
}
