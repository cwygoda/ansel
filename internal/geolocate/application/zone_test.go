package application

import (
	"strings"
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

func TestParseUTCOffsetAcceptsTheFormsAPersonTypes(t *testing.T) {
	for _, tc := range []struct {
		value    string
		expected time.Duration
	}{
		{"+02:00", 2 * time.Hour},
		{"-05:00", -5 * time.Hour},
		{"+0200", 2 * time.Hour},
		{"-0530", -(5*time.Hour + 30*time.Minute)},
		{"+2", 2 * time.Hour},
		{"-5", -5 * time.Hour},
		{"+05:45", 5*time.Hour + 45*time.Minute},
		{"+00:00", 0},
	} {
		t.Run(tc.value, func(t *testing.T) {
			got, err := parseUTCOffset(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("parseUTCOffset(%q) = %s, expected %s", tc.value, got, tc.expected)
			}
		})
	}
}

func TestParseUTCOffsetRejectsNonsense(t *testing.T) {
	for _, value := range []string{"", "02:00", "Europe/Berlin", "+", "+abc", "+15:00", "+02:99"} {
		t.Run(value, func(t *testing.T) {
			if _, err := parseUTCOffset(value); err == nil {
				t.Errorf("parseUTCOffset(%q) accepted an invalid offset", value)
			}
		})
	}
}

// The whole point of preferring a named zone: a fixed offset cannot know that
// the clocks went back.
func TestZoneFollowsSummerTime(t *testing.T) {
	zone, err := NewZone("Europe/Berlin", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tc := range []struct {
		name     string
		wall     time.Time
		expected time.Duration
	}{
		{"august is summer time", time.Date(2017, 8, 8, 20, 28, 54, 0, time.UTC), 2 * time.Hour},
		{"january is not", time.Date(2017, 1, 8, 20, 28, 54, 0, time.UTC), time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := zone.OffsetAt(tc.wall)
			if !ok {
				t.Fatal("expected the zone to yield an offset")
			}
			if got != tc.expected {
				t.Errorf("offset = %s, expected %s", got, tc.expected)
			}
		})
	}
}

func TestZoneRejectsAnUnknownLocation(t *testing.T) {
	if _, err := NewZone("Mars/Olympus_Mons", ""); err == nil {
		t.Fatal("expected an error for an unknown timezone")
	}
}

func TestZoneIsUnknownWhenNothingWasGiven(t *testing.T) {
	zone, err := NewZone("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if zone.Known() {
		t.Error("expected an empty zone to report itself unknown")
	}
	if _, ok := zone.OffsetAt(time.Now()); ok {
		t.Error("expected no offset from an empty zone")
	}
}

func TestResolveOffsetPrefersTheCameraThenTheTrackThenTheOperator(t *testing.T) {
	wall := time.Date(2017, 8, 8, 20, 28, 54, 0, time.UTC)
	berlin, err := NewZone("Europe/Berlin", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tc := range []struct {
		name        string
		clock       domain.CaptureClock
		trackOffset time.Duration
		trackKnows  bool
		zone        Zone
		expected    time.Duration
	}{
		{
			name:        "camera wins",
			clock:       domain.CaptureClock{Wall: wall, Offset: 9 * time.Hour, HasOffset: true},
			trackOffset: 2 * time.Hour,
			trackKnows:  true,
			zone:        berlin,
			expected:    9 * time.Hour,
		},
		{
			name:        "track is next",
			clock:       domain.CaptureClock{Wall: wall},
			trackOffset: 9 * time.Hour,
			trackKnows:  true,
			zone:        berlin,
			expected:    9 * time.Hour,
		},
		{
			name:     "operator is the last resort",
			clock:    domain.CaptureClock{Wall: wall},
			zone:     berlin,
			expected: 2 * time.Hour,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveOffset(tc.clock, tc.trackOffset, tc.trackKnows, tc.zone)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("offset = %s, expected %s", got, tc.expected)
			}
		})
	}
}

func TestResolveOffsetErrorsRatherThanGuessing(t *testing.T) {
	clock := domain.CaptureClock{Wall: time.Date(2017, 8, 8, 20, 28, 54, 0, time.UTC)}

	_, err := resolveOffset(clock, 0, false, Zone{})
	if err == nil {
		t.Fatal("expected an error rather than a guessed zone")
	}
	for _, hint := range []string{"--tz", "--utc-offset"} {
		if !strings.Contains(err.Error(), hint) {
			t.Errorf("error %q should mention %s", err, hint)
		}
	}
}
