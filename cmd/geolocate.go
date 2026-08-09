package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/cwygoda/ansel/internal/exiftool"
	geoexiftool "github.com/cwygoda/ansel/internal/geolocate/adapters/exiftool"
	"github.com/cwygoda/ansel/internal/geolocate/adapters/fitxz"
	"github.com/cwygoda/ansel/internal/geolocate/application"
	"github.com/cwygoda/ansel/internal/geolocate/matching"
	"github.com/cwygoda/ansel/internal/geolocate/ports"
	"github.com/spf13/cobra"
)

var geolocateCmd = &cobra.Command{
	Use:   "geolocate [flags] [directory]",
	Short: "Place photographs on a GPS track",
	Long: `Match photographs against recorded GPS tracks and record where each was taken.

Every photograph's capture time is resolved against the track, interpolating
along the great circle between recorded points. Positions are written to XMP
sidecars beside the originals, leaving the photographs themselves untouched.

Nothing is written unless you ask for it. By default the command reports
exactly what a real run would do.

Only xz-compressed Garmin FIT activity files are read today.

TIMEZONES

Cameras usually write the local wall clock with no zone, while tracks record
UTC. The zone is resolved in this order, and never guessed from the machine
you are sitting at:

  1. the photograph's own OffsetTimeOriginal, when the camera recorded one
  2. the offset stated by the track itself, which FIT files carry
  3. --tz or --utc-offset

DRIFT

--drift is how far the camera clock runs AHEAD of true time. A camera reading
20:30:54 when it was really 20:29:24 has drifted 90s, so pass --drift 90s. A
camera running slow takes a negative value. The correction shifts the position
lookup and, when writing, the photograph's own timestamps by the same amount.

Requires exiftool (brew install exiftool).

Configuration is read from ~/.ansel/config.toml:

  [geolocate]
  tracks_dir = "~/Documents/Activities"
  max_gap_seconds = 120
  buffer_seconds = 300
  timezone = "Europe/Berlin"

Examples:
  # Report only, writes nothing
  ansel geolocate ~/Pictures/Ansel/Imports/2026-08-09 --track ride.fit.xz

  # Search the configured tracks_dir for recordings covering the shoot
  ansel geolocate ~/Pictures/Ansel/Imports/2026-08-09

  # Write XMP sidecars
  ansel geolocate ~/Pictures/shoot --track ride.fit.xz --write

  # Correct a camera running 90 seconds fast, rewriting its timestamps
  ansel geolocate ~/Pictures/shoot --track ride.fit.xz --drift 90s --write

  # Embed into the photographs themselves rather than sidecars
  ansel geolocate ~/Pictures/shoot --track ride.fit.xz --write --in-place`,
	Args: cobra.MaximumNArgs(1),
	RunE: runGeolocate,
}

var (
	geolocateDryRun    bool
	geolocateWrite     bool
	geolocateInPlace   bool
	geolocateForce     bool
	geolocateJSON      bool
	geolocateTracks    []string
	geolocateDrift     time.Duration
	geolocateMaxGap    time.Duration
	geolocateBuffer    time.Duration
	geolocateTimezone  string
	geolocateUTCOffset string
	geolocateTracksDir string
	geolocateExiftool  string
)

func init() {
	rootCmd.AddCommand(geolocateCmd)

	geolocateCmd.Flags().BoolVar(&geolocateDryRun, "dry-run", true, "Report what would be written without writing it")
	geolocateCmd.Flags().BoolVar(&geolocateWrite, "write", false, "Write coordinates (disables the default dry run)")
	geolocateCmd.Flags().BoolVar(&geolocateInPlace, "in-place", false, "Embed into the photographs instead of writing XMP sidecars")
	geolocateCmd.Flags().BoolVar(&geolocateForce, "force", false, "Replace coordinates that are already present")
	geolocateCmd.Flags().BoolVar(&geolocateJSON, "json", false, "Emit results as JSON, including each position and how it was derived")
	geolocateCmd.Flags().StringArrayVar(&geolocateTracks, "track", nil, "Track file, glob or directory (repeatable)")
	geolocateCmd.Flags().DurationVar(&geolocateDrift, "drift", 0, "How far the camera clock runs ahead of true time (e.g. 90s, -1m30s)")
	geolocateCmd.Flags().DurationVar(&geolocateMaxGap, "max-gap", 0, "Largest track gap to interpolate across (overrides config)")
	geolocateCmd.Flags().DurationVar(&geolocateBuffer, "buffer", 0, "How far outside a track photographs may still be placed (overrides config)")
	geolocateCmd.Flags().StringVar(&geolocateTimezone, "tz", "", "Camera timezone, e.g. Europe/Berlin (overrides config)")
	geolocateCmd.Flags().StringVar(&geolocateUTCOffset, "utc-offset", "", "Camera clock UTC offset, e.g. +02:00 (overrides config)")
	geolocateCmd.Flags().StringVar(&geolocateTracksDir, "tracks-dir", "", "Directory to search when no --track is given (overrides config)")
	geolocateCmd.Flags().StringVar(&geolocateExiftool, "exiftool", "", "Path to exiftool binary (overrides config)")
}

