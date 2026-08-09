package domain

import "time"

// CaptureClock is a camera's wall-clock reading of when a frame was taken.
//
// The reading and the zone are kept apart on purpose. Most cameras write
// "2017:08:08 20:28:54" with no zone at all, and the instant that denotes
// depends entirely on where the photographer was standing. Resolving it
// against the machine's current local zone — as the cull pipeline does — is
// harmless for grouping bursts but silently wrong for geolocation, so this
// type refuses to make that choice and leaves it to the application.
type CaptureClock struct {
	// Wall is the reading as written, carried in time.UTC as a placeholder.
	// Its zone is meaningless; only its date and clock fields are.
	Wall time.Time

	// Offset is the zone the camera recorded, from EXIF OffsetTimeOriginal.
	Offset    time.Duration
	HasOffset bool
}

// IsZero reports whether the camera gave us no usable reading at all.
func (c CaptureClock) IsZero() bool { return c.Wall.IsZero() }

// Instant resolves the reading to a point in time, using the camera's own
// recorded offset when it has one and the supplied fallback otherwise.
func (c CaptureClock) Instant(fallback time.Duration) time.Time {
	offset := fallback
	if c.HasOffset {
		offset = c.Offset
	}
	return c.Wall.Add(-offset)
}

// WallAt renders an instant back as a camera-style wall-clock reading in the
// given offset. It is the inverse of Instant and is how a drift-corrected
// timestamp is written back in the same zone the camera was using.
func WallAt(instant time.Time, offset time.Duration) time.Time {
	return instant.UTC().Add(offset)
}
