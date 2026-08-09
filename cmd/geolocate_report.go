package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

// printGeolocateResult renders one run in the same shape as the cull report:
// results to stdout, failures to stderr, two spaces per level of nesting.
func printGeolocateResult(result domain.Result, write bool) {
	fmt.Fprintf(os.Stdout, "%s: %s, %s\n",
		filepath.Base(result.Root),
		count(result.Discovered, "photograph", "photographs"),
		count(len(result.Tracks), "track", "tracks"))

	if result.Discovered == 0 {
		fmt.Fprintln(os.Stdout, "  Nothing to do.")
		return
	}

	printLocatedSummary(result)
	printUnlocated(result)
	printWriteSummary(result, write)
	printGeolocateFailures(result)
}

func printLocatedSummary(result domain.Result) {
	if len(result.Plans) == 0 {
		fmt.Fprintln(os.Stdout, "  Located: 0")
		return
	}
	fmt.Fprintf(os.Stdout, "  Located: %d (%s)\n", len(result.Plans), methodBreakdown(result))
}

// methodBreakdown names how the positions were arrived at, in order of how
// much each rests on inference.
func methodBreakdown(result domain.Result) string {
	counts := result.MethodCounts()
	ordered := []domain.Method{
		domain.MethodExact,
		domain.MethodInterpolated,
		domain.MethodClampedStart,
		domain.MethodClampedEnd,
	}

	parts := make([]string, 0, len(ordered))
	for _, method := range ordered {
		if counts[method] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", method, counts[method]))
		}
	}
	return strings.Join(parts, ", ")
}

func printUnlocated(result domain.Result) {
	if len(result.Unlocated) == 0 {
		return
	}

	fmt.Fprintf(os.Stdout, "  Unlocated: %d\n", len(result.Unlocated))
	for _, unlocated := range groupUnlocated(result.Unlocated) {
		fmt.Fprintf(os.Stdout, "    %-24s %s\n",
			filepath.Base(unlocated.PhotoPath), unlocated.Reason)
	}
	fmt.Fprintln(os.Stdout)
}

// groupUnlocated caps the list so one badly mismatched shoot does not bury the
// rest of the report in identical lines.
func groupUnlocated(unlocated []domain.Unlocated) []domain.Unlocated {
	const maxListed = 10
	if len(unlocated) <= maxListed {
		return unlocated
	}
	return unlocated[:maxListed]
}

func printWriteSummary(result domain.Result, write bool) {
	// Kept means "left alone because it already had coordinates". Under
	// --force those same plans are written instead, and carry no reason, so
	// this correctly falls to zero rather than claiming they were preserved.
	kept := 0
	for _, plan := range result.Plans {
		if plan.Existing && plan.Skipped != "" {
			kept++
		}
	}

	singular, plural := "sidecar", "sidecars"
	if len(result.Plans) > 0 && result.Plans[0].InPlace {
		singular, plural = "photograph", "photographs"
	}

	if !write {
		fmt.Fprintf(os.Stdout, "  Would write: %s\n", count(len(result.Plans)-kept, singular, plural))
	} else {
		fmt.Fprintf(os.Stdout, "  Wrote: %s\n", count(result.Written, singular, plural))
	}
	if kept > 0 {
		fmt.Fprintf(os.Stdout, "    %d already had coordinates and were kept (use --force to replace)\n", kept)
	}
}

func printGeolocateFailures(result domain.Result) {
	if len(result.Failures) == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "\n  %d failures:\n", len(result.Failures))
	for _, failure := range result.Failures {
		fmt.Fprintf(os.Stderr, "    %s [%s] %s\n",
			filepath.Base(failure.Path), failure.Stage, failure.Err)
	}
}

// JSON output.

