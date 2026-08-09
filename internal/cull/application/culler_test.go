package application

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/cull/analysis"
	"github.com/cwygoda/ansel/internal/cull/domain"
)

// The fakes below stand in for exiftool, libvips, SQLite and the sidecar
// writer. That the pipeline can be exercised without any of them is the point
// of keeping them behind ports.

type fakeMetadata struct {
	captureTimes map[string]time.Time

	// ratedSidecars names sidecar paths a photographer has already rated.
	ratedSidecars map[string]bool
}

func (f fakeMetadata) HasRating(_ context.Context, paths []string) (map[string]bool, error) {
	rated := make(map[string]bool, len(paths))
	for _, path := range paths {
		if f.ratedSidecars[path] {
			rated[path] = true
		}
	}
	return rated, nil
}

func (f fakeMetadata) Read(_ context.Context, paths []string) (map[string]domain.Metadata, error) {
	metadata := make(map[string]domain.Metadata, len(paths))
	for _, path := range paths {
		metadata[path] = domain.Metadata{
			CaptureTime: f.captureTimes[filepath.Base(path)],
			Camera:      "Test Camera",
		}
	}
	return metadata, nil
}

type fakePreviews struct{}

func (fakePreviews) Preview(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

// fakeDecoder turns the first byte of a file into a distinct image, so
// different fixtures produce different perceptual hashes.
type fakeDecoder struct{}

func (fakeDecoder) DecodeLuma(data []byte, _ int) (*domain.Luma, error) {
	luma := domain.NewLuma(32, 32)
	seed := 0
	if len(data) > 0 {
		seed = int(data[0])
	}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if (x+y+seed)%8 < 4 {
				luma.Set(x, y, 1)
			}
		}
	}
	return luma, nil
}

type fakeStore struct {
	cached      map[string]domain.Observations
	savedGroups []domain.SimilarityGroup
	savedTags   map[string][]domain.Tag
	tagSource   string
	saveCalls   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{cached: map[string]domain.Observations{}}
}

func (f *fakeStore) CachedAnalysis(imageID, _, _ string) (domain.Observations, uint64, bool, error) {
	observations, ok := f.cached[imageID]
	return observations, 1, ok, nil
}

func (f *fakeStore) SaveAnalysis(domain.Image, string, domain.Observations) error {
	f.saveCalls++
	return nil
}

func (f *fakeStore) SaveGrouping(groups []domain.SimilarityGroup, _ map[string]domain.RankResult) error {
	f.savedGroups = groups
	return nil
}

func (f *fakeStore) SaveTags(source string, tags map[string][]domain.Tag) error {
	f.tagSource = source
	f.savedTags = tags
	return nil
}

type fakeSidecars struct {
	written []string
}

func (f *fakeSidecars) Write(_ context.Context, plan domain.SidecarPlan) error {
	f.written = append(f.written, plan.SidecarPath)
	return os.WriteFile(plan.SidecarPath, []byte("<xmp/>"), 0644)
}

func newTestCuller(t *testing.T, store *fakeStore, sidecars *fakeSidecars) *Culler {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Workers = 2

	return &Culler{
		Metadata:  fakeMetadata{captureTimes: map[string]time.Time{}},
		Previews:  fakePreviews{},
		Decoder:   fakeDecoder{},
		Store:     store,
		Sidecars:  sidecars,
		Config:    cfg,
		Analyzers: analysis.Default(),
	}
}

func shootDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, fixture := range []struct {
		name    string
		content []byte
	}{
		{"DSC_0001.jpg", []byte{10, 'a'}},
		{"DSC_0002.jpg", []byte{90, 'b'}},
		{"DSC_0003.jpg", []byte{200, 'c'}},
	} {
		if err := os.WriteFile(filepath.Join(dir, fixture.name), fixture.content, 0644); err != nil {
			t.Fatalf("failed to write fixture: %v", err)
		}
	}
	return dir
}

func countXMP(t *testing.T, dir string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.xmp"))
	if err != nil {
		t.Fatalf("failed to list sidecars: %v", err)
	}
	return len(matches)
}

