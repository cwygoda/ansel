package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	imglib "github.com/cwygoda/ansel/internal/image"
	"github.com/spf13/cobra"
)

// Size presets for common platforms
var sizePresets = map[string][2]int{
	// Instagram
	"ig-post":      {1080, 1080},
	"ig-portrait":  {1080, 1350},
	"ig-landscape": {1080, 566},
	"ig-story":     {1080, 1920},
	"ig-reel":      {1080, 1920},
	// Facebook
	"fb-post":  {1200, 630},
	"fb-cover": {820, 312},
	// Twitter/X
	"x-post":   {1200, 675},
	"x-header": {1500, 500},
	// YouTube
	"yt-thumb": {1280, 720},
	// LinkedIn
	"li-post":  {1200, 627},
	"li-cover": {1584, 396},
	// Print (300 DPI)
	"4x6":  {1800, 1200},
	"5x7":  {2100, 1500},
	"8x10": {3000, 2400},
}

var processCmd = &cobra.Command{
	Use:   "process [flags] <input>...",
	Short: "Resize and frame images",
	Long: `Process images by resizing and adding a frame.

Output files are created next to the input files with a version suffix:
  photo.jpg → photo_v0.jpg
  photo_v0.jpg → photo_v1.jpg

Output size can be specified as:
  - Two numbers: --size 1920x1080 or --size 1920,1080
  - A preset name: --size ig-post, --size ig-story, etc.

Available presets:
  Instagram: ig-post (1080x1080), ig-portrait (1080x1350), ig-landscape (1080x566),
             ig-story (1080x1920), ig-reel (1080x1920)
  Facebook:  fb-post (1200x630), fb-cover (820x312)
  Twitter/X: x-post (1200x675), x-header (1500x500)
  YouTube:   yt-thumb (1280x720)
  LinkedIn:  li-post (1200x627), li-cover (1584x396)
  Print:     4x6 (1800x1200), 5x7 (2100x1500), 8x10 (3000x2400)

Fit modes:
  - expand: Output is exactly the specified size. Image is resized to fit within
            the frame area and centered. Frame fills remaining space.
  - wrap:   Frame wraps tightly around the resized image. Output size equals
            image size plus frame on all sides.

Examples:
  # Process a single image for Instagram
  ansel process --size ig-post photo.jpg

  # Process multiple images with black frame
  ansel process --size 1920x1080 --color black *.jpg

  # Wrap mode with 3% frame
  ansel process --size 800x600 --fit wrap --frame 3 photo.jpg`,
	Args: cobra.MinimumNArgs(1),
	RunE: runProcess,
}

var (
	processSize    string
	processFilter  string
	processFit     string
	processFrame   float64
	processColor   string
	processQuality int
	processOutDir  string
)

func init() {
	rootCmd.AddCommand(processCmd)

	processCmd.Flags().StringVar(&processSize, "size", "", "Output size: WxH, W,H, or preset name (required)")
	processCmd.Flags().StringVar(&processFilter, "filter", "mks2021", "Resize filter: lanczos, catmull-rom, bilinear, mks2021")
	processCmd.Flags().StringVar(&processFit, "fit", "expand", "Fit mode: expand or wrap")
	processCmd.Flags().Float64Var(&processFrame, "frame", 5, "Frame width as percentage of shorter side")
	processCmd.Flags().StringVar(&processColor, "color", "#fff", "Frame color (hex or named)")
	processCmd.Flags().IntVar(&processQuality, "quality", 92, "JPEG quality (1-100)")
	processCmd.Flags().StringVarP(&processOutDir, "outdir", "o", "", "Output directory (created if needed)")

	processCmd.MarkFlagRequired("size")
}

