package domain

import "time"

// Method records how a position was arrived at. It travels with every fix so
// the report can justify each coordinate rather than presenting interpolated
// and clamped results as equally certain.
type Method string

const (
	// MethodExact means a recorded point carries the photograph's timestamp.
	MethodExact Method = "exact"

	// MethodInterpolated means the position was computed between two
	// recorded points bracketing the timestamp.
	MethodInterpolated Method = "interpolated"

	// MethodClampedStart means the photograph predates the recording and was
	// placed at its first point, within the allowed buffer.
	MethodClampedStart Method = "clamped-start"

	// MethodClampedEnd means the photograph postdates the recording and was
	// placed at its last point, within the allowed buffer.
	MethodClampedEnd Method = "clamped-end"
)

// Clamped reports whether the position was taken from outside the recording.
func (m Method) Clamped() bool {
	return m == MethodClampedStart || m == MethodClampedEnd
}

// Fix is a resolved position for one photograph.
type Fix struct {
	Position TrackPoint
	Method   Method

	// Source is the track the position came from.
	Source string

	// Gap is the uncertainty behind the fix: for an interpolated position the
	// spacing of the two bracketing points, for a clamped one the distance in
	// time from the end of the recording. A caller comparing two candidate
	// fixes prefers the smaller.
	Gap time.Duration
}
