// Package domain holds the pure types and geometry of geolocation. It imports
// nothing but the standard library: no decoder, no subprocess and no file
// format is visible from here.
package domain

import "time"

// TrackPoint is one recorded position. Time is always UTC, whatever the
// recording device called it locally.
type TrackPoint struct {
	Time         time.Time
	Latitude     float64
	Longitude    float64
	Elevation    float64
	HasElevation bool
}

// Track is one continuous recording, its points sorted ascending by time.
//
// Tracks stay separate rather than being merged into one long list of points,
// because the gap between two recordings is not a gap in a recording. A photo
// taken between Tuesday's ride and Wednesday's run must not be placed halfway
// along the line joining them.
type Track struct {
	// Source is the path the track was read from, used only for reporting.
	Source string

	Points []TrackPoint

	// UTCOffset is the offset the recording device was set to, when the
	// format states it. FIT does, by carrying a local timestamp alongside the
	// UTC one, which lets a camera's unzoned wall clock be interpreted without
	// asking the operator what zone they were in.
	UTCOffset    time.Duration
	HasUTCOffset bool
}

// Span reports the time range the track covers. The boolean is false for a
// track with no points, which has no span rather than a zero-length one.
func (t Track) Span() (start, end time.Time, ok bool) {
	if len(t.Points) == 0 {
		return time.Time{}, time.Time{}, false
	}
	return t.Points[0].Time, t.Points[len(t.Points)-1].Time, true
}

// Covers reports whether an instant falls inside the track's span, widened at
// both ends by buffer.
func (t Track) Covers(at time.Time, buffer time.Duration) bool {
	start, end, ok := t.Span()
	if !ok {
		return false
	}
	return !at.Before(start.Add(-buffer)) && !at.After(end.Add(buffer))
}
