package exiftool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

var trackedAt = time.Date(2017, 8, 8, 18, 28, 54, 0, time.UTC)

func planFor(position domain.TrackPoint) domain.WritePlan {
	return domain.WritePlan{
		PhotoPath: "/shoot/DSC_1234.NEF",
		Target:    "/shoot/DSC_1234.jpg",
		InPlace:   true,
		Fix:       domain.Fix{Position: position, Method: domain.MethodInterpolated},
	}
}

// sidecarPlanFor is the default mode: a sidecar beside the photograph.
func sidecarPlanFor(position domain.TrackPoint) domain.WritePlan {
	return domain.WritePlan{
		PhotoPath: "/shoot/DSC_1234.NEF",
		Target:    "/shoot/DSC_1234.xmp",
		Fix:       domain.Fix{Position: position, Method: domain.MethodInterpolated},
	}
}

func argumentValue(t *testing.T, args []string, tag string) string {
	t.Helper()
	prefix := "-" + tag + "="
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	t.Fatalf("no %s argument in %v", tag, args)
	return ""
}

func hasTag(args []string, tag string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-"+tag+"=") {
			return true
		}
	}
	return false
}

func TestWriteArgumentsStateTheHemisphere(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		latitude, longitude       float64
		latitudeRef, longitudeRef string
	}{
		{"north east", 52.5048, 13.2995, "N", "E"},
		{"south west", -33.8688, -70.6693, "S", "W"},
		{"north west", 42.3601, -71.0589, "N", "W"},
		{"south east", -33.8688, 151.2093, "S", "E"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := writeArguments(planFor(domain.TrackPoint{
				Time: trackedAt, Latitude: tc.latitude, Longitude: tc.longitude,
			}))

			if got := argumentValue(t, args, "EXIF:GPSLatitudeRef"); got != tc.latitudeRef {
				t.Errorf("GPSLatitudeRef = %q, expected %q", got, tc.latitudeRef)
			}
			if got := argumentValue(t, args, "EXIF:GPSLongitudeRef"); got != tc.longitudeRef {
				t.Errorf("GPSLongitudeRef = %q, expected %q", got, tc.longitudeRef)
			}
		})
	}
}

// exiftool rejects exponent notation, which is exactly how Go formats a small
// coordinate by default.
func TestWriteArgumentsAvoidExponentNotation(t *testing.T) {
	args := writeArguments(planFor(domain.TrackPoint{
		Time: trackedAt, Latitude: 0.0000123, Longitude: 13.2995,
	}))

	if got := argumentValue(t, args, "EXIF:GPSLatitude"); strings.ContainsAny(got, "eE") {
		t.Errorf("GPSLatitude = %q, expected plain decimal notation", got)
	}
}

