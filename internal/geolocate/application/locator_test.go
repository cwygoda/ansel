package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
	"github.com/cwygoda/ansel/internal/geolocate/matching"
	"github.com/cwygoda/ansel/internal/geolocate/ports"
)

// Fakes for the ports. They record what they were asked to do so the tests can
// assert on intent rather than on files.

type fakeMetadata struct {
	clocks  map[string]domain.CaptureClock
	located map[string]bool
}

func (f *fakeMetadata) Read(_ context.Context, paths []string) (map[string]domain.Photo, error) {
	photos := make(map[string]domain.Photo, len(paths))
	for _, path := range paths {
		clock, ok := f.clocks[filepath.Base(path)]
		if !ok {
			continue
		}
		photos[path] = domain.Photo{Path: path, Clock: clock}
	}
	return photos, nil
}

func (f *fakeMetadata) HasCoordinates(_ context.Context, paths []string) (map[string]bool, error) {
	found := make(map[string]bool, len(paths))
	for _, path := range paths {
		found[path] = f.located[filepath.Base(path)]
	}
	return found, nil
}

type fakeWriter struct {
	targets []string
}

func (f *fakeWriter) Write(_ context.Context, plans []domain.WritePlan) error {
	for i := range plans {
		if plans[i].Skipped != "" {
			continue
		}
		f.targets = append(f.targets, plans[i].Target)
		plans[i].Written = true
	}
	return nil
}

type fakeDecoder struct {
	track domain.Track
}

func (f fakeDecoder) Supports(path string) bool {
	return strings.HasSuffix(path, ".fit.xz")
}

func (f fakeDecoder) Decode(context.Context, string) (domain.Track, error) {
	return f.track, nil
}

// Helpers.

var trackStart = time.Date(2017, 8, 8, 18, 28, 54, 0, time.UTC)

// berlinRide runs due north for ten minutes from 18:28:54 UTC, sampled once a
// minute, and states the +02:00 it was recorded in exactly as a real FIT file
// does. The spacing is well inside the default gap limit, so these tests
// exercise the matching rules rather than trip over them.
func berlinRide() domain.Track {
	track := domain.Track{
		Source:       "ride.fit.xz",
		UTCOffset:    2 * time.Hour,
		HasUTCOffset: true,
	}
	for minute := 0; minute <= 10; minute++ {
		track.Points = append(track.Points, domain.TrackPoint{
			Time:         trackStart.Add(time.Duration(minute) * time.Minute),
			Latitude:     52.50 + 0.01*float64(minute),
			Longitude:    13.30,
			Elevation:    30 + float64(minute),
			HasElevation: true,
		})
	}
	return track
}

// wall builds an unzoned camera reading, which is what most cameras write.
func wall(hour, minute, second int) domain.CaptureClock {
	return domain.CaptureClock{
		Wall: time.Date(2017, 8, 8, hour, minute, second, 0, time.UTC),
	}
}

func shootDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return dir
}

func trackFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ride.fit.xz")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return path
}

func newLocator(t *testing.T, metadata *fakeMetadata, writer *fakeWriter, track domain.Track) *Locator {
	t.Helper()
	return &Locator{
		Metadata:   metadata,
		Writer:     writer,
		Decoders:   []ports.TrackDecoder{fakeDecoder{track: track}},
		Config:     DefaultConfig(),
		Matching:   matching.Options{MaxGap: 2 * time.Minute, Buffer: 5 * time.Minute},
		TrackPaths: []string{trackFile(t)},
	}
}

func locate(t *testing.T, locator *Locator, root string) domain.Result {
	t.Helper()
	result, err := locator.Locate(context.Background(), root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

// Tests.

// 20:28:54 Berlin is 18:28:54 UTC, the track's first point. Nothing about the
// machine running this test may enter into that.
func TestLocateUsesTheTrackOffsetForAnUnzonedCamera(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(20, 28, 54),
	}}

	result := locate(t, newLocator(t, metadata, &fakeWriter{}, berlinRide()), root)

	if len(result.Plans) != 1 {
		t.Fatalf("got %d plans, expected 1 (unlocated: %+v)", len(result.Plans), result.Unlocated)
	}
	if got := result.Plans[0].Fix.Method; got != domain.MethodExact {
		t.Errorf("method = %q, expected the first track point exactly", got)
	}
}

// A camera that recorded its own zone is the better authority, even when the
// track disagrees.
func TestLocatePrefersTheCameraOffsetOverTheTrack(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	clock := wall(23, 28, 54)
	clock.Offset, clock.HasOffset = 5*time.Hour, true // 23:28:54+05:00 == 18:28:54Z

	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{"DSC_0001.jpg": clock}}

	result := locate(t, newLocator(t, metadata, &fakeWriter{}, berlinRide()), root)

	if len(result.Plans) != 1 {
		t.Fatalf("got %d plans, expected 1 (unlocated: %+v)", len(result.Plans), result.Unlocated)
	}
	if got := result.Plans[0].Fix.Method; got != domain.MethodExact {
		t.Errorf("method = %q, expected the camera's own offset to be used", got)
	}
}

