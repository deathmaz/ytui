package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Player: config.PlayerConfig{
			Video: config.VideoPlayerConfig{
				Command: "echo",
				Quality: "best",
			},
		},
		Download: config.DownloadConfig{
			Command: "echo",
		},
		Auth: config.AuthConfig{
			Browser: "brave",
		},
	}
}

func newTestVideoProgram(t *testing.T, client *mockYTClient) *teatest.TestModel {
	t.Helper()
	return newTestVideoProgramWithOpts(t, client, Options{})
}

func newTestVideoProgramWithOpts(t *testing.T, client *mockYTClient, opts Options) *teatest.TestModel {
	t.Helper()
	if client == nil {
		client = &mockYTClient{authenticated: true}
	}
	cfg := testConfig()
	m := New(client, cfg, opts)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(50 * time.Millisecond)
	return tm
}

func newTestMusicProgram(t *testing.T, ytClient *mockYTClient, musicClient *mockMusicClient) *teatest.TestModel {
	t.Helper()
	return newTestMusicProgramWithOpts(t, ytClient, musicClient, nil, Options{})
}

func newTestMusicProgramWithOpts(t *testing.T, ytClient *mockYTClient, musicClient *mockMusicClient, imgR *ytimage.Renderer, opts Options) *teatest.TestModel {
	t.Helper()
	if ytClient == nil {
		ytClient = &mockYTClient{authenticated: true}
	}
	if musicClient == nil {
		musicClient = &mockMusicClient{authenticated: true}
	}
	cfg := testConfig()
	m := NewMusic(musicClient, ytClient, cfg, imgR, opts)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(50 * time.Millisecond)
	return tm
}

func sendKey(tm *teatest.TestModel, key string) {
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(key),
	})
}

func sendSpecialKey(tm *teatest.TestModel, keyType tea.KeyType) {
	tm.Send(tea.KeyMsg{Type: keyType})
}

func waitForContent(t *testing.T, tm *teatest.TestModel, substring string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), substring)
	}, teatest.WithDuration(3*time.Second))
}

func quitAndGetVideoModel(t *testing.T, tm *teatest.TestModel) *Model {
	t.Helper()
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	fm := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	m, ok := fm.(*Model)
	if !ok {
		t.Fatalf("expected *Model, got %T", fm)
	}
	return m
}

func quitAndGetMusicModel(t *testing.T, tm *teatest.TestModel) *MusicModel {
	t.Helper()
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	fm := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	m, ok := fm.(*MusicModel)
	if !ok {
		t.Fatalf("expected *MusicModel, got %T", fm)
	}
	return m
}
