package image

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bep/imagemeta"
)

// debugLog prints debug messages when ANSEL_LOG_LEVEL is set to "debug"
func debugLog(format string, args ...interface{}) {
	if strings.ToLower(os.Getenv("ANSEL_LOG_LEVEL")) == "debug" {
		fmt.Fprintf(os.Stderr, "[metadata] "+format+"\n", args...)
	}
}

// ReadIPTCHeadline extracts the IPTC headline from an image file.
// It first checks for a DXO PhotoLab sidecar file (.dop), then falls back
// to embedded IPTC metadata in the image.
// Returns empty string if no headline is found or on error.
func ReadIPTCHeadline(path string) string {
	debugLog("reading headline for %s", path)

	// First, try DXO sidecar file
	if headline := readDXOHeadline(path); headline != "" {
		debugLog("using DXO sidecar headline: %q", headline)
		return headline
	}

	// Fall back to embedded IPTC
	headline := readEmbeddedIPTCHeadline(path)
	if headline != "" {
		debugLog("using embedded IPTC headline: %q", headline)
	} else {
		debugLog("no headline found")
	}
	return headline
}

// readDXOHeadline reads the headline from a DXO PhotoLab sidecar file (.dop).
func readDXOHeadline(imagePath string) string {
	// DXO sidecar is image.ext.dop (e.g., photo.jpg.dop)
	dopPath := imagePath + ".dop"
	debugLog("checking for DXO sidecar: %s", dopPath)

	data, err := os.ReadFile(dopPath)
	if err != nil {
		debugLog("no DXO sidecar found: %v", err)
		return ""
	}
	debugLog("found DXO sidecar (%d bytes)", len(data))

	// Parse contentHeadline from DXO format:
	// contentHeadline = "Some Headline",
	re := regexp.MustCompile(`contentHeadline\s*=\s*"([^"]*)"`)
	matches := re.FindSubmatch(data)
	if len(matches) >= 2 {
		headline := string(matches[1])
		debugLog("parsed contentHeadline from DXO: %q", headline)
		return headline
	}

	debugLog("no contentHeadline field in DXO sidecar")
	return ""
}

// readEmbeddedIPTCHeadline reads IPTC headline from embedded image metadata.
func readEmbeddedIPTCHeadline(path string) string {
	debugLog("reading embedded IPTC from %s", path)

	f, err := os.Open(path)
	if err != nil {
		debugLog("failed to open file: %v", err)
		return ""
	}
	defer f.Close()

	// Determine image format from extension
	format := detectImageFormat(path)
	if format == 0 {
		debugLog("unsupported image format")
		return ""
	}
	debugLog("detected format: %v", format)

	var headline string
	var tagCount int
	err = imagemeta.Decode(imagemeta.Options{
		R:           f,
		ImageFormat: format,
		Sources:     imagemeta.IPTC,
		HandleTag: func(tag imagemeta.TagInfo) error {
			tagCount++
			debugLog("IPTC tag: %s = %v", tag.Tag, tag.Value)
			if tag.Tag == "Headline" {
				if s, ok := tag.Value.(string); ok {
					headline = s
				}
			}
			return nil
		},
	})
	if err != nil {
		debugLog("IPTC decode error: %v", err)
		return ""
	}

	debugLog("parsed %d IPTC tags, headline: %q", tagCount, headline)
	return headline
}

// detectImageFormat returns the imagemeta format for a file path.
func detectImageFormat(path string) imagemeta.ImageFormat {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return imagemeta.JPEG
	case ".png":
		return imagemeta.PNG
	case ".tif", ".tiff":
		return imagemeta.TIFF
	case ".webp":
		return imagemeta.WebP
	default:
		return 0
	}
}