func TestLocateFallsBackToTheOperatorZone(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(20, 28, 54),
	}}

	track := berlinRide()
	track.HasUTCOffset = false // a format that does not state its zone

	locator := newLocator(t, metadata, &fakeWriter{}, track)
	zone, err := NewZone("Europe/Berlin", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	locator.Zone = zone

	result := locate(t, locator, root)

	if len(result.Plans) != 1 {
		t.Fatalf("got %d plans, expected 1 (unlocated: %+v)", len(result.Plans), result.Unlocated)
	}
}

// The one case that must never be guessed. Resolving against the machine's own
// zone would place the shoot wherever the operator happens to be sitting.
func TestLocateRefusesToGuessTheZone(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(20, 28, 54),
	}}

	track := berlinRide()
	track.HasUTCOffset = false

	result := locate(t, newLocator(t, metadata, &fakeWriter{}, track), root)

	if len(result.Plans) != 0 {
		t.Fatalf("expected no plans, got %d", len(result.Plans))
	}
	if len(result.Unlocated) != 1 {
		t.Fatalf("got %d unlocated, expected 1", len(result.Unlocated))
	}
	if !strings.Contains(result.Unlocated[0].Reason, "--tz") {
		t.Errorf("reason %q should tell the operator how to fix it", result.Unlocated[0].Reason)
	}
}

// Drift must move the position and the timestamp by the same amount, or the
// photograph ends up claiming a place and a time that disagree.
func TestLocateAppliesDriftToBothPositionAndTimestamp(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	// The camera reads 20:30:54, ninety seconds fast, so the shutter really
	// fired at 20:29:24 Berlin == 18:29:24 UTC, half a minute into the track.
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(20, 30, 54),
	}}

	locator := newLocator(t, metadata, &fakeWriter{}, berlinRide())
	locator.Drift = 90 * time.Second

	result := locate(t, locator, root)

	if len(result.Plans) != 1 {
		t.Fatalf("got %d plans, expected 1 (unlocated: %+v)", len(result.Plans), result.Unlocated)
	}
	plan := result.Plans[0]

	if !plan.WriteTime {
		t.Fatal("expected the corrected timestamp to be written")
	}
	expectedWall := time.Date(2017, 8, 8, 20, 29, 24, 0, time.UTC)
	if !plan.CorrectedWall.Equal(expectedWall) {
		t.Errorf("corrected wall = %s, expected %s", plan.CorrectedWall, expectedWall)
	}
	// Still the camera's zone: correcting drift is not a change of zone.
	if plan.CorrectedOffset != 2*time.Hour {
		t.Errorf("corrected offset = %s, expected 2h", plan.CorrectedOffset)
	}
	// Half a minute along a ten-minute leg from 52.50 to 52.60.
	if got := plan.Fix.Position.Latitude; got < 52.504 || got > 52.506 {
		t.Errorf("latitude = %.6f, expected the position 30s into the track", got)
	}
}

func TestLocateLeavesTimestampsAloneWithoutDrift(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(20, 28, 54),
	}}

	result := locate(t, newLocator(t, metadata, &fakeWriter{}, berlinRide()), root)

	if result.Plans[0].WriteTime {
		t.Error("expected no timestamp rewrite when no drift was given")
	}
}

func TestLocateTargetsASidecarByDefault(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(20, 28, 54),
	}}

	result := locate(t, newLocator(t, metadata, &fakeWriter{}, berlinRide()), root)

	expected := filepath.Join(root, "DSC_0001.xmp")
	if result.Plans[0].Target != expected {
		t.Errorf("target = %q, expected %q", result.Plans[0].Target, expected)
	}
}

func TestLocateTargetsThePhotographInPlace(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(20, 28, 54),
	}}

	locator := newLocator(t, metadata, &fakeWriter{}, berlinRide())
	locator.InPlace = true

	result := locate(t, locator, root)

	expected := filepath.Join(root, "DSC_0001.jpg")
	if result.Plans[0].Target != expected {
		t.Errorf("target = %q, expected the photograph itself", result.Plans[0].Target)
	}
}

