package exiftool

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

// Writer records positions through exiftool.
//
// Writing deliberately does not share the reader's long-lived `-stay_open`
// process. Every photograph carries different tag values, so nothing can be
// batched into one command anyway, and a write that fails must be attributed
// to the one file it touched. A separate invocation per photograph makes that
// attribution exact, at a cost only paid on runs that write at all — the
// default is a dry run.
//
// exiftool is used for sidecars as well as for in-place edits rather than
// rendering XMP here, because a sidecar beside a photograph is very often one
// `ansel cull` already wrote, holding a rating and a label. Re-rendering that
// file from a template would silently discard an evening's work; exiftool
// updates it in place and leaves untouched what it was not asked about.
type Writer struct {
	binary string
}

// NewWriter returns a Writer driving the given exiftool binary.
func NewWriter(binary string) *Writer {
	if binary == "" {
		binary = "exiftool"
	}
	return &Writer{binary: binary}
}

// Write records the position for every plan not already marked skipped,
// updating each in place.
//
// Writing is best-effort: a target that cannot be written records why on its
// own plan and the rest still go out. One unreadable file in a shoot of eight
// hundred should not cost the operator the other seven hundred and ninety
// nine. The returned error is reserved for a failure of the run as a whole,
// such as a cancelled context.
func (w *Writer) Write(ctx context.Context, plans []domain.WritePlan) error {
	for i := range plans {
		if plans[i].Skipped != "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		w.writeOne(ctx, &plans[i])
	}
	return nil
}

func (w *Writer) writeOne(ctx context.Context, plan *domain.WritePlan) {
	args := append(writeArguments(*plan), plan.Target)

	output, err := exec.CommandContext(ctx, w.binary, args...).CombinedOutput()
	if err != nil {
		plan.Skipped = fmt.Sprintf("failed to write %s: %v (%s)",
			plan.Target, err, strings.TrimSpace(string(output)))
		return
	}
	plan.Written = true
}

// writeArguments builds the tag assignments for one plan.
//
// The two targets need genuinely different tags, and the differences are not
// cosmetic. EXIF splits a position's magnitude from its hemisphere and its
// date from its time, and carries a zone in its own OffsetTimeOriginal field.
// XMP folds the hemisphere into the value, keeps one combined GPS timestamp,
// and has no offset field at all — a zone travels inside the datetime itself.
// Left unqualified, exiftool resolves several of these to whichever group it
// prefers, which for a JPEG quietly means writing an XMP block instead of the
// EXIF the photograph was supposed to get.
func writeArguments(plan domain.WritePlan) []string {
	if plan.InPlace {
		return exifArguments(plan)
	}
	return xmpArguments(plan)
}

// exifArguments writes into the photograph itself.
func exifArguments(plan domain.WritePlan) []string {
	position := plan.Fix.Position
	args := []string{
		"-n",
		"-overwrite_original",
		assign("EXIF:GPSLatitude", formatFloat(position.Latitude)),
		assign("EXIF:GPSLatitudeRef", hemisphere(position.Latitude, "N", "S")),
		assign("EXIF:GPSLongitude", formatFloat(position.Longitude)),
		assign("EXIF:GPSLongitudeRef", hemisphere(position.Longitude, "E", "W")),
	}

	if position.HasElevation {
		args = append(args,
			assign("EXIF:GPSAltitude", formatFloat(position.Elevation)),
			// 0 is above sea level, 1 below.
			assign("EXIF:GPSAltitudeRef", hemisphere(position.Elevation, "0", "1")))
	}

	// The track's own instant, which is UTC by construction and independent
	// of whatever zone the camera believed it was in.
	args = append(args,
		assign("EXIF:GPSDateStamp", position.Time.UTC().Format("2006:01:02")),
		assign("EXIF:GPSTimeStamp", position.Time.UTC().Format("15:04:05")))

	if plan.WriteTime {
		stamp := plan.CorrectedWall.Format(exifTimeLayout)
		args = append(args,
			assign("EXIF:DateTimeOriginal", stamp),
			assign("EXIF:CreateDate", stamp),
			assign("EXIF:OffsetTimeOriginal", formatOffset(plan.CorrectedOffset)),
			assign("EXIF:OffsetTimeDigitized", formatOffset(plan.CorrectedOffset)))
	}
	return args
}

// xmpArguments writes into a sidecar. Hemisphere references are omitted
// because XMP encodes them in the value itself, which exiftool derives from
// the sign under -n.
func xmpArguments(plan domain.WritePlan) []string {
	position := plan.Fix.Position
	args := []string{
		"-n",
		"-overwrite_original",
		assign("XMP:GPSLatitude", formatFloat(position.Latitude)),
		assign("XMP:GPSLongitude", formatFloat(position.Longitude)),
	}

	if position.HasElevation {
		args = append(args,
			assign("XMP:GPSAltitude", formatFloat(position.Elevation)),
			assign("XMP:GPSAltitudeRef", hemisphere(position.Elevation, "0", "1")))
	}

	args = append(args, assign("XMP:GPSDateTime",
		position.Time.UTC().Format("2006:01:02 15:04:05Z")))

	if plan.WriteTime {
		// No OffsetTimeOriginal exists here; the zone rides along in the
		// timestamp, which is how XMP has always carried it.
		stamp := plan.CorrectedWall.Format(exifTimeLayout) + formatOffset(plan.CorrectedOffset)
		args = append(args,
			assign("XMP:DateTimeOriginal", stamp),
			assign("XMP:CreateDate", stamp))
	}
	return args
}

const exifTimeLayout = "2006:01:02 15:04:05"

func assign(tag, value string) string { return "-" + tag + "=" + value }

func hemisphere(value float64, positive, negative string) string {
	if value < 0 {
		return negative
	}
	return positive
}

// formatFloat renders a coordinate without an exponent, which exiftool would
// reject, and without trailing noise from float64's decimal expansion.
func formatFloat(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "0"
	}
	return strconv.FormatFloat(value, 'f', 8, 64)
}
