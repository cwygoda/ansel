package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config is the application-level camera import configuration.
type Config struct {
	BaseDir           string   `toml:"base_dir"`
	StatePath         string   `toml:"state_path"`
	Backend           string   `toml:"backend"`
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
		Backend:      "gphoto2",
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
	if path == "" || path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		if path == "" || path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
