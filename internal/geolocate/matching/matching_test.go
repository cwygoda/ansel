package matching

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

var origin = time.Date(2017, 8, 8, 18, 0, 0, 0, time.UTC)

func at(seconds int) time.Time {
	return origin.Add(time.Duration(seconds) * time.Second)
}

func point(seconds int, latitude, longitude float64) domain.TrackPoint {
	return domain.TrackPoint{Time: at(seconds), Latitude: latitude, Longitude: longitude}
}

// ride is a short, evenly spaced recording along a meridian.
func ride() domain.Track {
	return domain.Track{
		Source: "ride.fit.xz",
		Points: []domain.TrackPoint{
			point(0, 52.50, 13.30),
			point(10, 52.51, 13.30),
			point(20, 52.52, 13.30),
		},
	}
}

func defaults() Options {
	return Options{MaxGap: 2 * time.Minute, Buffer: 5 * time.Minute}
}

func TestLocateFindsAnExactPoint(t *testing.T) {
	got := Locate([]domain.Track{ride()}, at(10), defaults())

	assertFound(t, got)
	if got.Fix.Method != domain.MethodExact {
		t.Errorf("method = %q, expected %q", got.Fix.Method, domain.MethodExact)
	}
	assertClose(t, "latitude", got.Fix.Position.Latitude, 52.51)
	if got.Fix.Gap != 0 {
		t.Errorf("gap = %s, expected 0 for an exact point", got.Fix.Gap)
	}
	if got.Fix.Source != "ride.fit.xz" {
		t.Errorf("source = %q, expected the track path", got.Fix.Source)
	}
}

func TestLocateInterpolatesBetweenPoints(t *testing.T) {
	got := Locate([]domain.Track{ride()}, at(5), defaults())

	assertFound(t, got)
	if got.Fix.Method != domain.MethodInterpolated {
		t.Errorf("method = %q, expected %q", got.Fix.Method, domain.MethodInterpolated)
	}
	assertClose(t, "latitude", got.Fix.Position.Latitude, 52.505)
	if got.Fix.Gap != 10*time.Second {
		t.Errorf("gap = %s, expected the 10s spacing of the bracketing points", got.Fix.Gap)
	}
}

// A device that stopped recording for an hour has not described where the
// rider went; inferring a straight line across that is a fabrication.
func TestLocateRefusesToInterpolateAcrossATooWideGap(t *testing.T) {
	sparse := domain.Track{
		Source: "sparse.fit.xz",
		Points: []domain.TrackPoint{point(0, 52.50, 13.30), point(3600, 52.90, 13.30)},
	}

	got := Locate([]domain.Track{sparse}, at(1800), defaults())

	assertNotFound(t, got)
	if !strings.Contains(got.Reason, "gap") {
		t.Errorf("reason = %q, expected it to name the gap", got.Reason)
	}
}

func TestLocateClampsWithinTheBuffer(t *testing.T) {
	for _, tc := range []struct {
		name     string
		second   int
		method   domain.Method
		latitude float64
		gap      time.Duration
	}{
		{"before the start", -60, domain.MethodClampedStart, 52.50, time.Minute},
		{"after the end", 80, domain.MethodClampedEnd, 52.52, time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Locate([]domain.Track{ride()}, at(tc.second), defaults())

			assertFound(t, got)
			if got.Fix.Method != tc.method {
				t.Errorf("method = %q, expected %q", got.Fix.Method, tc.method)
			}
			assertClose(t, "latitude", got.Fix.Position.Latitude, tc.latitude)
			if got.Fix.Gap != tc.gap {
				t.Errorf("gap = %s, expected %s", got.Fix.Gap, tc.gap)
			}
		})
	}
}

func TestLocateRejectsBeyondTheBuffer(t *testing.T) {
	for _, tc := range []struct {
		name   string
		second int
	}{
		{"long before the start", -3600},
		{"long after the end", 3600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Locate([]domain.Track{ride()}, at(tc.second), defaults())

			assertNotFound(t, got)
			if !strings.Contains(got.Reason, "no track covers") {
				t.Errorf("reason = %q, expected it to say no track covers the time", got.Reason)
			}
		})
	}
}

func TestLocateDoesNotClampWhenTheBufferIsZero(t *testing.T) {
	got := Locate([]domain.Track{ride()}, at(-1), Options{MaxGap: 2 * time.Minute})

	assertNotFound(t, got)
}

