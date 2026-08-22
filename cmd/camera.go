package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cwygoda/ansel/internal/camera/adapters/gphoto2"
	"github.com/cwygoda/ansel/internal/camera/adapters/launchagent"
	"github.com/cwygoda/ansel/internal/camera/adapters/massstorage"
	"github.com/cwygoda/ansel/internal/camera/adapters/statejson"
	"github.com/cwygoda/ansel/internal/camera/application"
	"github.com/cwygoda/ansel/internal/camera/domain"
	"github.com/cwygoda/ansel/internal/camera/ports"
	"github.com/cwygoda/ansel/internal/config"
)

var cameraCmd = &cobra.Command{
	Use:   "camera",
	Short: "Detect cameras and import new pictures",
	Long: `Detect supported USB cameras and mounted camera cards, then import only new pictures.

Supported cameras/cards:
  - Nikon Z6 III
  - Ricoh GR IIIx
  - Mounted cards with a DCIM directory, such as a CFExpress card in a USB-C reader

Configuration is read from ~/.ansel/config.toml:

  [camera_import]
  base_dir = "~/Pictures/Ansel/Imports"
  state_path = "~/.ansel/camera-import-state.json"
  backend = "auto"
  card_roots = ["/Volumes"]
  folder_layout = "2006-01-02"`,
}

var cameraDetectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detect attached cameras and mounted cards",
	RunE:  runCameraDetect,
}

var cameraImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import new pictures from attached cameras and mounted cards",
	RunE:  runCameraImport,
}

var cameraInstallAgentCmd = &cobra.Command{
	Use:   "install-agent",
	Short: "Install a user LaunchAgent that imports on camera attach or card mount",
	RunE:  runCameraInstallAgent,
}

var cameraUninstallAgentCmd = &cobra.Command{
	Use:   "uninstall-agent",
	Short: "Uninstall the camera import LaunchAgent",
	RunE:  runCameraUninstallAgent,
}

var cameraAgentTriggerCmd = &cobra.Command{
	Use:    "agent-trigger",
	Short:  "Consume a launchd USB event and run camera import",
	Hidden: true,
	RunE:   runCameraAgentTrigger,
}

var (
	cameraBaseDir        string
	cameraStatePath      string
	cameraBackend        string
	cameraCardRoots      []string
	cameraGphoto2Binary  string
	cameraDryRun         bool
	cameraIncludeUnknown bool
	cameraAnselPath      string
)

func init() {
	rootCmd.AddCommand(cameraCmd)
	cameraCmd.AddCommand(cameraDetectCmd, cameraImportCmd, cameraInstallAgentCmd, cameraUninstallAgentCmd, cameraAgentTriggerCmd)

	cameraDetectCmd.Flags().StringVar(&cameraBackend, "backend", "", "Import backend: auto, gphoto2, or card (overrides config)")
	cameraDetectCmd.Flags().StringArrayVar(&cameraCardRoots, "card-root", nil, "Root directory to scan for mounted cards (can be repeated)")
	cameraDetectCmd.Flags().StringVar(&cameraGphoto2Binary, "gphoto2", "gphoto2", "Path to gphoto2 binary")
	cameraImportCmd.Flags().StringVar(&cameraBaseDir, "base-dir", "", "Base import directory (overrides config)")
	cameraImportCmd.Flags().StringVar(&cameraStatePath, "state", "", "Bookmark state path (overrides config)")
	cameraImportCmd.Flags().StringVar(&cameraBackend, "backend", "", "Import backend: auto, gphoto2, or card (overrides config)")
	cameraImportCmd.Flags().StringArrayVar(&cameraCardRoots, "card-root", nil, "Root directory to scan for mounted cards (can be repeated)")
	cameraImportCmd.Flags().StringVar(&cameraGphoto2Binary, "gphoto2", "gphoto2", "Path to gphoto2 binary")
	cameraImportCmd.Flags().BoolVar(&cameraDryRun, "dry-run", false, "Plan imports without downloading")
	cameraImportCmd.Flags().BoolVar(&cameraIncludeUnknown, "include-unknown", false, "Import from cameras/cards not recognized as Nikon Z6 III or Ricoh GR IIIx")

	cameraInstallAgentCmd.Flags().StringVar(&cameraAnselPath, "ansel-path", "", "Absolute path to ansel binary (default: current executable)")
}

func runCameraDetect(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	backends, err := newCameraBackendsFromConfig()
	if err != nil {
		return err
	}
	importer := &application.Importer{Backends: backends}
	cameras, err := importer.Detect(ctx)
	if err != nil {
		return err
	}
	if len(cameras) == 0 {
		fmt.Fprintln(os.Stdout, "No cameras or mounted cards detected.")
		return nil
	}
	for _, camera := range cameras {
		known := camera.KnownName()
		if known == "" {
			known = "unknown"
		}
		fmt.Fprintf(os.Stdout, "%s\n  port: %s\n  known: %s\n", camera.Model, camera.Port, known)
	}
	return nil
}

