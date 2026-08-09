package domain

// GroupKind classifies why a set of photographs ended up together.
type GroupKind string

const (
	GroupSingle        GroupKind = "single"
	GroupBurst         GroupKind = "burst"
	GroupNearDuplicate GroupKind = "near_duplicate"
)

// SimilarityGroup is a set of photographs depicting approximately the same
// moment or composition. Representative is a real member, not a synthetic
// centroid.
type SimilarityGroup struct {
	ID             string
	Kind           GroupKind
	Members        []string
	Representative string

	// Similarities scores each member against the representative, 0..1.
	Similarities map[string]float64
}

// RankResult is one photograph's standing inside its group, with the
// reasoning that produced it.
type RankResult struct {
	GroupID  string
	Score    float64
	Position int
	OutOf    int
	Reasons  []string
}

// BestInGroup reports whether this photograph ranked first among alternatives.
// A group of one has no alternatives, so nothing is "best".
func (r RankResult) BestInGroup() bool {
	return r.Position == 1 && r.OutOf > 1
}

// Tag is a human-readable label derived from scores by policy code. The raw
// score behind it stays available in the observation store.
type Tag struct {
	Name   string
	Source string
	Value  string
}

// Tag names emitted by policy.
const (
	TagSharp             = "sharp"
	TagSoft              = "soft"
	TagUnderexposed      = "underexposed"
	TagOverexposed       = "overexposed"
	TagHighlightsClipped = "highlights_clipped"
	TagShadowsClipped    = "shadows_clipped"
	TagSimilarGroup      = "similar_group"
	TagNearDuplicate     = "near_duplicate"
	TagBestInGroup       = "best_in_group"
	TagTechnicalWarning  = "technical_warning"
)

// PolicyTags is the complete vocabulary above.
//
// A sidecar write withdraws every one of these before adding back the ones
// that currently apply. That is how "replaces the tags contributed by one
// source" reads in a file format with no notion of a source: naming the whole
// vocabulary makes this run's contribution replaceable, while a keyword the
// photographer wrote is never named and so never touched. Without it a frame
// that was sharp last run and soft this one would end up carrying both.
func PolicyTags() []string {
	return []string{
		TagSharp, TagSoft,
		TagUnderexposed, TagOverexposed,
		TagHighlightsClipped, TagShadowsClipped,
		TagSimilarGroup, TagNearDuplicate, TagBestInGroup,
		TagTechnicalWarning,
	}
}

// SidecarPlan is one XMP sidecar the run intends to write. It is populated
// whether or not writing is enabled, so a dry run reports exactly what a real
// run would do.
type SidecarPlan struct {
	ImagePath   string
	SidecarPath string
	Rating      int
	Label       string
	Tags        []string

	// Exists reports that a sidecar is already on disk. Those are user data
	// (Lightroom, Capture One) and are preserved unless overwriting is forced.
	Exists  bool
	Written bool
	Skipped string
}

// CullResult summarizes one cull run.
type CullResult struct {
	Root     string
	Images   []Image
	Groups   []SimilarityGroup
	Ranks    map[string]RankResult
	Tags     map[string][]Tag
	Sidecars []SidecarPlan
	Failures []Failure

	Discovered int
	Analyzed   int
	Reused     int
	Written    int
}

// ImageByID indexes the run's images for report rendering.
func (r CullResult) ImageByID() map[string]Image {
	index := make(map[string]Image, len(r.Images))
	for _, img := range r.Images {
		index[img.ID] = img
	}
	return index
}
