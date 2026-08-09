package image

import (
	"bytes"
	"fmt"
	"image"
	"image/png"

	"github.com/davidbyttow/govips/v2/vips"
)

// LoadVipsBuffer loads an image from memory. Embedded RAW previews arrive as
// bytes on a pipe, never as a file on disk.
func LoadVipsBuffer(data []byte) (*VipsImage, error) {
	img, err := vips.NewImageFromBuffer(data)
	if err != nil {
		return nil, fmt.Errorf("failed to load image from buffer: %w", err)
	}
	return &VipsImage{ref: img}, nil
}

// Grayscale returns one luminance byte per pixel.
//
// The pixels are taken straight from vips memory rather than round-tripped
// through JPEG. That matters for analysis: JPEG re-encoding introduces
// high-frequency ringing, which is precisely the signal a sharpness
// measurement is looking for, so a re-encoded frame would score as sharper
// than it is.
func (v *VipsImage) Grayscale() (pix []byte, width, height int, err error) {
	if err := v.ref.ToColorSpace(vips.InterpretationBW); err != nil {
		return nil, 0, 0, fmt.Errorf("grayscale conversion failed: %w", err)
	}

	width, height = v.ref.Width(), v.ref.Height()
	raw, err := v.ref.ToBytes()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to read pixels: %w", err)
	}

	bands := v.ref.Bands()
	// A mismatch means the buffer is not 8 bits per sample — a 16-bit TIFF
	// preview, for example. Rather than misinterpret the bytes, fall back to a
	// lossless decode that normalizes depth for us.
	if bands <= 0 || len(raw) != width*height*bands {
		return v.grayscaleViaPNG()
	}
	return firstBand(raw, width*height, bands), width, height, nil
}

// firstBand extracts the luminance channel, skipping any alpha that survived
// the colorspace conversion.
func firstBand(raw []byte, pixels, bands int) []byte {
	if bands == 1 {
		return raw
	}
	pix := make([]byte, pixels)
	for i := 0; i < pixels; i++ {
		pix[i] = raw[i*bands]
	}
	return pix
}

func (v *VipsImage) grayscaleViaPNG() (pix []byte, width, height int, err error) {
	params := vips.NewPngExportParams()
	params.Compression = 0
	encoded, _, err := v.ref.ExportPng(params)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("fallback PNG export failed: %w", err)
	}

	decoded, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("fallback PNG decode failed: %w", err)
	}

	bounds := decoded.Bounds()
	gray := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x, y, decoded.At(x, y))
		}
	}
	return gray.Pix, bounds.Dx(), bounds.Dy(), nil
}
