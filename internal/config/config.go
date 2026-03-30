package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all ytui configuration.
type Config struct {
	General  GeneralConfig  `toml:"general"`
	Player   PlayerConfig   `toml:"player"`
	Download DownloadConfig `toml:"download"`
	Auth     AuthConfig     `toml:"auth"`
}

// GeneralConfig holds general settings.
type GeneralConfig struct {
	Mode string `toml:"mode"` // "video" (default) or "music"
}

// PlayerConfig configures the video player.
type PlayerConfig struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	Quality string   `toml:"quality"` // default quality (e.g., "1080", "720", "best", "audio")
}

// DownloadConfig configures video downloads.
type DownloadConfig struct {
	Command   string `toml:"command"`
	OutputDir string `toml:"output_dir"`
	Format    string `toml:"format"`
}

// AuthConfig configures authentication.
type AuthConfig struct {
	Browser       string `toml:"browser"`
	AuthOnStartup bool   `toml:"auth_on_startup"`
}

// Default returns the default configuration.
func Default() *Config {
	return &Config{
		General: GeneralConfig{
			Mode: "video",
		},
		Player: PlayerConfig{
			Command: "mpv",
			Args:    []string{"--no-terminal"},
		},
		Download: DownloadConfig{
			Command:   "yt-dlp",
			OutputDir: "~/Videos/ytui",
		},
		Auth: AuthConfig{
			Browser:       "brave",
			AuthOnStartup: false,
		},
	}
}

// Load reads the config file, falling back to defaults for missing values.
func Load() (*Config, error) {
	cfg := Default()

	path := Path()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Dir returns the ytui config directory.
func Dir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "ytui")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "ytui")
}

// Path returns the path to the config file.
func Path() string {
	d := Dir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "config.toml")
}
