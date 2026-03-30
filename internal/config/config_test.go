package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDir_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/fake_xdg")
	got := Dir()
	if got != "/tmp/fake_xdg/ytui" {
		t.Errorf("Dir() = %q, want %q", got, "/tmp/fake_xdg/ytui")
	}
}

func TestDir_FallbackHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	got := Dir()
	if got == "" {
		t.Error("Dir() should not be empty when HOME is set")
	}
	if !strings.HasSuffix(got, "/.config/ytui") {
		t.Errorf("Dir() = %q, want suffix %q", got, "/.config/ytui")
	}
}

func TestPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/fake_xdg")
	got := Path()
	if got != "/tmp/fake_xdg/ytui/config.toml" {
		t.Errorf("Path() = %q, want %q", got, "/tmp/fake_xdg/ytui/config.toml")
	}
}

func TestLoad_InvalidToml(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "ytui")
	os.MkdirAll(cfgDir, 0755)
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("not valid [[ toml"), 0644)

	_, err := Load()
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Player.Command != "mpv" {
		t.Errorf("Player.Command = %q, want %q", cfg.Player.Command, "mpv")
	}
	if len(cfg.Player.Args) != 1 || cfg.Player.Args[0] != "--no-terminal" {
		t.Errorf("Player.Args = %v, want [--no-terminal]", cfg.Player.Args)
	}
	if cfg.Download.Command != "yt-dlp" {
		t.Errorf("Download.Command = %q, want %q", cfg.Download.Command, "yt-dlp")
	}
	if cfg.Download.OutputDir != "~/Videos/ytui" {
		t.Errorf("Download.OutputDir = %q, want %q", cfg.Download.OutputDir, "~/Videos/ytui")
	}
	if cfg.Auth.Browser != "brave" {
		t.Errorf("Auth.Browser = %q, want %q", cfg.Auth.Browser, "brave")
	}
	if cfg.Auth.AuthOnStartup {
		t.Error("Auth.AuthOnStartup should default to false")
	}
}

func TestLoad_NoFile(t *testing.T) {
	// With no config file, should return defaults
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Player.Command != "mpv" {
		t.Errorf("expected default player command, got %q", cfg.Player.Command)
	}
}

func TestLoad_PartialOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "ytui")
	os.MkdirAll(cfgDir, 0755)

	content := `
[player]
command = "vlc"

[auth]
auth_on_startup = true
`
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0644)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	// Overridden values
	if cfg.Player.Command != "vlc" {
		t.Errorf("Player.Command = %q, want %q", cfg.Player.Command, "vlc")
	}
	if !cfg.Auth.AuthOnStartup {
		t.Error("Auth.AuthOnStartup should be true")
	}

	// Non-overridden values keep defaults
	if cfg.Download.Command != "yt-dlp" {
		t.Errorf("Download.Command = %q, want default %q", cfg.Download.Command, "yt-dlp")
	}
	if cfg.Auth.Browser != "brave" {
		t.Errorf("Auth.Browser = %q, want default %q", cfg.Auth.Browser, "brave")
	}
}

func TestLoad_PlayerArgs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "ytui")
	os.MkdirAll(cfgDir, 0755)

	content := `
[player]
command = "mpv"
args = ["--ytdl-format=bestvideo[height<=1080]+bestaudio/best", "--no-terminal"]
`
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0644)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Player.Args) != 2 {
		t.Fatalf("Player.Args len = %d, want 2", len(cfg.Player.Args))
	}
	if cfg.Player.Args[0] != "--ytdl-format=bestvideo[height<=1080]+bestaudio/best" {
		t.Errorf("Player.Args[0] = %q", cfg.Player.Args[0])
	}
}
