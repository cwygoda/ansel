package application

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pelletier/go-toml/v2"

	"github.com/cwygoda/ansel/internal/config"
)

// Config is the application-level camera import configuration.
type Config struct {
	BaseDir           string   `toml:"base_dir"`
	StatePath         string   `toml:"state_path"`
	Backend           string   `toml:"backend"`
	CardRoots         []string `toml:"card_roots"`
	FolderLayout      string   `toml:"folder_layout"`
	IncludeExtensions []string `toml:"include_extensions"`
	IncludeUnknown    bool     `toml:"include_unknown"`
}

type rootConfig struct {
	CameraImport Config `toml:"camera_import"`
}

func DefaultConfig() Config {
	return Config{
		BaseDir:      "~/Pictures/Ansel/Imports",
		StatePath:    "~/.ansel/camera-import-state.json",
		Backend:      "auto",
		CardRoots:    defaultCardRoots(),
		FolderLayout: "2006-01-02",
		IncludeExtensions: []string{
			".jpg", ".jpeg", ".nef", ".dng", ".mov", ".mp4", ".tif", ".tiff",
		},
	}
}

func LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	path, err := userConfigPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return expandConfig(cfg)
		}
		return cfg, fmt.Errorf("failed to read camera config: %w", err)
	}
	var root rootConfig
	if err := toml.Unmarshal(data, &root); err != nil {
		return cfg, fmt.Errorf("failed to parse camera config: %w", err)
	}
	overlayConfig(&cfg, root.CameraImport)
	return expandConfig(cfg)
}

func overlayConfig(base *Config, override Config) {
	if override.BaseDir != "" {
		base.BaseDir = override.BaseDir
	}
	if override.StatePath != "" {
		base.StatePath = override.StatePath
	}
	if override.Backend != "" {
		base.Backend = override.Backend
	}
	if len(override.CardRoots) > 0 {
		base.CardRoots = override.CardRoots
	}
	if override.FolderLayout != "" {
		base.FolderLayout = override.FolderLayout
	}
	if len(override.IncludeExtensions) > 0 {
		base.IncludeExtensions = override.IncludeExtensions
	}
	base.IncludeUnknown = override.IncludeUnknown
}

func expandConfig(cfg Config) (Config, error) {
	var err error
	cfg.BaseDir, err = expandPath(cfg.BaseDir)
	if err != nil {
		return cfg, err
	}
	cfg.StatePath, err = expandPath(cfg.StatePath)
	if err != nil {
		return cfg, err
	}
	for idx, root := range cfg.CardRoots {
		cfg.CardRoots[idx], err = expandPath(root)
		if err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func userConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".ansel", "config.toml"), nil
}

func expandPath(path string) (string, error) {
	return config.ExpandPath(path)
}

func defaultCardRoots() []string {
	if runtime.GOOS == "darwin" {
		return []string{"/Volumes"}
	}
	roots := []string{"/media", "/mnt"}
	if user := os.Getenv("USER"); user != "" {
		roots = append(roots, filepath.Join("/run/media", user))
	}
	return roots
}
