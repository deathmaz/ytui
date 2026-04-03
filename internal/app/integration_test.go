package app

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/youtube"
)

// TestVideoMode_AuthDoesNotLoadInactiveViews verifies that after auth,
// only the active view is loaded — not feed/subs in the background.
func TestVideoMode_AuthDoesNotLoadInactiveViews(t *testing.T) {
	client := &mockYTClient{
		authenticated: false,
		getFeedFn: func(_ context.Context, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "v1", Title: "Feed Video 1"}},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)

	// Start on search tab, feed should not be loading
	time.Sleep(200 * time.Millisecond)

	m := quitAndGetVideoModel(t, tm)
	if len(client.feedCalls) > 0 {
		t.Errorf("feed was loaded while on search tab; feedCalls=%d", len(client.feedCalls))
	}
	if m.activeView != ViewSearch {
		t.Errorf("expected active view to be search, got %d", m.activeView)
	}
}

// TestVideoMode_TabSwitchTriggersLoad verifies that switching to feed
// triggers loading, and switching back doesn't re-load.
func TestVideoMode_TabSwitchTriggersLoad(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getFeedFn: func(_ context.Context, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "v1", Title: "Feed Video 1"},
					{ID: "v2", Title: "Feed Video 2"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)

	// Switch to Feed tab
	sendKey(tm, "1")
	waitForContent(t, tm, "Feed Video")

	client.mu.Lock()
	initialCalls := len(client.feedCalls)
	client.mu.Unlock()

	// Switch away and back — should not trigger another load
	sendKey(tm, "3") // back to search (focuses input)
	time.Sleep(100 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEscape) // blur input
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "1") // back to feed
	time.Sleep(200 * time.Millisecond)

	m := quitAndGetVideoModel(t, tm)
	if m.activeView != ViewFeed {
		t.Errorf("expected active view to be feed, got %d", m.activeView)
	}
	client.mu.Lock()
	finalCalls := len(client.feedCalls)
	client.mu.Unlock()
	if finalCalls > initialCalls {
		t.Errorf("feed was re-loaded on second switch; calls before=%d after=%d", initialCalls, finalCalls)
	}
}

// TestMusicMode_TabSwitchTriggersLoad mirrors the video mode test for music.
func TestMusicMode_TabSwitchTriggersLoad(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		getHomeFn: func(_ context.Context) ([]youtube.MusicShelf, error) {
			return []youtube.MusicShelf{
				{Title: "Test Shelf", Items: []youtube.MusicItem{
					{Title: "Song 1", Type: youtube.MusicSong},
				}},
			}, nil
		},
	}
	tm := newTestMusicProgram(t, nil, mc)

	// Switch to Home tab
	sendKey(tm, "1")
	waitForContent(t, tm, "Test Shelf")

	mc.mu.Lock()
	initialCalls := mc.homeCalls
	mc.mu.Unlock()

	// Switch away and back
	sendKey(tm, "3") // search
	time.Sleep(100 * time.Millisecond)
	sendKey(tm, "1") // home again
	time.Sleep(200 * time.Millisecond)

	quitAndGetMusicModel(t, tm)
	mc.mu.Lock()
	finalCalls := mc.homeCalls
	mc.mu.Unlock()
	if finalCalls > initialCalls {
		t.Errorf("home was re-loaded on second switch; calls before=%d after=%d", initialCalls, finalCalls)
	}
}

// TestGlobalKeys_Parity verifies that shared global keys behave the same
// in both video and music modes.
func TestGlobalKeys_Parity(t *testing.T) {
	t.Run("help_toggle", func(t *testing.T) {
		vtm := newTestVideoProgram(t, nil)
		sendKey(vtm, "?")
		time.Sleep(100 * time.Millisecond)
		vm := quitAndGetVideoModel(t, vtm)

		mtm := newTestMusicProgram(t, nil, nil)
		sendKey(mtm, "?")
		time.Sleep(100 * time.Millisecond)
		mm := quitAndGetMusicModel(t, mtm)

		if vm.help.ShowAll != mm.help.ShowAll {
			t.Errorf("help toggle mismatch: video=%v music=%v", vm.help.ShowAll, mm.help.ShowAll)
		}
		if !vm.help.ShowAll {
			t.Error("expected help to be shown after pressing ?")
		}
	})

	t.Run("url_input", func(t *testing.T) {
		vtm := newTestVideoProgram(t, nil)
		sendKey(vtm, "O")
		waitForContent(t, vtm, "Open URL")
		// Cancel the modal before quitting
		sendSpecialKey(vtm, tea.KeyEscape)
		time.Sleep(100 * time.Millisecond)
		quitAndGetVideoModel(t, vtm) // just verify it doesn't crash

		mtm := newTestMusicProgram(t, nil, nil)
		sendKey(mtm, "O")
		waitForContent(t, mtm, "Open URL")
		sendSpecialKey(mtm, tea.KeyEscape)
		time.Sleep(100 * time.Millisecond)
		quitAndGetMusicModel(t, mtm)
	})

	t.Run("quit", func(t *testing.T) {
		vtm := newTestVideoProgram(t, nil)
		sendKey(vtm, "q")
		vtm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

		mtm := newTestMusicProgram(t, nil, nil)
		sendKey(mtm, "q")
		mtm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
	})
}

