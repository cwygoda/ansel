package exiftool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cwygoda/ansel/internal/cull/domain"
	"github.com/cwygoda/ansel/internal/exiftool"
)

func requireExiftool(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not installed")
	}
}

func newTestWriter(t *testing.T) *Writer {
	t.Helper()
	requireExiftool(t)

	session := exiftool.New("")
	t.Cleanup(func() { _ = session.Close() })
	return NewWriter(session)
}

func readTag(t *testing.T, path, tag string) string {
	t.Helper()
	output, err := exec.Command("exiftool", "-n", "-s3", "-"+tag, path).Output()
	if err != nil {
		t.Fatalf("failed to read %s from %s: %v", tag, path, err)
	}
	return strings.TrimSpace(string(output))
}

func readKeywords(t *testing.T, path string) []string {
	t.Helper()
	raw := readTag(t, path, "XMP:Subject")
	if raw == "" {
		return nil
	}

	keywords := strings.Split(raw, ",")
	for i := range keywords {
		keywords[i] = strings.TrimSpace(keywords[i])
	}
	slices.Sort(keywords)
	return keywords
}

func planIn(dir string) domain.SidecarPlan {
	return domain.SidecarPlan{
		ImagePath:   filepath.Join(dir, "DSC_1234.NEF"),
		SidecarPath: filepath.Join(dir, "DSC_1234.xmp"),
		Rating:      5,
		Label:       "green",
		Tags:        []string{domain.TagSharp, domain.TagBestInGroup},
	}
}

func TestWriteCreatesASidecarAndRoundTripsThePlan(t *testing.T) {
	writer := newTestWriter(t)
	plan := planIn(t.TempDir())

	if err := writer.Write(context.Background(), plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := readTag(t, plan.SidecarPath, "XMP:Rating"); got != "5" {
		t.Errorf("XMP:Rating = %q, expected 5", got)
	}
	if got := readTag(t, plan.SidecarPath, "XMP:Label"); got != "green" {
		t.Errorf("XMP:Label = %q, expected green", got)
	}

	expected := []string{domain.TagBestInGroup, domain.TagSharp}
	if got := readKeywords(t, plan.SidecarPath); !slices.Equal(got, expected) {
		t.Errorf("XMP:Subject = %v, expected %v", got, expected)
	}
}

// The reason this adapter exists. `ansel geolocate` writes coordinates into
// the same sidecar, and a cull run afterwards must not cost the photographer
// an evening's positions.
func TestWritePreservesCoordinatesAlreadyInTheSidecar(t *testing.T) {
	writer := newTestWriter(t)
	plan := planIn(t.TempDir())

	located := exec.Command("exiftool", "-n", "-overwrite_original",
		"-XMP:GPSLatitude=52.5048", "-XMP:GPSLongitude=13.2995", plan.SidecarPath)
	if output, err := located.CombinedOutput(); err != nil {
		t.Fatalf("failed to seed the sidecar: %v (%s)", err, output)
	}

	if err := writer.Write(context.Background(), plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := readTag(t, plan.SidecarPath, "XMP:GPSLatitude"); got != "52.5048" {
		t.Errorf("XMP:GPSLatitude = %q, expected the position to survive the rating", got)
	}
	if got := readTag(t, plan.SidecarPath, "XMP:Rating"); got != "5" {
		t.Errorf("XMP:Rating = %q, expected 5", got)
	}
}

// A photographer's own keywords are not this run's to withdraw, but a keyword
// this run wrote last time and no longer stands behind is.
func TestWriteReplacesOnlyItsOwnKeywords(t *testing.T) {
	writer := newTestWriter(t)
	dir := t.TempDir()

	first := planIn(dir)
	first.Tags = []string{domain.TagSharp, domain.TagBestInGroup}
	if err := writer.Write(context.Background(), first); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	keyworded := exec.Command("exiftool", "-overwrite_original",
		"-XMP:Subject+=Iceland", first.SidecarPath)
	if output, err := keyworded.CombinedOutput(); err != nil {
		t.Fatalf("failed to add a keyword: %v (%s)", err, output)
	}

	// The frame is soft this time round and no longer best in its group.
	second := planIn(dir)
	second.Tags = []string{domain.TagSoft}
	if err := writer.Write(context.Background(), second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"Iceland", domain.TagSoft}
	slices.Sort(expected)
	if got := readKeywords(t, second.SidecarPath); !slices.Equal(got, expected) {
		t.Errorf("XMP:Subject = %v, expected %v", got, expected)
	}
}

// Re-running an unchanged shoot must not pile the same keywords up again.
func TestWriteIsIdempotent(t *testing.T) {
	writer := newTestWriter(t)
	plan := planIn(t.TempDir())

	for range 3 {
		if err := writer.Write(context.Background(), plan); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	expected := []string{domain.TagBestInGroup, domain.TagSharp}
	if got := readKeywords(t, plan.SidecarPath); !slices.Equal(got, expected) {
		t.Errorf("XMP:Subject = %v, expected %v", got, expected)
	}
}

// A label this run did not award must not survive from the last one.
func TestWriteClearsAStaleLabel(t *testing.T) {
	writer := newTestWriter(t)
	dir := t.TempDir()

	if err := writer.Write(context.Background(), planIn(dir)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	unlabelled := planIn(dir)
	unlabelled.Label = ""
	if err := writer.Write(context.Background(), unlabelled); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := readTag(t, unlabelled.SidecarPath, "XMP:Label"); got != "" {
		t.Errorf("XMP:Label = %q, expected it to be cleared", got)
	}
}

// exiftool keeps a _original copy by default, which would litter the shoot.
func TestSidecarArgumentsNeverKeepABackupFile(t *testing.T) {
	if !slices.Contains(sidecarArguments(planIn("/shoot")), "-overwrite_original") {
		t.Error("expected -overwrite_original")
	}
}

func TestWriteReportsAFailingTarget(t *testing.T) {
	writer := newTestWriter(t)

	plan := planIn(t.TempDir())
	plan.SidecarPath = filepath.Join(t.TempDir(), "no-such-directory", "DSC_1234.xmp")

	err := writer.Write(context.Background(), plan)
	if err == nil {
		t.Fatal("expected an error for an unwritable target")
	}
	if !strings.Contains(err.Error(), "DSC_1234.xmp") {
		t.Errorf("error %q does not name the target", err)
	}
	if _, statErr := os.Stat(plan.SidecarPath); statErr == nil {
		t.Error("expected no sidecar to have been created")
	}
}
