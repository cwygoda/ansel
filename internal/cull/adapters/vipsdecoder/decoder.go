// Package vipsdecoder adapts the libvips-backed image library to the cull
// preview-decoding port.
package vipsdecoder

import (
	"fmt"

	"github.com/cwygoda/ansel/internal/cull/domain"
	imglib "github.com/cwygoda/ansel/internal/image"
)

// Decoder turns encoded preview bytes into a downscaled grayscale buffer.
type Decoder struct{}

// New returns a Decoder. It requires imglib.InitVips to have been called.
func New() Decoder { return Decoder{} }

// DecodeLuma decodes, downscales and flattens a preview to luminance.
//
// Downscaling happens here rather than in the caller so a full-resolution
// decode is never handed out and never retained: a shoot analyzed in parallel
// would otherwise hold several 45-megapixel frames in memory at once.
func (Decoder) DecodeLuma(data []byte, maxEdge int) (*domain.Luma, error) {
	img, err := imglib.LoadVipsBuffer(data)
	if err != nil {
		return nil, err
	}
	defer img.Close()

	if maxEdge > 0 && (img.Width() > maxEdge || img.Height() > maxEdge) {
		// Bilinear, deliberately: the Magic Kernel Sharp filter used for
		// output sharpens by design, which would inflate the very
		// high-frequency energy the sharpness analyzers measure.
		if err := img.ResizeToFit(maxEdge, maxEdge, imglib.Bilinear); err != nil {
			return nil, err
		}
	}

	pix, width, height, err := img.Grayscale()
	if err != nil {
		return nil, err
	}
	return toLuma(pix, width, height)
}

func toLuma(pix []byte, width, height int) (*domain.Luma, error) {
	if width <= 0 || height <= 0 || len(pix) < width*height {
		return nil, fmt.Errorf("unexpected preview buffer: %d bytes for %dx%d", len(pix), width, height)
	}

	luma := domain.NewLuma(width, height)
	for i := 0; i < width*height; i++ {
		luma.Pix[i] = float64(pix[i]) / 255
	}
	return luma, nil
}