// TestVideoMode_TabLifecycle tests opening and closing video tabs.
func TestVideoMode_TabLifecycle(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			return &youtube.Video{
				ID:    id,
				Title: "Video " + id,
				URL:   "https://www.youtube.com/watch?v=" + id,
			}, nil
		},
		getCommentsFn: func(_ context.Context, videoID, token string) (*youtube.Page[youtube.Comment], error) {
			return &youtube.Page[youtube.Comment]{}, nil
		},
	}
	tm := newTestVideoProgram(t, client)

	// Open a video tab via VideoSelectedMsg
	tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "vid1", Title: "Test Video 1"}})
	waitForContent(t, tm, "Test Video 1")

	// Open a second tab
	tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "vid2", Title: "Test Video 2"}})
	waitForContent(t, tm, "Test Video 2")

	// Close with Esc
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(200 * time.Millisecond)

	result := quitAndGetVideoModel(t, tm)
	if result.videoTabs.Len() > 1 {
		t.Errorf("expected at most 1 tab after closing, got %d", result.videoTabs.Len())
	}
}

// TestVideoMode_StatusLifecycle tests the StatusManager behavior.
func TestVideoMode_StatusLifecycle(t *testing.T) {
	tm := newTestVideoProgram(t, &mockYTClient{authenticated: true})
	time.Sleep(100 * time.Millisecond)

	// Stale clear should be ignored
	tm.Send(clearStatusMsg{seq: 999})
	time.Sleep(100 * time.Millisecond)

	m := quitAndGetVideoModel(t, tm)
	if m.status.Msg != "" {
		t.Errorf("expected empty status, got %q", m.status.Msg)
	}
}

// TestVideoMode_SearchRender verifies the search view renders results.
func TestVideoMode_SearchRender(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "v1", Title: "Golang Tutorial", ChannelName: "Go Channel"},
					{ID: "v2", Title: "Rust Tutorial", ChannelName: "Rust Channel"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "tutorial"})
	waitForContent(t, tm, "Golang Tutorial")
	quitAndGetVideoModel(t, tm)
}

func TestMusicMode_SearchRender(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
			return &youtube.MusicSearchResult{
				Shelves: []youtube.MusicShelf{
					{Title: "Songs", Items: []youtube.MusicItem{
						{Title: "Test Song", Subtitle: "Test Artist", Type: youtube.MusicSong},
					}},
				},
			}, nil
		},
	}
	tm := newTestMusicProgramWithOpts(t, nil, mc, nil, Options{SearchQuery: "test"})
	waitForContent(t, tm, "Test Song")
	quitAndGetMusicModel(t, tm)
}

// TestBothModes_InitialViewIsSearch verifies both modes start on search.
func TestBothModes_InitialViewIsSearch(t *testing.T) {
	vtm := newTestVideoProgram(t, nil)
	waitForContent(t, vtm, "Search")
	vm := quitAndGetVideoModel(t, vtm)
	if vm.activeView != ViewSearch {
		t.Errorf("video mode: expected ViewSearch, got %d", vm.activeView)
	}

	mtm := newTestMusicProgram(t, nil, nil)
	waitForContent(t, mtm, "Search")
	mm := quitAndGetMusicModel(t, mtm)
	if !mm.onFixedView || mm.activeFixed != musicViewSearch {
		t.Errorf("music mode: expected search view, got onFixed=%v activeFixed=%d", mm.onFixedView, mm.activeFixed)
	}
}