func TestWriteArgumentsIncludeElevationOnlyWhenKnown(t *testing.T) {
	t.Run("known", func(t *testing.T) {
		args := writeArguments(planFor(domain.TrackPoint{
			Time: trackedAt, Latitude: 52.5, Longitude: 13.3,
			Elevation: 29.2, HasElevation: true,
		}))

		if got := argumentValue(t, args, "EXIF:GPSAltitudeRef"); got != "0" {
			t.Errorf("GPSAltitudeRef = %q, expected 0 for above sea level", got)
		}
	})

	t.Run("below sea level", func(t *testing.T) {
		args := writeArguments(planFor(domain.TrackPoint{
			Time: trackedAt, Latitude: 31.5, Longitude: 35.5,
			Elevation: -420, HasElevation: true,
		}))

		if got := argumentValue(t, args, "EXIF:GPSAltitudeRef"); got != "1" {
			t.Errorf("GPSAltitudeRef = %q, expected 1 for below sea level", got)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		args := writeArguments(planFor(domain.TrackPoint{
			Time: trackedAt, Latitude: 52.5, Longitude: 13.3,
		}))

		if hasTag(args, "EXIF:GPSAltitude") {
			t.Error("expected no altitude when the track carried none")
		}
	})
}

// The GPS timestamp is the track's, in UTC, and has nothing to do with the
// zone the camera thought it was in.
func TestWriteArgumentsStampTheTrackInstantInUTC(t *testing.T) {
	args := writeArguments(planFor(domain.TrackPoint{
		Time: trackedAt, Latitude: 52.5, Longitude: 13.3,
	}))

	if got := argumentValue(t, args, "EXIF:GPSDateStamp"); got != "2017:08:08" {
		t.Errorf("GPSDateStamp = %q, expected 2017:08:08", got)
	}
	if got := argumentValue(t, args, "EXIF:GPSTimeStamp"); got != "18:28:54" {
		t.Errorf("GPSTimeStamp = %q, expected 18:28:54", got)
	}
}

// Without a drift correction the camera's own timestamps are none of our
// business and must be left exactly as written.
func TestWriteArgumentsLeaveTimestampsAloneByDefault(t *testing.T) {
	args := writeArguments(planFor(domain.TrackPoint{
		Time: trackedAt, Latitude: 52.5, Longitude: 13.3,
	}))

	for _, tag := range []string{"EXIF:DateTimeOriginal", "EXIF:CreateDate", "EXIF:OffsetTimeOriginal"} {
		if hasTag(args, tag) {
			t.Errorf("expected no %s when no drift was applied", tag)
		}
	}
}

// The sidecar is the default target and needs different tags entirely.

func TestSidecarArgumentsUseTheXMPGroup(t *testing.T) {
	args := writeArguments(sidecarPlanFor(domain.TrackPoint{
		Time: trackedAt, Latitude: 52.5, Longitude: 13.3,
		Elevation: 29.2, HasElevation: true,
	}))

	for _, tag := range []string{"XMP:GPSLatitude", "XMP:GPSLongitude", "XMP:GPSAltitude", "XMP:GPSDateTime"} {
		if !hasTag(args, tag) {
			t.Errorf("expected %s in %v", tag, args)
		}
	}
	// Unqualified, exiftool would resolve some of these to EXIF.
	for _, arg := range args {
		if strings.HasPrefix(arg, "-EXIF:") {
			t.Errorf("sidecar write must not name EXIF tags, got %q", arg)
		}
	}
}

// XMP folds the hemisphere into the value, so a separate reference would be
// meaningless here.
func TestSidecarArgumentsOmitHemisphereReferences(t *testing.T) {
	args := writeArguments(sidecarPlanFor(domain.TrackPoint{
		Time: trackedAt, Latitude: -33.8688, Longitude: -70.6693,
	}))

	for _, tag := range []string{"XMP:GPSLatitudeRef", "XMP:GPSLongitudeRef"} {
		if hasTag(args, tag) {
			t.Errorf("expected no %s for an XMP target", tag)
		}
	}
	if got := argumentValue(t, args, "XMP:GPSLatitude"); !strings.HasPrefix(got, "-33.8688") {
		t.Errorf("XMP:GPSLatitude = %q, expected the sign to carry the hemisphere", got)
	}
}

// XMP has no OffsetTimeOriginal; the zone rides inside the timestamp.
func TestSidecarArgumentsCarryTheZoneInsideTheTimestamp(t *testing.T) {
	plan := sidecarPlanFor(domain.TrackPoint{Time: trackedAt, Latitude: 52.5, Longitude: 13.3})
	plan.WriteTime = true
	plan.CorrectedWall = time.Date(2017, 8, 8, 20, 27, 24, 0, time.UTC)
	plan.CorrectedOffset = 2 * time.Hour

	args := writeArguments(plan)

	if hasTag(args, "XMP:OffsetTimeOriginal") {
		t.Error("XMP has no OffsetTimeOriginal field")
	}
	expected := "2017:08:08 20:27:24+02:00"
	if got := argumentValue(t, args, "XMP:DateTimeOriginal"); got != expected {
		t.Errorf("XMP:DateTimeOriginal = %q, expected %q", got, expected)
	}
}

func TestWriteArgumentsRewriteTimestampsWhenDriftWasApplied(t *testing.T) {
	plan := planFor(domain.TrackPoint{Time: trackedAt, Latitude: 52.5, Longitude: 13.3})
	plan.WriteTime = true
	plan.CorrectedWall = time.Date(2017, 8, 8, 20, 27, 24, 0, time.UTC)
	plan.CorrectedOffset = 2 * time.Hour

	args := writeArguments(plan)

	if got := argumentValue(t, args, "EXIF:DateTimeOriginal"); got != "2017:08:08 20:27:24" {
		t.Errorf("DateTimeOriginal = %q, expected the corrected wall clock", got)
	}
	if got := argumentValue(t, args, "EXIF:CreateDate"); got != "2017:08:08 20:27:24" {
		t.Errorf("CreateDate = %q, expected the corrected wall clock", got)
	}
	if got := argumentValue(t, args, "EXIF:OffsetTimeOriginal"); got != "+02:00" {
		t.Errorf("OffsetTimeOriginal = %q, expected +02:00", got)
	}
}

func TestWriteArgumentsNeverKeepABackupFile(t *testing.T) {
	args := writeArguments(planFor(domain.TrackPoint{Time: trackedAt, Latitude: 52.5, Longitude: 13.3}))

	var overwrites bool
	for _, arg := range args {
		if arg == "-overwrite_original" {
			overwrites = true
		}
	}
	if !overwrites {
		t.Error("expected -overwrite_original so no _original files are left behind")
	}
}

// The tests below drive the real binary. They are the only proof that the
// arguments above mean what they are meant to mean.

func requireExiftool(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not installed")
	}
}

