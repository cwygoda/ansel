package exiftool

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

// wallLayouts are the forms a camera timestamp arrives in. They are parsed
// against time.UTC as a placeholder rather than time.Local: the result stands
// for a clock reading, not an instant, and attaching the operator's own zone
// to it here would be a guess disguised as a fact.
var wallLayouts = []string{
	"2006:01:02 15:04:05",
	"2006:01:02 15:04:05.999999",
}

// zonedLayouts are the same forms for the rare camera that appends its offset
// directly to the timestamp. Here the zone is stated, so it is honoured.
var zonedLayouts = []string{
	"2006:01:02 15:04:05-07:00",
	"2006:01:02 15:04:05.999999-07:00",
}

// clockFrom builds a capture clock from one exiftool JSON entry.
func clockFrom(entry map[string]any) domain.CaptureClock {
	clock, ok := parseWall(asString(entry["DateTimeOriginal"]))
	if !ok {
		clock, ok = parseWall(asString(entry["CreateDate"]))
		if !ok {
			return domain.CaptureClock{}
		}
	}

	// An offset carried inside the timestamp already won; only consult the
	// dedicated tags when it did not.
	if !clock.HasOffset {
		if offset, found := parseOffset(asString(entry["OffsetTimeOriginal"])); found {
			clock.Offset, clock.HasOffset = offset, true
		} else if offset, found := parseOffset(asString(entry["OffsetTime"])); found {
			clock.Offset, clock.HasOffset = offset, true
		}
	}
	return clock
}

// parseWall reads a camera timestamp, keeping any stated offset separate from
// the reading itself.
func parseWall(value string) (domain.CaptureClock, bool) {
	value = strings.TrimSpace(value)
	// Cameras write this for an unset clock; it is not a real timestamp.
	if value == "" || strings.HasPrefix(value, "0000") {
		return domain.CaptureClock{}, false
	}

	for _, layout := range zonedLayouts {
		parsed, err := time.Parse(layout, value)
		if err != nil {
			continue
		}
		_, seconds := parsed.Zone()
		offset := time.Duration(seconds) * time.Second
		wall := time.Date(parsed.Year(), parsed.Month(), parsed.Day(),
			parsed.Hour(), parsed.Minute(), parsed.Second(), parsed.Nanosecond(), time.UTC)
		return domain.CaptureClock{Wall: wall, Offset: offset, HasOffset: true}, true
	}

	for _, layout := range wallLayouts {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return domain.CaptureClock{Wall: parsed}, true
		}
	}
	return domain.CaptureClock{}, false
}

// parseOffset reads an EXIF zone offset such as "+02:00". Cameras with an
// unset zone write "+00:00" for real, so it is accepted as given.
func parseOffset(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if len(value) != 6 || (value[0] != '+' && value[0] != '-') || value[3] != ':' {
		return 0, false
	}

	hours, err := strconv.Atoi(value[1:3])
	if err != nil {
		return 0, false
	}
	minutes, err := strconv.Atoi(value[4:6])
	if err != nil {
		return 0, false
	}

	offset := time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute
	if value[0] == '-' {
		return -offset, true
	}
	return offset, true
}

// formatOffset renders a duration as an EXIF zone offset.
func formatOffset(offset time.Duration) string {
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	return fmt.Sprintf("%s%02d:%02d", sign, int(offset.Hours()), int(offset.Minutes())%60)
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", typed)
	}
}
