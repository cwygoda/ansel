package image

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
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

// getTextDimensions uses vips CLI to measure actual text dimensions.
func getTextDimensions(text, font string) (width, height int, err error) {
	tmpFile := "/tmp/ansel_text_measure.png"
	defer os.Remove(tmpFile)

	// Use vips text command to create a text image
	cmd := exec.Command("vips", "text", tmpFile, text, "--font", font)
	if err := cmd.Run(); err != nil {
		return 0, 0, err
	}

	// Read dimensions using vipsheader
	out, err := exec.Command("vipsheader", "-f", "width", tmpFile).Output()
	if err != nil {
		return 0, 0, err
	}
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &width)

	out, err = exec.Command("vipsheader", "-f", "height", tmpFile).Output()
	if err != nil {
		return 0, 0, err
	}
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &height)

	return width, height, nil
}

// AddLabel renders a text label on the image with a white background.
// Font should be in Pango format, e.g. "sans 24" or "Arial Bold 18".
// fontSize is the point size used in the font string (needed for positioning).
// offsetX is the X position where the image starts (left edge alignment).
// imageBottomY is the Y position where the image ends (bottom edge).
// paddingY is the vertical padding between image bottom and label top.
func (v *VipsImage) AddLabel(text, font string, fontSize int, offsetX, imageBottomY, paddingY int) error {
	if text == "" {
		return nil
	}

	// Get actual text dimensions by rendering to a temporary image
	textWidth, textHeight, err := getTextDimensions(text, font)
	if err != nil {
		// Fallback to estimation
		debugLog("AddLabel: getTextDimensions failed: %v, using estimation", err)
		textWidth = int(float64(len(text)) * float64(fontSize) * 0.6)
		textHeight = int(float64(fontSize) * 1.35)
	}

	padding := fontSize / 3
	bgWidth := textWidth + 2*padding
	bgHeight := textHeight + padding

	// Position label so its top aligns with imageBottomY + paddingY
	bgTop := imageBottomY + paddingY
	bgLeft := offsetX - padding/2
	if bgLeft < 0 {
		bgLeft = 0
	}
	if bgTop < 0 {
		bgTop = 0
	}
	// Clamp to image bounds
	if bgLeft+bgWidth > v.Width() {
		bgWidth = v.Width() - bgLeft
	}
	if bgTop+bgHeight > v.Height() {
		bgHeight = v.Height() - bgTop
	}

	debugLog("AddLabel: text=%q font=%s fontSize=%d", text, font, fontSize)
	debugLog("AddLabel: offsetX=%d imageBottomY=%d paddingY=%d textSize=%dx%d imageSize=%dx%d",
		offsetX, imageBottomY, paddingY, textWidth, textHeight, v.Width(), v.Height())
	debugLog("AddLabel: background rect at (%d,%d) size %dx%d", bgLeft, bgTop, bgWidth, bgHeight)

	// Draw white background rectangle
	bgColor := vips.ColorRGBA{
		R: 255,
		G: 255,
		B: 255,
		A: 255,
	}

	if err := v.ref.DrawRect(bgColor, bgLeft, bgTop, bgWidth, bgHeight, true); err != nil {
		debugLog("AddLabel: DrawRect error: %v", err)
		return err
	}

	// Now overlay black text (solid, no transparency)
	// Position text inside the background box
	textX := bgLeft + padding/2
	textY := bgTop + padding/2

	labelColor := vips.Color{R: 0, G: 0, B: 0}
	params := &vips.LabelParams{
		Text:      text,
		Font:      font,
		Width:     vips.Scalar{Value: float64(v.Width() - textX), Relative: false}, // Max width for text
		Height:    vips.Scalar{Value: float64(textHeight), Relative: false},
		OffsetX:   vips.Scalar{Value: float64(textX), Relative: false},
		OffsetY:   vips.Scalar{Value: float64(textY), Relative: false},
		Opacity:   1.0, // Solid black text
		Color:     labelColor,
		Alignment: vips.AlignLow, // Left align
	}

	debugLog("AddLabel: text position (%d,%d) maxWidth=%d height=%d", textX, textY, v.Width()-textX, textHeight)

	if err := v.ref.Label(params); err != nil {
		debugLog("AddLabel: Label error: %v", err)
		return err
	}
	return nil
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
