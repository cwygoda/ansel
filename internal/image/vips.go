package image

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"github.com/davidbyttow/govips/v2/vips"
)

// KernelMKS2021 is Magic Kernel Sharp 2021, added in libvips 8.15
// govips doesn't export this yet, so we define it here.
// Value 7 corresponds to VIPS_KERNEL_MKS2021 in libvips.
const KernelMKS2021 vips.Kernel = 7

// VipsImage wraps a govips image reference.
type VipsImage struct {
	ref *vips.ImageRef
}

// InitVips initializes the vips library. Call once at startup.
// Log level defaults to error. Set ANSEL_LOG_LEVEL env var to change:
// error, warning, info, debug
func InitVips() {
	vips.LoggingSettings(nil, parseLogLevel())
	vips.Startup(&vips.Config{
		ConcurrencyLevel: 0, // Use default (number of CPUs)
		MaxCacheFiles:    0,
		MaxCacheMem:      0,
		MaxCacheSize:     0,
	})
}

// parseLogLevel returns the vips log level from ANSEL_LOG_LEVEL env var.
func parseLogLevel() vips.LogLevel {
	level := strings.ToLower(os.Getenv("ANSEL_LOG_LEVEL"))
	switch level {
	case "debug":
		return vips.LogLevelDebug
	case "info":
		return vips.LogLevelInfo
	case "warning", "warn":
		return vips.LogLevelWarning
	case "error", "":
		return vips.LogLevelError
	default:
		return vips.LogLevelError
	}
}

// ShutdownVips cleans up vips resources. Call before exit.
func ShutdownVips() {
	vips.Shutdown()
}

// LoadVips loads an image using libvips.
func LoadVips(path string) (*VipsImage, error) {
	img, err := vips.NewImageFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load image: %w", err)
	}
	return &VipsImage{ref: img}, nil
}

// Width returns the image width.
func (v *VipsImage) Width() int {
	return v.ref.Width()
}

// Height returns the image height.
func (v *VipsImage) Height() int {
	return v.ref.Height()
}

// Close releases the image resources.
func (v *VipsImage) Close() {
	if v.ref != nil {
		v.ref.Close()
		v.ref = nil
	}
}

// ResizeToFit resizes to fit within the given dimensions, maintaining aspect ratio.
func (v *VipsImage) ResizeToFit(maxWidth, maxHeight int, filter Filter) error {
	srcWidth := float64(v.ref.Width())
	srcHeight := float64(v.ref.Height())

	scaleX := float64(maxWidth) / srcWidth
	scaleY := float64(maxHeight) / srcHeight
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	kernel := filterToVipsKernel(filter)
	err := v.ref.Resize(scale, kernel)
	if err != nil {
		return fmt.Errorf("resize failed: %w", err)
	}

	return nil
}

// AddFrame adds a colored frame around the image.
func (v *VipsImage) AddFrame(top, right, bottom, left int, c color.Color) error {
	r, g, b, _ := c.RGBA()
	// Convert from 16-bit to 8-bit
	bgColor := &vips.Color{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
	}

	err := v.ref.EmbedBackground(left, top, v.ref.Width()+left+right, v.ref.Height()+top+bottom, bgColor)
	if err != nil {
		return fmt.Errorf("add frame failed: %w", err)
	}

	return nil
}

// AddUniformFrame adds a uniform frame on all sides.
func (v *VipsImage) AddUniformFrame(width int, c color.Color) error {
	return v.AddFrame(width, width, width, width, c)
}

// SaveJPEG saves the image as JPEG.
func (v *VipsImage) SaveJPEG(path string, quality int) error {
	params := vips.NewJpegExportParams()
	params.Quality = quality
	params.StripMetadata = true

	bytes, _, err := v.ref.ExportJpeg(params)
	if err != nil {
		return fmt.Errorf("export JPEG failed: %w", err)
	}

	return os.WriteFile(path, bytes, 0644)
}

// Save saves the image, detecting format from extension.
func (v *VipsImage) Save(path string, quality int) error {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return v.SaveJPEG(path, quality)
	case ".png":
		params := vips.NewPngExportParams()
		params.Compression = 6
		bytes, _, err := v.ref.ExportPng(params)
		if err != nil {
			return fmt.Errorf("export PNG failed: %w", err)
		}
		return os.WriteFile(path, bytes, 0644)
	case ".tif", ".tiff":
		params := vips.NewTiffExportParams()
		params.Compression = vips.TiffCompressionDeflate
		bytes, _, err := v.ref.ExportTiff(params)
		if err != nil {
			return fmt.Errorf("export TIFF failed: %w", err)
		}
		return os.WriteFile(path, bytes, 0644)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}

// filterToVipsKernel converts our Filter type to vips kernel.
func filterToVipsKernel(f Filter) vips.Kernel {
	switch f {
	case Bilinear:
		return vips.KernelLinear
	case CatmullRom:
		return vips.KernelCubic
	case Lanczos:
		return vips.KernelLanczos3
	case MagicKernelSharp2021:
		return KernelMKS2021
	default:
		return vips.KernelLanczos3
	}
}
