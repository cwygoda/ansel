package grouping

import (
	"fmt"
	"sort"
	"time"

	"github.com/cwygoda/ansel/internal/cull/analysis"
	"github.com/cwygoda/ansel/internal/cull/domain"
)

// hashBits is the width of the perceptual hash, used to turn a Hamming
// distance into a 0..1 similarity.
const hashBits = 64

// Options controls how aggressively photographs are grouped.
type Options struct {
	// Window is how far apart in capture time two photographs may be and
	// still be considered candidates for the same group.
	Window time.Duration

	// MaxDistance is the dHash distance within which a candidate must sit
	// relative to the group medoid.
	MaxDistance int

	// MaxDiameter caps the distance between the two most dissimilar members,
	// preventing a group from drifting across a long sequence.
	MaxDiameter int

	// BurstGap distinguishes a high-speed burst from deliberate alternate
	// frames of the same subject.
	BurstGap time.Duration

	// IDPrefix scopes generated group identifiers. The store is shared across
	// shoots, so a bare sequential number would collide between directories
	// and silently reassign one shoot's frames to another's group.
	IDPrefix string
}

// Build partitions photographs into similarity groups.
//
// Temporal proximity only proposes candidates; hash similarity decides. The
// two constraints together avoid the classic failure where A resembles B and B
// resembles C, but A and C are unrelated: candidates are compared against the
// group medoid rather than their nearest neighbour, and any group whose
// diameter would exceed the limit is split instead of merged.
func Build(images []domain.Image, opts Options) []domain.SimilarityGroup {
	ordered := make([]domain.Image, len(images))
	copy(ordered, images)
	sort.SliceStable(ordered, func(a, b int) bool {
		return captureTime(ordered[a]).Before(captureTime(ordered[b]))
	})

	var groups []domain.SimilarityGroup
	for _, segment := range splitByTime(ordered, opts.Window) {
		groups = append(groups, clusterSegment(segment, opts)...)
	}

	prefix := opts.IDPrefix
	if prefix == "" {
		prefix = "g"
	}
	for i := range groups {
		groups[i].ID = fmt.Sprintf("%s-%04d", prefix, i+1)
	}
	return groups
}

// splitByTime cuts the sequence wherever the gap between consecutive frames
// exceeds the candidate window, so unrelated parts of a shoot are never
// compared at all.
func splitByTime(ordered []domain.Image, window time.Duration) [][]domain.Image {
	var segments [][]domain.Image
	var current []domain.Image

	for i, img := range ordered {
		if i > 0 && captureTime(img).Sub(captureTime(ordered[i-1])) > window {
			segments = append(segments, current)
			current = nil
		}
		current = append(current, img)
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}
	return segments
}

// clusterSegment grows medoid-anchored clusters within one temporal segment.
func clusterSegment(segment []domain.Image, opts Options) []domain.SimilarityGroup {
	var clusters [][]domain.Image

	for _, img := range segment {
		placed := false
		for i := range clusters {
			if admits(clusters[i], img, opts) {
				clusters[i] = append(clusters[i], img)
				placed = true
				break
			}
		}
		if !placed {
			clusters = append(clusters, []domain.Image{img})
		}
	}

	groups := make([]domain.SimilarityGroup, 0, len(clusters))
	for _, cluster := range clusters {
		groups = append(groups, describe(cluster, opts))
	}
	return groups
}

// admits reports whether a candidate belongs in an existing cluster. An
// unhashable frame (analysis failed) joins nothing, so it stays a single.
func admits(cluster []domain.Image, candidate domain.Image, opts Options) bool {
	if candidate.PerceptualHash == 0 {
		return false
	}
	medoid := medoidOf(cluster)
	if medoid.PerceptualHash == 0 {
		return false
	}
	if analysis.HammingDistance(medoid.PerceptualHash, candidate.PerceptualHash) > opts.MaxDistance {
		return false
	}
	for _, member := range cluster {
		if analysis.HammingDistance(member.PerceptualHash, candidate.PerceptualHash) > opts.MaxDiameter {
			return false
		}
	}
	return true
}

// medoidOf returns the member with the smallest total distance to the others —
// a real photograph, never a synthetic average that corresponds to nothing.
func medoidOf(cluster []domain.Image) domain.Image {
	best, bestCost := cluster[0], -1
	for _, candidate := range cluster {
		cost := 0
		for _, other := range cluster {
			cost += analysis.HammingDistance(candidate.PerceptualHash, other.PerceptualHash)
		}
		if bestCost < 0 || cost < bestCost {
			best, bestCost = candidate, cost
		}
	}
	return best
}

func describe(cluster []domain.Image, opts Options) domain.SimilarityGroup {
	representative := medoidOf(cluster)
	members := make([]string, 0, len(cluster))
	similarities := make(map[string]float64, len(cluster))

	for _, img := range cluster {
		members = append(members, img.ID)
		distance := analysis.HammingDistance(representative.PerceptualHash, img.PerceptualHash)
		similarities[img.ID] = 1 - float64(distance)/hashBits
	}

	return domain.SimilarityGroup{
		Kind:           classify(cluster, opts.BurstGap),
		Members:        members,
		Representative: representative.ID,
		Similarities:   similarities,
	}
}

// classify names the group by how tightly its frames are spaced in time.
func classify(cluster []domain.Image, burstGap time.Duration) domain.GroupKind {
	if len(cluster) < 2 {
		return domain.GroupSingle
	}
	span := captureTime(cluster[len(cluster)-1]).Sub(captureTime(cluster[0]))
	if span <= burstGap*time.Duration(len(cluster)-1) {
		return domain.GroupBurst
	}
	return domain.GroupNearDuplicate
}

// captureTime falls back to file modification time when EXIF has no capture
// timestamp, so grouping still has a usable ordering.
func captureTime(img domain.Image) time.Time {
	if !img.Metadata.CaptureTime.IsZero() {
		return img.Metadata.CaptureTime
	}
	return time.Unix(0, img.MTimeNs)
}