type geolocateJSONPlan struct {
	Photo     string  `json:"photo"`
	Target    string  `json:"target"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation float64 `json:"elevation,omitempty"`
	Method    string  `json:"method"`
	Source    string  `json:"source"`
	GapSecond float64 `json:"gap_seconds"`
	TrackTime string  `json:"track_time"`
	Corrected string  `json:"corrected_time,omitempty"`
	Existing  bool    `json:"existing,omitempty"`
	Written   bool    `json:"written,omitempty"`
	Skipped   string  `json:"skipped,omitempty"`
}

type geolocateJSONUnlocated struct {
	Photo  string `json:"photo"`
	At     string `json:"at,omitempty"`
	Reason string `json:"reason"`
}

type geolocateJSONReport struct {
	Root       string                   `json:"root"`
	DryRun     bool                     `json:"dry_run"`
	Tracks     []string                 `json:"tracks"`
	Discovered int                      `json:"discovered"`
	Located    int                      `json:"located"`
	Written    int                      `json:"written"`
	Plans      []geolocateJSONPlan      `json:"plans"`
	Unlocated  []geolocateJSONUnlocated `json:"unlocated,omitempty"`
	Failures   []geolocateJSONFailure   `json:"failures,omitempty"`
}

type geolocateJSONFailure struct {
	Path  string `json:"path"`
	Stage string `json:"stage"`
	Error string `json:"error"`
}

func printGeolocateJSON(result domain.Result, write bool) error {
	report := geolocateJSONReport{
		Root:       result.Root,
		DryRun:     !write,
		Tracks:     sortedCopy(result.Tracks),
		Discovered: result.Discovered,
		Located:    len(result.Plans),
		Written:    result.Written,
		Plans:      make([]geolocateJSONPlan, 0, len(result.Plans)),
	}

	for _, plan := range result.Plans {
		report.Plans = append(report.Plans, jsonPlan(plan))
	}
	for _, unlocated := range result.Unlocated {
		entry := geolocateJSONUnlocated{Photo: unlocated.PhotoPath, Reason: unlocated.Reason}
		if !unlocated.At.IsZero() {
			entry.At = unlocated.At.UTC().Format("2006-01-02T15:04:05Z")
		}
		report.Unlocated = append(report.Unlocated, entry)
	}
	for _, failure := range result.Failures {
		report.Failures = append(report.Failures, geolocateJSONFailure{
			Path: failure.Path, Stage: failure.Stage, Error: failure.Err,
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func jsonPlan(plan domain.WritePlan) geolocateJSONPlan {
	entry := geolocateJSONPlan{
		Photo:     plan.PhotoPath,
		Target:    plan.Target,
		Latitude:  plan.Fix.Position.Latitude,
		Longitude: plan.Fix.Position.Longitude,
		Method:    string(plan.Fix.Method),
		Source:    plan.Fix.Source,
		GapSecond: plan.Fix.Gap.Seconds(),
		TrackTime: plan.Fix.Position.Time.UTC().Format("2006-01-02T15:04:05Z"),
		Existing:  plan.Existing,
		Written:   plan.Written,
		Skipped:   plan.Skipped,
	}
	if plan.Fix.Position.HasElevation {
		entry.Elevation = plan.Fix.Position.Elevation
	}
	if plan.WriteTime {
		entry.Corrected = plan.CorrectedWall.Format("2006-01-02T15:04:05") +
			offsetSuffix(plan)
	}
	return entry
}

func offsetSuffix(plan domain.WritePlan) string {
	sign := "+"
	offset := plan.CorrectedOffset
	if offset < 0 {
		sign, offset = "-", -offset
	}
	return fmt.Sprintf("%s%02d:%02d", sign, int(offset.Hours()), int(offset.Minutes())%60)
}

// count renders a tally with the right noun, so a run over one track does not
// announce "1 tracks".
func count(number int, singular, plural string) string {
	if number == 1 {
		return fmt.Sprintf("%d %s", number, singular)
	}
	return fmt.Sprintf("%d %s", number, plural)
}

func sortedCopy(values []string) []string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return sorted
}
