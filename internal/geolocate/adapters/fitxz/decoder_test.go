package fitxz

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/ulikunitz/xz"
)

// Fixtures are built here rather than checked in. A real recording would be a
// binary blob nobody can review in a diff, and it would commit this
// repository to holding the exact coordinates of somebody's front door.

var recordedAt = time.Date(2017, 8, 8, 18, 28, 54, 0, time.UTC)

type sample struct {
	offsetSeconds       int
	latitude, longitude float64
	elevation           float64
	invalidPosition     bool
}

// writeActivity encodes an activity, xz-compresses it and returns its path.
func writeActivity(t *testing.T, name string, localOffset time.Duration, samples []sample) string {
	t.Helper()

	activity := filedef.NewActivity()
	activity.FileId.TimeCreated = recordedAt
	activity.FileId.Manufacturer = typedef.ManufacturerGarmin
	activity.Activity = mesgdef.NewActivity(nil).
		SetTimestamp(recordedAt).
		SetLocalTimestamp(recordedAt.Add(localOffset))

	for _, s := range samples {
		activity.Records = append(activity.Records, recordFrom(s))
	}

	path := filepath.Join(t.TempDir(), name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer file.Close()

	compressor, err := xz.NewWriter(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fit := activity.ToFIT(nil)
	if err := encoder.New(compressor).Encode(&fit); err != nil {
		t.Fatalf("failed to encode fixture: %v", err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return path
}

func recordFrom(s sample) *mesgdef.Record {
	record := mesgdef.NewRecord(nil).
		SetTimestamp(recordedAt.Add(time.Duration(s.offsetSeconds) * time.Second))

	if s.invalidPosition {
		// What a device writes before the GPS has a lock.
		return record.SetPositionLat(basetype.Sint32Invalid).
			SetPositionLong(basetype.Sint32Invalid)
	}
	return record.
		SetPositionLatDegrees(s.latitude).
		SetPositionLongDegrees(s.longitude).
		SetEnhancedAltitudeScaled(s.elevation)
}

func TestSupports(t *testing.T) {
	for _, tc := range []struct {
		path     string
		expected bool
	}{
		{"ride.fit.xz", true},
		{"RIDE.FIT.XZ", true},
		{"/a/b/2017-08-08_cycling.fit.xz", true},
		{"ride.fit", false},
		{"ride.gpx.xz", false},
		{"ride.xz", false},
		{"ride.gpx", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := New().Supports(tc.path); got != tc.expected {
				t.Errorf("Supports(%q) = %v, expected %v", tc.path, got, tc.expected)
			}
		})
	}
}

func TestDecodeReadsPositionsAndElevation(t *testing.T) {
	path := writeActivity(t, "ride.fit.xz", 2*time.Hour, []sample{
		{offsetSeconds: 0, latitude: 52.504880, longitude: 13.299566, elevation: 29.2},
		{offsetSeconds: 1, latitude: 52.504874, longitude: 13.299567, elevation: 30.8},
		{offsetSeconds: 6, latitude: 52.504825, longitude: 13.299506, elevation: 32.2},
	})

	track, err := New().Decode(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(track.Points) != 3 {
		t.Fatalf("got %d points, expected 3", len(track.Points))
	}
	if track.Source != path {
		t.Errorf("source = %q, expected %q", track.Source, path)
	}

	first := track.Points[0]
	if !first.Time.Equal(recordedAt) {
		t.Errorf("first time = %s, expected %s", first.Time, recordedAt)
	}
	// FIT stores position in semicircles, so degrees survive a round trip only
	// to about a centimetre.
	assertClose(t, "latitude", first.Latitude, 52.504880)
	assertClose(t, "longitude", first.Longitude, 13.299566)
	if !first.HasElevation {
		t.Fatal("expected elevation")
	}
	assertClose(t, "elevation", first.Elevation, 29.2)
}

// The five-second step between the second and third sample is smart
// recording, not a decoding error, and must survive into the track so the
// matcher can judge it against --max-gap.
func TestDecodePreservesRecordingGaps(t *testing.T) {
	path := writeActivity(t, "ride.fit.xz", 2*time.Hour, []sample{
		{offsetSeconds: 0, latitude: 52.5048, longitude: 13.2995, elevation: 29.2},
		{offsetSeconds: 1, latitude: 52.5048, longitude: 13.2995, elevation: 30.8},
		{offsetSeconds: 6, latitude: 52.5048, longitude: 13.2995, elevation: 32.2},
	})

	track, err := New().Decode(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gap := track.Points[2].Time.Sub(track.Points[1].Time)
	if gap != 5*time.Second {
		t.Errorf("gap = %s, expected 5s", gap)
	}
}

func TestDecodeSkipsRecordsWithoutAPosition(t *testing.T) {
	path := writeActivity(t, "ride.fit.xz", 2*time.Hour, []sample{
		{offsetSeconds: 0, invalidPosition: true},
		{offsetSeconds: 1, invalidPosition: true},
		{offsetSeconds: 2, latitude: 52.5048, longitude: 13.2995, elevation: 29.2},
	})

	track, err := New().Decode(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(track.Points) != 1 {
		t.Fatalf("got %d points, expected the 2 unlocked records to be dropped", len(track.Points))
	}
	if math.IsNaN(track.Points[0].Latitude) {
		t.Error("a NaN position survived into the track")
	}
}

// This is what spares the operator from passing a timezone by hand.
func TestDecodeDerivesTheUTCOffset(t *testing.T) {
	path := writeActivity(t, "ride.fit.xz", 2*time.Hour, []sample{
		{offsetSeconds: 0, latitude: 52.5048, longitude: 13.2995, elevation: 29.2},
	})

	track, err := New().Decode(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !track.HasUTCOffset {
		t.Fatal("expected an offset derived from the local timestamp")
	}
	if track.UTCOffset != 2*time.Hour {
		t.Errorf("offset = %s, expected 2h", track.UTCOffset)
	}
}

func TestDecodeRejectsAFileThatIsNotXZ(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.fit.xz")
	if err := os.WriteFile(path, []byte("not compressed"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := New().Decode(context.Background(), path); err == nil {
		t.Fatal("expected an error for a file that is not xz-compressed")
	}
}

func TestDecodeRejectsAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.fit.xz")

	if _, err := New().Decode(context.Background(), path); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func assertClose(t *testing.T, name string, got, expected float64) {
	t.Helper()
	// A semicircle is about 1.7cm at the equator.
	if math.Abs(got-expected) > 1e-6 {
		t.Errorf("%s = %.9f, expected %.9f", name, got, expected)
	}
}
