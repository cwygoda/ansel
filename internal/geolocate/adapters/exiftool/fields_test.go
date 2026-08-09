package exiftool

import (
	"testing"
	"time"
)

func TestParseWallKeepsTheReadingUnzoned(t *testing.T) {
	clock, ok := parseWall("2017:08:08 20:28:54")
	if !ok {
		t.Fatal("expected the timestamp to parse")
	}

	// The reading is 20:28:54 whatever zone the photographer was in. Nothing
	// here may quietly turn that into an instant.
	if clock.HasOffset {
		t.Error("expected no offset for an unzoned timestamp")
	}
	expected := time.Date(2017, 8, 8, 20, 28, 54, 0, time.UTC)
	if !clock.Wall.Equal(expected) {
		t.Errorf("wall = %s, expected %s", clock.Wall, expected)
	}
}

// The cull pipeline resolves these against time.Local. Were this reader to do
// the same, a shoot culled in a different zone from where it happened would
// land somewhere else entirely on the track.
func TestParseWallDoesNotUseTheMachineZone(t *testing.T) {
	clock, ok := parseWall("2017:08:08 20:28:54")
	if !ok {
		t.Fatal("expected the timestamp to parse")
	}

	if zone, offset := clock.Wall.Zone(); offset != 0 {
		t.Errorf("wall carries zone %s (%ds); expected the UTC placeholder", zone, offset)
	}
}

func TestParseWallHonoursAnEmbeddedOffset(t *testing.T) {
	clock, ok := parseWall("2017:08:08 20:28:54+02:00")
	if !ok {
		t.Fatal("expected the timestamp to parse")
	}

	if !clock.HasOffset {
		t.Fatal("expected the embedded offset to be recognised")
	}
	if clock.Offset != 2*time.Hour {
		t.Errorf("offset = %s, expected 2h", clock.Offset)
	}
	// The reading stays local; only the offset says what it means.
	if clock.Wall.Hour() != 20 {
		t.Errorf("wall hour = %d, expected the local reading 20", clock.Wall.Hour())
	}
	expected := time.Date(2017, 8, 8, 18, 28, 54, 0, time.UTC)
	if got := clock.Instant(0); !got.Equal(expected) {
		t.Errorf("instant = %s, expected %s", got, expected)
	}
}

func TestParseWallRejectsUnsetClocks(t *testing.T) {
	for _, value := range []string{"", "   ", "0000:00:00 00:00:00"} {
		t.Run(value, func(t *testing.T) {
			if _, ok := parseWall(value); ok {
				t.Errorf("parseWall(%q) accepted an unset clock", value)
			}
		})
	}
}

func TestParseOffset(t *testing.T) {
	for _, tc := range []struct {
		value    string
		expected time.Duration
		ok       bool
	}{
		{"+02:00", 2 * time.Hour, true},
		{"-05:00", -5 * time.Hour, true},
		{"+05:30", 5*time.Hour + 30*time.Minute, true},
		{"-03:30", -(3*time.Hour + 30*time.Minute), true},
		{"+00:00", 0, true},
		{"", 0, false},
		{"+2:00", 0, false},
		{"02:00", 0, false},
		{"+02-00", 0, false},
		{"gibberish", 0, false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			got, ok := parseOffset(tc.value)
			if ok != tc.ok {
				t.Fatalf("parseOffset(%q) ok = %v, expected %v", tc.value, ok, tc.ok)
			}
			if got != tc.expected {
				t.Errorf("parseOffset(%q) = %s, expected %s", tc.value, got, tc.expected)
			}
		})
	}
}

func TestFormatOffset(t *testing.T) {
	for _, tc := range []struct {
		offset   time.Duration
		expected string
	}{
		{2 * time.Hour, "+02:00"},
		{-5 * time.Hour, "-05:00"},
		{5*time.Hour + 30*time.Minute, "+05:30"},
		{-(3*time.Hour + 30*time.Minute), "-03:30"},
		{0, "+00:00"},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			if got := formatOffset(tc.offset); got != tc.expected {
				t.Errorf("formatOffset(%s) = %q, expected %q", tc.offset, got, tc.expected)
			}
		})
	}
}

func TestOffsetSurvivesAFormatRoundTrip(t *testing.T) {
	for _, original := range []time.Duration{
		2 * time.Hour, -5 * time.Hour, 5*time.Hour + 30*time.Minute, 0,
	} {
		t.Run(original.String(), func(t *testing.T) {
			got, ok := parseOffset(formatOffset(original))
			if !ok {
				t.Fatalf("formatOffset(%s) produced an unparseable value", original)
			}
			if got != original {
				t.Errorf("round trip = %s, expected %s", got, original)
			}
		})
	}
}

func TestClockFrom(t *testing.T) {
	for _, tc := range []struct {
		name           string
		entry          map[string]any
		expectedHour   int
		expectedOffset time.Duration
		hasOffset      bool
		empty          bool
	}{
		{
			name:         "date time original wins",
			entry:        map[string]any{"DateTimeOriginal": "2017:08:08 20:28:54", "CreateDate": "2001:01:01 00:00:00"},
			expectedHour: 20,
		},
		{
			name:         "falls back to create date",
			entry:        map[string]any{"CreateDate": "2017:08:08 21:00:00"},
			expectedHour: 21,
		},
		{
			name:           "offset time original is picked up",
			entry:          map[string]any{"DateTimeOriginal": "2017:08:08 20:28:54", "OffsetTimeOriginal": "+02:00"},
			expectedHour:   20,
			expectedOffset: 2 * time.Hour,
			hasOffset:      true,
		},
		{
			name:           "offset time is the fallback",
			entry:          map[string]any{"DateTimeOriginal": "2017:08:08 20:28:54", "OffsetTime": "-05:00"},
			expectedHour:   20,
			expectedOffset: -5 * time.Hour,
			hasOffset:      true,
		},
		{
			name:           "offset time original beats offset time",
			entry:          map[string]any{"DateTimeOriginal": "2017:08:08 20:28:54", "OffsetTimeOriginal": "+02:00", "OffsetTime": "-05:00"},
			expectedHour:   20,
			expectedOffset: 2 * time.Hour,
			hasOffset:      true,
		},
		{
			name:  "nothing usable",
			entry: map[string]any{},
			empty: true,
		},
		{
			name:  "unset camera clock",
			entry: map[string]any{"DateTimeOriginal": "0000:00:00 00:00:00"},
			empty: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := clockFrom(tc.entry)

			if tc.empty {
				if !clock.IsZero() {
					t.Fatalf("expected no clock, got %+v", clock)
				}
				return
			}
			if clock.Wall.Hour() != tc.expectedHour {
				t.Errorf("wall hour = %d, expected %d", clock.Wall.Hour(), tc.expectedHour)
			}
			if clock.HasOffset != tc.hasOffset {
				t.Fatalf("hasOffset = %v, expected %v", clock.HasOffset, tc.hasOffset)
			}
			if clock.Offset != tc.expectedOffset {
				t.Errorf("offset = %s, expected %s", clock.Offset, tc.expectedOffset)
			}
		})
	}
}
