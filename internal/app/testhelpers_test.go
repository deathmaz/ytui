package app

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/config"
	"github.com/deathmaz/ytui/internal/youtube"
)

// videoFactory builds a getVideoFn that returns a Video titled
// "{titlePrefix} {id}" with a canonical URL. Counter, if non-nil, is
// incremented per call — used by tests asserting the API was hit.
func videoFactory(titlePrefix string, counter *atomic.Int32) func(context.Context, string) (*youtube.Video, error) {
	return func(_ context.Context, id string) (*youtube.Video, error) {
		if counter != nil {
			counter.Add(1)
		}
		return &youtube.Video{
			ID:    id,
			Title: titlePrefix + " " + id,
			URL:   "https://www.youtube.com/watch?v=" + id,
		}, nil
	}
}

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
	for _, r := range key {
		tm.Send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func sendSpecialKey(tm *teatest.TestModel, code rune) {
	tm.Send(tea.KeyPressMsg{Code: code})
}

func waitForContent(t *testing.T, tm *teatest.TestModel, substring string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), substring)
	}, teatest.WithDuration(3*time.Second))
}

// waitForCounter polls an atomic counter until it reaches target or the
// deadline elapses. Useful where the Cursed Renderer emits only cell diffs,
// so the expected string may never appear as contiguous bytes in the output
// stream even though the corresponding action fired.
func waitForCounter(t *testing.T, c *atomic.Int32, target int32, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.Load() >= target {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("counter did not reach %d within %s (got %d)", target, timeout, c.Load())
}

func quitAndGetVideoModel(t *testing.T, tm *teatest.TestModel) *Model {
	t.Helper()
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	fm := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	m, ok := fm.(*Model)
	if !ok {
		t.Fatalf("expected *Model, got %T", fm)
	}
	return m
}

func quitAndGetMusicModel(t *testing.T, tm *teatest.TestModel) *MusicModel {
	t.Helper()
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	fm := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	m, ok := fm.(*MusicModel)
	if !ok {
		t.Fatalf("expected *MusicModel, got %T", fm)
	}
	return m
}
