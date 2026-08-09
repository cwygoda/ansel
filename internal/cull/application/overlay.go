package application

// overlayConfig applies user-supplied values over the defaults. Only fields
// the user actually set are copied, so a config file may specify a single
// threshold without having to restate everything else.
func overlayConfig(base *Config, override Config) {
	if override.DBPath != "" {
		base.DBPath = override.DBPath
	}
	if len(override.IncludeExtensions) > 0 {
		base.IncludeExtensions = override.IncludeExtensions
	}
	if override.MaxPreviewEdge > 0 {
		base.MaxPreviewEdge = override.MaxPreviewEdge
	}
	if override.Workers > 0 {
		base.Workers = override.Workers
	}
	if override.ExiftoolBinary != "" {
		base.ExiftoolBinary = override.ExiftoolBinary
	}

	overlaySimilarity(&base.Similarity, override.Similarity)
	overlayRanking(&base.Ranking, override.Ranking)
	overlayPolicy(&base.Policy, override.Policy)
	overlaySidecar(&base.Sidecar, override.Sidecar)
}

func overlaySimilarity(base *SimilarityConfig, override SimilarityConfig) {
	if override.WindowSeconds > 0 {
		base.WindowSeconds = override.WindowSeconds
	}
	if override.MaxDistance > 0 {
		base.MaxDistance = override.MaxDistance
	}
	if override.MaxDiameter > 0 {
		base.MaxDiameter = override.MaxDiameter
	}
	if override.BurstGapSeconds > 0 {
		base.BurstGapSeconds = override.BurstGapSeconds
	}
}

// overlayRanking merges per key rather than replacing whole maps, so adjusting
// one weight does not silently drop the others.
func overlayRanking(base *RankingConfig, override RankingConfig) {
	base.Weights = mergeFloats(base.Weights, override.Weights)
	base.Penalties = mergeFloats(base.Penalties, override.Penalties)
	base.Thresholds = mergeFloats(base.Thresholds, override.Thresholds)
}

func overlayPolicy(base *PolicyConfig, override PolicyConfig) {
	if override.SharpAbove > 0 {
		base.SharpAbove = override.SharpAbove
	}
	if override.SoftBelow > 0 {
		base.SoftBelow = override.SoftBelow
	}
	if override.SoftRelativeBelow > 0 {
		base.SoftRelativeBelow = override.SoftRelativeBelow
	}
	if override.HighlightClipWarning > 0 {
		base.HighlightClipWarning = override.HighlightClipWarning
	}
	if override.ShadowClipWarning > 0 {
		base.ShadowClipWarning = override.ShadowClipWarning
	}
	if override.UnderexposedMedian > 0 {
		base.UnderexposedMedian = override.UnderexposedMedian
	}
	if override.OverexposedMedian > 0 {
		base.OverexposedMedian = override.OverexposedMedian
	}
}

func overlaySidecar(base *SidecarConfig, override SidecarConfig) {
	if override.RatingBest > 0 {
		base.RatingBest = override.RatingBest
	}
	if override.RatingAlternate > 0 {
		base.RatingAlternate = override.RatingAlternate
	}
	if override.RatingUsable > 0 {
		base.RatingUsable = override.RatingUsable
	}
	if override.LabelBest != "" {
		base.LabelBest = override.LabelBest
	}
	if override.LabelWarning != "" {
		base.LabelWarning = override.LabelWarning
	}
}

func mergeFloats(base, override map[string]float64) map[string]float64 {
	if len(override) == 0 {
		return base
	}
	merged := make(map[string]float64, len(base)+len(override))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}