// The default is a dry run, and a dry run must touch nothing at all.
func TestLocateWritesNothingByDefault(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(20, 28, 54),
	}}
	writer := &fakeWriter{}

	result := locate(t, newLocator(t, metadata, writer, berlinRide()), root)

	if len(writer.targets) != 0 {
		t.Errorf("a dry run wrote to %v", writer.targets)
	}
	if result.Written != 0 {
		t.Errorf("written = %d, expected 0", result.Written)
	}
	// The plan is still built, so the report says what a real run would do.
	if len(result.Plans) != 1 {
		t.Errorf("got %d plans, expected the dry run to plan the write anyway", len(result.Plans))
	}
}

func TestLocateWritesWhenAsked(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(20, 28, 54),
	}}
	writer := &fakeWriter{}

	locator := newLocator(t, metadata, writer, berlinRide())
	locator.Write = true

	result := locate(t, locator, root)

	if len(writer.targets) != 1 {
		t.Fatalf("wrote %d targets, expected 1", len(writer.targets))
	}
	if result.Written != 1 {
		t.Errorf("written = %d, expected 1", result.Written)
	}
}

// Coordinates already on disk may have been placed by hand.
func TestLocatePreservesExistingCoordinates(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{
		clocks:  map[string]domain.CaptureClock{"DSC_0001.jpg": wall(20, 28, 54)},
		located: map[string]bool{"DSC_0001.xmp": true},
	}
	writer := &fakeWriter{}

	locator := newLocator(t, metadata, writer, berlinRide())
	locator.Write = true

	result := locate(t, locator, root)

	if len(writer.targets) != 0 {
		t.Errorf("overwrote existing coordinates at %v", writer.targets)
	}
	if !result.Plans[0].Existing {
		t.Error("expected the plan to report existing coordinates")
	}
	if result.Plans[0].Skipped == "" {
		t.Error("expected the plan to record why it was skipped")
	}
	if len(result.Failures) != 0 {
		t.Errorf("a deliberate skip is not a failure: %+v", result.Failures)
	}
}

func TestLocateReplacesExistingCoordinatesWhenForced(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{
		clocks:  map[string]domain.CaptureClock{"DSC_0001.jpg": wall(20, 28, 54)},
		located: map[string]bool{"DSC_0001.xmp": true},
	}
	writer := &fakeWriter{}

	locator := newLocator(t, metadata, writer, berlinRide())
	locator.Write, locator.Force = true, true

	result := locate(t, locator, root)

	if len(writer.targets) != 1 {
		t.Fatalf("wrote %d targets, expected the forced replacement", len(writer.targets))
	}
	if result.Written != 1 {
		t.Errorf("written = %d, expected 1", result.Written)
	}
}

func TestLocateReportsPhotographsWithoutATimestamp(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{}}

	result := locate(t, newLocator(t, metadata, &fakeWriter{}, berlinRide()), root)

	if len(result.Unlocated) != 1 {
		t.Fatalf("got %d unlocated, expected 1", len(result.Unlocated))
	}
	if !strings.Contains(result.Unlocated[0].Reason, "timestamp") {
		t.Errorf("reason = %q, expected it to mention the missing timestamp", result.Unlocated[0].Reason)
	}
}

func TestLocateReportsPhotographsOutsideTheTrack(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(23, 0, 0), // hours after the ride ended
	}}

	result := locate(t, newLocator(t, metadata, &fakeWriter{}, berlinRide()), root)

	if len(result.Plans) != 0 {
		t.Fatalf("expected no plans, got %d", len(result.Plans))
	}
	if len(result.Unlocated) != 1 {
		t.Fatalf("got %d unlocated, expected 1", len(result.Unlocated))
	}
}

func TestLocateOnlyConsidersConfiguredExtensions(t *testing.T) {
	root := shootDir(t, "DSC_0001.jpg", "notes.txt", "DSC_0002.xmp")
	metadata := &fakeMetadata{clocks: map[string]domain.CaptureClock{
		"DSC_0001.jpg": wall(20, 28, 54),
	}}

	result := locate(t, newLocator(t, metadata, &fakeWriter{}, berlinRide()), root)

	if result.Discovered != 1 {
		t.Errorf("discovered = %d, expected only the photograph", result.Discovered)
	}
}

func TestSidecarPathForReplacesTheExtension(t *testing.T) {
	for _, tc := range []struct{ photo, expected string }{
		{"/shoot/DSC_1234.NEF", "/shoot/DSC_1234.xmp"},
		{"/shoot/DSC_1234.jpg", "/shoot/DSC_1234.xmp"},
		{"/shoot/a.b/DSC_1234.jpeg", "/shoot/a.b/DSC_1234.xmp"},
	} {
		t.Run(tc.photo, func(t *testing.T) {
			if got := SidecarPathFor(tc.photo); got != tc.expected {
				t.Errorf("SidecarPathFor(%q) = %q, expected %q", tc.photo, got, tc.expected)
			}
		})
	}
}
