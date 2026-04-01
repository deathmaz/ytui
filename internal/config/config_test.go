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

	if cfg.Player.Video.Command != "mpv" {
		t.Errorf("Player.Video.Command = %q, want %q", cfg.Player.Video.Command, "mpv")
	}
	if len(cfg.Player.Video.Args) != 1 || cfg.Player.Video.Args[0] != "--no-terminal" {
		t.Errorf("Player.Video.Args = %v, want [--no-terminal]", cfg.Player.Video.Args)
	}
	if cfg.Player.Music.Command != "" {
		t.Errorf("Player.Music.Command = %q, want empty (inherits from video)", cfg.Player.Music.Command)
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Player.Video.Command != "mpv" {
		t.Errorf("expected default player command, got %q", cfg.Player.Video.Command)
	}
}

func TestLoad_PartialOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "ytui")
	os.MkdirAll(cfgDir, 0755)

	content := `
[player.video]
command = "vlc"

[auth]
auth_on_startup = true
`
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0644)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Player.Video.Command != "vlc" {
		t.Errorf("Player.Video.Command = %q, want %q", cfg.Player.Video.Command, "vlc")
	}
	if !cfg.Auth.AuthOnStartup {
		t.Error("Auth.AuthOnStartup should be true")
	}
	if cfg.Download.Command != "yt-dlp" {
		t.Errorf("Download.Command = %q, want default %q", cfg.Download.Command, "yt-dlp")
	}
}

func TestLoad_VideoPlayerArgs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "ytui")
	os.MkdirAll(cfgDir, 0755)

	content := `
[player.video]
command = "mpv"
args = ["--ytdl-format=bestvideo[height<=1080]+bestaudio/best", "--no-terminal"]
`
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0644)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Player.Video.Args) != 2 {
		t.Fatalf("Player.Video.Args len = %d, want 2", len(cfg.Player.Video.Args))
	}
	if cfg.Player.Video.Args[0] != "--ytdl-format=bestvideo[height<=1080]+bestaudio/best" {
		t.Errorf("Player.Video.Args[0] = %q", cfg.Player.Video.Args[0])
	}
}

func TestLoad_MusicPlayerArgs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "ytui")
	os.MkdirAll(cfgDir, 0755)

	content := `
[player.video]
command = "mpv"
args = ["--no-terminal"]

[player.music]
args = ["--no-terminal", "--profile=music"]
`
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0644)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	// Video mode uses video args
	videoArgs := cfg.Player.EffectiveArgs(false)
	if len(videoArgs) != 1 || videoArgs[0] != "--no-terminal" {
		t.Errorf("EffectiveArgs(false) = %v, want [--no-terminal]", videoArgs)
	}

	// Music mode uses music args
	musicArgs := cfg.Player.EffectiveArgs(true)
	if len(musicArgs) != 2 || musicArgs[1] != "--profile=music" {
		t.Errorf("EffectiveArgs(true) = %v, want [--no-terminal --profile=music]", musicArgs)
	}

	// Music command falls back to video command
	if cfg.Player.EffectiveCommand(true) != "mpv" {
		t.Errorf("EffectiveCommand(true) = %q, want %q", cfg.Player.EffectiveCommand(true), "mpv")
	}
}

func TestEffective_FallbackToVideo(t *testing.T) {
	cfg := Default()

	// No music config — should fall back to video
	musicArgs := cfg.Player.EffectiveArgs(true)
	if len(musicArgs) != 1 || musicArgs[0] != "--no-terminal" {
		t.Errorf("EffectiveArgs(true) with no music args = %v, want [--no-terminal]", musicArgs)
	}
	if cfg.Player.EffectiveCommand(true) != "mpv" {
		t.Errorf("EffectiveCommand(true) = %q, want %q", cfg.Player.EffectiveCommand(true), "mpv")
	}
}

func TestLoad_MusicCustomCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "ytui")
	os.MkdirAll(cfgDir, 0755)

	content := `
[player.video]
command = "mpv"

[player.music]
command = "vlc"
args = ["--intf", "dummy"]
`
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0644)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Player.EffectiveCommand(false) != "mpv" {
		t.Errorf("EffectiveCommand(false) = %q, want %q", cfg.Player.EffectiveCommand(false), "mpv")
	}
	if cfg.Player.EffectiveCommand(true) != "vlc" {
		t.Errorf("EffectiveCommand(true) = %q, want %q", cfg.Player.EffectiveCommand(true), "vlc")
	}
}
