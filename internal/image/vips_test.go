package image

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

const testImageVips = "../../testdata/input.jpg"

func TestMain(m *testing.M) {
	InitVips()
	code := m.Run()
	ShutdownVips()
	os.Exit(code)
}

func TestLoadVips(t *testing.T) {
	img, err := LoadVips(testImageVips)
	if err != nil {
		t.Fatalf("LoadVips failed: %v", err)
	}
	defer img.Close()

	if img.Width() == 0 || img.Height() == 0 {
		t.Error("Loaded image has zero dimensions")
	}

	t.Logf("Loaded image: %dx%d", img.Width(), img.Height())
}

func TestVipsResizeToFit(t *testing.T) {
	img, err := LoadVips(testImageVips)
	if err != nil {
		t.Fatalf("LoadVips failed: %v", err)
	}
	defer img.Close()

	origWidth := img.Width()
	origHeight := img.Height()

	// Resize to fit within 400x400
	err = img.ResizeToFit(400, 400, MagicKernelSharp2021)
	if err != nil {
		t.Fatalf("ResizeToFit failed: %v", err)
	}

	// Check that dimensions are within bounds
	if img.Width() > 400 || img.Height() > 400 {
		t.Errorf("Image exceeds bounds: got %dx%d, max 400x400", img.Width(), img.Height())
	}

	// Check that at least one dimension hits the target
	if img.Width() != 400 && img.Height() != 400 {
		t.Errorf("Image should fit exactly on one dimension: got %dx%d", img.Width(), img.Height())
	}

	t.Logf("Resized from %dx%d to %dx%d", origWidth, origHeight, img.Width(), img.Height())
}

func TestVipsResizeFilters(t *testing.T) {
	filters := []Filter{Bilinear, CatmullRom, Lanczos, MagicKernelSharp2021}

	for _, filter := range filters {
		t.Run(filter.String(), func(t *testing.T) {
			img, err := LoadVips(testImageVips)
			if err != nil {
				t.Fatalf("LoadVips failed: %v", err)
			}
			defer img.Close()

			err = img.ResizeToFit(300, 300, filter)
			if err != nil {
				t.Fatalf("ResizeToFit with %s failed: %v", filter, err)
			}

			if img.Width() > 300 || img.Height() > 300 {
				t.Errorf("Image exceeds bounds with %s filter", filter)
			}
		})
	}
}

func TestVipsAddFrame(t *testing.T) {
	img, err := LoadVips(testImageVips)
	if err != nil {
		t.Fatalf("LoadVips failed: %v", err)
	}
	defer img.Close()

	// Resize first to speed up test
	err = img.ResizeToFit(200, 200, Bilinear)
	if err != nil {
		t.Fatalf("ResizeToFit failed: %v", err)
	}

	widthBefore := img.Width()
	heightBefore := img.Height()

	// Add asymmetric frame
	err = img.AddFrame(10, 20, 30, 40, color.White)
	if err != nil {
		t.Fatalf("AddFrame failed: %v", err)
	}

	expectedWidth := widthBefore + 20 + 40  // left + right
	expectedHeight := heightBefore + 10 + 30 // top + bottom

	if img.Width() != expectedWidth || img.Height() != expectedHeight {
		t.Errorf("Frame size wrong: expected %dx%d, got %dx%d",
			expectedWidth, expectedHeight, img.Width(), img.Height())
	}
}

func TestVipsAddUniformFrame(t *testing.T) {
	img, err := LoadVips(testImageVips)
	if err != nil {
		t.Fatalf("LoadVips failed: %v", err)
	}
	defer img.Close()

	// Resize first
	err = img.ResizeToFit(200, 200, Bilinear)
	if err != nil {
		t.Fatalf("ResizeToFit failed: %v", err)
	}

	widthBefore := img.Width()
	heightBefore := img.Height()
	frameWidth := 25

	err = img.AddUniformFrame(frameWidth, color.Black)
	if err != nil {
		t.Fatalf("AddUniformFrame failed: %v", err)
	}

	expectedWidth := widthBefore + 2*frameWidth
	expectedHeight := heightBefore + 2*frameWidth

	if img.Width() != expectedWidth || img.Height() != expectedHeight {
		t.Errorf("Uniform frame size wrong: expected %dx%d, got %dx%d",
			expectedWidth, expectedHeight, img.Width(), img.Height())
	}
}

func TestVipsSaveJPEG(t *testing.T) {
	img, err := LoadVips(testImageVips)
	if err != nil {
		t.Fatalf("LoadVips failed: %v", err)
	}
	defer img.Close()

	// Resize to make test faster
	err = img.ResizeToFit(200, 200, Bilinear)
	if err != nil {
		t.Fatalf("ResizeToFit failed: %v", err)
	}

	outputDir := "../../testdata/output"
	os.MkdirAll(outputDir, 0755)
	outputPath := filepath.Join(outputDir, "vips_test_output.jpg")

	err = img.SaveJPEG(outputPath, 85)
	if err != nil {
		t.Fatalf("SaveJPEG failed: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Output file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Output file is empty")
	}

	t.Logf("Saved JPEG: %s (%d bytes)", outputPath, info.Size())
}

