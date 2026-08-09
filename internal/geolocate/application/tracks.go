package application

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
	"github.com/cwygoda/ansel/internal/geolocate/ports"
)

// dateLayout is the prefix the activity exporter puts on every filename.
const dateLayout = "2006-01-02"

// discoveryMargin widens the filename date filter. A day either side covers
// any timezone on Earth plus a recording that ran past midnight, and it is
// only a coarse filter: the real judgement is made against decoded timestamps.
const discoveryMargin = 24 * time.Hour

// resolveTracks turns the run's track arguments into decoded tracks.
func (l *Locator) resolveTracks(ctx context.Context, photos []domain.Photo) ([]domain.Track, []domain.Failure, error) {
	paths, err := l.trackPaths(photos)
	if err != nil {
		return nil, nil, err
	}

	var tracks []domain.Track
	var failures []domain.Failure
	for _, path := range paths {
		decoder, ok := l.decoderFor(path)
		if !ok {
			failures = append(failures, domain.Failure{
				Path: path, Stage: "track", Err: "no decoder handles this format",
			})
			continue
		}
		track, err := decoder.Decode(ctx, path)
		if err != nil {
			failures = append(failures, domain.Failure{Path: path, Stage: "track", Err: err.Error()})
			continue
		}
		if len(track.Points) > 0 {
			tracks = append(tracks, track)
		}
	}
	return tracks, failures, nil
}

func (l *Locator) decoderFor(path string) (ports.TrackDecoder, bool) {
	for _, candidate := range l.Decoders {
		if candidate.Supports(path) {
			return candidate, true
		}
	}
	return nil, false
}

// trackPaths resolves the explicit arguments, or searches the configured
// directory when none were given.
func (l *Locator) trackPaths(photos []domain.Photo) ([]string, error) {
	if len(l.TrackPaths) > 0 {
		return l.expandTrackArguments()
	}
	if l.Config.TracksDir == "" {
		return nil, fmt.Errorf("no track data given: pass --track, " +
			"or set tracks_dir in the [geolocate] section of ~/.ansel/config.toml " +
			"to search a directory automatically")
	}
	return l.discoverTracks(photos)
}

// expandTrackArguments accepts a file, a glob or a directory for each --track.
func (l *Locator) expandTrackArguments() ([]string, error) {
	var paths []string

	for _, argument := range l.TrackPaths {
		matches, err := l.expandOne(argument)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no track files matched %s", argument)
		}
		paths = append(paths, matches...)
	}
	return deduplicate(paths), nil
}

// expandOne resolves one argument.
//
// A directory or a glob is a request for "the track files in here", so
// anything no decoder claims is quietly passed over — activity exports
// routinely sit beside JSON metadata of the same name. A single named file is
// different: the operator meant that file, so an unsupported one is reported
// rather than silently dropped.
func (l *Locator) expandOne(argument string) ([]string, error) {
	info, err := os.Stat(argument)
	switch {
	case err == nil && info.IsDir():
		return l.supportedUnder(argument)
	case err == nil:
		return []string{argument}, nil
	}

	matches, err := filepath.Glob(argument)
	if err != nil {
		return nil, fmt.Errorf("failed to expand %s: %w", argument, err)
	}
	return l.onlySupported(matches), nil
}

// onlySupported keeps the paths some decoder is willing to read.
func (l *Locator) onlySupported(paths []string) []string {
	supported := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := l.decoderFor(path); ok {
			supported = append(supported, path)
		}
	}
	return supported
}

// discoverTracks searches the configured directory, narrowing by the dates in
// the filenames before anything is opened. That directory holds years of
// recordings, and decoding all of them to find one afternoon would be absurd.
func (l *Locator) discoverTracks(photos []domain.Photo) ([]string, error) {
	candidates, err := l.supportedUnder(l.Config.TracksDir)
	if err != nil {
		return nil, err
	}

	earliest, latest, ok := photoDateRange(photos)
	if !ok {
		return nil, fmt.Errorf("no photograph carries a usable timestamp, " +
			"so there is nothing to search for")
	}
	// The filter compares whole days, because a filename states the day a
	// recording began and nothing finer. Widening a timestamp by 24 hours
	// instead would exclude a file named for the previous morning even when
	// its recording ran into the evening the shoot began.
	earliest, latest = startOfDay(earliest).Add(-discoveryMargin), startOfDay(latest).Add(discoveryMargin)

	var selected []string
	for _, path := range candidates {
		date, dated := dateInName(path)
		// A file whose name says nothing about its date is kept: it is
		// cheaper to decode a few extras than to miss the right one.
		if !dated || (!date.Before(earliest) && !date.After(latest)) {
			selected = append(selected, path)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no track in %s covers %s to %s",
			l.Config.TracksDir, earliest.Format(dateLayout), latest.Format(dateLayout))
	}
	return selected, nil
}

// supportedUnder collects every file below root that some decoder claims.
func (l *Locator) supportedUnder(root string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return skipHidden(path, entry)
		}
		if _, ok := l.decoderFor(path); ok {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search %s: %w", root, err)
	}

	sort.Strings(paths)
	return paths, nil
}

// startOfDay drops the time of day, putting a reading on the same footing as
// the dates read out of filenames.
func startOfDay(moment time.Time) time.Time {
	return time.Date(moment.Year(), moment.Month(), moment.Day(), 0, 0, 0, 0, time.UTC)
}

// dateInName reads the leading YYYY-MM-DD an exporter puts on a filename.
func dateInName(path string) (time.Time, bool) {
	name := filepath.Base(path)
	if len(name) < len(dateLayout) {
		return time.Time{}, false
	}
	date, err := time.ParseInLocation(dateLayout, name[:len(dateLayout)], time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return date, true
}

// photoDateRange reports the span of the shoot by the cameras' own readings.
// Zones are ignored here on purpose: this only feeds a filter already widened
// by a day at each end.
func photoDateRange(photos []domain.Photo) (earliest, latest time.Time, ok bool) {
	for _, photo := range photos {
		if photo.Clock.IsZero() {
			continue
		}
		wall := photo.Clock.Wall
		if !ok || wall.Before(earliest) {
			earliest = wall
		}
		if !ok || wall.After(latest) {
			latest = wall
		}
		ok = true
	}
	return earliest, latest, ok
}

func deduplicate(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		unique = append(unique, path)
	}
	return unique
}

// trackOffset reports the UTC offset the recordings agree on, if any states
// one. Tracks selected for a run come from the same outing, so the first
// answer is the run's answer.
func trackOffset(tracks []domain.Track) (time.Duration, bool) {
	for _, track := range tracks {
		if track.HasUTCOffset {
			return track.UTCOffset, true
		}
	}
	return 0, false
}