func readTag(t *testing.T, path, tag string) string {
	t.Helper()
	output, err := exec.Command("exiftool", "-n", "-s3", "-"+tag, path).Output()
	if err != nil {
		t.Fatalf("failed to read %s from %s: %v", tag, path, err)
	}
	return strings.TrimSpace(string(output))
}

func TestWriteCreatesASidecarAndRoundTripsThePosition(t *testing.T) {
	requireExiftool(t)

	plan := domain.WritePlan{
		PhotoPath: "DSC_1234.NEF",
		Target:    filepath.Join(t.TempDir(), "DSC_1234.xmp"),
		Fix: domain.Fix{Position: domain.TrackPoint{
			Time: trackedAt, Latitude: -33.86880000, Longitude: -70.66930000,
			Elevation: 520.5, HasElevation: true,
		}},
	}

	plans := []domain.WritePlan{plan}
	if err := NewWriter("exiftool").Write(context.Background(), plans); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !plans[0].Written {
		t.Error("expected the plan to be marked written")
	}
	if readTag(t, plan.Target, "GPSLatitude") != "-33.8688" {
		t.Errorf("GPSLatitude = %q, expected -33.8688", readTag(t, plan.Target, "GPSLatitude"))
	}
	if readTag(t, plan.Target, "GPSLongitude") != "-70.6693" {
		t.Errorf("GPSLongitude = %q, expected -70.6693", readTag(t, plan.Target, "GPSLongitude"))
	}
	if readTag(t, plan.Target, "GPSAltitude") != "520.5" {
		t.Errorf("GPSAltitude = %q, expected 520.5", readTag(t, plan.Target, "GPSAltitude"))
	}
}

