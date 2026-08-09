package application

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
	"github.com/cwygoda/ansel/internal/geolocate/matching"
	"github.com/cwygoda/ansel/internal/geolocate/ports"
)

// Locator is the geolocate orchestrator. Its collaborators are interfaces, so
// nothing here knows that tracks are FIT files or that metadata comes from a
// subprocess.
type Locator struct {
	Metadata ports.MetadataReader
	Writer   ports.MetadataWriter
	Decoders []ports.TrackDecoder

	Config   Config
	Matching matching.Options

	// Zone is the operator's fallback answer for cameras that recorded no
	// offset and tracks that state none.
	Zone Zone

	// Drift is how far the camera clock runs ahead of true time. It shifts
	// both the track lookup and, when writing, the photograph's own
	// timestamps — one value, so the two can never disagree.
	Drift time.Duration

	// TrackPaths are the explicit --track arguments. Empty means search the
	// configured directory.
	TrackPaths []string

	Write   bool
	InPlace bool
	Force   bool
}

// Locate resolves every photograph under root against the run's tracks.
func (l *Locator) Locate(ctx context.Context, root string) (domain.Result, error) {
	result := domain.Result{Root: root}

	photos, err := l.readPhotos(ctx, root)
	if err != nil {
		return result, err
	}
	result.Discovered = len(photos)
	if len(photos) == 0 {
		return result, nil
	}

	tracks, failures, err := l.resolveTracks(ctx, photos)
	if err != nil {
		return result, err
	}
	result.Failures = append(result.Failures, failures...)
	result.Tracks = sourcesOf(tracks)
	if len(tracks) == 0 {
		return result, fmt.Errorf("no usable track data was found")
	}

	l.plan(&result, photos, tracks)

	if err := l.markExisting(ctx, &result); err != nil {
		return result, err
	}
	if l.Write {
		if err := l.writePlans(ctx, &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

// readPhotos discovers photographs and reads their capture clocks.
func (l *Locator) readPhotos(ctx context.Context, root string) ([]domain.Photo, error) {
	paths, err := discoverPhotos(root, l.Config.IncludeExtensions)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}

	byPath, err := l.Metadata.Read(ctx, paths)
	if err != nil {
		return nil, err
	}

	// Iterate the sorted paths rather than the map, so the report is stable.
	photos := make([]domain.Photo, 0, len(paths))
	for _, path := range paths {
		photo, ok := byPath[path]
		if !ok {
			photo = domain.Photo{Path: path}
		}
		photos = append(photos, photo)
	}
	return photos, nil
}

// plan resolves each photograph to a position, recording either an intended
// write or the reason there is none.
func (l *Locator) plan(result *domain.Result, photos []domain.Photo, tracks []domain.Track) {
	offset, trackKnows := trackOffset(tracks)

	for _, photo := range photos {
		if photo.Clock.IsZero() {
			result.Unlocated = append(result.Unlocated, domain.Unlocated{
				PhotoPath: photo.Path,
				Reason:    "no capture timestamp in the file",
			})
			continue
		}

		instant, zoneOffset, err := l.instantOf(photo, offset, trackKnows)
		if err != nil {
			result.Unlocated = append(result.Unlocated, domain.Unlocated{
				PhotoPath: photo.Path, Reason: err.Error(),
			})
			continue
		}

		located := matching.Locate(tracks, instant, l.Matching)
		if !located.Found {
			result.Unlocated = append(result.Unlocated, domain.Unlocated{
				PhotoPath: photo.Path, At: instant, Reason: located.Reason,
			})
			continue
		}
		result.Plans = append(result.Plans, l.planFor(photo, located.Fix, zoneOffset))
	}
}

// instantOf converts a camera's wall-clock reading into the true instant the
// shutter fired, correcting for the camera's zone and for clock drift. The
// zone offset is returned alongside, because the corrected timestamp has to be
// written back in that same zone and recomputing it from the drifted instant
// would fold the drift in twice.
func (l *Locator) instantOf(photo domain.Photo, trackOffset time.Duration, trackKnows bool) (time.Time, time.Duration, error) {
	zoneOffset, err := resolveOffset(photo.Clock, trackOffset, trackKnows, l.Zone)
	if err != nil {
		return time.Time{}, 0, err
	}
	return photo.Clock.Instant(zoneOffset).Add(-l.Drift), zoneOffset, nil
}

// planFor describes the write a located photograph calls for.
//
// The corrected timestamp is the camera's own reading moved by the drift, so
// it stays in the zone the camera was set to. Rewriting it in UTC would place
// the position correctly and leave every timestamp in the shoot looking wrong.
func (l *Locator) planFor(photo domain.Photo, fix domain.Fix, zoneOffset time.Duration) domain.WritePlan {
	plan := domain.WritePlan{
		PhotoPath: photo.Path,
		Target:    l.targetFor(photo.Path),
		InPlace:   l.InPlace,
		Fix:       fix,
	}

	if l.Drift != 0 {
		plan.WriteTime = true
		plan.CorrectedWall = photo.Clock.Wall.Add(-l.Drift)
		plan.CorrectedOffset = zoneOffset
	}
	return plan
}

// targetFor decides what actually gets written.
func (l *Locator) targetFor(photoPath string) string {
	if l.InPlace {
		return photoPath
	}
	return SidecarPathFor(photoPath)
}

// SidecarPathFor returns the XMP sidecar beside a photograph. The extension is
// replaced rather than appended, matching what `ansel cull` writes and what
// Lightroom and Capture One expect to find.
func SidecarPathFor(photoPath string) string {
	extension := filepath.Ext(photoPath)
	return strings.TrimSuffix(photoPath, extension) + ".xmp"
}

// markExisting flags plans whose target already carries a position, and skips
// them unless replacement was forced. Coordinates already on disk are user
// data: someone may have placed them by hand.
func (l *Locator) markExisting(ctx context.Context, result *domain.Result) error {
	targets := make([]string, 0, len(result.Plans))
	for _, plan := range result.Plans {
		targets = append(targets, plan.Target)
	}

	located, err := l.Metadata.HasCoordinates(ctx, targets)
	if err != nil {
		return err
	}

	for i := range result.Plans {
		plan := &result.Plans[i]
		plan.Existing = located[plan.Target]
		if plan.Existing && !l.Force {
			plan.Skipped = "already has coordinates"
		}
	}
	return nil
}

// writePlans records the positions and tallies what actually landed. Plans
// already marked skipped — because coordinates were there and --force was not
// given — are passed through untouched and the writer leaves them alone.
func (l *Locator) writePlans(ctx context.Context, result *domain.Result) error {
	if err := l.Writer.Write(ctx, result.Plans); err != nil {
		return err
	}

	for _, plan := range result.Plans {
		switch {
		case plan.Written:
			result.Written++
		case plan.Existing && !l.Force:
			// Deliberately left alone, not a failure.
		case plan.Skipped != "":
			result.Failures = append(result.Failures, domain.Failure{
				Path: plan.Target, Stage: "write", Err: plan.Skipped,
			})
		}
	}
	return nil
}

func sourcesOf(tracks []domain.Track) []string {
	sources := make([]string, 0, len(tracks))
	for _, track := range tracks {
		sources = append(sources, track.Source)
	}
	return sources
}
