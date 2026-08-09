package exiftool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// previewTags are tried in order of decreasing size. A Nikon NEF carries a
// full-resolution JpgFromRaw alongside smaller previews; taking the largest
// available means sharpness is measured on real detail rather than on a
// thumbnail that would look uniformly soft.
var previewTags = []string{"JpgFromRaw", "PreviewImage", "ThumbnailImage"}

// jpegExtensions are already their own analysis image and need no extraction.
var jpegExtensions = map[string]bool{".jpg": true, ".jpeg": true}

// Preview returns the bytes of the best analysis image for a photograph.
//
// The RAW file is only ever read, never modified: extraction writes to stdout
// and the original is left byte for byte identical.
func (r *Reader) Preview(ctx context.Context, path string) ([]byte, error) {
	if jpegExtensions[strings.ToLower(filepath.Ext(path))] {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}
		return data, nil
	}
	return r.extractEmbedded(ctx, path)
}

func (r *Reader) extractEmbedded(ctx context.Context, path string) ([]byte, error) {
	var lastErr error
	for _, tag := range previewTags {
		data, err := r.runBinary(ctx, tag, path)
		if err != nil {
			lastErr = err
			continue
		}
		if len(data) > 0 {
			return data, nil
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("failed to extract preview from %s: %w", path, lastErr)
	}
	return nil, fmt.Errorf("no embedded preview found in %s", path)
}

// runBinary asks exiftool for one binary tag. This runs as its own process
// rather than through the stay_open stream, because binary payloads cannot be
// framed against the textual ready marker.
func (r *Reader) runBinary(ctx context.Context, tag, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.binary, "-b", "-"+tag, path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s -b -%s failed: %w\n%s",
			r.binary, tag, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
