package publish

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const configFileName = ".ansel.toml"

// ProjectConfig represents the project-local configuration.
type ProjectConfig struct {
	Publish PublishConfig `toml:"publish"`
}

// PublishConfig holds publishing-related settings.
type PublishConfig struct {
	Subdomain    string `toml:"subdomain"`
	HostedZoneID string `toml:"hosted_zone_id"`
	DomainName   string `toml:"domain_name"`
}

// LoadProjectConfig loads the configuration from .ansel.toml in the current directory.
// Returns an empty config if the file doesn't exist.
func LoadProjectConfig() (*ProjectConfig, error) {
	data, err := os.ReadFile(configFileName)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProjectConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", configFileName, err)
	}

	var cfg ProjectConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", configFileName, err)
	}

	return &cfg, nil
}

// SaveProjectConfig writes the configuration to .ansel.toml in the current directory.
func SaveProjectConfig(cfg *ProjectConfig) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(configFileName, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", configFileName, err)
	}

	return nil
}
