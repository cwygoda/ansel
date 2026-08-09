package grouping

import (
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

var base = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

func frame(id string, hash uint64, offset time.Duration) domain.Image {
	return domain.Image{
		ID:             id,
		Path:           "/shoot/" + id + ".NEF",
		PerceptualHash: hash,
		Metadata:       domain.Metadata{CaptureTime: base.Add(offset)},
	}
}

func defaultOptions() Options {
	return Options{
		Window:      8 * time.Second,
		MaxDistance: 10,
		MaxDiameter: 10,
		BurstGap:    1500 * time.Millisecond,
	}
}

func groupContaining(groups []domain.SimilarityGroup, imageID string) *domain.SimilarityGroup {
	for i := range groups {
		for _, member := range groups[i].Members {
			if member == imageID {
				return &groups[i]
			}
		}
	}
	return nil
}

// The architecture calls this out explicitly as the failure mode to avoid: A
// resembles B and B resembles C, but A and C are unrelated. Naive connected
// components would merge all three.
func TestBuildDoesNotChainTransitiveSimilarity(t *testing.T) {
	images := []domain.Image{
		frame("a", 0x00000000000000FF, 0),
		frame("b", 0x000000000000FFFF, time.Second),
		frame("c", 0x0000000000FFFFFF, 2*time.Second),
	}

	groups := Build(images, defaultOptions())

	if len(groups) != 2 {
		t.Fatalf("Build produced %d groups, expected 2 (a+b, then c)", len(groups))
	}
	if groupContaining(groups, "a") != groupContaining(groups, "b") {
		t.Error("a and b are similar but landed in different groups")
	}
	if groupContaining(groups, "a") == groupContaining(groups, "c") {
		t.Error("a and c are dissimilar but were chained into one group through b")
	}
}

func TestBuildSplitsOnTemporalGap(t *testing.T) {
	identical := uint64(0x00000000000000FF)
	images := []domain.Image{
		frame("early", identical, 0),
		frame("late", identical, time.Hour),
	}

	groups := Build(images, defaultOptions())

	if len(groups) != 2 {
		t.Fatalf("Build produced %d groups, expected 2: identical frames an hour apart are not one moment", len(groups))
	}
}

func TestBuildGroupsIdenticalFramesInWindow(t *testing.T) {
	identical := uint64(0x00000000000000FF)
	images := []domain.Image{
		frame("one", identical, 0),
		frame("two", identical, 200*time.Millisecond),
		frame("three", identical, 400*time.Millisecond),
	}

	groups := Build(images, defaultOptions())

	if len(groups) != 1 {
		t.Fatalf("Build produced %d groups, expected 1", len(groups))
	}
	if got := len(groups[0].Members); got != 3 {
		t.Errorf("group has %d members, expected 3", got)
	}
	if groups[0].Kind != domain.GroupBurst {
		t.Errorf("group kind = %q, expected %q", groups[0].Kind, domain.GroupBurst)
	}
	if groups[0].Representative == "" {
		t.Error("group has no representative")
	}
	if similarity := groups[0].Similarities["one"]; similarity != 1 {
		t.Errorf("similarity of an identical frame = %.3f, expected 1", similarity)
	}
}

func TestBuildClassifiesLoneFrameAsSingle(t *testing.T) {
	groups := Build([]domain.Image{frame("only", 0x0F0F0F0F0F0F0F0F, 0)}, defaultOptions())

	if len(groups) != 1 {
		t.Fatalf("Build produced %d groups, expected 1", len(groups))
	}
	if groups[0].Kind != domain.GroupSingle {
		t.Errorf("group kind = %q, expected %q", groups[0].Kind, domain.GroupSingle)
	}
}

// A hash of zero means analysis never produced one. Such a frame must not be
// grouped, or every unanalyzable file in a shoot would collapse together.
func TestBuildKeepsUnhashableFramesApart(t *testing.T) {
	images := []domain.Image{
		frame("failed-one", 0, 0),
		frame("failed-two", 0, time.Second),
	}

	groups := Build(images, defaultOptions())

	if len(groups) != 2 {
		t.Fatalf("Build produced %d groups, expected 2 separate singles", len(groups))
	}
}

func TestBuildScopesGroupIDs(t *testing.T) {
	options := defaultOptions()
	options.IDPrefix = "gabc123"

	groups := Build([]domain.Image{frame("only", 0x0F, 0)}, options)

	if groups[0].ID != "gabc123-0001" {
		t.Errorf("group ID = %q, expected %q", groups[0].ID, "gabc123-0001")
	}
}

func TestBuildFallsBackToModificationTime(t *testing.T) {
	// No EXIF capture time, so ordering must come from the filesystem.
	images := []domain.Image{
		{ID: "b", PerceptualHash: 0xFF, MTimeNs: base.Add(time.Hour).UnixNano()},
		{ID: "a", PerceptualHash: 0xFF, MTimeNs: base.UnixNano()},
	}

	groups := Build(images, defaultOptions())

	if len(groups) != 2 {
		t.Fatalf("Build produced %d groups, expected 2", len(groups))
	}
	if groups[0].Members[0] != "a" {
		t.Errorf("first group holds %q, expected the earlier frame %q", groups[0].Members[0], "a")
	}
}