func TestLocateInterpolatesAnyGapWhenMaxGapIsZero(t *testing.T) {
	sparse := domain.Track{
		Source: "sparse.fit.xz",
		Points: []domain.TrackPoint{point(0, 52.50, 13.30), point(3600, 52.90, 13.30)},
	}

	got := Locate([]domain.Track{sparse}, at(1800), Options{Buffer: time.Minute})

	assertFound(t, got)
	if got.Fix.Method != domain.MethodInterpolated {
		t.Errorf("method = %q, expected %q", got.Fix.Method, domain.MethodInterpolated)
	}
}

// The reason tracks are never merged: a photograph taken in the hours between
// the morning ride and the evening one belongs to neither, and must not be
// placed on the line joining the end of one to the start of the other.
func TestLocateNeverInterpolatesBetweenTwoTracks(t *testing.T) {
	morning := domain.Track{
		Source: "morning.fit.xz",
		Points: []domain.TrackPoint{point(0, 52.50, 13.30), point(60, 52.51, 13.30)},
	}
	evening := domain.Track{
		Source: "evening.fit.xz",
		Points: []domain.TrackPoint{point(36000, 48.13, 11.58), point(36060, 48.14, 11.58)},
	}

	got := Locate([]domain.Track{morning, evening}, at(18000), defaults())

	assertNotFound(t, got)
}

// Two recordings running at once — a watch and a bike computer — should yield
// the position resting on the least inference.
func TestLocatePrefersTheMoreDirectMethod(t *testing.T) {
	exact := domain.Track{
		Source: "watch.fit.xz",
		Points: []domain.TrackPoint{point(0, 52.50, 13.30), point(5, 52.55, 13.30), point(10, 52.51, 13.30)},
	}

	got := Locate([]domain.Track{ride(), exact}, at(5), defaults())

	assertFound(t, got)
	if got.Fix.Method != domain.MethodExact {
		t.Errorf("method = %q, expected the exact point to win over interpolation", got.Fix.Method)
	}
	if got.Fix.Source != "watch.fit.xz" {
		t.Errorf("source = %q, expected watch.fit.xz", got.Fix.Source)
	}
}

func TestLocatePrefersTheSmallerGapAmongEqualMethods(t *testing.T) {
	coarse := domain.Track{
		Source: "coarse.fit.xz",
		Points: []domain.TrackPoint{point(0, 52.50, 13.30), point(100, 52.60, 13.30)},
	}

	got := Locate([]domain.Track{ride(), coarse}, at(5), defaults())

	assertFound(t, got)
	if got.Fix.Source != "ride.fit.xz" {
		t.Errorf("source = %q, expected the densely recorded track", got.Fix.Source)
	}
	if got.Fix.Gap != 10*time.Second {
		t.Errorf("gap = %s, expected 10s", got.Fix.Gap)
	}
}

func TestLocateWithNoTracks(t *testing.T) {
	got := Locate(nil, at(5), defaults())

	assertNotFound(t, got)
	if got.Reason == "" {
		t.Error("expected a reason explaining the miss")
	}
}

func TestLocateSkipsEmptyTracks(t *testing.T) {
	got := Locate([]domain.Track{{Source: "empty.fit.xz"}}, at(5), defaults())

	assertNotFound(t, got)
}

func TestLocateCarriesElevation(t *testing.T) {
	climb := domain.Track{
		Source: "climb.fit.xz",
		Points: []domain.TrackPoint{
			{Time: at(0), Latitude: 52.50, Longitude: 13.30, Elevation: 30, HasElevation: true},
			{Time: at(10), Latitude: 52.51, Longitude: 13.30, Elevation: 40, HasElevation: true},
		},
	}

	got := Locate([]domain.Track{climb}, at(5), defaults())

	assertFound(t, got)
	if !got.Fix.Position.HasElevation {
		t.Fatal("expected elevation to survive matching")
	}
	assertClose(t, "elevation", got.Fix.Position.Elevation, 35)
}

func assertFound(t *testing.T, result Result) {
	t.Helper()
	if !result.Found {
		t.Fatalf("expected a fix, got none: %s", result.Reason)
	}
}

func assertNotFound(t *testing.T, result Result) {
	t.Helper()
	if result.Found {
		t.Fatalf("expected no fix, got %+v", result.Fix)
	}
	if result.Reason == "" {
		t.Error("expected a reason explaining the miss")
	}
}

func assertClose(t *testing.T, name string, got, expected float64) {
	t.Helper()
	if math.Abs(got-expected) > 1e-6 {
		t.Errorf("%s = %.9f, expected %.9f", name, got, expected)
	}
}
