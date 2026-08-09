package domain

import "time"

// Photo is a discovered photograph, described before any track is consulted.
//
// Whether a position is already recorded is deliberately not here: that is a
// property of the write target, which may be the photograph or a sidecar
// beside it, and is only known once the run's mode is settled. It lives on
// WritePlan.Existing instead.
type Photo struct {
	Path  string
	Clock CaptureClock
}

// WritePlan is one write the run intends to make. It is populated whether or
// not writing is enabled, so a dry run reports exactly what a real run would
// do.
type WritePlan struct {
	PhotoPath string

	// Target is what actually gets written: an XMP sidecar beside the
	// photograph, or the photograph itself under --in-place. The application
	// decides which; the writer just writes where it is told.
	Target  string
	InPlace bool

	Fix Fix

	// CorrectedWall is the drift-corrected capture time to write back, in the
	// camera's own zone, and is only meaningful when WriteTime is set. A run
	// without drift leaves the photograph's timestamps untouched.
	CorrectedWall   time.Time
	CorrectedOffset time.Duration
	WriteTime       bool

	// Existing reports that the target already carries coordinates.
	Existing bool
	Written  bool
	Skipped  string
}

// Unlocated is one photograph no track could place, with the reason.
type Unlocated struct {
	PhotoPath string
	At        time.Time
	Reason    string
}

// Failure is one non-fatal error. Batch operations record and continue.
type Failure struct {
	Path  string
	Stage string
	Err   string
}

// Result summarizes one geolocate run.
type Result struct {
	Root string

	// Tracks are the paths actually used, after discovery and filtering.
	Tracks []string

	Plans     []WritePlan
	Unlocated []Unlocated
	Failures  []Failure

	Discovered int
	Written    int
}

// MethodCounts tallies how the located photographs were placed, for the
// report's one-line breakdown.
func (r Result) MethodCounts() map[Method]int {
	counts := make(map[Method]int, 4)
	for _, plan := range r.Plans {
		counts[plan.Fix.Method]++
	}
	return counts
}
