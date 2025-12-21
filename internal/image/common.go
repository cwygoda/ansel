package image

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// Type aliases for convenient use in other packages
type (
	// Color is an alias for the standard library color.Color interface.
	Color = color.Color
)

// Filter represents a resampling filter type.
type Filter int

const (
	// Lanczos is a high-quality filter, good for downscaling.
	Lanczos Filter = iota
	// CatmullRom is a cubic filter with good sharpness.
	CatmullRom
	// Bilinear is a fast, simple filter.
	Bilinear
	// MagicKernelSharp2021 is the gold-standard resizing filter used by Facebook/Instagram.
	// It combines the Magic Kernel with optimized sharpening for superior results.
	MagicKernelSharp2021
)

// ParseFilter converts a string to a Filter type.
func ParseFilter(s string) (Filter, error) {
	switch s {
	case "lanczos", "lanczos3":
		return Lanczos, nil
	case "catmull-rom", "catmullrom", "cubic":
		return CatmullRom, nil
	case "bilinear", "linear":
		return Bilinear, nil
	case "mks2021", "magic", "magickernel", "magic-kernel-sharp-2021":
		return MagicKernelSharp2021, nil
	default:
		return Lanczos, fmt.Errorf("unknown filter: %s", s)
	}
}

// String returns the filter name.
func (f Filter) String() string {
	switch f {
	case Lanczos:
		return "lanczos"
	case CatmullRom:
		return "catmull-rom"
	case Bilinear:
		return "bilinear"
	case MagicKernelSharp2021:
		return "mks2021"
	default:
		return "unknown"
	}
}

// ParseColor parses a color string into a color.Color.
// Supports hex colors (#RGB, #RRGGBB, #RRGGBBAA) and named colors.
func ParseColor(s string) (color.Color, error) {
	s = strings.TrimSpace(s)

	// Try named colors first
	if c, ok := namedColors[strings.ToLower(s)]; ok {
		return c, nil
	}

	// Try hex color
	if strings.HasPrefix(s, "#") {
		return parseHexColor(s[1:])
	}

	// Try hex without #
	if c, err := parseHexColor(s); err == nil {
		return c, nil
	}

	return nil, fmt.Errorf("invalid color: %s", s)
}

func parseHexColor(hex string) (color.Color, error) {
	var r, g, b, a uint8 = 0, 0, 0, 255

	switch len(hex) {
	case 3: // RGB
		rv, err := strconv.ParseUint(string(hex[0])+string(hex[0]), 16, 8)
		if err != nil {
			return nil, err
		}
		gv, err := strconv.ParseUint(string(hex[1])+string(hex[1]), 16, 8)
		if err != nil {
			return nil, err
		}
		bv, err := strconv.ParseUint(string(hex[2])+string(hex[2]), 16, 8)
		if err != nil {
			return nil, err
		}
		r, g, b = uint8(rv), uint8(gv), uint8(bv)

	case 6: // RRGGBB
		rv, err := strconv.ParseUint(hex[0:2], 16, 8)
		if err != nil {
			return nil, err
		}
		gv, err := strconv.ParseUint(hex[2:4], 16, 8)
		if err != nil {
			return nil, err
		}
		bv, err := strconv.ParseUint(hex[4:6], 16, 8)
		if err != nil {
			return nil, err
		}
		r, g, b = uint8(rv), uint8(gv), uint8(bv)

	case 8: // RRGGBBAA
		rv, err := strconv.ParseUint(hex[0:2], 16, 8)
		if err != nil {
			return nil, err
		}
		gv, err := strconv.ParseUint(hex[2:4], 16, 8)
		if err != nil {
			return nil, err
		}
		bv, err := strconv.ParseUint(hex[4:6], 16, 8)
		if err != nil {
			return nil, err
		}
		av, err := strconv.ParseUint(hex[6:8], 16, 8)
		if err != nil {
			return nil, err
		}
		r, g, b, a = uint8(rv), uint8(gv), uint8(bv), uint8(av)

	default:
		return nil, fmt.Errorf("invalid hex color length: %d", len(hex))
	}

	return color.RGBA{R: r, G: g, B: b, A: a}, nil
}

// Common named colors
var namedColors = map[string]color.Color{
	"black":   color.RGBA{0, 0, 0, 255},
	"white":   color.RGBA{255, 255, 255, 255},
	"red":     color.RGBA{255, 0, 0, 255},
	"green":   color.RGBA{0, 128, 0, 255},
	"blue":    color.RGBA{0, 0, 255, 255},
	"yellow":  color.RGBA{255, 255, 0, 255},
	"cyan":    color.RGBA{0, 255, 255, 255},
	"magenta": color.RGBA{255, 0, 255, 255},
	"gray":    color.RGBA{128, 128, 128, 255},
	"grey":    color.RGBA{128, 128, 128, 255},
	"orange":  color.RGBA{255, 165, 0, 255},
	"purple":  color.RGBA{128, 0, 128, 255},
	"pink":    color.RGBA{255, 192, 203, 255},
	"brown":   color.RGBA{165, 42, 42, 255},
	"navy":    color.RGBA{0, 0, 128, 255},
	"teal":    color.RGBA{0, 128, 128, 255},
	"olive":   color.RGBA{128, 128, 0, 255},
	"maroon":  color.RGBA{128, 0, 0, 255},
	"silver":  color.RGBA{192, 192, 192, 255},
	"lime":    color.RGBA{0, 255, 0, 255},
	"aqua":    color.RGBA{0, 255, 255, 255},
	"fuchsia": color.RGBA{255, 0, 255, 255},
}
