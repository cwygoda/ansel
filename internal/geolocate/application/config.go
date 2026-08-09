package application

import (
	"fmt"
	"os"
	"time"

	"github.com/cwygoda/ansel/internal/config"
	"github.com/cwygoda/ansel/internal/geolocate/matching"
	"github.com/pelletier/go-toml/v2"
)

// Config is the application-level geolocate configuration, read from the
// [geolocate] section of ~/.ansel/config.toml.
type Config struct {
	ExiftoolBinary    string   `toml:"exiftool"`
	IncludeExtensions []string `toml:"include_extensions"`

	// TracksDir is searched when no track is named on the command line. It is
	// empty by default: scanning a directory nobody asked about would be a
	// surprising thing for a tool to do on its own.
	TracksDir string `toml:"tracks_dir"`

	MaxGapSeconds float64 `toml:"max_gap_seconds"`
	BufferSeconds float64 `toml:"buffer_seconds"`

	// UTCOffset and Timezone are the last resort for photographs whose camera
	// recorded no zone and whose track does not state one. Timezone is the
	// better of the two, since a named zone knows about summer time while a
	// fixed offset does not.
	UTCOffset string `toml:"utc_offset"`
	Timezone  string `toml:"timezone"`
}

type rootConfig struct {
	Geolocate Config `toml:"geolocate"`
}

// DefaultConfig returns settings that work without any config file.
func DefaultConfig() Config {
	return Config{
		ExiftoolBinary:    "exiftool",
		IncludeExtensions: []string{".nef", ".jpg", ".jpeg"},
		// Two minutes spans the smart-recording gaps devices leave on
		// straight, steady sections without inviting a guess across a pause.
		MaxGapSeconds: 120,
		// Five minutes covers the frames taken while unpacking at the
		// trailhead, before the recording was started.
		BufferSeconds: 300,
	}
}

// LoadConfig reads the [geolocate] section, treating a missing file as "use
// defaults". It deliberately does not go through config.Load, whose Validate
// requires Instagram credentials that have nothing to do with geolocation.
func LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	path, err := config.ConfigPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return expandConfig(cfg)
		}
		return cfg, fmt.Errorf("failed to read geolocate config: %w", err)
	}
	var root rootConfig
	if err := toml.Unmarshal(data, &root); err != nil {
		return cfg, fmt.Errorf("failed to parse geolocate config: %w", err)
	}
	overlayConfig(&cfg, root.Geolocate)
	return expandConfig(cfg)
}

// expandConfig resolves tilde paths. An unset TracksDir is left alone, since
// config.ExpandPath turns the empty string into the home directory and this
// feature must not scan a whole home directory by accident.
func expandConfig(cfg Config) (Config, error) {
	if cfg.TracksDir == "" {
		return cfg, nil
	}
	expanded, err := config.ExpandPath(cfg.TracksDir)
	if err != nil {
		return cfg, err
	}
	cfg.TracksDir = expanded
	return cfg, nil
}

// MatchingOptions projects the config onto the matching policy.
func (c Config) MatchingOptions() matching.Options {
	return matching.Options{
		MaxGap: time.Duration(c.MaxGapSeconds * float64(time.Second)),
		Buffer: time.Duration(c.BufferSeconds * float64(time.Second)),
	}
}