func runCameraImport(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	return runImport(ctx, cameraDryRun)
}

func runCameraAgentTrigger(cmd *cobra.Command, args []string) error {
	consumed := launchagent.ConsumeIOKitEvent(5)
	if consumed {
		fmt.Fprintln(os.Stderr, "Consumed launchd IOKit USB attach event")
	} else {
		fmt.Fprintln(os.Stderr, "No launchd IOKit event received before timeout; continuing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	return runImport(ctx, false)
}

func runCameraInstallAgent(cmd *cobra.Command, args []string) error {
	path, err := launchagent.Install(cameraAnselPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Installed and loaded LaunchAgent: %s\n", path)
	fmt.Fprintln(os.Stdout, "Logs: ~/Library/Logs/ansel/camera-import.{out,err}.log")
	return nil
}

func runCameraUninstallAgent(cmd *cobra.Command, args []string) error {
	if err := launchagent.Uninstall(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "Uninstalled LaunchAgent")
	return nil
}

func runImport(ctx context.Context, dryRun bool) error {
	importer, err := newCameraImporter(dryRun)
	if err != nil {
		return err
	}
	results, err := importer.Import(ctx)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Fprintln(os.Stdout, "No supported cameras or mounted cards detected.")
		return nil
	}
	for _, result := range results {
		printImportResult(result, dryRun)
	}
	return nil
}

func newCameraImporter(dryRun bool) (*application.Importer, error) {
	cfg, err := application.LoadConfig()
	if err != nil {
		return nil, err
	}
	if cameraBaseDir != "" {
		cfg.BaseDir, err = expandCLIPath(cameraBaseDir)
		if err != nil {
			return nil, err
		}
	}
	if cameraStatePath != "" {
		cfg.StatePath, err = expandCLIPath(cameraStatePath)
		if err != nil {
			return nil, err
		}
	}
	if cameraBackend != "" {
		cfg.Backend = cameraBackend
	}
	if len(cameraCardRoots) > 0 {
		cfg.CardRoots, err = expandCLIPaths(cameraCardRoots)
		if err != nil {
			return nil, err
		}
	}
	if cameraIncludeUnknown {
		cfg.IncludeUnknown = true
	}
	state, err := statejson.Open(cfg.StatePath)
	if err != nil {
		return nil, err
	}
	backends, err := newCameraBackends(cfg)
	if err != nil {
		return nil, err
	}
	return &application.Importer{Backends: backends, State: state, Config: cfg, DryRun: dryRun}, nil
}

func newCameraBackendsFromConfig() ([]ports.CameraBackend, error) {
	cfg, err := application.LoadConfig()
	if err != nil {
		return nil, err
	}
	if cameraBackend != "" {
		cfg.Backend = cameraBackend
	}
	if len(cameraCardRoots) > 0 {
		cfg.CardRoots, err = expandCLIPaths(cameraCardRoots)
		if err != nil {
			return nil, err
		}
	}
	return newCameraBackends(cfg)
}

func newCameraBackends(cfg application.Config) ([]ports.CameraBackend, error) {
	switch strings.ToLower(cfg.Backend) {
	case "", "auto":
		return []ports.CameraBackend{gphoto2.New(cameraGphoto2Binary), massstorage.New(cfg.CardRoots)}, nil
	case "gphoto2":
		return []ports.CameraBackend{gphoto2.New(cameraGphoto2Binary)}, nil
	case "card", "cards", "filesystem", "massstorage", "mass-storage":
		return []ports.CameraBackend{massstorage.New(cfg.CardRoots)}, nil
	default:
		return nil, fmt.Errorf("unsupported camera backend %q", cfg.Backend)
	}
}

func printImportResult(result domain.ImportResult, dryRun bool) {
	known := result.Camera.KnownName()
	if known == "" {
		known = result.Camera.Model
	}
	fmt.Fprintf(os.Stdout, "%s (%s)\n", known, result.Camera.Port)
	fmt.Fprintf(os.Stdout, "  Seen: %d, skipped: %d\n", result.Seen, result.Skipped)
	if dryRun {
		fmt.Fprintf(os.Stdout, "  Would download: %d\n", len(result.Planned))
		for _, file := range result.Planned {
			fmt.Fprintf(os.Stdout, "    %s/%s\n", file.Folder, file.Name)
		}
		return
	}
	fmt.Fprintf(os.Stdout, "  Downloaded: %d\n", result.Downloaded)
	for _, record := range result.Records {
		fmt.Fprintf(os.Stdout, "    %s/%s -> %s\n", record.Folder, record.Name, record.Destination)
	}
}

func expandCLIPath(path string) (string, error) {
	return config.ExpandPath(path)
}

func expandCLIPaths(paths []string) ([]string, error) {
	expanded := make([]string, 0, len(paths))
	for _, path := range paths {
		value, err := expandCLIPath(path)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, value)
	}
	return expanded, nil
}
