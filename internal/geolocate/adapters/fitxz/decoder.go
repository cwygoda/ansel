// Package fitxz adapts xz-compressed Garmin FIT activity files to the
// geolocate ports. It is an adapter: nothing outside this package knows that
// a track was ever a FIT file, or that it arrived compressed.
package fitxz

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/ulikunitz/xz"
)

// extension is the double suffix these files carry. Matching the whole thing
// rather than just ".xz" keeps this decoder from claiming, say, a .gpx.xz
// that a future adapter will want.
const extension = ".fit.xz"

// Decoder reads activity tracks from xz-compressed FIT files.
type Decoder struct{}

// New returns a decoder for xz-compressed Garmin FIT activities.
func New() *Decoder { return &Decoder{} }

// Supports reports whether the path looks like an xz-compressed FIT file.
func (d *Decoder) Supports(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), extension)
}

// Decode reads one activity into a track.
func (d *Decoder) Decode(ctx context.Context, path string) (domain.Track, error) {
	activity, err := readActivity(ctx, path)
	if err != nil {
		return domain.Track{}, err
	}

	track := domain.Track{Source: path, Points: pointsFrom(activity.Records)}
	track.UTCOffset, track.HasUTCOffset = offsetFrom(activity.Activity)

	// Devices write records in order, but a merged or repaired file need not
	// be, and the matcher binary-searches on this.
	sort.Slice(track.Points, func(i, j int) bool {
		return track.Points[i].Time.Before(track.Points[j].Time)
	})
	return track, nil
}

func readActivity(ctx context.Context, path string) (*filedef.Activity, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open track %s: %w", path, err)
	}
	defer file.Close()

	compressed, err := xz.NewReader(bufio.NewReader(file))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress %s: %w", path, err)
	}

	listener := filedef.NewListener()
	defer listener.Close()

	if _, err := decoder.New(bufio.NewReader(compressed),
		decoder.WithMesgListener(listener),
		decoder.WithBroadcastOnly(),
	).DecodeWithContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to parse fit file %s: %w", path, err)
	}

	activity, ok := listener.File().(*filedef.Activity)
	if !ok {
		return nil, fmt.Errorf("%s is not a fit activity file", path)
	}
	return activity, nil
}

// pointsFrom converts record messages, dropping those without a usable
// position. A recording routinely opens with a handful of records logged
// before the GPS has a lock, and those carry the format's invalid marker
// rather than a coordinate.
func pointsFrom(records []*mesgdef.Record) []domain.TrackPoint {
	points := make([]domain.TrackPoint, 0, len(records))
	for _, record := range records {
		latitude := record.PositionLatDegrees()
		longitude := record.PositionLongDegrees()
		if record.Timestamp.IsZero() || math.IsNaN(latitude) || math.IsNaN(longitude) {
			continue
		}

		elevation, hasElevation := elevationOf(record)
		points = append(points, domain.TrackPoint{
			Time:         record.Timestamp.UTC(),
			Latitude:     latitude,
			Longitude:    longitude,
			Elevation:    elevation,
			HasElevation: hasElevation,
		})
	}
	return points
}

// elevationOf prefers the enhanced altitude field, which carries the wider
// range and finer resolution newer devices record.
func elevationOf(record *mesgdef.Record) (float64, bool) {
	if enhanced := record.EnhancedAltitudeScaled(); !math.IsNaN(enhanced) {
		return enhanced, true
	}
	if altitude := record.AltitudeScaled(); !math.IsNaN(altitude) {
		return altitude, true
	}
	return 0, false
}

// offsetFrom recovers the UTC offset the device was set to.
//
// FIT records each activity twice over: once in UTC and once as the local
// clock reading, and the difference between them is the offset that was in
// force where the activity happened. That is precisely what is needed to
// interpret a camera's unzoned EXIF timestamp, and it means the common case
// needs no flag from the operator at all.
func offsetFrom(activity *mesgdef.Activity) (time.Duration, bool) {
	if activity == nil || activity.Timestamp.IsZero() || activity.LocalTimestamp.IsZero() {
		return 0, false
	}
	// Zones are whole minutes; the seconds here are recording jitter.
	return activity.LocalTimestamp.Sub(activity.Timestamp).Round(time.Minute), true
}
