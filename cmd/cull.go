package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cwygoda/ansel/internal/cull/adapters/exiftool"
	"github.com/cwygoda/ansel/internal/cull/adapters/sqlite"
	"github.com/cwygoda/ansel/internal/cull/adapters/vipsdecoder"
	"github.com/cwygoda/ansel/internal/cull/adapters/xmp"
	"github.com/cwygoda/ansel/internal/cull/application"
	imglib "github.com/cwygoda/ansel/internal/image"
	"github.com/spf13/cobra"
)

var cullCmd = &cobra.Command{
	Use:   "cull [flags] [directory]",
	Short: "Analyze, group and rate photographs",
	Long: `Analyze a shoot, group near-identical frames and rank each group.

Photographs are never modified. Measurements go to a SQLite database and,
when writing is enabled, ratings are written to XMP sidecars beside the
originals.

Each photograph is measured for sharpness, exposure and clipping using its
embedded preview, so RAW files are never demosaiced. Frames close together in
time and visually alike are grouped, and the strongest frame in each group is
marked.

Nothing is written unless you ask for it. By default the command reports
exactly what a real run would do.

Requires exiftool (brew install exiftool).

Configuration is read from ~/.ansel/config.toml:

  [cull]
  db_path = "~/.ansel/cull.db"
  max_preview_edge = 2048

  [cull.similarity]
  window_seconds = 8
  max_distance = 10

  [cull.policy]
  sharp_above = 0.65
  soft_below = 0.25

Examples:
  # Report only, writes nothing
  ansel cull ~/Pictures/Ansel/Imports/2026-08-09

  # Write XMP sidecars
  ansel cull --write ~/Pictures/Ansel/Imports/2026-08-09

  # Re-measure, ignoring cached results
  ansel cull --reanalyze ~/Pictures/Ansel/Imports/2026-08-09`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCull,
}

var (
	cullDryRun      bool
	cullWrite       bool
	cullForce       bool
	cullReanalyze   bool
	cullJSON        bool
	cullDBPath      string
	cullExiftool    string
	cullWorkers     int
	cullMaxEdge     int
	cullGroupWindow float64
)

func init() {
	rootCmd.AddCommand(cullCmd)

	cullCmd.Flags().BoolVar(&cullDryRun, "dry-run", true, "Report what would be written without writing it")
	cullCmd.Flags().BoolVar(&cullWrite, "write", false, "Write XMP sidecars (disables the default dry run)")
	cullCmd.Flags().BoolVar(&cullForce, "force", false, "Replace XMP sidecars that already exist")
	cullCmd.Flags().BoolVar(&cullReanalyze, "reanalyze", false, "Ignore cached results and measure everything again")
	cullCmd.Flags().BoolVar(&cullJSON, "json", false, "Emit results as JSON, including rank scores and reasons")
	cullCmd.Flags().StringVar(&cullDBPath, "db", "", "Analysis database path (overrides config)")
	cullCmd.Flags().StringVar(&cullExiftool, "exiftool", "", "Path to exiftool binary (overrides config)")
	cullCmd.Flags().IntVar(&cullWorkers, "workers", 0, "Concurrent analysis workers (overrides config)")
	cullCmd.Flags().IntVar(&cullMaxEdge, "max-edge", 0, "Longest edge of the analysis preview in pixels (overrides config)")
	cullCmd.Flags().Float64Var(&cullGroupWindow, "group-window", 0, "Seconds between frames to consider them related (overrides config)")
}

func runCull(cmd *cobra.Command, args []string) error {
	write, err := resolveWriteMode(cmd.Flags().Changed("dry-run"), cullDryRun, cullWrite)
	if err != nil {
		return err
	}

	root, err := resolveRoot(args)
	if err != nil {
		return err
	}

	imglib.InitVips()
	defer imglib.ShutdownVips()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	culler, closeAll, err := newCuller(write)
	if err != nil {
		return err
	}
	defer closeAll()

	result, err := culler.Cull(ctx, root)
	if err != nil {
		return err
	}

	if cullJSON {
		return printCullJSON(result, write)
	}
	printCullResult(result, write)
	return nil
}

// resolveWriteMode decides whether sidecars are actually written.
//
// Two flags express one decision, so the combination that means opposite
// things at once is rejected rather than silently resolved. Note that
// cobra.MarkFlagsMutuallyExclusive cannot express this: it fires whenever both
// flags are set regardless of value, which would wrongly reject the perfectly
// consistent `--write --dry-run=false`.
func resolveWriteMode(dryRunChanged, dryRun, write bool) (bool, error) {
	if write && dryRunChanged && dryRun {
		return false, fmt.Errorf("--write conflicts with --dry-run=true; pass only one")
	}
	if write {
		return true, nil
	}
	return dryRunChanged && !dryRun, nil
}

func resolveRoot(args []string) (string, error) {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	expanded, err := expandCLIPath(root)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", root, err)
	}

	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("failed to open %s: %w", absolute, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", absolute)
	}
	return absolute, nil
}

// newCuller wires the concrete adapters. This is the only place that knows a
// database, a subprocess and an image library are involved.
func newCuller(write bool) (*application.Culler, func(), error) {
	cfg, err := cullConfig()
	if err != nil {
		return nil, nil, err
	}

	store, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		return nil, nil, err
	}
	reader := exiftool.New(cfg.ExiftoolBinary)

	culler := &application.Culler{
		Metadata:  reader,
		Previews:  reader,
		Decoder:   vipsdecoder.New(),
		Store:     store,
		Sidecars:  xmp.New(),
		Config:    cfg,
		Write:     write,
		Force:     cullForce,
		Reanalyze: cullReanalyze,
	}

	return culler, func() {
		_ = reader.Close()
		_ = store.Close()
	}, nil
}

// cullConfig applies the precedence the other commands use: built-in
// defaults, then the config file, then flags.
func cullConfig() (application.Config, error) {
	cfg, err := application.LoadConfig()
	if err != nil {
		return cfg, err
	}

	if cullDBPath != "" {
		cfg.DBPath, err = expandCLIPath(cullDBPath)
		if err != nil {
			return cfg, err
		}
	}
	if cullExiftool != "" {
		cfg.ExiftoolBinary = cullExiftool
	}
	if cullWorkers > 0 {
		cfg.Workers = cullWorkers
	}
	if cullMaxEdge > 0 {
		cfg.MaxPreviewEdge = cullMaxEdge
	}
	if cullGroupWindow > 0 {
		cfg.Similarity.WindowSeconds = cullGroupWindow
	}
	return cfg, nil
}
