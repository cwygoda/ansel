// Package kmlpreview renders a geolocate result as a small map preview.
//
// It is an adapter because it performs file I/O, image decoding and optional
// exiftool preview extraction. The geolocate domain stays as plain positions
// and write plans.
package kmlpreview

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"hash/fnv"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cwygoda/ansel/internal/exiftool"
	"github.com/cwygoda/ansel/internal/geolocate/domain"
	imglib "github.com/cwygoda/ansel/internal/image"
)

const (
	FormatKML = "kml"
	FormatKMZ = "kmz"

	defaultName      = "geolocate-preview"
	thumbnailMaxEdge = 320
	thumbnailQuality = 82
)

var (
	previewTags    = []string{"JpgFromRaw", "PreviewImage", "ThumbnailImage"}
	jpegExtensions = map[string]bool{".jpg": true, ".jpeg": true}
)

// Writer creates KML/KMZ preview maps for located photographs.
type Writer struct {
	session *exiftool.Session
}

// Report describes the preview artifact that was written.
type Report struct {
	Path              string
	Format            string
	Placemarks        int
	Thumbnails        int
	ThumbnailFailures []Failure
}

// Failure is one non-fatal thumbnail-generation failure.
type Failure struct {
	Path string
	Err  string
}

type placemark struct {
	plan      domain.WritePlan
	thumbHref string
}

type thumbnail struct {
	href string
	data []byte
}

// New returns a preview writer using the shared exiftool session for RAW
// embedded previews.
func New(session *exiftool.Session) *Writer {
	return &Writer{session: session}
}

// Write writes root/geolocate-preview.<format>. KML uses a sibling thumbnail
// directory; KMZ embeds doc.kml and the thumbnails in one archive.
func (w *Writer) Write(ctx context.Context, result domain.Result, format string) (Report, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = FormatKMZ
	}
	if format != FormatKML && format != FormatKMZ {
		return Report{}, fmt.Errorf("unknown preview map format %q (use kml or kmz)", format)
	}

	path := filepath.Join(result.Root, defaultName+"."+format)
	prepared := w.prepare(ctx, result.Plans)
	report := Report{Path: path, Format: format, Placemarks: len(prepared.placemarks), Thumbnails: len(prepared.thumbnails), ThumbnailFailures: prepared.failures}

	switch format {
	case FormatKML:
		if err := writeKML(path, prepared.placemarks); err != nil {
			return report, err
		}
		if err := writeThumbnailFiles(filepath.Join(result.Root, defaultName+"-thumbnails"), prepared.thumbnails); err != nil {
			return report, err
		}
	case FormatKMZ:
		if err := writeKMZ(path, prepared.placemarks, prepared.thumbnails); err != nil {
			return report, err
		}
	}
	return report, nil
}

type preparedMap struct {
	placemarks []placemark
	thumbnails []thumbnail
	failures   []Failure
}

func (w *Writer) prepare(ctx context.Context, plans []domain.WritePlan) preparedMap {
	prepared := preparedMap{
		placemarks: make([]placemark, 0, len(plans)),
	}

	for _, plan := range plans {
		if err := ctx.Err(); err != nil {
			prepared.failures = append(prepared.failures, Failure{Path: plan.PhotoPath, Err: err.Error()})
			prepared.placemarks = append(prepared.placemarks, placemark{plan: plan})
			continue
		}

		href := thumbnailHref(plan.PhotoPath)
		data, err := w.thumbnail(ctx, plan.PhotoPath)
		if err != nil {
			prepared.failures = append(prepared.failures, Failure{Path: plan.PhotoPath, Err: err.Error()})
			prepared.placemarks = append(prepared.placemarks, placemark{plan: plan})
			continue
		}

		prepared.thumbnails = append(prepared.thumbnails, thumbnail{href: href, data: data})
		prepared.placemarks = append(prepared.placemarks, placemark{plan: plan, thumbHref: href})
	}
	return prepared
}

func (w *Writer) thumbnail(ctx context.Context, path string) ([]byte, error) {
	data, err := w.previewBytes(ctx, path)
	if err != nil {
		return nil, err
	}

	img, err := imglib.LoadVipsBuffer(data)
	if err != nil {
		return nil, err
	}
	defer img.Close()

	if img.Width() > thumbnailMaxEdge || img.Height() > thumbnailMaxEdge {
		if err := img.ResizeToFit(thumbnailMaxEdge, thumbnailMaxEdge, imglib.Bilinear); err != nil {
			return nil, err
		}
	}
	return exportJPEG(img, thumbnailQuality)
}

func (w *Writer) previewBytes(ctx context.Context, path string) ([]byte, error) {
	if jpegExtensions[strings.ToLower(filepath.Ext(path))] {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}
		return data, nil
	}
	if w.session == nil {
		return nil, fmt.Errorf("no exiftool session available for embedded previews")
	}

	var lastErr error
	for _, tag := range previewTags {
		data, err := w.session.RunBinary(ctx, "-b", "-"+tag, path)
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

func exportJPEG(img *imglib.VipsImage, quality int) ([]byte, error) {
	tmp, err := os.CreateTemp("", "ansel-geolocate-thumb-*.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary thumbnail: %w", err)
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temporary thumbnail: %w", err)
	}
	defer os.Remove(path)

	if err := img.SaveJPEG(path, quality); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read temporary thumbnail: %w", err)
	}
	return data, nil
}

