package domain

import (
	"math"
	"time"
)

// coincidentEpsilon is the angular separation, in radians, below which two
// points are treated as the same place. Slerp divides by sin(angle), which
// collapses as the angle approaches zero; at this separation — under a
// millimetre on the Earth's surface — the linear fallback is exact to far
// beyond GPS precision anyway.
const coincidentEpsilon = 1e-10

// straightEpsilon guards the opposite degenerate case: points very nearly
// antipodal, where the great circle through them is not unique. Real tracks
// never contain such a pair, but a corrupt file might.
const straightEpsilon = 1e-9

// Interpolate returns the position along the great circle from a to b at the
// given instant.
//
// Position uses spherical interpolation rather than straight averaging of
// latitude and longitude. The two agree to sub-millimetre over the one-second
// spacing of a dense recording, but devices drop to smart recording on
// straight sections — the sample activities here skip five seconds and more —
// and linear coordinates visibly leave the road over such gaps, increasingly
// so away from the equator. Elevation is interpolated linearly, since it is a
// scalar and not a position on the sphere.
//
// The instant is expected to lie between the two points; outside that range
// the result extrapolates, which callers avoid by bracketing first.
func Interpolate(from, to TrackPoint, at time.Time) TrackPoint {
	fraction := fractionBetween(from.Time, to.Time, at)

	latitude, longitude := slerp(
		from.Latitude, from.Longitude,
		to.Latitude, to.Longitude,
		fraction,
	)

	return TrackPoint{
		Time:         at,
		Latitude:     latitude,
		Longitude:    longitude,
		Elevation:    interpolateElevation(from, to, fraction),
		HasElevation: from.HasElevation && to.HasElevation,
	}
}

// fractionBetween reports how far at lies from start towards end, as 0..1.
// Two points sharing a timestamp yield 0 rather than a division by zero.
func fractionBetween(start, end, at time.Time) float64 {
	span := end.Sub(start)
	if span <= 0 {
		return 0
	}
	return float64(at.Sub(start)) / float64(span)
}

// interpolateElevation returns the blended altitude, and is meaningful only
// when both points carry one. A track that gains elevation data partway
// through must not report the one known value as if it applied to both ends.
func interpolateElevation(from, to TrackPoint, fraction float64) float64 {
	if !from.HasElevation || !to.HasElevation {
		return 0
	}
	return from.Elevation + (to.Elevation-from.Elevation)*fraction
}

// slerp interpolates along the great circle between two positions in degrees.
func slerp(fromLat, fromLon, toLat, toLon, fraction float64) (latitude, longitude float64) {
	from := toCartesian(fromLat, fromLon)
	to := toCartesian(toLat, toLon)

	angle := math.Acos(clamp(dot(from, to), -1, 1))
	if math.IsNaN(angle) || angle < coincidentEpsilon || math.Pi-angle < straightEpsilon {
		return lerpDegrees(fromLat, fromLon, toLat, toLon, fraction)
	}

	sine := math.Sin(angle)
	fromWeight := math.Sin((1-fraction)*angle) / sine
	toWeight := math.Sin(fraction*angle) / sine

	blended := vector{
		x: fromWeight*from.x + toWeight*to.x,
		y: fromWeight*from.y + toWeight*to.y,
		z: fromWeight*from.z + toWeight*to.z,
	}
	return toDegrees(blended)
}

// lerpDegrees is the fallback for degenerate geometry. Longitude is blended
// along the shorter way around so a pair straddling the antimeridian does not
// travel the long way home.
func lerpDegrees(fromLat, fromLon, toLat, toLon, fraction float64) (latitude, longitude float64) {
	difference := math.Mod(toLon-fromLon+540, 360) - 180
	return fromLat + (toLat-fromLat)*fraction,
		math.Mod(fromLon+difference*fraction+540, 360) - 180
}

type vector struct{ x, y, z float64 }

func toCartesian(latitude, longitude float64) vector {
	lat := latitude * math.Pi / 180
	lon := longitude * math.Pi / 180
	return vector{
		x: math.Cos(lat) * math.Cos(lon),
		y: math.Cos(lat) * math.Sin(lon),
		z: math.Sin(lat),
	}
}

func toDegrees(v vector) (latitude, longitude float64) {
	length := math.Sqrt(v.x*v.x + v.y*v.y + v.z*v.z)
	if length == 0 {
		return 0, 0
	}
	return math.Asin(clamp(v.z/length, -1, 1)) * 180 / math.Pi,
		math.Atan2(v.y, v.x) * 180 / math.Pi
}

func dot(a, b vector) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
