package policy

import (
	"testing"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

func defaultOptions() Options {
	return Options{
		SharpAbove:           0.65,
		SoftBelow:            0.25,
		SoftRelativeBelow:    0.5,
		HighlightClipWarning: 0.03,
		ShadowClipWarning:    0.08,
		UnderexposedMedian:   0.15,
		OverexposedMedian:    0.85,
	}
}

func tagsFor(feature domain.Features, opts Options) map[string]bool {
	features := map[string]domain.Features{"img": feature}
	tagged := Tags(features, nil, map[string]domain.RankResult{}, opts)

	present := make(map[string]bool)
	for _, tag := range tagged["img"] {
		present[tag.Name] = true
	}
	return present
}

func TestTagsClassifySharpness(t *testing.T) {
	tests := []struct {
		name      string
		sharpness float64
		want      string
		notWant   string
	}{
		{"sharp", 0.90, domain.TagSharp, domain.TagSoft},
		{"soft", 0.10, domain.TagSoft, domain.TagSharp},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			present := tagsFor(domain.Features{
				Sharpness: tc.sharpness, SharpnessRelative: 0.2, Analyzed: true,
			}, defaultOptions())
			if !present[tc.want] {
				t.Errorf("expected tag %q, got %v", tc.want, present)
			}
			if present[tc.notWant] {
				t.Errorf("did not expect tag %q, got %v", tc.notWant, present)
			}
		})
	}
}

// A deliberately dark frame is a choice, not a mistake. Only darkness that has
// actually crushed detail should be called underexposed.
func TestTagsDoNotPunishIntentionalLowKey(t *testing.T) {
	lowKey := domain.Features{
		Sharpness:       0.80,
		LuminanceMedian: 0.08,
		ShadowClipping:  0.001,
		Analyzed:        true,
	}

	present := tagsFor(lowKey, defaultOptions())

	if present[domain.TagUnderexposed] {
		t.Error("a low-key frame with intact shadows was tagged underexposed")
	}
	if present[domain.TagTechnicalWarning] {
		t.Error("a low-key frame with intact shadows raised a technical warning")
	}
}

func TestTagsFlagCrushedShadows(t *testing.T) {
	crushed := domain.Features{
		Sharpness:       0.80,
		LuminanceMedian: 0.08,
		ShadowClipping:  0.40,
		Analyzed:        true,
	}

	present := tagsFor(crushed, defaultOptions())

	for _, want := range []string{domain.TagUnderexposed, domain.TagShadowsClipped, domain.TagTechnicalWarning} {
		if !present[want] {
			t.Errorf("expected tag %q, got %v", want, present)
		}
	}
}

func TestTagsDoNotPunishIntentionalHighKey(t *testing.T) {
	highKey := domain.Features{
		Sharpness:         0.80,
		LuminanceMedian:   0.92,
		HighlightClipping: 0.001,
		Analyzed:          true,
	}

	if present := tagsFor(highKey, defaultOptions()); present[domain.TagOverexposed] {
		t.Error("a high-key frame with intact highlights was tagged overexposed")
	}
}

func TestTagsMarkBestInGroup(t *testing.T) {
	features := map[string]domain.Features{
		"winner": {Sharpness: 0.9, Analyzed: true},
		"loser":  {Sharpness: 0.7, Analyzed: true},
	}
	groups := []domain.SimilarityGroup{{
		ID: "g-0001", Kind: domain.GroupBurst, Members: []string{"winner", "loser"},
	}}
	ranks := map[string]domain.RankResult{
		"winner": {GroupID: "g-0001", Position: 1, OutOf: 2},
		"loser":  {GroupID: "g-0001", Position: 2, OutOf: 2},
	}

	tagged := Tags(features, groups, ranks, defaultOptions())

	if !hasTag(tagged["winner"], domain.TagBestInGroup) {
		t.Error("the top-ranked frame is missing best_in_group")
	}
	if hasTag(tagged["loser"], domain.TagBestInGroup) {
		t.Error("a runner-up was tagged best_in_group")
	}
	if !hasTag(tagged["loser"], domain.TagSimilarGroup) {
		t.Error("a grouped frame is missing similar_group")
	}
}

// An unmeasured frame is not silently treated as fine.
func TestTagsWarnOnUnanalyzedFrame(t *testing.T) {
	if present := tagsFor(domain.Features{}, defaultOptions()); !present[domain.TagTechnicalWarning] {
		t.Error("an unanalyzed frame did not raise a technical warning")
	}
}

// Somebody is always last. Ranking low is not by itself evidence of a defect,
// so a shoot where every frame is comparably sharp must report none as soft.
func TestTagsDoNotCallLowestRankSoftWhenAllAreSharp(t *testing.T) {
	lastButFine := domain.Features{
		Sharpness:         0.02, // bottom of the run
		SharpnessRelative: 0.95, // yet almost exactly typical
		Analyzed:          true,
	}

	present := tagsFor(lastButFine, defaultOptions())

	if present[domain.TagSoft] {
		t.Error("the lowest-ranked frame of a uniformly sharp shoot was tagged soft")
	}
	if present[domain.TagTechnicalWarning] {
		t.Error("ranking last raised a technical warning on its own")
	}
}

