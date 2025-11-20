package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	GameName    = "Minae"
	GameVersion = "0.1.0"
	ChunkWidth  = 16
	ChunkHeight = 256
)

type GameConfig struct {
	ScreenWidth  int     `yaml:"screen_width"`
	ScreenHeight int     `yaml:"screen_height"`
	TargetFPS    int     `yaml:"target_fps"`
	FOV          float32 `yaml:"fov"`
	PlayerSpeed  float32 `yaml:"player_speed"`
	MouseSens    float32 `yaml:"mouse_sens"`
}

// DefaultConfig provides sensible defaults
func DefaultConfig() GameConfig {
	return GameConfig{
		ScreenWidth:  1280,
		ScreenHeight: 720,
		TargetFPS:    60,
		FOV:          60.0,
		PlayerSpeed:  10.0,
		MouseSens:    0.003,
	}
}

var Current = DefaultConfig()

// BootstrapDataFolder ensures the data folder exists and returns its path.
// It checks MINAE_DATA_FOLDER or defaults to ~/.minae.
// It does NOT copy assets anymore, assuming vanilla content is hardcoded.
func BootstrapDataFolder() (string, error) {
	path := os.Getenv("MINAE_DATA_FOLDER")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		path = filepath.Join(home, ".minae")
	}

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("could not create data directory: %w", err)
	}

	return path, nil
}

// Load reads the configuration from config.yaml in the data folder.
// If the file doesn't exist, it writes the default config to it.
// If the file exists but is invalid, it returns an error but leaves Current as defaults (or partial).
func Load(dataFolder string) error {
	configPath := filepath.Join(dataFolder, "config.yaml")

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Config file doesn't exist. Write default config.
		cfg := DefaultConfig()
		data, err := yaml.Marshal(&cfg)
		if err != nil {
			return fmt.Errorf("failed to marshal default config: %w", err)
		}

		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write default config to %s: %w", configPath, err)
		}

		Current = cfg
		return nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Create a temporary struct with defaults to ensure partial configs work
	// We start with the current defaults
	cfg := DefaultConfig()

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	Current = cfg
	return nil
}