func TestVipsSavePNG(t *testing.T) {
	img, err := LoadVips(testImageVips)
	if err != nil {
		t.Fatalf("LoadVips failed: %v", err)
	}
	defer img.Close()

	err = img.ResizeToFit(100, 100, Bilinear)
	if err != nil {
		t.Fatalf("ResizeToFit failed: %v", err)
	}

	outputDir := "../../testdata/output"
	os.MkdirAll(outputDir, 0755)
	outputPath := filepath.Join(outputDir, "vips_test_output.png")

	err = img.Save(outputPath, 0) // quality ignored for PNG
	if err != nil {
		t.Fatalf("Save PNG failed: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Output file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Output file is empty")
	}

	t.Logf("Saved PNG: %s (%d bytes)", outputPath, info.Size())
}

func TestVipsFullPipeline(t *testing.T) {
	// Test the full processing pipeline: load -> resize -> frame -> save
	img, err := LoadVips(testImageVips)
	if err != nil {
		t.Fatalf("LoadVips failed: %v", err)
	}
	defer img.Close()

	origWidth := img.Width()
	origHeight := img.Height()

	// Resize using MKS2021
	err = img.ResizeToFit(500, 500, MagicKernelSharp2021)
	if err != nil {
		t.Fatalf("ResizeToFit failed: %v", err)
	}

	resizedWidth := img.Width()
	resizedHeight := img.Height()

	// Add frame
	frameWidth := 20
	err = img.AddUniformFrame(frameWidth, color.White)
	if err != nil {
		t.Fatalf("AddUniformFrame failed: %v", err)
	}

	// Save
	outputDir := "../../testdata/output"
	os.MkdirAll(outputDir, 0755)
	outputPath := filepath.Join(outputDir, "vips_pipeline_output.jpg")

	err = img.SaveJPEG(outputPath, 92)
	if err != nil {
		t.Fatalf("SaveJPEG failed: %v", err)
	}

	info, _ := os.Stat(outputPath)
	t.Logf("Pipeline: %dx%d → %dx%d → %dx%d (%d bytes)",
		origWidth, origHeight,
		resizedWidth, resizedHeight,
		img.Width(), img.Height(),
		info.Size())
}

func BenchmarkVipsResize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		img, err := LoadVips(testImageVips)
		if err != nil {
			b.Fatalf("LoadVips failed: %v", err)
		}

		err = img.ResizeToFit(1080, 1080, MagicKernelSharp2021)
		if err != nil {
			b.Fatalf("ResizeToFit failed: %v", err)
		}

		img.Close()
	}
}

func BenchmarkVipsFullPipeline(b *testing.B) {
	outputDir := "../../testdata/output"
	os.MkdirAll(outputDir, 0755)

	for i := 0; i < b.N; i++ {
		img, err := LoadVips(testImageVips)
		if err != nil {
			b.Fatalf("LoadVips failed: %v", err)
		}

		err = img.ResizeToFit(1080, 1080, MagicKernelSharp2021)
		if err != nil {
			b.Fatalf("ResizeToFit failed: %v", err)
		}

		err = img.AddUniformFrame(54, color.White)
		if err != nil {
			b.Fatalf("AddUniformFrame failed: %v", err)
		}

		err = img.SaveJPEG(filepath.Join(outputDir, "bench_output.jpg"), 92)
		if err != nil {
			b.Fatalf("SaveJPEG failed: %v", err)
		}

		img.Close()
	}
}

func TestParseColor(t *testing.T) {
	tests := []struct {
		input   string
		r, g, b, a uint8
	}{
		{"#fff", 255, 255, 255, 255},
		{"#000", 0, 0, 0, 255},
		{"#ffffff", 255, 255, 255, 255},
		{"#000000", 0, 0, 0, 255},
		{"#ff0000", 255, 0, 0, 255},
		{"#00ff00", 0, 255, 0, 255},
		{"#0000ff", 0, 0, 255, 255},
		{"white", 255, 255, 255, 255},
		{"black", 0, 0, 0, 255},
		{"red", 255, 0, 0, 255},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			c, err := ParseColor(tc.input)
			if err != nil {
				t.Fatalf("ParseColor(%q) failed: %v", tc.input, err)
			}

			r, g, b, a := c.RGBA()
			// RGBA returns 16-bit values
			if uint8(r>>8) != tc.r || uint8(g>>8) != tc.g || uint8(b>>8) != tc.b || uint8(a>>8) != tc.a {
				t.Errorf("ParseColor(%q) = (%d,%d,%d,%d), expected (%d,%d,%d,%d)",
					tc.input, r>>8, g>>8, b>>8, a>>8, tc.r, tc.g, tc.b, tc.a)
			}
		})
	}
}

func TestParseFilter(t *testing.T) {
	tests := []struct {
		input    string
		expected Filter
	}{
		{"lanczos", Lanczos},
		{"lanczos3", Lanczos},
		{"catmull-rom", CatmullRom},
		{"catmullrom", CatmullRom},
		{"cubic", CatmullRom},
		{"bilinear", Bilinear},
		{"linear", Bilinear},
		{"mks2021", MagicKernelSharp2021},
		{"magic", MagicKernelSharp2021},
		{"magickernel", MagicKernelSharp2021},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			f, err := ParseFilter(tc.input)
			if err != nil {
				t.Fatalf("ParseFilter(%q) failed: %v", tc.input, err)
			}
			if f != tc.expected {
				t.Errorf("ParseFilter(%q) = %v, expected %v", tc.input, f, tc.expected)
			}
		})
	}
}
