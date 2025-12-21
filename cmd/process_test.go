package cmd

import (
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		input  string
		width  int
		height int
		hasErr bool
	}{
		// Presets
		{"ig-post", 1080, 1080, false},
		{"ig-story", 1080, 1920, false},
		{"ig-portrait", 1080, 1350, false},
		{"fb-post", 1200, 630, false},
		{"yt-thumb", 1280, 720, false},
		{"4x6", 1800, 1200, false},

		// WxH format
		{"1920x1080", 1920, 1080, false},
		{"800x600", 800, 600, false},
		{"100x100", 100, 100, false},

		// W,H format
		{"1920,1080", 1920, 1080, false},
		{"800,600", 800, 600, false},

		// Case insensitive presets
		{"IG-POST", 1080, 1080, false},
		{"Ig-Story", 1080, 1920, false},

		// Invalid
		{"invalid", 0, 0, true},
		{"", 0, 0, true},
		{"1920", 0, 0, true},
		{"axb", 0, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			w, h, err := parseSize(tc.input)

			if tc.hasErr {
				if err == nil {
					t.Errorf("parseSize(%q) expected error, got nil", tc.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseSize(%q) unexpected error: %v", tc.input, err)
			}

			if w != tc.width || h != tc.height {
				t.Errorf("parseSize(%q) = (%d, %d), expected (%d, %d)",
					tc.input, w, h, tc.width, tc.height)
			}
		})
	}
}

func TestSizePresets(t *testing.T) {
	// Verify all presets have valid dimensions
	for name, size := range sizePresets {
		if size[0] <= 0 || size[1] <= 0 {
			t.Errorf("Preset %q has invalid dimensions: %dx%d", name, size[0], size[1])
		}
	}

	// Verify expected presets exist
	expectedPresets := []string{
		"ig-post", "ig-story", "ig-portrait", "ig-landscape", "ig-reel",
		"fb-post", "fb-cover",
		"x-post", "x-header",
		"yt-thumb",
		"li-post", "li-cover",
		"4x6", "5x7", "8x10",
	}

	for _, name := range expectedPresets {
		if _, ok := sizePresets[name]; !ok {
			t.Errorf("Expected preset %q not found", name)
		}
	}
}

func TestGenerateOutputPath(t *testing.T) {
	tests := []struct {
		input    string
		outDir   string
		expected string
	}{
		// No version suffix, no outDir
		{"photo.jpg", "", "photo_v0.jpg"},
		{"image.png", "", "image_v0.jpg"},
		{"test.tiff", "", "test_v0.jpg"},

		// With version suffix - increment
		{"photo_v0.jpg", "", "photo_v1.jpg"},
		{"photo_v1.jpg", "", "photo_v2.jpg"},
		{"photo_v9.jpg", "", "photo_v10.jpg"},
		{"photo_v99.jpg", "", "photo_v100.jpg"},

		// With input directory, no outDir
		{"/path/to/photo.jpg", "", "/path/to/photo_v0.jpg"},
		{"/path/to/photo_v0.jpg", "", "/path/to/photo_v1.jpg"},
		{"./photo.jpg", "", "photo_v0.jpg"},

		// Edge cases
		{"my_photo.jpg", "", "my_photo_v0.jpg"},
		{"my_photo_v0.jpg", "", "my_photo_v1.jpg"},
		{"photo_v0_edited.jpg", "", "photo_v0_edited_v0.jpg"},

		// With outDir specified
		{"photo.jpg", "/output", "/output/photo_v0.jpg"},
		{"/path/to/photo.jpg", "/output", "/output/photo_v0.jpg"},
		{"photo_v0.jpg", "/output", "/output/photo_v1.jpg"},
		{"photo.jpg", "out", "out/photo_v0.jpg"},
	}

	for _, tc := range tests {
		name := tc.input
		if tc.outDir != "" {
			name += " -> " + tc.outDir
		}
		t.Run(name, func(t *testing.T) {
			result := generateOutputPath(tc.input, tc.outDir)
			if result != tc.expected {
				t.Errorf("generateOutputPath(%q, %q) = %q, expected %q",
					tc.input, tc.outDir, result, tc.expected)
			}
		})
	}
}
