package application

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

// Zone is the operator's stated answer to "what zone was the camera set to",
// used only when neither the photograph nor the track says.
//
// A named location beats a fixed offset because it knows when summer time
// started. A shoot spanning the last Sunday in October is placed correctly by
// "Europe/Berlin" and an hour out by "+02:00".
type Zone struct {
	location *time.Location

	offset    time.Duration
	hasOffset bool
}

// NewZone builds a zone from a location name and a fixed offset, either of
// which may be empty. The name wins when both are given.
func NewZone(timezone, utcOffset string) (Zone, error) {
	var zone Zone

	if timezone != "" {
		location, err := time.LoadLocation(timezone)
		if err != nil {
			return Zone{}, fmt.Errorf("failed to load timezone %q: %w "+
				"(expected a name like Europe/Berlin)", timezone, err)
		}
		zone.location = location
		return zone, nil
	}

	if utcOffset != "" {
		offset, err := parseUTCOffset(utcOffset)
		if err != nil {
			return Zone{}, err
		}
		zone.offset, zone.hasOffset = offset, true
	}
	return zone, nil
}

// Known reports whether the operator supplied anything at all.
func (z Zone) Known() bool { return z.location != nil || z.hasOffset }

// OffsetAt returns the offset in force for a given wall-clock reading. The
// reading matters: a named zone's offset depends on the date it names.
func (z Zone) OffsetAt(wall time.Time) (time.Duration, bool) {
	if z.location != nil {
		local := time.Date(wall.Year(), wall.Month(), wall.Day(),
			wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), z.location)
		_, seconds := local.Zone()
		return time.Duration(seconds) * time.Second, true
	}
	if z.hasOffset {
		return z.offset, true
	}
	return 0, false
}

// parseUTCOffset reads an offset such as "+02:00", tolerating the shorter
// forms a person is likely to type.
func parseUTCOffset(value string) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	malformed := fmt.Errorf("failed to parse utc offset %q "+
		"(expected a form like +02:00, -0500 or +2)", value)

	if trimmed == "" || (trimmed[0] != '+' && trimmed[0] != '-') {
		return 0, malformed
	}
	sign := time.Duration(1)
	if trimmed[0] == '-' {
		sign = -1
	}

	hours, minutes, err := splitOffset(strings.TrimLeft(trimmed[1:], " "))
	if err != nil {
		return 0, malformed
	}
	if hours > 14 || minutes > 59 {
		return 0, fmt.Errorf("utc offset %q is out of range", value)
	}
	return sign * (time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute), nil
}

func splitOffset(digits string) (hours, minutes int, err error) {
	switch {
	case strings.Contains(digits, ":"):
		parts := strings.SplitN(digits, ":", 2)
		hours, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, err
		}
		minutes, err = strconv.Atoi(parts[1])
	case len(digits) == 4:
		hours, err = strconv.Atoi(digits[:2])
		if err != nil {
			return 0, 0, err
		}
		minutes, err = strconv.Atoi(digits[2:])
	default:
		hours, err = strconv.Atoi(digits)
	}
	return hours, minutes, err
}

// resolveOffset walks the ladder that turns a camera's wall-clock reading
// into a real instant, in order of how much each source actually knows.
//
// It never falls back to the machine's own zone. That would silently place a
// shoot wherever the operator happens to be sitting rather than where it
// happened, and a wrong position that looks plausible is worse than none.
func resolveOffset(clock domain.CaptureClock, trackOffset time.Duration, trackKnows bool, zone Zone) (time.Duration, error) {
	if clock.HasOffset {
		return clock.Offset, nil
	}
	if trackKnows {
		return trackOffset, nil
	}
	if offset, ok := zone.OffsetAt(clock.Wall); ok {
		return offset, nil
	}
	return 0, fmt.Errorf("cannot tell which timezone the camera was set to: " +
		"the photograph has no OffsetTimeOriginal and no track states its own offset; " +
		"pass --tz Europe/Berlin or --utc-offset +02:00")
}
