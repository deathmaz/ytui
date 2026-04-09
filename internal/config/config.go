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
	Thumbnails ThumbnailConfig `toml:"thumbnails"`
}

// GeneralConfig holds general settings.
type GeneralConfig struct {
	Mode        string `toml:"mode"`         // "video" (default) or "music"
	RestoreTabs bool   `toml:"restore_tabs"` // persist and restore open tabs across sessions
}

// PlayerConfig configures playback for video and music modes.
type PlayerConfig struct {
	Video VideoPlayerConfig `toml:"video"`
	Music MusicPlayerConfig `toml:"music"`
}

// VideoPlayerConfig configures the player for video mode.
type VideoPlayerConfig struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	Quality string   `toml:"quality"` // default quality (e.g., "1080", "720", "best", "audio")
}

// MusicPlayerConfig configures the player for music mode.
type MusicPlayerConfig struct {
	Command string   `toml:"command"` // defaults to player.video.command if empty
	Args    []string `toml:"args"`    // defaults to player.video.args if empty
}

// EffectiveCommand returns the music command if set, otherwise the video command.
func (p PlayerConfig) EffectiveCommand(music bool) string {
	if music && p.Music.Command != "" {
		return p.Music.Command
	}
	return p.Video.Command
}

// EffectiveArgs returns the music args if set, otherwise the video args.
func (p PlayerConfig) EffectiveArgs(music bool) []string {
	if music && len(p.Music.Args) > 0 {
		return p.Music.Args
	}
	return p.Video.Args
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

// ThumbnailConfig configures thumbnail display in lists (requires Kitty terminal).
type ThumbnailConfig struct {
	Enabled bool `toml:"enabled"`          // show thumbnails in lists
	Height  int  `toml:"height"`           // thumbnail height in terminal rows (default: 5)
}

// Default returns the default configuration.
func Default() *Config {
	return &Config{
		General: GeneralConfig{
			Mode: "video",
		},
		Player: PlayerConfig{
			Video: VideoPlayerConfig{
				Command: "mpv",
				Args:    []string{"--no-terminal"},
			},
		},
		Download: DownloadConfig{
			Command:   "yt-dlp",
			OutputDir: "~/Videos/ytui",
		},
		Auth: AuthConfig{
			Browser:       "brave",
			AuthOnStartup: false,
		},
		Thumbnails: ThumbnailConfig{
			Height: 5,
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
