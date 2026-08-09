package exiftool

import (
	"strings"
	"time"

	"github.com/cwygoda/ansel/internal/cull/domain"
	"github.com/cwygoda/ansel/internal/exiftool"
)

// timeLayouts covers what EXIF timestamps look like in practice: the base
// format, plus optional sub-seconds and offset when the camera records them.
var timeLayouts = []string{
	"2006:01:02 15:04:05.999999-07:00",
	"2006:01:02 15:04:05-07:00",
	"2006:01:02 15:04:05.999999",
	"2006:01:02 15:04:05",
}

func metadataFrom(entry map[string]any) domain.Metadata {
	return domain.Metadata{
		CaptureTime:    captureTimeFrom(entry),
		Camera:         cameraFrom(entry),
		Lens:           exiftool.AsString(entry["LensModel"]),
		FocalLength:    exiftool.AsFloat(entry["FocalLength"]),
		Aperture:       exiftool.AsFloat(entry["FNumber"]),
		ShutterSeconds: exiftool.AsFloat(entry["ExposureTime"]),
		ISO:            exiftool.AsInt(entry["ISO"]),
		Orientation:    exiftool.AsInt(entry["Orientation"]),
		Width:          exiftool.AsInt(entry["ImageWidth"]),
		Height:         exiftool.AsInt(entry["ImageHeight"]),
	}
}

// captureTimeFrom prefers DateTimeOriginal, the moment the shutter fired,
// over CreateDate, which some pipelines rewrite.
func captureTimeFrom(entry map[string]any) time.Time {
	for _, key := range []string{"DateTimeOriginal", "CreateDate"} {
		if parsed, ok := parseTime(exiftool.AsString(entry[key])); ok {
			return parsed
		}
	}
	return time.Time{}
}

func cameraFrom(entry map[string]any) string {
	maker, model := exiftool.AsString(entry["Make"]), exiftool.AsString(entry["Model"])
	switch {
	case maker == "":
		return model
	case model == "":
		return maker
	case strings.HasPrefix(strings.ToLower(model), strings.ToLower(maker)):
		return model
	default:
		return maker + " " + model
	}
}

func parseTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	// Cameras write this for an unset clock; it is not a real timestamp.
	if value == "" || strings.HasPrefix(value, "0000") {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
