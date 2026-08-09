package exiftool

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cwygoda/ansel/internal/cull/domain"
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
		Lens:           asString(entry["LensModel"]),
		FocalLength:    asFloat(entry["FocalLength"]),
		Aperture:       asFloat(entry["FNumber"]),
		ShutterSeconds: asFloat(entry["ExposureTime"]),
		ISO:            asInt(entry["ISO"]),
		Orientation:    asInt(entry["Orientation"]),
		Width:          asInt(entry["ImageWidth"]),
		Height:         asInt(entry["ImageHeight"]),
	}
}

// captureTimeFrom prefers DateTimeOriginal, the moment the shutter fired,
// over CreateDate, which some pipelines rewrite.
func captureTimeFrom(entry map[string]any) time.Time {
	for _, key := range []string{"DateTimeOriginal", "CreateDate"} {
		if parsed, ok := parseTime(asString(entry[key])); ok {
			return parsed
		}
	}
	return time.Time{}
}

func cameraFrom(entry map[string]any) string {
	maker, model := asString(entry["Make"]), asString(entry["Model"])
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

// asString coerces a JSON value that exiftool may report as a string or a
// number depending on the tag and the file.
func asString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return fmt.Sprint(typed)
	}
}

func asFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func asInt(value any) int {
	return int(asFloat(value))
}