// The reason this adapter drives exiftool instead of rendering XMP itself.
func TestWritePreservesRatingsAlreadyInTheSidecar(t *testing.T) {
	requireExiftool(t)

	target := filepath.Join(t.TempDir(), "DSC_1234.xmp")
	if err := os.WriteFile(target, []byte(culledSidecar), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	plans := []domain.WritePlan{{
		PhotoPath: "DSC_1234.NEF",
		Target:    target,
		Fix: domain.Fix{Position: domain.TrackPoint{
			Time: trackedAt, Latitude: 52.5048, Longitude: 13.2995,
		}},
	}}
	if err := NewWriter("exiftool").Write(context.Background(), plans); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := readTag(t, target, "Rating"); got != "5" {
		t.Errorf("Rating = %q, expected the cull rating 5 to survive", got)
	}
	if got := readTag(t, target, "Label"); got != "green" {
		t.Errorf("Label = %q, expected the cull label to survive", got)
	}
	if got := readTag(t, target, "GPSLatitude"); got != "52.5048" {
		t.Errorf("GPSLatitude = %q, expected the position to be recorded", got)
	}
}

func TestWriteReportsAFailingTarget(t *testing.T) {
	requireExiftool(t)

	plans := []domain.WritePlan{{
		PhotoPath: "DSC_1234.NEF",
		Target:    filepath.Join(t.TempDir(), "no-such-directory", "DSC_1234.xmp"),
		Fix:       domain.Fix{Position: domain.TrackPoint{Time: trackedAt, Latitude: 52.5, Longitude: 13.3}},
	}}

	if err := NewWriter("exiftool").Write(context.Background(), plans); err != nil {
		t.Fatalf("a single bad target must not fail the batch: %v", err)
	}

	if plans[0].Written {
		t.Error("expected the plan not to be marked written")
	}
	if plans[0].Skipped == "" {
		t.Fatal("expected the plan to record why it could not be written")
	}
	if !strings.Contains(plans[0].Skipped, "DSC_1234.xmp") {
		t.Errorf("reason %q does not name the target", plans[0].Skipped)
	}
}

// A plan the application already ruled out must not be touched.
func TestWriteLeavesSkippedPlansAlone(t *testing.T) {
	requireExiftool(t)

	target := filepath.Join(t.TempDir(), "DSC_1234.xmp")
	plans := []domain.WritePlan{{
		PhotoPath: "DSC_1234.NEF",
		Target:    target,
		Skipped:   "already has coordinates",
		Fix:       domain.Fix{Position: domain.TrackPoint{Time: trackedAt, Latitude: 52.5, Longitude: 13.3}},
	}}

	if err := NewWriter("exiftool").Write(context.Background(), plans); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plans[0].Written {
		t.Error("expected a skipped plan not to be written")
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("expected no sidecar to be created for a skipped plan")
	}
}

// What `ansel cull --write` leaves beside a photograph.
const culledSidecar = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/" x:xmptk="ansel cull">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
    xmlns:xmp="http://ns.adobe.com/xap/1.0/"
    xmlns:dc="http://purl.org/dc/elements/1.1/">
   <xmp:Rating>5</xmp:Rating>
   <xmp:Label>green</xmp:Label>
   <dc:subject><rdf:Bag><rdf:li>best_in_group</rdf:li></rdf:Bag></dc:subject>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>
`

// Round-trips through the real binary, for both targets, including the GPS
// timestamp that a silently-ignored tag name would leave absent.

func TestWriteRecordsTheGPSTimestampInASidecar(t *testing.T) {
	requireExiftool(t)

	plans := []domain.WritePlan{{
		PhotoPath: "DSC_1234.NEF",
		Target:    filepath.Join(t.TempDir(), "DSC_1234.xmp"),
		Fix:       domain.Fix{Position: domain.TrackPoint{Time: trackedAt, Latitude: 52.5048, Longitude: 13.2995}},
	}}
	if err := NewWriter("exiftool").Write(context.Background(), plans); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := readTag(t, plans[0].Target, "GPSDateTime"); got != "2017:08:08 18:28:54Z" {
		t.Errorf("GPSDateTime = %q, expected the track instant in UTC", got)
	}
}

func TestWriteRecordsADriftCorrectedTimestampInASidecar(t *testing.T) {
	requireExiftool(t)

	plans := []domain.WritePlan{{
		PhotoPath:       "DSC_1234.NEF",
		Target:          filepath.Join(t.TempDir(), "DSC_1234.xmp"),
		Fix:             domain.Fix{Position: domain.TrackPoint{Time: trackedAt, Latitude: 52.5048, Longitude: 13.2995}},
		WriteTime:       true,
		CorrectedWall:   time.Date(2017, 8, 8, 20, 27, 24, 0, time.UTC),
		CorrectedOffset: 2 * time.Hour,
	}}
	if err := NewWriter("exiftool").Write(context.Background(), plans); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := readTag(t, plans[0].Target, "DateTimeOriginal"); got != "2017:08:08 20:27:24+02:00" {
		t.Errorf("DateTimeOriginal = %q, expected the corrected time with its zone", got)
	}
}

func TestWriteInPlaceRecordsEXIFGPS(t *testing.T) {
	requireExiftool(t)

	source := filepath.Join("..", "..", "..", "..", "testdata", "input.jpg")
	target := filepath.Join(t.TempDir(), "DSC_1234.jpg")
	original, err := os.ReadFile(source)
	if err != nil {
		t.Skipf("no sample image available: %v", err)
	}
	if err := os.WriteFile(target, original, 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	plans := []domain.WritePlan{{
		PhotoPath: target,
		Target:    target,
		InPlace:   true,
		Fix: domain.Fix{Position: domain.TrackPoint{
			Time: trackedAt, Latitude: 52.5048, Longitude: 13.2995,
			Elevation: 29.2, HasElevation: true,
		}},
	}}
	if err := NewWriter("exiftool").Write(context.Background(), plans); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reading the EXIF group specifically: an XMP block inside the JPEG would
	// satisfy an unqualified read while leaving the EXIF GPS empty.
	for _, tc := range []struct{ tag, expected string }{
		{"EXIF:GPSLatitude", "52.5048"},
		{"EXIF:GPSLongitude", "13.2995"},
		{"EXIF:GPSAltitude", "29.2"},
		{"EXIF:GPSDateStamp", "2017:08:08"},
		{"EXIF:GPSTimeStamp", "18:28:54"},
	} {
		if got := readTag(t, target, tc.tag); got != tc.expected {
			t.Errorf("%s = %q, expected %q", tc.tag, got, tc.expected)
		}
	}
}
