// Package matching resolves a photograph's instant to a position on a track.
// It is pure policy: no file, no clock and no camera appears here, which is
// what lets every rule below be tested against a handful of literal points.
package matching

import (
	"fmt"
	"sort"
	"time"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

// Options are the limits a run places on how far a position may be inferred.
type Options struct {
	// MaxGap is the widest spacing between two recorded points that may be
	// interpolated across. Zero means no limit.
	MaxGap time.Duration

	// Buffer is how far outside a recording a photograph may still be placed,
	// at its first or last point. Zero disables clamping entirely.
	Buffer time.Duration
}

// Result is a located position, or the reason there is none.
type Result struct {
	Fix   domain.Fix
	Found bool

	// Reason explains a miss in terms the operator can act on, naming the
	// limit that rejected the candidate rather than just reporting failure.
	Reason string
}

// Locate resolves an instant against every track and returns the best fix.
//
// Tracks are searched one at a time and never merged. Concatenating their
// points would let a photograph taken between two recordings be placed
// halfway along the line joining the end of one to the start of the next —
// a position on no route that was ever ridden.
func Locate(tracks []domain.Track, instant time.Time, opts Options) Result {
	var best domain.Fix
	var found bool
	var closest miss

	for _, track := range tracks {
		if len(track.Points) == 0 {
			continue
		}
		candidate, reject := locateIn(track, instant, opts)
		if reject.kind != "" {
			closest = closest.nearer(reject)
			continue
		}
		if !found || preferable(candidate, best) {
			best, found = candidate, true
		}
	}

	if !found {
		return Result{Reason: closest.describe(instant, opts)}
	}
	return Result{Fix: best, Found: true}
}

// preferable ranks one fix above another: a more direct method first, and
// among equals the one resting on less inference.
func preferable(candidate, incumbent domain.Fix) bool {
	if rank(candidate.Method) != rank(incumbent.Method) {
		return rank(candidate.Method) < rank(incumbent.Method)
	}
	return candidate.Gap < incumbent.Gap
}

func rank(method domain.Method) int {
	switch method {
	case domain.MethodExact:
		return 0
	case domain.MethodInterpolated:
		return 1
	default:
		return 2
	}
}

// locateIn resolves an instant within a single non-empty track. Exactly one
// of the returned values is meaningful: a zero miss means the fix is good.
func locateIn(track domain.Track, instant time.Time, opts Options) (domain.Fix, miss) {
	next := sort.Search(len(track.Points), func(i int) bool {
		return !track.Points[i].Time.Before(instant)
	})

	if next < len(track.Points) && track.Points[next].Time.Equal(instant) {
		return fixAt(track, track.Points[next], domain.MethodExact, 0), miss{}
	}
	if next == 0 {
		return clamp(track, instant, opts, domain.MethodClampedStart)
	}
	if next == len(track.Points) {
		return clamp(track, instant, opts, domain.MethodClampedEnd)
	}
	return interpolate(track, instant, opts, next)
}

// interpolate places the instant between the two points bracketing it,
// provided they are close enough together to justify inferring a line.
func interpolate(track domain.Track, instant time.Time, opts Options, next int) (domain.Fix, miss) {
	from, to := track.Points[next-1], track.Points[next]
	spacing := to.Time.Sub(from.Time)

	if opts.MaxGap > 0 && spacing > opts.MaxGap {
		return domain.Fix{}, miss{kind: missGap, amount: spacing}
	}
	return fixAt(track, domain.Interpolate(from, to, instant), domain.MethodInterpolated, spacing), miss{}
}

// clamp places an instant lying outside the recording at its nearest end,
// which is right for the frame taken while still unclipping at the trailhead
// but wrong once the distance grows, hence the buffer.
func clamp(track domain.Track, instant time.Time, opts Options, method domain.Method) (domain.Fix, miss) {
	edge := track.Points[0]
	distance := edge.Time.Sub(instant)
	if method == domain.MethodClampedEnd {
		edge = track.Points[len(track.Points)-1]
		distance = instant.Sub(edge.Time)
	}

	if opts.Buffer <= 0 || distance > opts.Buffer {
		return domain.Fix{}, miss{kind: missOutside, amount: distance}
	}
	return fixAt(track, edge, method, distance), miss{}
}

func fixAt(track domain.Track, position domain.TrackPoint, method domain.Method, gap time.Duration) domain.Fix {
	return domain.Fix{Position: position, Method: method, Source: track.Source, Gap: gap}
}

const (
	missOutside = "outside"
	missGap     = "gap"
)

// miss is why one track could not place a photograph, carried so the report
// can name the limit that was hit instead of shrugging.
type miss struct {
	kind   string
	amount time.Duration
}

// nearer keeps whichever miss came closest to succeeding, so a photograph
// rejected by several tracks is explained by the one that nearly matched.
func (m miss) nearer(other miss) miss {
	if m.kind == "" || other.amount < m.amount {
		return other
	}
	return m
}

func (m miss) describe(instant time.Time, opts Options) string {
	stamp := instant.UTC().Format("2006-01-02 15:04:05Z")
	switch m.kind {
	case missOutside:
		return fmt.Sprintf("no track covers %s (nearest is %s away, buffer is %s)",
			stamp, m.amount.Round(time.Second), opts.Buffer)
	case missGap:
		return fmt.Sprintf("track gap of %s around %s exceeds the %s limit",
			m.amount.Round(time.Second), stamp, opts.MaxGap)
	default:
		return fmt.Sprintf("no track data for %s", stamp)
	}
}