func writeKML(path string, placemarks []placemark) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create preview map directory: %w", err)
	}
	return os.WriteFile(path, []byte(kmlDocument(placemarks)), 0o644)
}

func writeThumbnailFiles(root string, thumbnails []thumbnail) error {
	if len(thumbnails) == 0 {
		return nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("failed to create thumbnail directory: %w", err)
	}
	for _, thumb := range thumbnails {
		path := filepath.Join(root, strings.TrimPrefix(thumb.href, defaultName+"-thumbnails/"))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("failed to create thumbnail directory: %w", err)
		}
		if err := os.WriteFile(path, thumb.data, 0o644); err != nil {
			return fmt.Errorf("failed to write thumbnail %s: %w", path, err)
		}
	}
	return nil
}

func writeKMZ(path string, placemarks []placemark, thumbnails []thumbnail) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create preview map directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	defer file.Close()

	archive := zip.NewWriter(file)
	if err := addZipFile(archive, "doc.kml", []byte(kmlDocument(placemarks))); err != nil {
		_ = archive.Close()
		return err
	}
	for _, thumb := range thumbnails {
		if err := addZipFile(archive, thumb.href, thumb.data); err != nil {
			_ = archive.Close()
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("failed to finish %s: %w", path, err)
	}
	return nil
}

func addZipFile(archive *zip.Writer, name string, data []byte) error {
	entry, err := archive.Create(name)
	if err != nil {
		return fmt.Errorf("failed to add %s to preview map: %w", name, err)
	}
	if _, err := entry.Write(data); err != nil {
		return fmt.Errorf("failed to write %s to preview map: %w", name, err)
	}
	return nil
}

func kmlDocument(placemarks []placemark) string {
	var out strings.Builder
	out.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	out.WriteString("<kml xmlns=\"http://www.opengis.net/kml/2.2\">\n")
	out.WriteString("<Document>\n")
	out.WriteString("  <name>Geolocate preview</name>\n")
	out.WriteString("  <Style id=\"photo\"><IconStyle><Icon><href>http://maps.google.com/mapfiles/kml/pal4/icon38.png</href></Icon></IconStyle></Style>\n")

	for _, mark := range placemarks {
		writePlacemark(&out, mark)
	}

	out.WriteString("</Document>\n")
	out.WriteString("</kml>\n")
	return out.String()
}

func writePlacemark(out *strings.Builder, mark placemark) {
	plan := mark.plan
	out.WriteString("  <Placemark>\n")
	fmt.Fprintf(out, "    <name>%s</name>\n", xmlEscape(filepath.Base(plan.PhotoPath)))
	out.WriteString("    <styleUrl>#photo</styleUrl>\n")
	fmt.Fprintf(out, "    <description>%s</description>\n", xmlEscape(description(mark)))
	out.WriteString("    <Point>\n")
	fmt.Fprintf(out, "      <coordinates>%s</coordinates>\n", coordinates(plan.Fix.Position))
	out.WriteString("    </Point>\n")
	out.WriteString("  </Placemark>\n")
}

func description(mark placemark) string {
	plan := mark.plan
	var out strings.Builder
	if mark.thumbHref != "" {
		fmt.Fprintf(&out, `<img src="%s" width="%d"/><br/>`, html.EscapeString(mark.thumbHref), thumbnailMaxEdge)
	}
	fmt.Fprintf(&out, "<b>Photo:</b> %s<br/>", html.EscapeString(plan.PhotoPath))
	fmt.Fprintf(&out, "<b>Method:</b> %s<br/>", html.EscapeString(string(plan.Fix.Method)))
	fmt.Fprintf(&out, "<b>Gap:</b> %s<br/>", html.EscapeString(plan.Fix.Gap.Round(time.Second).String()))
	fmt.Fprintf(&out, "<b>Track time:</b> %s<br/>", plan.Fix.Position.Time.UTC().Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(&out, "<b>Source:</b> %s", html.EscapeString(filepath.Base(plan.Fix.Source)))
	return out.String()
}

func coordinates(point domain.TrackPoint) string {
	if point.HasElevation {
		return fmt.Sprintf("%.8f,%.8f,%.2f", point.Longitude, point.Latitude, point.Elevation)
	}
	return fmt.Sprintf("%.8f,%.8f", point.Longitude, point.Latitude)
}

func thumbnailHref(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = sanitizeName(base)
	if base == "" {
		base = "photo"
	}
	return fmt.Sprintf("%s-thumbnails/%s-%08x.jpg", defaultName, base, stableHash(path))
}

func sanitizeName(name string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
		case r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '-' || r == '_':
			out.WriteRune(r)
		default:
			out.WriteByte('-')
		}
	}
	return strings.Trim(out.String(), "-")
}

func stableHash(value string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(value))
	return hash.Sum32()
}

func xmlEscape(value string) string {
	var out bytes.Buffer
	if err := xml.EscapeText(&out, []byte(value)); err != nil {
		return value
	}
	return out.String()
}
