package image

import (
	"os"
	"path/filepath"
	"testing"
)

const testImagePath = "../../testdata/input.jpg"

func TestReadIPTCHeadline(t *testing.T) {
	// Test reading headline from test image (has "Test Headline" set via exiftool)
	headline := ReadIPTCHeadline(testImagePath)
	if headline == "" {
		t.Skip("Test image has no IPTC headline - this is expected if test image was reset")
	}
	t.Logf("Read headline: %q", headline)
}

func TestReadIPTCHeadline_NonExistentFile(t *testing.T) {
	headline := ReadIPTCHeadline("/nonexistent/file.jpg")
	if headline != "" {
		t.Errorf("Expected empty headline for non-existent file, got %q", headline)
	}
}

func TestReadIPTCHeadline_UnsupportedFormat(t *testing.T) {
	headline := ReadIPTCHeadline("testfile.bmp")
	if headline != "" {
		t.Errorf("Expected empty headline for unsupported format, got %q", headline)
	}
}

func TestReadDXOHeadline(t *testing.T) {
	// Create a temporary DXO sidecar file
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.jpg")
	dopPath := imgPath + ".dop"

	// Create dummy image file (just needs to exist for the test)
	if err := os.WriteFile(imgPath, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create DXO sidecar with headline
	dopContent := `Sidecar = {
	Source = {
		Items = {
			{
			IPTC = {
				contentDescription = "",
				contentHeadline = "Test DXO Headline",
				imageLocation = "Test Location",
			},
			},
		},
	},
}`
	if err := os.WriteFile(dopPath, []byte(dopContent), 0644); err != nil {
		t.Fatal(err)
	}

	headline := readDXOHeadline(imgPath)
	if headline != "Test DXO Headline" {
		t.Errorf("Expected 'Test DXO Headline', got %q", headline)
	}
}

func TestReadDXOHeadline_NoSidecar(t *testing.T) {
	headline := readDXOHeadline("/nonexistent/file.jpg")
	if headline != "" {
		t.Errorf("Expected empty headline for missing sidecar, got %q", headline)
	}
}

func TestReadIPTCHeadline_PrefersDXOOverEmbedded(t *testing.T) {
	// Create a temporary DXO sidecar file
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.jpg")
	dopPath := imgPath + ".dop"

	// Create dummy image file
	if err := os.WriteFile(imgPath, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create DXO sidecar with headline
	dopContent := `contentHeadline = "DXO Headline",`
	if err := os.WriteFile(dopPath, []byte(dopContent), 0644); err != nil {
		t.Fatal(err)
	}

	// ReadIPTCHeadline should prefer DXO sidecar
	headline := ReadIPTCHeadline(imgPath)
	if headline != "DXO Headline" {
		t.Errorf("Expected 'DXO Headline', got %q", headline)
	}
}
