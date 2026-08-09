package exiftool

import (
	"fmt"
	"strconv"
	"strings"
)

// AsString coerces a JSON value that exiftool may report as a string or a
// number depending on the tag and the file.
func AsString(value any) string {
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

// AsFloat coerces the same way for numeric tags, yielding zero for anything
// that is not a number.
func AsFloat(value any) float64 {
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

// AsInt truncates AsFloat. Every integer tag exiftool reports under -n arrives
// as a JSON number, so there is nothing to round.
func AsInt(value any) int {
	return int(AsFloat(value))
}
