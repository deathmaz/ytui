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
	return newTestVideoProgramFull(t, client, nil, opts)
}

// newTestVideoProgramFull creates a video-mode test program with optional
// pre-loaded dummy thumbnails. Pass video IDs to thumbIDs to pre-populate
// the renderer cache, preventing real network fetches in golden tests.
func newTestVideoProgramFull(t *testing.T, client *mockYTClient, cfg *config.Config, opts Options, thumbIDs ...string) *teatest.TestModel {
	t.Helper()
	if client == nil {
		client = &mockYTClient{authenticated: true}
	}
	if cfg == nil {
		cfg = testConfig()
	}
	m := New(client, cfg, opts)
	for _, id := range thumbIDs {
		preloadDummyThumb(m.imgR, id)
	}
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(50 * time.Millisecond)
	return tm
}

// preloadDummyThumb stores a deterministic dummy thumbnail in the renderer
// cache so the detail view finds a cache hit instead of making a network request.
func preloadDummyThumb(imgR *ytimage.Renderer, videoID string) {
	url := "https://i.ytimg.com/vi/" + videoID + "/hqdefault.jpg"
	placeholder := makeDummyPlaceholder(40, 10)
	imgR.Store(url, "", placeholder)
}

// makeDummyPlaceholder creates a simple text placeholder grid of the given size.
func makeDummyPlaceholder(cols, rows int) string {
	var b strings.Builder
	for row := 0; row < rows; row++ {
		b.WriteString(strings.Repeat(" ", cols))
		if row < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func newTestMusicProgram(t *testing.T, ytClient *mockYTClient, musicClient *mockMusicClient) *teatest.TestModel {
	t.Helper()
	return newTestMusicProgramWithOpts(t, ytClient, musicClient, nil, Options{})
}

func newTestMusicProgramWithOpts(t *testing.T, ytClient *mockYTClient, musicClient *mockMusicClient, imgR *ytimage.Renderer, opts Options) *teatest.TestModel {
	t.Helper()
	return newTestMusicProgramFull(t, ytClient, musicClient, imgR, nil, opts)
}

func newTestMusicProgramFull(t *testing.T, ytClient *mockYTClient, musicClient *mockMusicClient, imgR *ytimage.Renderer, cfg *config.Config, opts Options) *teatest.TestModel {
	t.Helper()
	if ytClient == nil {
		ytClient = &mockYTClient{authenticated: true}
	}
	if musicClient == nil {
		musicClient = &mockMusicClient{authenticated: true}
	}
	if cfg == nil {
		cfg = testConfig()
	}
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
