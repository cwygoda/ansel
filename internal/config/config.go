package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config represents the application configuration.
type Config struct {
	Instagram InstagramConfig `toml:"instagram"`
}

// InstagramConfig holds Instagram-related settings.
type InstagramConfig struct {
	AccessToken string `toml:"access_token"`
	UserID      string `toml:"user_id"`
	NgrokToken  string `toml:"ngrok_token"` // Optional, uses NGROK_AUTHTOKEN env if empty
}

// ConfigPath returns the path to the config file.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".ansel", "config.toml"), nil
}

// ExpandPath resolves a leading ~ against the user's home directory. Paths
// without a ~ prefix are returned unchanged.
func ExpandPath(path string) (string, error) {
	if path != "" && path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	if path == "" || path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

// EnsureConfigDir creates the config directory if it doesn't exist.
func EnsureConfigDir() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0700)
}

// Load loads the configuration from ~/.ansel/config.toml.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s\n\nCreate it with:\n\n[instagram]\naccess_token = \"your-access-token\"\nuser_id = \"your-instagram-user-id\"\nngrok_token = \"your-ngrok-auth-token\"", path)
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Validate checks that required fields are set.
func (c *Config) Validate() error {
	if c.Instagram.AccessToken == "" {
		return fmt.Errorf("instagram.access_token is required in config")
	}
	if c.Instagram.UserID == "" {
		return fmt.Errorf("instagram.user_id is required in config")
	}
	// ngrok_token is optional, can use NGROK_AUTHTOKEN env
	return nil
}

// NgrokAuthToken returns the ngrok auth token, checking config then env.
func (c *Config) NgrokAuthToken() string {
	if c.Instagram.NgrokToken != "" {
		return c.Instagram.NgrokToken
	}
	return os.Getenv("NGROK_AUTHTOKEN")
}