func runGeolocate(cmd *cobra.Command, args []string) error {
	write, err := resolveWriteMode(cmd.Flags().Changed("dry-run"), geolocateDryRun, geolocateWrite)
	if err != nil {
		return err
	}
	if geolocateInPlace && !write {
		return fmt.Errorf("--in-place only makes sense together with --write")
	}

	root, err := resolveRoot(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	locator, closeAll, err := newLocator(cmd, write)
	if err != nil {
		return err
	}
	defer closeAll()

	result, err := locator.Locate(ctx, root)
	if err != nil {
		return err
	}

	if geolocateJSON {
		return printGeolocateJSON(result, write)
	}
	printGeolocateResult(result, write)
	return nil
}

// newLocator wires the concrete adapters. This is the only place that knows a
// subprocess and a compressed activity format are involved.
func newLocator(cmd *cobra.Command, write bool) (*application.Locator, func(), error) {
	cfg, err := geolocateConfig()
	if err != nil {
		return nil, nil, err
	}

	zone, err := application.NewZone(cfg.Timezone, cfg.UTCOffset)
	if err != nil {
		return nil, nil, err
	}

	// One exiftool process serves the whole run, reading and writing alike.
	session := exiftool.New(cfg.ExiftoolBinary)
	locator := &application.Locator{
		Metadata: geoexiftool.New(session),
		Writer:   geoexiftool.NewWriter(session),
		// Adding GPX or TCX support means adding its adapter here and
		// nowhere else.
		Decoders:   []ports.TrackDecoder{fitxz.New()},
		Config:     cfg,
		Matching:   matchingOptions(cmd, cfg),
		Zone:       zone,
		Drift:      geolocateDrift,
		TrackPaths: geolocateTracks,
		Write:      write,
		InPlace:    geolocateInPlace,
		Force:      geolocateForce,
	}

	return locator, func() { _ = session.Close() }, nil
}

// matchingOptions applies the flags over the config. Both limits are read via
// Changed rather than by testing for zero, since --max-gap=0 and --buffer=0
// are meaningful: no gap limit, and no clamping at all.
func matchingOptions(cmd *cobra.Command, cfg application.Config) matching.Options {
	options := cfg.MatchingOptions()
	if cmd.Flags().Changed("max-gap") {
		options.MaxGap = geolocateMaxGap
	}
	if cmd.Flags().Changed("buffer") {
		options.Buffer = geolocateBuffer
	}
	return options
}

// geolocateConfig applies built-in defaults, then the config file, then flags.
func geolocateConfig() (application.Config, error) {
	cfg, err := application.LoadConfig()
	if err != nil {
		return cfg, err
	}

	if geolocateExiftool != "" {
		cfg.ExiftoolBinary = geolocateExiftool
	}
	// The two zone flags are one answer, not two halves of one. Naming either
	// on the command line replaces whatever the config said about zones, so a
	// configured fixed offset cannot survive to override a --tz given here.
	if geolocateTimezone != "" || geolocateUTCOffset != "" {
		cfg.Timezone, cfg.UTCOffset = geolocateTimezone, geolocateUTCOffset
	}
	if geolocateTracksDir != "" {
		expanded, err := expandCLIPath(geolocateTracksDir)
		if err != nil {
			return cfg, err
		}
		cfg.TracksDir = expanded
	}
	return cfg, nil
}