func TestTagsCallGenuinelySoftFramesSoft(t *testing.T) {
	genuinelySoft := domain.Features{
		Sharpness:         0.02,
		SharpnessRelative: 0.15, // a fraction of a typical frame's detail
		Analyzed:          true,
	}

	present := tagsFor(genuinelySoft, defaultOptions())

	if !present[domain.TagSoft] {
		t.Errorf("a frame with a sixth of typical detail was not tagged soft, got %v", present)
	}
}

// A camera's embedded RAW preview carries far less sharpening than a
// delivered JPEG. Ranking them in one population measures the export
// settings, not the photograph, and reports every RAW file in a mixed
// directory as soft.
func TestNormalizeSeparatesRawFromRenderedPreviews(t *testing.T) {
	images := []domain.Image{
		{ID: "raw-best", Path: "/shoot/A.NEF"},
		{ID: "raw-worst", Path: "/shoot/B.NEF"},
		{ID: "jpeg", Path: "/shoot/C.jpg"},
	}
	observations := map[string]domain.Observations{
		// Both RAW previews sit far below the sharpened JPEG.
		"raw-best":  {{Key: domain.KeySharpnessLaplacian, Value: 0.009}},
		"raw-worst": {{Key: domain.KeySharpnessLaplacian, Value: 0.001}},
		"jpeg":      {{Key: domain.KeySharpnessLaplacian, Value: 0.220}},
	}

	features := Normalize(images, observations, defaultOptions())

	// Judged among RAW files, the better of the two must rank high rather than
	// being buried beneath an incomparable JPEG.
	if features["raw-best"].Sharpness < 0.5 {
		t.Errorf("the sharper RAW ranked %.3f, expected it to rank high among RAW files",
			features["raw-best"].Sharpness)
	}
	if features["raw-best"].Sharpness <= features["raw-worst"].Sharpness {
		t.Error("RAW files were not ordered against each other")
	}
	// A lone JPEG is typical of its own population, not exceptional.
	if features["jpeg"].SharpnessRelative != 1 {
		t.Errorf("the only JPEG scored %.3f relative to its class, expected 1",
			features["jpeg"].SharpnessRelative)
	}
}

func TestNormalizeReportsRelativeSharpness(t *testing.T) {
	images := []domain.Image{
		{ID: "low", Path: "/shoot/A.NEF"},
		{ID: "mid", Path: "/shoot/B.NEF"},
		{ID: "high", Path: "/shoot/C.NEF"},
	}
	observations := map[string]domain.Observations{
		"low":  {{Key: domain.KeySharpnessLaplacian, Value: 0.5}},
		"mid":  {{Key: domain.KeySharpnessLaplacian, Value: 1.0}},
		"high": {{Key: domain.KeySharpnessLaplacian, Value: 2.0}},
	}

	features := Normalize(images, observations, defaultOptions())

	if got := features["mid"].SharpnessRelative; got != 1 {
		t.Errorf("the median frame scored %.3f, expected 1", got)
	}
	if got := features["low"].SharpnessRelative; got != 0.5 {
		t.Errorf("a frame with half the median detail scored %.3f, expected 0.5", got)
	}
}

func TestNormalizeRanksSharpnessWithinRun(t *testing.T) {
	images := []domain.Image{{ID: "soft", Path: "/s/a.NEF"}, {ID: "sharp", Path: "/s/b.NEF"}}
	observations := map[string]domain.Observations{
		"soft":  {{Key: domain.KeySharpnessLaplacian, Value: 0.001}},
		"sharp": {{Key: domain.KeySharpnessLaplacian, Value: 0.900}},
	}

	features := Normalize(images, observations, defaultOptions())

	if features["sharp"].Sharpness <= features["soft"].Sharpness {
		t.Errorf("sharp ranked %.3f, soft ranked %.3f; expected sharp to be higher",
			features["sharp"].Sharpness, features["soft"].Sharpness)
	}
	if !features["sharp"].Analyzed {
		t.Error("a measured frame is not marked analyzed")
	}
}

func TestNormalizeMarksMissingObservations(t *testing.T) {
	features := Normalize([]domain.Image{{ID: "missing", Path: "/s/a.NEF"}}, map[string]domain.Observations{}, defaultOptions())

	if features["missing"].Analyzed {
		t.Error("a frame with no observations is marked analyzed")
	}
}

func TestExposureQualityFallsWithClipping(t *testing.T) {
	clean := exposureQuality(0, 0, defaultOptions())
	clipped := exposureQuality(0.5, 0.5, defaultOptions())

	if clean != 1 {
		t.Errorf("exposure quality of a clean frame = %.3f, expected 1", clean)
	}
	if clipped != 0 {
		t.Errorf("exposure quality of a heavily clipped frame = %.3f, expected 0", clipped)
	}
}

func hasTag(tags []domain.Tag, name string) bool {
	for _, tag := range tags {
		if tag.Name == name {
			return true
		}
	}
	return false
}
