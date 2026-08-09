package domain

import (
	"math"
	"testing"
	"time"
)

func at(seconds int) time.Time {
	return time.Date(2017, 8, 8, 18, 28, 0, 0, time.UTC).Add(time.Duration(seconds) * time.Second)
}

func point(seconds int, latitude, longitude float64) TrackPoint {
	return TrackPoint{Time: at(seconds), Latitude: latitude, Longitude: longitude}
}

// The midpoint of a quarter-arc along the equator is its bisector, which is
// the one great-circle case that can be checked against an exact value.
func TestInterpolateFollowsTheGreatCircle(t *testing.T) {
	from := point(0, 0, 0)
	to := point(10, 0, 90)

	got := Interpolate(from, to, at(5))

	assertClose(t, "latitude", got.Latitude, 0)
	assertClose(t, "longitude", got.Longitude, 45)
}

// Along a meridian a great circle and a straight line in coordinates agree,
// so this pins the fraction arithmetic rather than the geometry.
func TestInterpolateSplitsTheIntervalByTime(t *testing.T) {
	from := point(0, 50, 0)
	to := point(10, 60, 0)

	for _, tc := range []struct {
		name     string
		second   int
		expected float64
	}{
		{"start", 0, 50},
		{"one fifth", 2, 52},
		{"midpoint", 5, 55},
		{"end", 10, 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Interpolate(from, to, at(tc.second))
			assertClose(t, "latitude", got.Latitude, tc.expected)
		})
	}
}

// Over a five-second smart-recording gap at Berlin's latitude the two methods
// must not have diverged meaningfully; the point of slerp is that it stays
// correct as gaps grow, not that it disagrees on ordinary ones.
func TestInterpolateAgreesWithLinearOverShortGaps(t *testing.T) {
	from := point(0, 52.504880, 13.299566)
	to := point(5, 52.504825, 13.299506)

	const fraction = 2.0 / 5.0
	got := Interpolate(from, to, at(2))
	linearLatitude := from.Latitude + (to.Latitude-from.Latitude)*fraction
	linearLongitude := from.Longitude + (to.Longitude-from.Longitude)*fraction

	// 1e-7 degrees is roughly a centimetre.
	if math.Abs(got.Latitude-linearLatitude) > 1e-6 {
		t.Errorf("latitude %.9f diverges from linear %.9f", got.Latitude, linearLatitude)
	}
	if math.Abs(got.Longitude-linearLongitude) > 1e-6 {
		t.Errorf("longitude %.9f diverges from linear %.9f", got.Longitude, linearLongitude)
	}
}

// Slerp divides by sin(angle); coincident points must take the linear path
// rather than producing NaN.
func TestInterpolateHandlesCoincidentPoints(t *testing.T) {
	from := point(0, 52.5, 13.3)
	to := point(10, 52.5, 13.3)

	got := Interpolate(from, to, at(5))

	if math.IsNaN(got.Latitude) || math.IsNaN(got.Longitude) {
		t.Fatalf("got NaN position: %.9f, %.9f", got.Latitude, got.Longitude)
	}
	assertClose(t, "latitude", got.Latitude, 52.5)
	assertClose(t, "longitude", got.Longitude, 13.3)
}

func TestInterpolateHandlesPointsSharingATimestamp(t *testing.T) {
	from := point(0, 52.5, 13.3)
	to := point(0, 52.6, 13.4)

	got := Interpolate(from, to, at(0))

	if math.IsNaN(got.Latitude) || math.IsNaN(got.Longitude) {
		t.Fatalf("got NaN position: %.9f, %.9f", got.Latitude, got.Longitude)
	}
	assertClose(t, "latitude", got.Latitude, 52.5)
}

// A pair straddling the antimeridian must interpolate the short way across it,
// not the long way back around the globe.
func TestInterpolateCrossesTheAntimeridian(t *testing.T) {
	from := point(0, 0, 179)
	to := point(10, 0, -179)

	got := Interpolate(from, to, at(5))

	if math.Abs(got.Longitude) < 179 {
		t.Errorf("longitude %.6f took the long way around; expected near ±180", got.Longitude)
	}
}

func TestInterpolateElevation(t *testing.T) {
	from := TrackPoint{Time: at(0), Latitude: 52.5, Longitude: 13.3, Elevation: 29.2, HasElevation: true}
	to := TrackPoint{Time: at(10), Latitude: 52.5, Longitude: 13.3, Elevation: 39.2, HasElevation: true}

	got := Interpolate(from, to, at(5))

	if !got.HasElevation {
		t.Fatal("expected elevation to be carried")
	}
	assertClose(t, "elevation", got.Elevation, 34.2)
}

// Elevation known at only one end is not elevation for the interval; reporting
// the one known value would invent an altitude the recording never had.
func TestInterpolateDropsElevationWhenOneSideLacksIt(t *testing.T) {
	from := TrackPoint{Time: at(0), Elevation: 29.2, HasElevation: true}
	to := TrackPoint{Time: at(10)}

	got := Interpolate(from, to, at(5))

	if got.HasElevation {
		t.Errorf("expected no elevation, got %.2f", got.Elevation)
	}
}

func assertClose(t *testing.T, name string, got, expected float64) {
	t.Helper()
	const tolerance = 1e-9
	if math.Abs(got-expected) > tolerance {
		t.Errorf("%s = %.12f, expected %.12f", name, got, expected)
	}
}
