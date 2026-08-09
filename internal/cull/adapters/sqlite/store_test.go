package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "cull.db"))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func sampleImage() domain.Image {
	return domain.Image{
		ID:             "abc123",
		Path:           "/shoot/DSC_1234.NEF",
		FileSize:       42,
		MTimeNs:        time.Now().UnixNano(),
		Fingerprint:    "fp-1",
		PerceptualHash: 0xDEADBEEFCAFEBABE,
		Metadata: domain.Metadata{
			CaptureTime: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
			Camera:      "Nikon Z6 III",
			ISO:         400,
		},
	}
}

func sampleObservations() domain.Observations {
	return domain.Observations{
		{Key: domain.KeySharpnessLaplacian, Value: 0.0421, Analyzer: "sharpness", Version: "1"},
		{Key: domain.KeyHighlightClipping, Value: 0.012, Analyzer: "clipping", Version: "1"},
	}
}

func TestSaveAndReuseAnalysis(t *testing.T) {
	store := openTestStore(t)
	img := sampleImage()

	if err := store.SaveAnalysis(img, "v1", sampleObservations()); err != nil {
		t.Fatalf("SaveAnalysis unexpected error: %v", err)
	}

	observations, hash, ok, err := store.CachedAnalysis(img.ID, img.Fingerprint, "v1")
	if err != nil {
		t.Fatalf("CachedAnalysis unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("analysis was not reused despite nothing changing")
	}
	if len(observations) != 2 {
		t.Errorf("recovered %d observations, expected 2", len(observations))
	}
	if hash != img.PerceptualHash {
		t.Errorf("perceptual hash = %#x, expected %#x", hash, img.PerceptualHash)
	}

	if value, present := observations.Value(domain.KeySharpnessLaplacian); !present || value != 0.0421 {
		t.Errorf("sharpness = %v (present %v), expected 0.0421", value, present)
	}
}

// A 64-bit hash must survive storage intact. Routing it through the REAL-typed
// observation store would silently truncate its low bits.
func TestPerceptualHashSurvivesFullRange(t *testing.T) {
	store := openTestStore(t)
	img := sampleImage()
	img.PerceptualHash = 0xFFFFFFFFFFFFFFFF

	if err := store.SaveAnalysis(img, "v1", sampleObservations()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, hash, _, err := store.CachedAnalysis(img.ID, img.Fingerprint, "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != 0xFFFFFFFFFFFFFFFF {
		t.Errorf("perceptual hash = %#x, expected %#x", hash, uint64(0xFFFFFFFFFFFFFFFF))
	}
}

func TestCacheInvalidation(t *testing.T) {
	tests := []struct {
		name        string
		fingerprint string
		version     string
	}{
		{"edited file", "fp-2", "v1"},
		{"revised analyzers", "fp-1", "v2"},
		{"both changed", "fp-2", "v2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := openTestStore(t)
			img := sampleImage()
			if err := store.SaveAnalysis(img, "v1", sampleObservations()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			_, _, ok, err := store.CachedAnalysis(img.ID, tc.fingerprint, tc.version)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Error("stale analysis was reused")
			}
		})
	}
}

func TestSaveAnalysisReplacesObservations(t *testing.T) {
	store := openTestStore(t)
	img := sampleImage()

	if err := store.SaveAnalysis(img, "v1", sampleObservations()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	replacement := domain.Observations{
		{Key: domain.KeySharpnessLaplacian, Value: 0.9, Analyzer: "sharpness", Version: "1"},
	}
	if err := store.SaveAnalysis(img, "v1", replacement); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	observations, _, _, err := store.CachedAnalysis(img.ID, img.Fingerprint, "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(observations) != 1 {
		t.Errorf("recovered %d observations, expected 1 after replacement", len(observations))
	}
}

func TestSaveGroupingIsIdempotent(t *testing.T) {
	store := openTestStore(t)
	groups := []domain.SimilarityGroup{{
		ID:             "gabc-0001",
		Kind:           domain.GroupBurst,
		Members:        []string{"one", "two"},
		Representative: "one",
		Similarities:   map[string]float64{"one": 1, "two": 0.9},
	}}
	ranks := map[string]domain.RankResult{
		"one": {GroupID: "gabc-0001", Score: 0.9, Position: 1, OutOf: 2},
		"two": {GroupID: "gabc-0001", Score: 0.7, Position: 2, OutOf: 2},
	}

	for attempt := 0; attempt < 2; attempt++ {
		if err := store.SaveGrouping(groups, ranks); err != nil {
			t.Fatalf("SaveGrouping attempt %d unexpected error: %v", attempt, err)
		}
	}

	var members int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM group_members`).Scan(&members); err != nil {
		t.Fatalf("failed to count members: %v", err)
	}
	if members != 2 {
		t.Errorf("group_members holds %d rows after two runs, expected 2", members)
	}
}

// Regrouping must produce the current answer, not accumulate every past one.
func TestSaveGroupingDropsEmptiedGroups(t *testing.T) {
	store := openTestStore(t)
	first := []domain.SimilarityGroup{{
		ID: "gabc-0001", Kind: domain.GroupBurst, Members: []string{"one", "two"}, Representative: "one",
	}}
	if err := store.SaveGrouping(first, map[string]domain.RankResult{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A stricter threshold splits them apart.
	second := []domain.SimilarityGroup{
		{ID: "gabc-0002", Kind: domain.GroupSingle, Members: []string{"one"}, Representative: "one"},
		{ID: "gabc-0003", Kind: domain.GroupSingle, Members: []string{"two"}, Representative: "two"},
	}
	if err := store.SaveGrouping(second, map[string]domain.RankResult{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var groups int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM "groups"`).Scan(&groups); err != nil {
		t.Fatalf("failed to count groups: %v", err)
	}
	if groups != 2 {
		t.Errorf("groups table holds %d rows, expected 2; the emptied group was not removed", groups)
	}
}

func TestSaveTagsReplacesOwnSourceOnly(t *testing.T) {
	store := openTestStore(t)

	if err := store.SaveTags("policy", map[string][]domain.Tag{
		"img": {{Name: "sharp", Source: "policy"}, {Name: "soft", Source: "policy"}},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.SaveTags("human", map[string][]domain.Tag{
		"img": {{Name: "keeper", Source: "human"}},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Policy changes its mind; the hand-applied tag must survive.
	if err := store.SaveTags("policy", map[string][]domain.Tag{
		"img": {{Name: "sharp", Source: "policy"}},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var policyTags, humanTags int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM tags WHERE source='policy'`).Scan(&policyTags); err != nil {
		t.Fatalf("failed to count policy tags: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM tags WHERE source='human'`).Scan(&humanTags); err != nil {
		t.Fatalf("failed to count human tags: %v", err)
	}

	if policyTags != 1 {
		t.Errorf("policy tags = %d, expected 1", policyTags)
	}
	if humanTags != 1 {
		t.Errorf("human tags = %d, expected 1; another source's tag was removed", humanTags)
	}
}

// Clearing must happen even when the new tag list is empty, which is why the
// source is passed explicitly rather than read off the tags.
func TestSaveTagsClearsWhenAllWithdrawn(t *testing.T) {
	store := openTestStore(t)

	if err := store.SaveTags("policy", map[string][]domain.Tag{
		"img": {{Name: "soft", Source: "policy"}},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.SaveTags("policy", map[string][]domain.Tag{"img": {}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var remaining int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM tags WHERE image_id='img'`).Scan(&remaining); err != nil {
		t.Fatalf("failed to count tags: %v", err)
	}
	if remaining != 0 {
		t.Errorf("tags = %d, expected 0 after all were withdrawn", remaining)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cull.db")

	for attempt := 0; attempt < 2; attempt++ {
		store, err := Open(path)
		if err != nil {
			t.Fatalf("Open attempt %d unexpected error: %v", attempt, err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("Close attempt %d unexpected error: %v", attempt, err)
		}
	}
}