func runProcess(cmd *cobra.Command, args []string) error {
	// Initialize vips
	imglib.InitVips()
	defer imglib.ShutdownVips()

	// Parse output size
	targetWidth, targetHeight, err := parseSize(processSize)
	if err != nil {
		return err
	}

	// Parse filter
	filter, err := imglib.ParseFilter(processFilter)
	if err != nil {
		return err
	}

	// Parse color
	frameColor, err := imglib.ParseColor(processColor)
	if err != nil {
		return fmt.Errorf("invalid color: %w", err)
	}

	// Create output directory if specified
	if processOutDir != "" {
		if err := os.MkdirAll(processOutDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Calculate frame width in pixels (percentage of shorter output side)
	shorterSide := targetWidth
	if targetHeight < targetWidth {
		shorterSide = targetHeight
	}
	frameWidthPx := int(float64(shorterSide) * processFrame / 100.0)

	// Process each input file
	for _, inputPath := range args {
		if err := processFile(inputPath, targetWidth, targetHeight, frameWidthPx, frameColor, filter); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", inputPath, err)
			continue
		}
	}

	return nil
}

func processFile(inputPath string, targetWidth, targetHeight, frameWidthPx int, frameColor imglib.Color, filter imglib.Filter) error {
	// Generate output path
	outputPath := generateOutputPath(inputPath, processOutDir)

	// Load image using vips
	img, err := imglib.LoadVips(inputPath)
	if err != nil {
		return fmt.Errorf("failed to load: %w", err)
	}
	defer img.Close()

	fmt.Fprintf(os.Stderr, "%s: %dx%d", inputPath, img.Width(), img.Height())

	switch processFit {
	case "expand":
		err = processExpandVips(img, targetWidth, targetHeight, frameWidthPx, frameColor, filter)
	case "wrap":
		err = processWrapVips(img, targetWidth, targetHeight, frameWidthPx, frameColor, filter)
	default:
		return fmt.Errorf("unknown fit mode: %s", processFit)
	}

	if err != nil {
		return err
	}

	// Save
	if err := img.SaveJPEG(outputPath, processQuality); err != nil {
		return fmt.Errorf("failed to save: %w", err)
	}

	fmt.Fprintf(os.Stderr, " → %s (%dx%d)\n", outputPath, img.Width(), img.Height())
	return nil
}

// processExpandVips creates output of exactly targetWidth x targetHeight.
// Image is resized to fit within the frame area and centered.
func processExpandVips(img *imglib.VipsImage, targetWidth, targetHeight, frameWidth int, frameColor imglib.Color, filter imglib.Filter) error {
	// Calculate available space for the image (inside frame)
	availWidth := targetWidth - 2*frameWidth
	availHeight := targetHeight - 2*frameWidth

	if availWidth <= 0 || availHeight <= 0 {
		return fmt.Errorf("frame too large for output size")
	}

	// Resize to fit within available space
	if err := img.ResizeToFit(availWidth, availHeight, filter); err != nil {
		return err
	}

	// Calculate centering offsets
	resizeWidth := img.Width()
	resizeHeight := img.Height()
	offsetX := (targetWidth - resizeWidth) / 2
	offsetY := (targetHeight - resizeHeight) / 2

	// Add frame with asymmetric borders to center the image
	return img.AddFrame(
		offsetY,                              // top
		targetWidth-resizeWidth-offsetX,      // right
		targetHeight-resizeHeight-offsetY,    // bottom
		offsetX,                              // left
		frameColor,
	)
}

// processWrapVips resizes image to fit target size, then wraps frame around it.
func processWrapVips(img *imglib.VipsImage, targetWidth, targetHeight, frameWidth int, frameColor imglib.Color, filter imglib.Filter) error {
	// Resize to fit target dimensions
	if err := img.ResizeToFit(targetWidth, targetHeight, filter); err != nil {
		return err
	}

	// Add uniform frame
	return img.AddUniformFrame(frameWidth, frameColor)
}

// generateOutputPath creates output filename with version suffix.
func generateOutputPath(inputPath string, outDir string) string {
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), ext)

	// Check if base already has a version suffix like _v0, _v1, etc.
	versionRegex := regexp.MustCompile(`^(.+)_v(\d+)$`)
	matches := versionRegex.FindStringSubmatch(base)

	var newBase string
	if matches != nil {
		// Already has version suffix, increment it
		baseName := matches[1]
		version, _ := strconv.Atoi(matches[2])
		newBase = fmt.Sprintf("%s_v%d", baseName, version+1)
	} else {
		// No version suffix, add _v0
		newBase = base + "_v0"
	}

	// Use output directory if specified, otherwise use input file's directory
	dir := outDir
	if dir == "" {
		dir = filepath.Dir(inputPath)
	}

	// Always output as JPEG
	return filepath.Join(dir, newBase+".jpg")
}

func parseSize(s string) (int, int, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	// Check for preset
	if preset, ok := sizePresets[s]; ok {
		return preset[0], preset[1], nil
	}

	// Try parsing as WxH or W,H
	var width, height int
	var err error

	// Try "WxH" format
	if strings.Contains(s, "x") {
		parts := strings.Split(s, "x")
		if len(parts) == 2 {
			width, err = strconv.Atoi(strings.TrimSpace(parts[0]))
			if err != nil {
				return 0, 0, fmt.Errorf("invalid width: %s", parts[0])
			}
			height, err = strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return 0, 0, fmt.Errorf("invalid height: %s", parts[1])
			}
			return width, height, nil
		}
	}

	// Try "W,H" format
	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		if len(parts) == 2 {
			width, err = strconv.Atoi(strings.TrimSpace(parts[0]))
			if err != nil {
				return 0, 0, fmt.Errorf("invalid width: %s", parts[0])
			}
			height, err = strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return 0, 0, fmt.Errorf("invalid height: %s", parts[1])
			}
			return width, height, nil
		}
	}

	// List available presets in error message
	presetList := make([]string, 0, len(sizePresets))
	for name := range sizePresets {
		presetList = append(presetList, name)
	}

	return 0, 0, fmt.Errorf("invalid size '%s'. Use WxH, W,H, or a preset: %s", s, strings.Join(presetList, ", "))
}