// The default contract: a run reports what it would do and changes nothing.
func TestCullDryRunWritesNothing(t *testing.T) {
	dir := shootDir(t)
	sidecars := &fakeSidecars{}
	culler := newTestCuller(t, newFakeStore(), sidecars)

	result, err := culler.Cull(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Discovered != 3 {
		t.Errorf("discovered %d images, expected 3", result.Discovered)
	}
	if result.Analyzed != 3 {
		t.Errorf("analyzed %d images, expected 3", result.Analyzed)
	}
	if len(sidecars.written) != 0 {
		t.Errorf("a dry run wrote %d sidecars, expected none", len(sidecars.written))
	}
	if count := countXMP(t, dir); count != 0 {
		t.Errorf("a dry run left %d sidecars on disk, expected none", count)
	}

	// The plan is still fully computed, which is what makes the report useful.
	if len(result.Sidecars) != 3 {
		t.Errorf("planned %d sidecars, expected 3", len(result.Sidecars))
	}
	if result.Written != 0 {
		t.Errorf("Written = %d, expected 0", result.Written)
	}
}

func TestCullWriteEmitsSidecars(t *testing.T) {
	dir := shootDir(t)
	sidecars := &fakeSidecars{}
	culler := newTestCuller(t, newFakeStore(), sidecars)
	culler.Write = true

	result, err := culler.Cull(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Written != 3 {
		t.Errorf("wrote %d sidecars, expected 3", result.Written)
	}
	if count := countXMP(t, dir); count != 3 {
		t.Errorf("found %d sidecars on disk, expected 3", count)
	}
}

// rateSidecars tells the metadata fake that a photographer has already rated
// the given sidecars.
func rateSidecars(culler *Culler, paths ...string) {
	metadata, _ := culler.Metadata.(fakeMetadata)
	metadata.ratedSidecars = make(map[string]bool, len(paths))
	for _, path := range paths {
		metadata.ratedSidecars[path] = true
	}
	culler.Metadata = metadata
}

// A rating in a sidecar is the photographer's own judgement; it must survive.
func TestCullPreservesRatedSidecars(t *testing.T) {
	dir := shootDir(t)
	existing := filepath.Join(dir, "DSC_0001.xmp")
	if err := os.WriteFile(existing, []byte("original"), 0644); err != nil {
		t.Fatalf("failed to seed sidecar: %v", err)
	}

	culler := newTestCuller(t, newFakeStore(), &fakeSidecars{})
	culler.Write = true
	rateSidecars(culler, existing)

	result, err := culler.Cull(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("failed to read sidecar: %v", err)
	}
	if string(content) != "original" {
		t.Errorf("a rated sidecar was overwritten: %q", content)
	}
	if result.Written != 2 {
		t.Errorf("wrote %d sidecars, expected 2 (one was kept)", result.Written)
	}
}

// The reported failure: `ansel geolocate --write` leaves a sidecar holding
// coordinates and nothing else. Nobody has judged that photograph, so a cull
// run afterwards must still rate it. Skipping on mere existence meant a shoot
// could be geolocated or culled, but never both.
func TestCullRatesASidecarThatOnlyHoldsCoordinates(t *testing.T) {
	dir := shootDir(t)
	located := filepath.Join(dir, "DSC_0001.xmp")
	if err := os.WriteFile(located, []byte("<gps-only/>"), 0644); err != nil {
		t.Fatalf("failed to seed sidecar: %v", err)
	}

	sidecars := &fakeSidecars{}
	culler := newTestCuller(t, newFakeStore(), sidecars)
	culler.Write = true

	result, err := culler.Cull(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Written != 3 {
		t.Errorf("wrote %d sidecars, expected all 3", result.Written)
	}
	if !slices.Contains(sidecars.written, located) {
		t.Errorf("the geolocated sidecar was skipped; written: %v", sidecars.written)
	}
}

func TestCullForceReplacesRatedSidecars(t *testing.T) {
	dir := shootDir(t)
	existing := filepath.Join(dir, "DSC_0001.xmp")
	if err := os.WriteFile(existing, []byte("original"), 0644); err != nil {
		t.Fatalf("failed to seed sidecar: %v", err)
	}

	culler := newTestCuller(t, newFakeStore(), &fakeSidecars{})
	culler.Write = true
	culler.Force = true
	rateSidecars(culler, existing)

	result, err := culler.Cull(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("failed to read sidecar: %v", err)
	}
	if string(content) == "original" {
		t.Error("--force did not replace the existing sidecar")
	}
	if result.Written != 3 {
		t.Errorf("wrote %d sidecars, expected 3", result.Written)
	}
}

func TestCullReusesCachedAnalysis(t *testing.T) {
	dir := shootDir(t)
	store := newFakeStore()
	culler := newTestCuller(t, store, &fakeSidecars{})

	first, err := culler.Cull(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, img := range first.Images {
		store.cached[img.ID] = domain.Observations{
			{Key: domain.KeySharpnessLaplacian, Value: 0.5, Analyzer: "sharpness", Version: "1"},
		}
	}

	second, err := culler.Cull(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if second.Reused != 3 {
		t.Errorf("reused %d results, expected 3", second.Reused)
	}
	if second.Analyzed != 0 {
		t.Errorf("re-analyzed %d images, expected 0", second.Analyzed)
	}
}

func TestCullReanalyzeIgnoresCache(t *testing.T) {
	dir := shootDir(t)
	store := newFakeStore()
	culler := newTestCuller(t, store, &fakeSidecars{})
	culler.Reanalyze = true

	result, err := culler.Cull(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, img := range result.Images {
		store.cached[img.ID] = domain.Observations{{Key: domain.KeySharpnessLaplacian, Value: 0.5}}
	}

	second, err := culler.Cull(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.Reused != 0 {
		t.Errorf("--reanalyze reused %d results, expected 0", second.Reused)
	}
}

func TestCullPersistsResults(t *testing.T) {
	store := newFakeStore()
	culler := newTestCuller(t, store, &fakeSidecars{})

	if _, err := culler.Cull(context.Background(), shootDir(t)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.saveCalls != 3 {
		t.Errorf("SaveAnalysis called %d times, expected 3", store.saveCalls)
	}
	if len(store.savedGroups) == 0 {
		t.Error("no groups were persisted")
	}
	if store.tagSource == "" {
		t.Error("tags were persisted without a source")
	}
}

func TestCullOnEmptyDirectory(t *testing.T) {
	culler := newTestCuller(t, newFakeStore(), &fakeSidecars{})

	result, err := culler.Cull(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Discovered != 0 {
		t.Errorf("discovered %d images in an empty directory", result.Discovered)
	}
}
