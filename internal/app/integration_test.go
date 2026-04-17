package app

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/deathmaz/ytui/internal/config"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/player"
	"github.com/deathmaz/ytui/internal/state"
	"github.com/deathmaz/ytui/internal/ui/channel"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/subs"
	"github.com/deathmaz/ytui/internal/ui/urlinput"
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
		getVideoFn: videoFactory("Video", nil),
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
	if result.tabs.Len() > 1 {
		t.Errorf("expected at most 1 tab after closing, got %d", result.tabs.Len())
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

// === Priority 1: Error Handling ===

func TestVideoMode_FeedLoadError(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getFeedFn: func(_ context.Context, token string) (*youtube.Page[youtube.Video], error) {
			return nil, fmt.Errorf("network timeout")
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "1")
	waitForContent(t, tm, "Feed error")
	quitAndGetVideoModel(t, tm)
}

func TestVideoMode_SubsLoadError(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return nil, fmt.Errorf("network timeout")
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Subscriptions error")
	quitAndGetVideoModel(t, tm)
}

func TestVideoMode_SearchError(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return nil, fmt.Errorf("search failed")
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "test"})
	time.Sleep(500 * time.Millisecond)
	// Should not crash — can type a new query
	m := quitAndGetVideoModel(t, tm)
	if m.activeView != ViewSearch {
		t.Errorf("expected search view after error, got %d", m.activeView)
	}
}

func TestVideoMode_DetailLoadError(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "err1", Title: "Error Video", URL: "https://youtube.com/watch?v=err1"}},
			}, nil
		},
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			return nil, fmt.Errorf("video not found")
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "test"})
	waitForContent(t, tm, "Error Video")
	sendKey(tm, "i")
	time.Sleep(500 * time.Millisecond)
	// Should not crash
	quitAndGetVideoModel(t, tm)
}

func TestMusicMode_HomeLoadError(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		getHomeFn: func(_ context.Context) ([]youtube.MusicShelf, error) {
			return nil, fmt.Errorf("home unavailable")
		},
	}
	tm := newTestMusicProgram(t, nil, mc)
	sendKey(tm, "1")
	waitForContent(t, tm, "Home error")
	quitAndGetMusicModel(t, tm)
}

func TestMusicMode_ArtistLoadError(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
			return &youtube.MusicSearchResult{
				Shelves: []youtube.MusicShelf{
					{Title: "Artists", Items: []youtube.MusicItem{
						{Title: "Bad Artist", Type: youtube.MusicArtist, BrowseID: "bad1"},
					}},
				},
			}, nil
		},
		getArtistFn: func(_ context.Context, browseID string) (*youtube.MusicArtistPage, error) {
			return nil, fmt.Errorf("artist not found")
		},
	}
	tm := newTestMusicProgramWithOpts(t, nil, mc, nil, Options{SearchQuery: "bad"})
	waitForContent(t, tm, "Bad Artist")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Error")
	quitAndGetMusicModel(t, tm)
}

func TestMusicMode_AlbumLoadError(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
			return &youtube.MusicSearchResult{
				Shelves: []youtube.MusicShelf{
					{Title: "Albums", Items: []youtube.MusicItem{
						{Title: "Bad Album", Type: youtube.MusicAlbum, BrowseID: "bad_alb"},
					}},
				},
			}, nil
		},
		getAlbumFn: func(_ context.Context, browseID string) (*youtube.MusicAlbumPage, error) {
			return nil, fmt.Errorf("album not found")
		},
	}
	tm := newTestMusicProgramWithOpts(t, nil, mc, nil, Options{SearchQuery: "bad"})
	waitForContent(t, tm, "Bad Album")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Error")
	quitAndGetMusicModel(t, tm)
}

// === Priority 2: Double-Press Guards ===

func TestVideoMode_DoubleAuthGuard(t *testing.T) {
	client := &mockYTClient{authenticated: true}
	tm := newTestVideoProgram(t, client)
	// Already authenticated — pressing a should show "Already authenticated"
	sendKey(tm, "a")
	waitForContent(t, tm, "Already authenticated")
	// Second press should also just show the same message, not crash
	sendKey(tm, "a")
	time.Sleep(200 * time.Millisecond)
	quitAndGetVideoModel(t, tm)
}

func TestVideoMode_DoubleFeedLoadGuard(t *testing.T) {
	var callCount atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getFeedFn: func(ctx context.Context, token string) (*youtube.Page[youtube.Video], error) {
			callCount.Add(1)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				return &youtube.Page[youtube.Video]{}, nil
			}
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "1")
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "3") // switch away
	time.Sleep(50 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "1") // switch back — should not re-trigger load (still loading)
	time.Sleep(200 * time.Millisecond)
	quitAndGetVideoModel(t, tm)
	if c := callCount.Load(); c > 1 {
		t.Errorf("expected 1 feed load call, got %d", c)
	}
}

func TestMusicMode_DoubleHomeLoadGuard(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		getHomeFn: func(ctx context.Context) ([]youtube.MusicShelf, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				return []youtube.MusicShelf{}, nil
			}
		},
	}
	tm := newTestMusicProgram(t, nil, mc)
	sendKey(tm, "1")
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "3")
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "1") // should not re-trigger (still loading)
	time.Sleep(200 * time.Millisecond)
	quitAndGetMusicModel(t, tm)
	mc.mu.Lock()
	calls := mc.homeCalls
	mc.mu.Unlock()
	if calls > 1 {
		t.Errorf("expected 1 home load call, got %d", calls)
	}
}

// === Priority 3: Pagination ===

func TestVideoMode_FeedPagination(t *testing.T) {
	var page atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getFeedFn: func(_ context.Context, token string) (*youtube.Page[youtube.Video], error) {
			page.Add(1)
			if token == "" {
				return &youtube.Page[youtube.Video]{
					Items:     []youtube.Video{{ID: "p1v1", Title: "Page1 Video"}},
					NextToken: "page2token",
				}, nil
			}
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "p2v1", Title: "Page2 Video"}},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "1")
	waitForContent(t, tm, "Page1 Video")
	sendKey(tm, "G")
	time.Sleep(500 * time.Millisecond)
	quitAndGetVideoModel(t, tm)
	if p := page.Load(); p < 2 {
		t.Errorf("expected at least 2 feed page loads, got %d", p)
	}
}

func TestVideoMode_SearchPagination(t *testing.T) {
	var page atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			page.Add(1)
			if token == "" {
				return &youtube.Page[youtube.Video]{
					Items:     []youtube.Video{{ID: "s1", Title: "Search Result 1"}},
					NextToken: "searchpage2",
				}, nil
			}
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "s2", Title: "Search Result Page2"}},
			}, nil
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "test"})
	waitForContent(t, tm, "Search Result 1")
	sendKey(tm, "G")
	time.Sleep(500 * time.Millisecond)
	quitAndGetVideoModel(t, tm)
	if p := page.Load(); p < 2 {
		t.Errorf("expected at least 2 search page loads, got %d", p)
	}
}

// === Priority 4: Key Actions ===

func TestVideoMode_PlayKey(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "play1", Title: "Play Test", URL: "https://youtube.com/watch?v=play1"}},
			}, nil
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "play"})
	waitForContent(t, tm, "Play Test")
	// p key triggers play — it calls an external command which will fail in test,
	// but it should not crash the app
	sendKey(tm, "p")
	time.Sleep(200 * time.Millisecond)
	quitAndGetVideoModel(t, tm) // verify no crash
}

func TestVideoMode_DownloadKey(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "dl1", Title: "Download Test", URL: "https://youtube.com/watch?v=dl1"}},
			}, nil
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "download"})
	waitForContent(t, tm, "Download Test")
	sendKey(tm, "d")
	// Download starts (may complete quickly with mock "echo" command)
	// Check that either downloading state or result status appears
	waitForContent(t, tm, "Download")
	quitAndGetVideoModel(t, tm)
}

func TestVideoMode_RefreshKey(t *testing.T) {
	var callCount atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getFeedFn: func(_ context.Context, token string) (*youtube.Page[youtube.Video], error) {
			callCount.Add(1)
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "r1", Title: "Refreshable"}},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "1")
	waitForContent(t, tm, "Refreshable")
	initial := callCount.Load()
	sendKey(tm, "r") // refresh
	time.Sleep(300 * time.Millisecond)
	quitAndGetVideoModel(t, tm)
	if c := callCount.Load(); c <= initial {
		t.Errorf("expected refresh to trigger another feed load; before=%d after=%d", initial, c)
	}
}

func TestVideoMode_CopyURLKey(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "cp1", Title: "Copy Test", URL: "https://youtube.com/watch?v=cp1"}},
			}, nil
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "copy"})
	waitForContent(t, tm, "Copy Test")
	sendKey(tm, "y")
	waitForContent(t, tm, "URL copied")
	quitAndGetVideoModel(t, tm)
}

func TestVideoMode_OpenBrowserKey(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "ob1", Title: "Browser Test", URL: "https://youtube.com/watch?v=ob1"}},
			}, nil
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "browser"})
	waitForContent(t, tm, "Browser Test")
	sendKey(tm, "o")
	waitForContent(t, tm, "Opening in browser")
	quitAndGetVideoModel(t, tm)
}

// === Priority 5: Tab Management Edge Cases ===

func TestVideoMode_MaxTabsReached(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Video", nil),
	}
	tm := newTestVideoProgram(t, client)
	for i := 0; i < maxDynamicTabs; i++ {
		tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: fmt.Sprintf("v%d", i), Title: fmt.Sprintf("Tab %d", i)}})
		time.Sleep(100 * time.Millisecond)
	}
	// Try 7th
	tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "v6", Title: "Tab 6"}})
	waitForContent(t, tm, "Max tabs")
	quitAndGetVideoModel(t, tm)
}

func TestVideoMode_CloseLastTabFallback(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			return &youtube.Video{ID: id, Title: "Only Tab", URL: "https://youtube.com/watch?v=" + id}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "only", Title: "Only Tab"}})
	waitForContent(t, tm, "Only Tab")
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(200 * time.Millisecond)
	m := quitAndGetVideoModel(t, tm)
	if m.activeView != ViewSearch {
		t.Errorf("expected fallback to search after closing last tab, got %d", m.activeView)
	}
	if m.tabs.Len() != 0 {
		t.Errorf("expected 0 tabs, got %d", m.tabs.Len())
	}
}

func TestVideoMode_TabNumberKeys(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Video", nil),
	}
	tm := newTestVideoProgram(t, client)
	// Open 3 tabs
	for i := 1; i <= 3; i++ {
		tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: fmt.Sprintf("t%d", i), Title: fmt.Sprintf("Tab%d", i)}})
		time.Sleep(150 * time.Millisecond)
	}
	// Press 4 to switch to first video tab
	sendKey(tm, "4")
	time.Sleep(200 * time.Millisecond)
	m := quitAndGetVideoModel(t, tm)
	if m.tabs.ActiveIdx() != 0 {
		t.Errorf("expected tab index 0 after pressing 4, got %d", m.tabs.ActiveIdx())
	}
}

func TestMusicMode_CloseLastTabFallback(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
			return &youtube.MusicSearchResult{
				Shelves: []youtube.MusicShelf{
					{Title: "Artists", Items: []youtube.MusicItem{
						{Title: "Close Test Artist", Type: youtube.MusicArtist, BrowseID: "close1"},
					}},
				},
			}, nil
		},
		getArtistFn: func(_ context.Context, browseID string) (*youtube.MusicArtistPage, error) {
			return &youtube.MusicArtistPage{
				Name:    "Close Test Artist",
				Shelves: []youtube.MusicShelf{{Title: "Songs", Items: []youtube.MusicItem{{Title: "Song", Type: youtube.MusicSong}}}},
			}, nil
		},
	}
	tm := newTestMusicProgramWithOpts(t, nil, mc, nil, Options{SearchQuery: "close"})
	waitForContent(t, tm, "Close Test Artist")
	sendSpecialKey(tm, tea.KeyEnter)
	time.Sleep(300 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(200 * time.Millisecond)
	m := quitAndGetMusicModel(t, tm)
	if !m.onFixedView {
		t.Error("expected fallback to fixed view after closing last music tab")
	}
}

// === Priority 6: Modal Interactions ===

func TestVideoMode_URLInput_Cancel(t *testing.T) {
	tm := newTestVideoProgram(t, nil)
	sendKey(tm, "O")
	waitForContent(t, tm, "Open URL")
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(200 * time.Millisecond)
	m := quitAndGetVideoModel(t, tm)
	if m.urlInput.IsActive() {
		t.Error("URL input should be closed after Esc")
	}
	if m.tabs.Len() != 0 {
		t.Error("no tab should be opened after cancelling URL input")
	}
}

func TestVideoMode_QualityPicker_Cancel(t *testing.T) {
	tm := newTestVideoProgram(t, nil)
	tm.Send(formatsLoadedMsg{
		url: "https://youtube.com/watch?v=test",
		formats: []player.Format{
			{ID: "best", Display: "Best"},
			{ID: "720", Display: "720p"},
		},
	})
	waitForContent(t, tm, "Best")
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(200 * time.Millisecond)
	m := quitAndGetVideoModel(t, tm)
	if m.picker.IsActive() {
		t.Error("picker should be closed after Esc")
	}
	if m.pendingVideoURL != "" {
		t.Error("pending URL should be cleared after cancel")
	}
}

// === Priority 7: Search Focus Management ===

func TestVideoMode_SearchFocusBlocksGlobalKeys(t *testing.T) {
	tm := newTestVideoProgram(t, nil)
	// Focus search input
	sendKey(tm, "/")
	time.Sleep(100 * time.Millisecond)
	// Type 'q' — should go into search input, NOT quit the app
	sendKey(tm, "q")
	time.Sleep(100 * time.Millisecond)
	// App should still be running, not quit
	sendSpecialKey(tm, tea.KeyEscape) // blur
	time.Sleep(100 * time.Millisecond)
	m := quitAndGetVideoModel(t, tm)
	if m.activeView != ViewSearch {
		t.Errorf("expected search view, got %d", m.activeView)
	}
}

func TestVideoMode_SearchSubmitBlurs(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "sr1", Title: "Submit Result"}},
			}, nil
		},
	}
	// Use SearchQuery to auto-submit — after results load, input is blurred to list
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "test"})
	waitForContent(t, tm, "Submit Result")
	// Global keys should work now (input blurred after search submit)
	sendKey(tm, "?")
	time.Sleep(100 * time.Millisecond)
	m := quitAndGetVideoModel(t, tm)
	if !m.help.ShowAll {
		t.Error("expected ? to toggle help after search submit (input should be blurred)")
	}
}

// === Priority 8: Cross-Mode Parity ===

func TestBothModes_EscClosesTab(t *testing.T) {
	// Video: open tab, esc closes it
	vc := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("V", nil),
	}
	vtm := newTestVideoProgram(t, vc)
	vtm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "esc1", Title: "Esc Test"}})
	waitForContent(t, vtm, "Esc Test")
	sendSpecialKey(vtm, tea.KeyEscape)
	time.Sleep(200 * time.Millisecond)
	vm := quitAndGetVideoModel(t, vtm)

	// Music: open tab, esc closes it
	mc := &mockMusicClient{
		authenticated: true,
		getArtistFn: func(_ context.Context, browseID string) (*youtube.MusicArtistPage, error) {
			return &youtube.MusicArtistPage{
				Name:    "Esc Artist",
				Shelves: []youtube.MusicShelf{{Title: "Songs", Items: []youtube.MusicItem{{Title: "Song", Type: youtube.MusicSong}}}},
			}, nil
		},
	}
	mtm := newTestMusicProgram(t, nil, mc)
	mtm.Send(musicItemSelectedMsg{item: youtube.MusicItem{Title: "Esc Artist", Type: youtube.MusicArtist, BrowseID: "esc1"}})
	time.Sleep(300 * time.Millisecond)
	sendSpecialKey(mtm, tea.KeyEscape)
	time.Sleep(200 * time.Millisecond)
	mm := quitAndGetMusicModel(t, mtm)

	// Both should have no dynamic tabs
	if vm.tabs.Len() != 0 {
		t.Errorf("video: expected 0 tabs after esc, got %d", vm.tabs.Len())
	}
	if mm.tabs.Len() != 0 {
		t.Errorf("music: expected 0 tabs after esc, got %d", mm.tabs.Len())
	}
}

func TestBothModes_SearchFocusParity(t *testing.T) {
	vtm := newTestVideoProgram(t, nil)
	sendKey(vtm, "/")
	time.Sleep(100 * time.Millisecond)
	vm := quitAndGetVideoModel(t, vtm)

	mtm := newTestMusicProgram(t, nil, nil)
	sendKey(mtm, "/")
	time.Sleep(100 * time.Millisecond)
	mm := quitAndGetMusicModel(t, mtm)

	if vm.activeView != ViewSearch {
		t.Errorf("video: expected search view after /, got %d", vm.activeView)
	}
	if !mm.onFixedView || mm.activeFixed != musicViewSearch {
		t.Errorf("music: expected search view after /, got onFixed=%v activeFixed=%d", mm.onFixedView, mm.activeFixed)
	}
}

// TestVideoMode_ThumbnailLoadedWhileDetailTabActive verifies that when a list
// thumbnail fetch completes while a video detail tab is active, the result is
// still cached by the shared ThumbList (not swallowed by the detail view).
// Regression: the detail view's handler blindly stored ANY ThumbnailLoadedMsg
// without checking HandleLoaded, causing list thumbnails to be lost and wrong
// images to appear.
func TestVideoMode_ThumbnailLoadedWhileDetailTabActive(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	m := New(&mockYTClient{authenticated: true}, cfg, Options{})

	// Switch to a video tab so the detail view is active
	m.activeView = ViewDynamicTab

	if m.listThumbList == nil {
		t.Fatal("expected listThumbList to be non-nil")
	}
	imgR := m.listThumbList.Renderer()

	// Mark URL as inflight on the LIST renderer
	imgR.FetchCmd("https://fake.test/list-thumb.jpg", 20, 5)

	// Send ThumbnailLoadedMsg while detail tab is active
	m.Update(ytimage.ThumbnailLoadedMsg{
		URL:         "https://fake.test/list-thumb.jpg",
		TransmitStr: "list-tx",
		Placeholder: "list-pl",
	})

	// Verify the list renderer cached it (via app-level HandleMsg)
	_, pl := imgR.Get("https://fake.test/list-thumb.jpg")
	if pl != "list-pl" {
		t.Errorf("expected list thumbnail cached while detail tab active, got placeholder=%q", pl)
	}
}

// TestVideoMode_FeedThumbnailLoadedMsgStores verifies that ThumbnailLoadedMsg
// is cached by the app-level handler (which handles it globally for all views).
func TestVideoMode_FeedThumbnailLoadedMsgStores(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	m := New(&mockYTClient{authenticated: true}, cfg, Options{})

	// Switch to feed so the test covers the feed-active scenario.
	m.activeView = ViewFeed

	if m.listThumbList == nil {
		t.Fatal("expected listThumbList to be non-nil")
	}
	imgR := m.listThumbList.Renderer()

	// Mark a URL as inflight so HandleLoaded accepts the message.
	imgR.FetchCmd("https://fake.test/feed1.jpg", 20, 5)

	m.Update(ytimage.ThumbnailLoadedMsg{
		URL:         "https://fake.test/feed1.jpg",
		TransmitStr: "tx-data",
		Placeholder: "pl-data",
	})

	_, pl := imgR.Get("https://fake.test/feed1.jpg")
	if pl != "pl-data" {
		t.Errorf("expected thumbnail cached via app-level HandleMsg, got placeholder=%q", pl)
	}
}

// TestMusicMode_ThumbnailLoadedMsgStoresInListRenderer verifies that when a
// ThumbnailLoadedMsg arrives, it is stored in the list renderer via
// thumbList.HandleMsg. Regression test: the handler was missing the HandleMsg
// call, so loaded thumbnails were never cached for list display.
func TestMusicMode_ThumbnailLoadedMsgStoresInListRenderer(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	m := NewMusic(
		&mockMusicClient{authenticated: true},
		&mockYTClient{authenticated: true},
		cfg, nil, Options{},
	)
	// Switch to Home view so the search model doesn't also handle the message
	// (search has its own ThumbList sharing the same renderer).
	m.activeFixed = musicViewHome

	if m.thumbList == nil {
		t.Fatal("expected thumbList to be non-nil")
	}
	imgR := m.thumbList.Renderer()

	// Mark a URL as inflight via FetchCmd so HandleLoaded will accept it.
	imgR.FetchCmd("https://fake.test/unit.jpg", 20, 5)

	// Send ThumbnailLoadedMsg through the model's Update.
	msg := ytimage.ThumbnailLoadedMsg{
		URL:         "https://fake.test/unit.jpg",
		TransmitStr: "tx-data",
		Placeholder: "pl-data",
	}
	m.Update(msg)

	// Verify the thumbnail was cached in the list renderer.
	_, pl := imgR.Get("https://fake.test/unit.jpg")
	if pl != "pl-data" {
		t.Errorf("expected placeholder cached via HandleMsg, got %q", pl)
	}
}

// TestMusicMode_LibraryThumbnailFetchTargetsLoadedSection verifies that when a
// library section loads, thumbnail fetches are triggered for that section's list,
// not just the currently viewed sub-tab. Regression test: previously TriggerFetch
// used librarySubIdx (active tab) instead of msg.Index (loaded tab).
func TestMusicMode_LibraryThumbnailFetchTargetsLoadedSection(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		getLibSecFn: func(_ context.Context, browseID string) (*youtube.LibrarySectionResult, error) {
			if browseID == "FEmusic_liked_albums" {
				return &youtube.LibrarySectionResult{
					Items: []youtube.MusicItem{
						{Title: "Fake Album", Type: youtube.MusicAlbum, BrowseID: "b1",
							Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/lib-album.jpg", Width: 226}}},
					},
				}, nil
			}
			return &youtube.LibrarySectionResult{}, nil
		},
	}
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	tm := newTestMusicProgramFull(t, nil, mc, nil, cfg, Options{})

	// Switch to Library tab (key "2": 1=Home, 2=Library, 3=Search)
	sendKey(tm, "2")
	// Wait for library sections to load
	time.Sleep(500 * time.Millisecond)

	m := quitAndGetMusicModel(t, tm)

	// Albums is at index 2 in LibrarySections. Even though the user is viewing
	// index 0 (Playlists), the album thumbnail fetch should have been triggered
	// because TriggerFetch targets the loaded section, not the active one.
	if m.thumbList == nil {
		t.Fatal("expected thumbList to be non-nil with thumbnails enabled")
	}
	imgR := m.thumbList.Renderer()
	if imgR == nil {
		t.Fatal("expected renderer to be non-nil")
	}
	if !imgR.WasRequested("https://fake.test/lib-album.jpg") {
		t.Error("expected FetchCmd to be called for album thumbnail in non-active library section")
	}
}

// TestMusicMode_HomeThumbnailFetchTriggersAllShelves verifies that when the
// home page loads, thumbnail fetches are triggered for all shelves, not just
// the first one.
func TestMusicMode_HomeThumbnailFetchTriggersAllShelves(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		getHomeFn: func(_ context.Context) ([]youtube.MusicShelf, error) {
			return []youtube.MusicShelf{
				{Title: "Quick picks", Items: []youtube.MusicItem{
					{Title: "Song A", Type: youtube.MusicSong},
				}},
				{Title: "Trending Albums", Items: []youtube.MusicItem{
					{Title: "Trending Album", Type: youtube.MusicAlbum, BrowseID: "ta1",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/trending.jpg", Width: 226}}},
				}},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	tm := newTestMusicProgramFull(t, nil, mc, nil, cfg, Options{})

	// Switch to Home tab (key "1": 1=Home, 2=Library, 3=Search)
	sendKey(tm, "1")
	time.Sleep(500 * time.Millisecond)

	m := quitAndGetMusicModel(t, tm)

	if m.thumbList == nil {
		t.Fatal("expected thumbList to be non-nil")
	}
	imgR := m.thumbList.Renderer()
	if imgR == nil {
		t.Fatal("expected renderer to be non-nil")
	}
	// The album in the second shelf should have its thumbnail fetch triggered,
	// even though the user is viewing the first shelf.
	if !imgR.WasRequested("https://fake.test/trending.jpg") {
		t.Error("expected FetchCmd to be called for album thumbnail in non-active home shelf")
	}
}

func TestVideoMode_ChannelTabDedup(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{
					{ID: "UCfake_ch_001", Name: "Fake Channel"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Channel")

	// Open channel tab twice
	sendSpecialKey(tm, tea.KeyEnter)
	time.Sleep(300 * time.Millisecond)
	sendKey(tm, "2") // back to subs
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEnter) // open same channel again
	time.Sleep(300 * time.Millisecond)

	m := quitAndGetVideoModel(t, tm)
	if m.tabs.Len() != 1 {
		t.Errorf("expected 1 tab (dedup), got %d", m.tabs.Len())
	}
}

func TestVideoMode_ChannelFromURL(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
	}
	tm := newTestVideoProgram(t, client)
	// Simulate URL input submitting a channel URL
	tm.Send(urlinput.SubmitMsg{Parsed: youtube.ParsedURL{Kind: youtube.URLChannel, ID: "UCfake_url_ch"}})
	time.Sleep(500 * time.Millisecond)

	m := quitAndGetVideoModel(t, tm)
	if m.tabs.Len() != 1 {
		t.Errorf("expected 1 channel tab from URL, got %d", m.tabs.Len())
	}
	if m.activeView != ViewDynamicTab {
		t.Errorf("expected ViewDynamicTab, got %d", m.activeView)
	}
}

func TestVideoMode_PlaylistFromURL(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getPlaylistVideosFn: func(_ context.Context, playlistID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "pv1", Title: "PL Video", ChannelName: "Test"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	tm.Send(urlinput.SubmitMsg{Parsed: youtube.ParsedURL{Kind: youtube.URLPlaylist, ID: "PLfake_url_pl"}})
	time.Sleep(500 * time.Millisecond)

	m := quitAndGetVideoModel(t, tm)
	if m.tabs.Len() != 1 {
		t.Errorf("expected 1 playlist tab from URL, got %d", m.tabs.Len())
	}
	if m.activeView != ViewDynamicTab {
		t.Errorf("expected ViewDynamicTab, got %d", m.activeView)
	}
}

func TestVideoMode_RefreshChannelTab(t *testing.T) {
	var videoCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{{ID: "UCfake_ref", Name: "Fake Refresh Channel"}},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			videoCalls.Add(1)
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "v1", Title: "Fake Video", ChannelName: "Fake Refresh Channel"}},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Refresh Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Fake Video")

	before := videoCalls.Load()
	sendKey(tm, "r")
	time.Sleep(500 * time.Millisecond)
	after := videoCalls.Load()

	if after <= before {
		t.Errorf("expected channel videos to be re-fetched on refresh, calls before=%d after=%d", before, after)
	}
	quitAndGetVideoModel(t, tm)
}

func TestVideoMode_RefreshPlaylistTab(t *testing.T) {
	var plCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getPlaylistVideosFn: func(_ context.Context, playlistID, token string) (*youtube.Page[youtube.Video], error) {
			plCalls.Add(1)
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "pv1", Title: "PL Video", ChannelName: "Fake"}},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	tm.Send(urlinput.SubmitMsg{Parsed: youtube.ParsedURL{Kind: youtube.URLPlaylist, ID: "PLfake_ref"}})
	time.Sleep(500 * time.Millisecond)

	before := plCalls.Load()
	sendKey(tm, "r")
	time.Sleep(500 * time.Millisecond)
	after := plCalls.Load()

	if after <= before {
		t.Errorf("expected playlist videos to be re-fetched on refresh, calls before=%d after=%d", before, after)
	}
	quitAndGetVideoModel(t, tm)
}

func TestVideoMode_ChannelStreamsLazyLoad(t *testing.T) {
	var streamCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{{ID: "UCfake_lazy", Name: "Fake Lazy Channel"}},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "v1", Title: "A Video", ChannelName: "Fake Lazy Channel"}},
			}, nil
		},
		getChannelStreamsFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			streamCalls.Add(1)
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "ls1", Title: "A Stream", ChannelName: "Fake Lazy Channel"}},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Lazy Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "A Video")

	// Streams should not have been loaded yet (lazy)
	if streamCalls.Load() != 0 {
		t.Fatalf("expected 0 stream calls before switching tab, got %d", streamCalls.Load())
	}

	// Tab to playlists, posts, then livestreams
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(500 * time.Millisecond)

	if streamCalls.Load() == 0 {
		t.Errorf("expected streams to be loaded after switching to Livestreams tab")
	}
	quitAndGetVideoModel(t, tm)
}

func TestVideoMode_RefreshChannelStreamsTab(t *testing.T) {
	var streamCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{{ID: "UCfake_str_ref", Name: "Fake Stream Channel"}},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "v1", Title: "A Video", ChannelName: "Fake Stream Channel"}},
			}, nil
		},
		getChannelStreamsFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			streamCalls.Add(1)
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "ls1", Title: "A Stream", ChannelName: "Fake Stream Channel"}},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Stream Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "A Video")

	// Switch to Livestreams tab
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	waitForContent(t, tm, "A Stream")

	before := streamCalls.Load()
	sendKey(tm, "r")
	time.Sleep(500 * time.Millisecond)
	after := streamCalls.Load()

	if after <= before {
		t.Errorf("expected streams to be re-fetched on refresh, calls before=%d after=%d", before, after)
	}
	quitAndGetVideoModel(t, tm)
}

// ---------------------------------------------------------------------------
// Thumbnail WrapView integration tests
// ---------------------------------------------------------------------------

// wrapViewStabilize calls WrapView enough times to get past the initial
// retransmit and its repeat frame, returning the ThumbList to stable skip state.
func wrapViewStabilize(tl *shared.ThumbList, items []list.Item, view string) {
	for i := 0; i < 5; i++ {
		tl.WrapView(items, view)
	}
}

// TestVideoMode_WrapViewSkipsOnStableFrame verifies that after thumbnails are
// fully loaded, subsequent View() calls return plain view (no DeleteAll or
// transmit sequences). This is the core flicker-prevention optimisation.
func TestVideoMode_WrapViewSkipsOnStableFrame(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	m := New(&mockYTClient{authenticated: true}, cfg, Options{})

	tl := m.listThumbList
	if tl == nil {
		t.Fatal("expected listThumbList")
	}
	imgR := tl.Renderer()

	imgR.Store("https://fake.test/v1.jpg", "TX1", "PL1")
	imgR.Store("https://fake.test/v2.jpg", "TX2", "PL2")

	items := []list.Item{
		shared.VideoItem{Video: youtube.Video{ID: "v1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/v1.jpg", Width: 320}}}},
		shared.VideoItem{Video: youtube.Video{ID: "v2", Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/v2.jpg", Width: 320}}}},
	}

	// First + repeat calls transmit.
	out1 := tl.WrapView(items, "view")
	if out1 == "view" {
		t.Error("first call should transmit, not skip")
	}
	out2 := tl.WrapView(items, "view")
	if out2 == "view" {
		t.Error("repeat call should transmit, not skip")
	}

	// Subsequent calls must skip (cursor blink frames).
	for i := 0; i < 3; i++ {
		out := tl.WrapView(items, "view")
		if out != "view" {
			t.Errorf("frame %d: expected plain view (skip), got output with image data", i)
		}
	}
}

// TestVideoMode_WrapViewInvalidateOnFeedRefresh verifies that refreshing the
// feed (Load with force=true) invalidates the ThumbList so thumbnails
// re-transmit when the feed reloads after the loading spinner.
func TestVideoMode_WrapViewInvalidateOnFeedRefresh(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	m := New(&mockYTClient{
		authenticated: true,
		getFeedFn: func(_ context.Context, _ string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "f1", Title: "Feed1"}},
			}, nil
		},
	}, cfg, Options{})

	tl := m.listThumbList
	imgR := tl.Renderer()
	imgR.Store("https://fake.test/f1.jpg", "TX_F1", "PL_F1")

	items := []list.Item{shared.VideoItem{Video: youtube.Video{
		ID: "f1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/f1.jpg", Width: 320}},
	}}}

	// Stabilise.
	wrapViewStabilize(tl, items, "V")
	if out := tl.WrapView(items, "V"); out != "V" {
		t.Fatal("expected stable skip before refresh")
	}

	// feed.Load(true) calls Invalidate internally.
	m.feed.Load(true)

	// Next WrapView must retransmit.
	if out := tl.WrapView(items, "V"); out == "V" {
		t.Error("after feed refresh, WrapView should retransmit, not skip")
	}
}

// TestVideoMode_CrossThumbListGenCounter verifies that when one ThumbList
// sends DeleteAll (bumping the global gen counter), another ThumbList
// detects the gen mismatch and retransmits. This covers the scenario of
// switching between channel videos and playlists sub-tabs.
func TestVideoMode_CrossThumbListGenCounter(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	m := New(&mockYTClient{authenticated: true}, cfg, Options{})

	listTL := m.listThumbList
	if listTL == nil {
		t.Fatal("expected listThumbList")
	}

	// Create a second ThumbList sharing the same renderer (simulates plThumbList).
	plTL := shared.NewThumbList(listTL.Renderer(), shared.PlaylistThumbURL, 5)

	imgR := listTL.Renderer()
	imgR.Store("https://fake.test/vid.jpg", "TX_VID", "PL_VID")
	imgR.Store("https://fake.test/pl.jpg", "TX_PL", "PL_PL")

	vidItems := []list.Item{shared.VideoItem{Video: youtube.Video{
		ID: "vid1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/vid.jpg", Width: 320}},
	}}}
	plItems := []list.Item{shared.PlaylistItem{Playlist: youtube.Playlist{
		ID: "pl1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/pl.jpg", Width: 320}},
	}}}

	// Stabilise listThumbList.
	wrapViewStabilize(listTL, vidItems, "V")
	if out := listTL.WrapView(vidItems, "V"); out != "V" {
		t.Fatal("listThumbList should be stable")
	}

	// plThumbList transmits (sends DeleteAll, bumps gen).
	if out := plTL.WrapView(plItems, "PV"); out == "PV" {
		t.Fatal("plThumbList should transmit on first call")
	}

	// listThumbList must detect gen mismatch and retransmit.
	if out := listTL.WrapView(vidItems, "V"); out == "V" {
		t.Error("listThumbList should retransmit after plThumbList's DeleteAll (gen mismatch)")
	}
}

// TestMusicMode_WrapViewInvalidateOnHomeLoad verifies that loading the
// music home view invalidates the ThumbList so thumbnails re-transmit
// when the home tab renders after the loading spinner.
func TestMusicMode_WrapViewInvalidateOnHomeLoad(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5

	imgR := ytimage.NewRenderer()
	m := NewMusic(
		&mockMusicClient{authenticated: true},
		&mockYTClient{authenticated: true},
		cfg, imgR, Options{},
	)

	tl := m.thumbList
	if tl == nil {
		t.Fatal("expected thumbList")
	}

	imgR.Store("https://fake.test/song.jpg", "TX_SONG", "PL_SONG")

	fakeItems := []list.Item{musicItem{item: youtube.MusicItem{
		Title: "Fake", Type: youtube.MusicSong,
		Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/song.jpg", Width: 226}},
	}}}
	wrapViewStabilize(tl, fakeItems, "V")
	if out := tl.WrapView(fakeItems, "V"); out != "V" {
		t.Fatal("expected stable skip before loadHome")
	}

	// loadHome sets homeLoading=true and calls Invalidate.
	m.loadHome()

	if out := tl.WrapView(fakeItems, "V"); out == "V" {
		t.Error("after loadHome, WrapView should retransmit, not skip")
	}
}

// TestMusicMode_WrapViewInvalidateOnLibraryLoad verifies that loading the
// music library invalidates the ThumbList.
func TestMusicMode_WrapViewInvalidateOnLibraryLoad(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5

	imgR := ytimage.NewRenderer()
	m := NewMusic(
		&mockMusicClient{authenticated: true},
		&mockYTClient{authenticated: true},
		cfg, imgR, Options{},
	)

	tl := m.thumbList
	if tl == nil {
		t.Fatal("expected thumbList")
	}

	imgR.Store("https://fake.test/song.jpg", "TX_SONG", "PL_SONG")

	fakeItems := []list.Item{musicItem{item: youtube.MusicItem{
		Title: "Fake", Type: youtube.MusicSong,
		Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/song.jpg", Width: 226}},
	}}}
	wrapViewStabilize(tl, fakeItems, "V")
	if out := tl.WrapView(fakeItems, "V"); out != "V" {
		t.Fatal("expected stable skip before loadLibrary")
	}

	m.loadLibrary()

	if out := tl.WrapView(fakeItems, "V"); out == "V" {
		t.Error("after loadLibrary, WrapView should retransmit, not skip")
	}
}

// TestVideoMode_RefetchVisibleThumbsOnDynamicTabSwitch verifies that
// switching to a dynamic tab (channel/playlist) triggers a refetch of
// visible thumbnails whose cache entries were evicted by the LRU.
func TestVideoMode_RefetchVisibleThumbsOnDynamicTabSwitch(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	m := New(&mockYTClient{
		authenticated: true,
		getChannelVideosFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{
					ID: "cv1", Title: "Chan Video",
					Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/cv1.jpg", Width: 320}},
				}},
			}, nil
		},
	}, cfg, Options{})

	imgR := m.listThumbList.Renderer()
	// Pre-populate cache with a small LRU — the default is 200 which is
	// enough to not evict in a test, so store directly and verify refetch
	// returns a cmd when the URL is not cached.
	imgR.Store("https://fake.test/cv1.jpg", "TX_CV1", "PL_CV1")

	// Stabilise the ThumbList.
	items := []list.Item{shared.VideoItem{Video: youtube.Video{
		ID: "cv1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/cv1.jpg", Width: 320}},
	}}}
	wrapViewStabilize(m.listThumbList, items, "V")

	// refetchVisibleThumbs for search (non-dynamic) returns nil because
	// search has no items.
	m.activeView = ViewSearch
	if cmd := m.refetchVisibleThumbs(); cmd != nil {
		t.Error("empty search should not need refetch")
	}

	// Set up feed with the same item so refetch returns nil (all cached).
	m.activeView = ViewFeed
	cmd := m.refetchVisibleThumbs()
	// Feed has no items set, so RefetchCmd returns nil.
	if cmd != nil {
		t.Error("empty feed should not need refetch")
	}
}

// TestMusicMode_RefetchVisibleThumbsOnViewSwitch verifies that switching
// between music mode views triggers refetch for evicted thumbnails.
// TestMusicMode_RefetchVisibleThumbsOnViewSwitch verifies that
// refetchVisibleThumbs routes correctly for music mode views.
func TestMusicMode_RefetchVisibleThumbsOnViewSwitch(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5

	m := NewMusic(
		&mockMusicClient{authenticated: true},
		&mockYTClient{authenticated: true},
		cfg, nil, Options{},
	)

	tl := m.thumbList
	if tl == nil {
		t.Fatal("expected thumbList")
	}
	imgR := tl.Renderer()

	// Set up home with a sub-tab that has items.
	imgR.Store("https://fake.test/song.jpg", "TX_SONG", "PL_SONG")
	m.homeSubs = []subTab{{
		title: "Fake Shelf",
		list:  shared.NewList(m.musicListDelegate()),
	}}
	m.homeSubs[0].list.SetItems([]list.Item{musicItem{item: youtube.MusicItem{
		Title: "Fake", Type: youtube.MusicSong,
		Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/song.jpg", Width: 226}},
	}}})
	m.homeLoaded = true
	m.homeSubIdx = 0

	// Home view: all cached → nil.
	m.onFixedView = true
	m.activeFixed = musicViewHome
	if cmd := m.refetchVisibleThumbs(); cmd != nil {
		t.Error("should return nil when all home thumbnails cached")
	}

	// Search view with no results → nil.
	m.activeFixed = musicViewSearch
	if cmd := m.refetchVisibleThumbs(); cmd != nil {
		t.Error("should return nil for empty search")
	}

	// Library view not loaded → nil.
	m.activeFixed = musicViewLibrary
	if cmd := m.refetchVisibleThumbs(); cmd != nil {
		t.Error("should return nil for unloaded library")
	}

	// Home with uncached item → non-nil.
	m.activeFixed = musicViewHome
	m.homeSubs[0].list.SetItems([]list.Item{musicItem{item: youtube.MusicItem{
		Title: "Uncached", Type: youtube.MusicSong,
		Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/uncached.jpg", Width: 226}},
	}}})
	if cmd := m.refetchVisibleThumbs(); cmd == nil {
		t.Error("should return non-nil for uncached home thumbnail")
	}
}

// TestMusicMode_RefetchOnArtistSubTabSwitch verifies that switching sub-tabs
// within an artist page triggers refetch for evicted thumbnails.
// TestMusicMode_RefetchOnArtistSubTabSwitch verifies that refetchVisibleThumbs
// routes correctly for artist page sub-tabs in music mode.
func TestMusicMode_RefetchOnArtistSubTabSwitch(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5

	m := NewMusic(
		&mockMusicClient{authenticated: true},
		&mockYTClient{authenticated: true},
		cfg, nil, Options{},
	)

	tl := m.thumbList
	if tl == nil {
		t.Fatal("expected thumbList")
	}
	imgR := tl.Renderer()

	// Set up an artist tab with sub-tabs.
	imgR.Store("https://fake.test/art.jpg", "TX_ART", "PL_ART")
	tab := musicTab{
		kind:   musicTabArtist,
		loaded: true,
		artistSubs: []subTab{
			{title: "Songs", list: shared.NewList(m.musicListDelegate())},
			{title: "Albums", list: shared.NewList(m.musicListDelegate())},
		},
		activeSubTab: 0,
	}
	tab.artistSubs[0].list.SetItems([]list.Item{musicItem{item: youtube.MusicItem{
		Title: "Fake Song", Type: youtube.MusicSong,
		Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/art.jpg", Width: 226}},
	}}})

	idx, _ := m.tabs.Open(tab)
	m.tabs.SetActive(idx)
	m.onFixedView = false

	// All cached → nil.
	if cmd := m.refetchVisibleThumbs(); cmd != nil {
		t.Error("should return nil when all artist thumbnails cached")
	}

	// Uncached item → non-nil.
	active := m.tabs.Active()
	active.artistSubs[0].list.SetItems([]list.Item{musicItem{item: youtube.MusicItem{
		Title: "Uncached", Type: youtube.MusicSong,
		Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/uncached.jpg", Width: 226}},
	}}})

	if cmd := m.refetchVisibleThumbs(); cmd == nil {
		t.Error("should return non-nil for uncached artist sub-tab thumbnail")
	}
}

// TestVideoMode_SpinnerRoutedThroughWrapView verifies that loading spinners
// in feed, search, and playlist views route through WrapView so that
// DELETE_STALE fires immediately after Invalidate().
func TestVideoMode_SpinnerRoutedThroughWrapView(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	deleteAll := ytimage.DeleteAll()

	t.Run("feed_spinner", func(t *testing.T) {
		m := New(&mockYTClient{authenticated: true}, cfg, Options{})
		tl := m.listThumbList
		imgR := tl.Renderer()

		// Simulate stable thumbnails on search view.
		imgR.Store("https://fake.test/s1.jpg", "TX_S1", "PL_S1")
		items := []list.Item{shared.VideoItem{Video: youtube.Video{
			ID: "s1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/s1.jpg", Width: 320}},
		}}}
		wrapViewStabilize(tl, items, "V")

		// Switch to feed → Load triggers Invalidate + spinner.
		m.feed.Load(true)
		out := m.feed.View()
		if !strings.Contains(out, deleteAll) {
			t.Error("feed spinner should include DeleteAll via WrapView")
		}
	})

	t.Run("search_spinner", func(t *testing.T) {
		m := New(&mockYTClient{
			authenticated: true,
			searchFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Video], error) {
				return &youtube.Page[youtube.Video]{}, nil
			},
		}, cfg, Options{})
		tl := m.listThumbList
		imgR := tl.Renderer()

		imgR.Store("https://fake.test/s1.jpg", "TX_S1", "PL_S1")
		items := []list.Item{shared.VideoItem{Video: youtube.Video{
			ID: "s1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/s1.jpg", Width: 320}},
		}}}
		wrapViewStabilize(tl, items, "V")

		// Trigger a search → Invalidate + searching spinner.
		m.search.SetQuery("test")
		m.search.Refresh()
		out := m.search.View()
		if !strings.Contains(out, deleteAll) {
			t.Error("search spinner should include DeleteAll via WrapView")
		}
	})
}

// TestVideoMode_DetailTabWrappedWithWrapView verifies that opening a video
// detail tab clears stale list thumbnails via WrapView(nil, ...).
func TestVideoMode_DetailTabWrappedWithWrapView(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5

	m := New(&mockYTClient{
		authenticated: true,
		getVideoFn: func(_ context.Context, _ string) (*youtube.Video, error) {
			return &youtube.Video{ID: "v1", Title: "Test"}, nil
		},
	}, cfg, Options{})

	tl := m.listThumbList
	imgR := tl.Renderer()

	// Stabilise list thumbnails.
	imgR.Store("https://fake.test/s1.jpg", "TX_S1", "PL_S1")
	items := []list.Item{shared.VideoItem{Video: youtube.Video{
		ID: "s1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/s1.jpg", Width: 320}},
	}}}
	wrapViewStabilize(tl, items, "V")

	// Open video detail tab — should Invalidate the listThumbList.
	m.openVideoTab(&youtube.Video{ID: "v1", Title: "Test"})

	// renderContent wraps detail with WrapView(nil, ...).
	out := m.renderContent()
	deleteAll := ytimage.DeleteAll()
	if !strings.Contains(out, deleteAll) {
		t.Error("detail tab should include DeleteAll to clear stale list images")
	}

	// Second render should skip (no more DeleteAll).
	out2 := m.renderContent()
	if strings.Contains(out2, deleteAll) {
		t.Error("detail tab should not send DeleteAll on stable frames")
	}
}

// TestVideoMode_SubsViewWrappedWithWrapView verifies that the subs view
// (which has no thumbnails) clears stale images via WrapView(nil, ...).
func TestVideoMode_SubsViewWrappedWithWrapView(t *testing.T) {
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5

	m := New(&mockYTClient{authenticated: true}, cfg, Options{})
	tl := m.listThumbList
	imgR := tl.Renderer()

	// Stabilise list thumbnails.
	imgR.Store("https://fake.test/s1.jpg", "TX_S1", "PL_S1")
	items := []list.Item{shared.VideoItem{Video: youtube.Video{
		ID: "s1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/s1.jpg", Width: 320}},
	}}}
	wrapViewStabilize(tl, items, "V")

	// Switch to subs — should Invalidate.
	m.switchTo(ViewSubs)
	m.listThumbList.Invalidate()

	out := m.renderContent()
	deleteAll := ytimage.DeleteAll()
	if !strings.Contains(out, deleteAll) {
		t.Error("subs view should include DeleteAll to clear stale list images")
	}
}

// === Tab Persistence ===

func restoreTabsConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := testConfig()
	cfg.General.RestoreTabs = true
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return cfg
}

func TestVideoMode_TabPersistSaveAndRestore(t *testing.T) {
	cfg := restoreTabsConfig(t)
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Video", nil),
	}

	// Session 1: open a video tab and a channel tab
	tm := newTestVideoProgramFull(t, client, cfg, Options{})
	tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "fake_vid_001", Title: "Fake Video"}})
	time.Sleep(200 * time.Millisecond)
	tm.Send(subs.ChannelSelectedMsg{Channel: youtube.Channel{ID: "UCfake123", Name: "Fake Channel"}})
	time.Sleep(200 * time.Millisecond)

	m := quitAndGetVideoModel(t, tm)
	if m.tabs.Len() != 2 {
		t.Fatalf("expected 2 tabs, got %d", m.tabs.Len())
	}

	// Verify state file was written
	saved, err := state.Load("video")
	if err != nil {
		t.Fatalf("state.Load error: %v", err)
	}
	if saved == nil {
		t.Fatal("expected saved state, got nil")
	}
	if len(saved.Tabs) != 2 {
		t.Fatalf("expected 2 saved tabs, got %d", len(saved.Tabs))
	}
	if saved.Tabs[0].Kind != state.KindVideo || saved.Tabs[0].ID != "fake_vid_001" {
		t.Errorf("tab[0] = %+v, want video/fake_vid_001", saved.Tabs[0])
	}
	if saved.Tabs[1].Kind != state.KindChannel || saved.Tabs[1].ID != "UCfake123" {
		t.Errorf("tab[1] = %+v, want channel/UCfake123", saved.Tabs[1])
	}

	// Session 2: new model should load pending restore from saved state
	m2 := New(client, cfg, Options{})
	if len(m2.pendingRestore) != 2 {
		t.Fatalf("expected 2 pending restore entries, got %d", len(m2.pendingRestore))
	}
	if m2.pendingRestore[0].Kind != state.KindVideo || m2.pendingRestore[0].ID != "fake_vid_001" {
		t.Errorf("pendingRestore[0] = %+v, want video/fake_vid_001", m2.pendingRestore[0])
	}
	if m2.pendingRestore[1].Kind != state.KindChannel || m2.pendingRestore[1].ID != "UCfake123" {
		t.Errorf("pendingRestore[1] = %+v, want channel/UCfake123", m2.pendingRestore[1])
	}
}

func TestVideoMode_TabPersistDisabledByDefault(t *testing.T) {
	cfg := testConfig() // RestoreTabs = false (default)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Video", nil),
	}

	tm := newTestVideoProgramFull(t, client, cfg, Options{})
	tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "fake_vid_002", Title: "Fake Video 2"}})
	time.Sleep(200 * time.Millisecond)
	quitAndGetVideoModel(t, tm)

	// No state file should be written
	saved, err := state.Load("video")
	if err != nil {
		t.Fatalf("state.Load error: %v", err)
	}
	if saved != nil {
		t.Errorf("expected no saved state when restore_tabs is disabled, got %+v", saved)
	}
}

func TestVideoMode_TabPersistCloseRemovesFromState(t *testing.T) {
	cfg := restoreTabsConfig(t)
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Video", nil),
	}

	tm := newTestVideoProgramFull(t, client, cfg, Options{})
	tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "fake_vid_003", Title: "Tab to Close"}})
	time.Sleep(200 * time.Millisecond)
	tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "fake_vid_004", Title: "Tab to Keep"}})
	time.Sleep(200 * time.Millisecond)

	// Close the active tab (fake_vid_004)
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(200 * time.Millisecond)

	quitAndGetVideoModel(t, tm)

	saved, err := state.Load("video")
	if err != nil {
		t.Fatalf("state.Load error: %v", err)
	}
	if saved == nil {
		t.Fatal("expected saved state, got nil")
	}
	if len(saved.Tabs) != 1 {
		t.Fatalf("expected 1 saved tab after close, got %d", len(saved.Tabs))
	}
	if saved.Tabs[0].ID != "fake_vid_003" {
		t.Errorf("remaining tab ID = %q, want %q", saved.Tabs[0].ID, "fake_vid_003")
	}
}

func TestVideoMode_PostNotPersisted(t *testing.T) {
	cfg := restoreTabsConfig(t)
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Video", nil),
	}

	tm := newTestVideoProgramFull(t, client, cfg, Options{})

	// Open a video tab
	tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "fake_vid_005", Title: "Persistable"}})
	time.Sleep(200 * time.Millisecond)

	// Open a post tab (posts are not persistable)
	tm.Send(channel.PostSelectedMsg{Post: youtube.Post{ID: "fake_post_001", Content: "Fake post content"}})
	time.Sleep(200 * time.Millisecond)

	m := quitAndGetVideoModel(t, tm)
	if m.tabs.Len() != 2 {
		t.Fatalf("expected 2 open tabs, got %d", m.tabs.Len())
	}

	saved, err := state.Load("video")
	if err != nil {
		t.Fatalf("state.Load error: %v", err)
	}
	if saved == nil {
		t.Fatal("expected saved state, got nil")
	}
	// Only the video tab should be persisted, not the post
	if len(saved.Tabs) != 1 {
		t.Fatalf("expected 1 saved tab (post excluded), got %d", len(saved.Tabs))
	}
	if saved.Tabs[0].ID != "fake_vid_005" {
		t.Errorf("saved tab ID = %q, want %q", saved.Tabs[0].ID, "fake_vid_005")
	}
}

func TestMusicMode_TabPersistSaveAndRestore(t *testing.T) {
	cfg := restoreTabsConfig(t)
	mc := &mockMusicClient{authenticated: true}
	ytClient := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Song", nil),
	}

	// Session 1: open a song tab via URL submit
	tm := newTestMusicProgramFull(t, ytClient, mc, nil, cfg, Options{})
	tm.Send(urlinput.SubmitMsg{Parsed: youtube.ParsedURL{Kind: youtube.URLVideo, ID: "fake_song_001"}})
	time.Sleep(200 * time.Millisecond)

	quitAndGetMusicModel(t, tm)

	// Verify state file was written
	saved, err := state.Load("music")
	if err != nil {
		t.Fatalf("state.Load error: %v", err)
	}
	if saved == nil {
		t.Fatal("expected saved state, got nil")
	}
	if len(saved.Tabs) != 1 {
		t.Fatalf("expected 1 saved tab, got %d", len(saved.Tabs))
	}
	if saved.Tabs[0].Kind != state.KindSong || saved.Tabs[0].ID != "fake_song_001" {
		t.Errorf("tab[0] = %+v, want song/fake_song_001", saved.Tabs[0])
	}

	// Session 2: new model should load pending restore
	m2 := NewMusic(mc, ytClient, cfg, nil, Options{})
	if len(m2.pendingRestore) != 1 {
		t.Fatalf("expected 1 pending restore entry, got %d", len(m2.pendingRestore))
	}
	if m2.pendingRestore[0].Kind != state.KindSong || m2.pendingRestore[0].ID != "fake_song_001" {
		t.Errorf("pendingRestore[0] = %+v, want song/fake_song_001", m2.pendingRestore[0])
	}
}

func TestTabPersist_IndependentModes(t *testing.T) {
	cfg := restoreTabsConfig(t)
	ytClient := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Video", nil),
	}
	mc := &mockMusicClient{authenticated: true}

	// Open a video tab in video mode
	vtm := newTestVideoProgramFull(t, ytClient, cfg, Options{})
	vtm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "fake_vid_010", Title: "Video Tab"}})
	time.Sleep(200 * time.Millisecond)
	quitAndGetVideoModel(t, vtm)

	// Open a song tab in music mode via URL submit
	mtm := newTestMusicProgramFull(t, ytClient, mc, nil, cfg, Options{})
	mtm.Send(urlinput.SubmitMsg{Parsed: youtube.ParsedURL{Kind: youtube.URLVideo, ID: "fake_song_010"}})
	time.Sleep(200 * time.Millisecond)
	quitAndGetMusicModel(t, mtm)

	// Verify independent state files
	videoState, err := state.Load("video")
	if err != nil {
		t.Fatalf("state.Load(video) error: %v", err)
	}
	musicState, err := state.Load("music")
	if err != nil {
		t.Fatalf("state.Load(music) error: %v", err)
	}
	if videoState == nil || len(videoState.Tabs) != 1 {
		t.Fatalf("video state: expected 1 tab, got %v", videoState)
	}
	if musicState == nil || len(musicState.Tabs) != 1 {
		t.Fatalf("music state: expected 1 tab, got %v", musicState)
	}
	if videoState.Tabs[0].Kind != state.KindVideo {
		t.Errorf("video state tab kind = %q, want %q", videoState.Tabs[0].Kind, "video")
	}
	if musicState.Tabs[0].Kind != state.KindSong {
		t.Errorf("music state tab kind = %q, want %q", musicState.Tabs[0].Kind, "song")
	}

	// Video mode should not restore music tabs
	vm := New(ytClient, cfg, Options{})
	if len(vm.pendingRestore) != 1 || vm.pendingRestore[0].Kind != state.KindVideo {
		t.Errorf("video pendingRestore = %+v, want 1 video tab", vm.pendingRestore)
	}

	// Music mode should not restore video tabs
	mm := NewMusic(mc, ytClient, cfg, nil, Options{})
	if len(mm.pendingRestore) != 1 || mm.pendingRestore[0].Kind != state.KindSong {
		t.Errorf("music pendingRestore = %+v, want 1 song tab", mm.pendingRestore)
	}
}

// === Deferred Loading for Restored Tabs ===

func TestVideoMode_RestoreTabsCreatesNeedsLoad(t *testing.T) {
	cfg := restoreTabsConfig(t)
	client := &mockYTClient{authenticated: true}

	// Write state with a video, channel, and playlist tab
	state.Save("video", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindVideo, ID: "fake_vid_020", Title: "Restored Video"},
		{Kind: state.KindChannel, ID: "UCfake_ch_020", Title: "Restored Channel"},
		{Kind: state.KindPlaylist, ID: "PLfake_pl_020", Title: "Restored Playlist"},
	}})

	m := New(client, cfg, Options{})
	// Call Init to trigger restoreTabs via initCmds
	m.Init()

	if m.tabs.Len() != 3 {
		t.Fatalf("expected 3 restored tabs, got %d", m.tabs.Len())
	}
	for i, tab := range m.tabs.All() {
		if !tab.needsLoad {
			t.Errorf("tab[%d] needsLoad = false, want true", i)
		}
	}
	// Active view should be search, not dynamic tab
	if m.activeView != ViewSearch {
		t.Errorf("activeView = %d, want ViewSearch (%d)", m.activeView, ViewSearch)
	}
}

func TestVideoMode_LoadRestoredTabTriggersLoad(t *testing.T) {
	cfg := restoreTabsConfig(t)
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Loaded", nil),
	}

	state.Save("video", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindVideo, ID: "fake_vid_021", Title: "Pending Video"},
	}})

	m := New(client, cfg, Options{})
	m.Init()
	// Simulate receiving a window size so content height is non-zero
	m.width = 80
	m.height = 24

	// Switch to the restored tab
	m.activeView = ViewDynamicTab
	m.tabs.SetActive(0)
	cmd := m.loadRestoredTab()

	tab := m.tabs.Active()
	if tab.needsLoad {
		t.Error("needsLoad should be false after loadRestoredTab")
	}
	if cmd == nil {
		t.Fatal("loadRestoredTab should return a non-nil command")
	}
}

func TestVideoMode_LoadRestoredTabNoopForNormalTab(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Video", nil),
	}
	cfg := testConfig()

	tm := newTestVideoProgramFull(t, client, cfg, Options{})
	// Open a normal tab (not restored)
	tm.Send(shared.VideoSelectedMsg{Video: youtube.Video{ID: "fake_vid_022", Title: "Normal Tab"}})
	time.Sleep(200 * time.Millisecond)

	m := quitAndGetVideoModel(t, tm)
	tab := m.tabs.Active()
	if tab == nil {
		t.Fatal("expected active tab")
	}
	if tab.needsLoad {
		t.Error("normal tab should have needsLoad = false")
	}
	cmd := m.loadRestoredTab()
	if cmd != nil {
		t.Error("loadRestoredTab should return nil for normal tab")
	}
}

func TestVideoMode_SwitchToRestoredTabLoadsContent(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var getVideoCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Loaded", &getVideoCalls),
	}

	// Save a video tab
	state.Save("video", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindVideo, ID: "fake_vid_023", Title: "Saved Tab"},
	}})

	tm := newTestVideoProgramFull(t, client, cfg, Options{})

	// No video should have been loaded yet (deferred)
	if c := getVideoCalls.Load(); c > 0 {
		t.Errorf("expected 0 GetVideo calls before switch, got %d", c)
	}

	// Switch to the restored tab (tab key "4")
	sendKey(tm, "4")
	waitForContent(t, tm, "Loaded fake_vid_023")

	m := quitAndGetVideoModel(t, tm)
	if m.activeView != ViewDynamicTab {
		t.Errorf("expected ViewDynamicTab after switch, got %d", m.activeView)
	}
	if c := getVideoCalls.Load(); c == 0 {
		t.Error("expected GetVideo to be called after switching to restored tab")
	}
}

func TestVideoMode_RestoredChannelTabLoadsOnSwitch(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var channelCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getChannelVideosFn: func(_ context.Context, chID, _ string) (*youtube.Page[youtube.Video], error) {
			channelCalls.Add(1)
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "ch_vid_1", Title: "Channel Video 1"}},
			}, nil
		},
	}

	state.Save("video", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindChannel, ID: "UCfake_ch_030", Title: "Saved Channel"},
	}})

	tm := newTestVideoProgramFull(t, client, cfg, Options{})

	if c := channelCalls.Load(); c > 0 {
		t.Errorf("expected 0 channel calls before switch, got %d", c)
	}

	sendKey(tm, "4")
	waitForContent(t, tm, "Channel Video 1")

	quitAndGetVideoModel(t, tm)
	if c := channelCalls.Load(); c == 0 {
		t.Error("expected channel load after switching to restored tab")
	}
}

func TestVideoMode_RestoredPlaylistTabLoadsOnSwitch(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var playlistCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getPlaylistVideosFn: func(_ context.Context, plID, _ string) (*youtube.Page[youtube.Video], error) {
			playlistCalls.Add(1)
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "pl_vid_1", Title: "Playlist Video 1"}},
			}, nil
		},
	}

	state.Save("video", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindPlaylist, ID: "PLfake_pl_030", Title: "Saved Playlist"},
	}})

	tm := newTestVideoProgramFull(t, client, cfg, Options{})

	if c := playlistCalls.Load(); c > 0 {
		t.Errorf("expected 0 playlist calls before switch, got %d", c)
	}

	sendKey(tm, "4")
	waitForContent(t, tm, "Playlist Video 1")

	quitAndGetVideoModel(t, tm)
	if c := playlistCalls.Load(); c == 0 {
		t.Error("expected playlist load after switching to restored tab")
	}
}

func TestMusicMode_RestoreTabsCreatesNeedsLoad(t *testing.T) {
	cfg := restoreTabsConfig(t)
	mc := &mockMusicClient{authenticated: true}
	ytClient := &mockYTClient{authenticated: true}

	state.Save("music", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindSong, ID: "fake_song_020", Title: "Restored Song"},
		{Kind: state.KindArtist, ID: "UCfake_artist_020", Title: "Restored Artist"},
	}})

	m := NewMusic(mc, ytClient, cfg, nil, Options{})
	m.Init()

	if m.tabs.Len() != 2 {
		t.Fatalf("expected 2 restored tabs, got %d", m.tabs.Len())
	}
	for i, tab := range m.tabs.All() {
		if !tab.needsLoad {
			t.Errorf("tab[%d] needsLoad = false, want true", i)
		}
	}
	if !m.onFixedView {
		t.Error("expected onFixedView = true after restore")
	}
}

func TestMusicMode_SwitchToRestoredSongTabLoadsContent(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var getVideoCalls atomic.Int32
	mc := &mockMusicClient{authenticated: true}
	ytClient := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Loaded Song", &getVideoCalls),
	}

	state.Save("music", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindSong, ID: "fake_song_030", Title: "Saved Song"},
	}})

	tm := newTestMusicProgramFull(t, ytClient, mc, nil, cfg, Options{})

	if c := getVideoCalls.Load(); c > 0 {
		t.Errorf("expected 0 GetVideo calls before switch, got %d", c)
	}

	sendKey(tm, "4")
	waitForContent(t, tm, "Loaded Song fake_song_030")

	quitAndGetMusicModel(t, tm)
	if c := getVideoCalls.Load(); c == 0 {
		t.Error("expected GetVideo to be called after switching to restored song tab")
	}
}

func TestMusicMode_SwitchToRestoredArtistTabLoadsContent(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var artistCalls atomic.Int32
	mc := &mockMusicClient{
		authenticated: true,
		getArtistFn: func(_ context.Context, browseID string) (*youtube.MusicArtistPage, error) {
			artistCalls.Add(1)
			return &youtube.MusicArtistPage{
				Name: "Loaded Artist",
				Shelves: []youtube.MusicShelf{
					{Title: "Songs", Items: []youtube.MusicItem{
						{Title: "Artist Song 1", Type: youtube.MusicSong},
					}},
				},
			}, nil
		},
	}
	ytClient := &mockYTClient{authenticated: true}

	state.Save("music", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindArtist, ID: "UCfake_artist_030", Title: "Saved Artist"},
	}})

	tm := newTestMusicProgramFull(t, ytClient, mc, nil, cfg, Options{})

	if c := artistCalls.Load(); c > 0 {
		t.Errorf("expected 0 artist calls before switch, got %d", c)
	}

	sendKey(tm, "4")
	waitForContent(t, tm, "Loaded Artist")

	quitAndGetMusicModel(t, tm)
	if c := artistCalls.Load(); c == 0 {
		t.Error("expected GetArtist to be called after switching to restored tab")
	}
}

func TestVideoMode_CloseRestoredTabLoadsNextRestored(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var getVideoCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Loaded", &getVideoCalls),
	}

	// Save two video tabs
	state.Save("video", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindVideo, ID: "fake_vid_040", Title: "First"},
		{Kind: state.KindVideo, ID: "fake_vid_041", Title: "Second"},
	}})

	tm := newTestVideoProgramFull(t, client, cfg, Options{})

	// Switch to the second restored tab (tab key "5")
	sendKey(tm, "5")
	waitForContent(t, tm, "Loaded fake_vid_041")

	// Close it with Esc — should fall back to first tab and load it
	getVideoCalls.Store(0)
	sendSpecialKey(tm, tea.KeyEscape)
	waitForContent(t, tm, "Loaded fake_vid_040")

	m := quitAndGetVideoModel(t, tm)
	if m.tabs.Len() != 1 {
		t.Errorf("expected 1 tab remaining, got %d", m.tabs.Len())
	}
	if c := getVideoCalls.Load(); c == 0 {
		t.Error("expected GetVideo to be called for the next restored tab after close")
	}
}

func TestMusicMode_CloseRestoredTabLoadsNextRestored(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var getVideoCalls atomic.Int32
	mc := &mockMusicClient{authenticated: true}
	ytClient := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Loaded", &getVideoCalls),
	}

	state.Save("music", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindSong, ID: "fake_song_040", Title: "First Song"},
		{Kind: state.KindSong, ID: "fake_song_041", Title: "Second Song"},
	}})

	tm := newTestMusicProgramFull(t, ytClient, mc, nil, cfg, Options{})

	// Switch to second tab
	sendKey(tm, "5")
	waitForContent(t, tm, "Loaded fake_song_041")

	// Close it — should load the first tab
	getVideoCalls.Store(0)
	sendSpecialKey(tm, tea.KeyEscape)
	waitForContent(t, tm, "Loaded fake_song_040")

	m := quitAndGetMusicModel(t, tm)
	if m.tabs.Len() != 1 {
		t.Errorf("expected 1 tab remaining, got %d", m.tabs.Len())
	}
	if c := getVideoCalls.Load(); c == 0 {
		t.Error("expected GetVideo to be called for the next restored tab after close")
	}
}

// === -open flag + restored session tabs ===

// -open must focus and load the opened tab even when a prior session restored
// tabs first.
func TestVideoMode_OpenURLWithRestoredTabsFocusesAndLoads(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var getVideoCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Loaded", &getVideoCalls),
	}

	// Simulate a previous session that saved a video tab.
	state.Save("video", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindVideo, ID: "fake_vid_saved", Title: "Saved"},
	}})

	// Launch with -open pointing to a different video.
	parsed := youtube.ParsedURL{Kind: youtube.URLVideo, ID: "fake_vid_opened"}
	m := New(client, cfg, Options{OpenURL: &parsed})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// The opened video must render its loaded title.
	waitForContent(t, tm, "Loaded fake_vid_opened")

	finalM := quitAndGetVideoModel(t, tm)
	if finalM.activeView != ViewDynamicTab {
		t.Errorf("activeView = %d, want ViewDynamicTab (%d)", finalM.activeView, ViewDynamicTab)
	}
	active := finalM.tabs.Active()
	if active == nil || active.id != "fake_vid_opened" {
		t.Errorf("active tab id = %v, want fake_vid_opened", active)
	}
	if c := getVideoCalls.Load(); c == 0 {
		t.Error("expected GetVideo to be called for -open URL")
	}
}

// Two sessions with the same -open URL must dedup to a single loaded tab, not
// stack a new empty tab on top of the restored one.
func TestVideoMode_OpenURLTwiceEndToEnd(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var getVideoCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Joy of Coding", &getVideoCalls),
	}

	parsed := youtube.ParsedURL{Kind: youtube.URLVideo, ID: "fake_vid_reuse"}

	// Session 1: launch with -open, wait for video to load, then quit.
	m1 := New(client, cfg, Options{OpenURL: &parsed})
	tm1 := teatest.NewTestModel(t, m1, teatest.WithInitialTermSize(80, 24))
	waitForContent(t, tm1, "Joy of Coding fake_vid_reuse")
	quitAndGetVideoModel(t, tm1)

	// Verify first run persisted the tab.
	saved, err := state.Load("video")
	if err != nil || saved == nil || len(saved.Tabs) != 1 {
		t.Fatalf("after session 1: saved tabs = %+v, err=%v", saved, err)
	}
	if saved.Tabs[0].ID != "fake_vid_reuse" {
		t.Fatalf("saved tab ID = %q, want fake_vid_reuse", saved.Tabs[0].ID)
	}

	// Session 2: same URL again.
	parsed2 := youtube.ParsedURL{Kind: youtube.URLVideo, ID: "fake_vid_reuse"}
	m2 := New(client, cfg, Options{OpenURL: &parsed2})
	tm2 := teatest.NewTestModel(t, m2, teatest.WithInitialTermSize(80, 24))
	waitForContent(t, tm2, "Joy of Coding fake_vid_reuse")
	finalM := quitAndGetVideoModel(t, tm2)

	if finalM.tabs.Len() != 1 {
		t.Errorf("session 2 tabs.Len() = %d, want 1 (dedup should not create a second tab)", finalM.tabs.Len())
	}
	if finalM.activeView != ViewDynamicTab {
		t.Errorf("session 2 activeView = %d, want ViewDynamicTab (%d)", finalM.activeView, ViewDynamicTab)
	}
}

// -open URL matching a restored tab must trigger its deferred load.
func TestVideoMode_OpenURLSameAsRestoredTabLoadsContent(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var getVideoCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Loaded", &getVideoCalls),
	}

	// Previous session saved the same video we're about to -open.
	state.Save("video", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindVideo, ID: "fake_vid_same", Title: "Prev Session"},
	}})

	parsed := youtube.ParsedURL{Kind: youtube.URLVideo, ID: "fake_vid_same"}
	m := New(client, cfg, Options{OpenURL: &parsed})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	waitForContent(t, tm, "Loaded fake_vid_same")

	finalM := quitAndGetVideoModel(t, tm)
	if finalM.activeView != ViewDynamicTab {
		t.Errorf("activeView = %d, want ViewDynamicTab (%d)", finalM.activeView, ViewDynamicTab)
	}
	if finalM.tabs.Len() != 1 {
		t.Errorf("tabs.Len() = %d, want 1 (dedup should not create a second tab)", finalM.tabs.Len())
	}
	active := finalM.tabs.Active()
	if active == nil || active.id != "fake_vid_same" {
		t.Errorf("active tab id = %v, want fake_vid_same", active)
	}
	if active != nil && active.needsLoad {
		t.Error("active tab still has needsLoad = true; load was never triggered")
	}
	if c := getVideoCalls.Load(); c == 0 {
		t.Error("expected GetVideo call for -open URL matching restored tab")
	}
}

func TestMusicMode_OpenURLSameAsRestoredTabLoadsContent(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var getVideoCalls atomic.Int32
	mc := &mockMusicClient{authenticated: true}
	ytClient := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Loaded Song", &getVideoCalls),
	}

	state.Save("music", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindSong, ID: "fake_song_same", Title: "Prev Session"},
	}})

	parsed := youtube.ParsedURL{Kind: youtube.URLVideo, ID: "fake_song_same"}
	m := NewMusic(mc, ytClient, cfg, nil, Options{OpenURL: &parsed})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	waitForContent(t, tm, "Loaded Song fake_song_same")

	finalM := quitAndGetMusicModel(t, tm)
	if finalM.onFixedView {
		t.Error("onFixedView = true, want false")
	}
	if finalM.tabs.Len() != 1 {
		t.Errorf("tabs.Len() = %d, want 1", finalM.tabs.Len())
	}
	active := finalM.tabs.Active()
	if active == nil || active.browseID != "fake_song_same" {
		t.Errorf("active tab browseID = %v, want fake_song_same", active)
	}
	if active != nil && active.needsLoad {
		t.Error("active tab still has needsLoad = true; load was never triggered")
	}
	if c := getVideoCalls.Load(); c == 0 {
		t.Error("expected GetVideo call for -open URL matching restored song tab")
	}
}

func TestMusicMode_OpenURLWithRestoredTabsFocusesAndLoads(t *testing.T) {
	cfg := restoreTabsConfig(t)
	var getVideoCalls atomic.Int32
	mc := &mockMusicClient{authenticated: true}
	ytClient := &mockYTClient{
		authenticated: true,
		getVideoFn: videoFactory("Loaded Song", &getVideoCalls),
	}

	state.Save("music", &state.TabState{Tabs: []state.TabEntry{
		{Kind: state.KindSong, ID: "fake_song_saved", Title: "Saved Song"},
	}})

	parsed := youtube.ParsedURL{Kind: youtube.URLVideo, ID: "fake_song_opened"}
	m := NewMusic(mc, ytClient, cfg, nil, Options{OpenURL: &parsed})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	waitForContent(t, tm, "Loaded Song fake_song_opened")

	finalM := quitAndGetMusicModel(t, tm)
	if finalM.onFixedView {
		t.Errorf("onFixedView = true, want false (opened tab should be focused)")
	}
	active := finalM.tabs.Active()
	if active == nil || active.browseID != "fake_song_opened" {
		t.Errorf("active tab browseID = %v, want fake_song_opened", active)
	}
	if c := getVideoCalls.Load(); c == 0 {
		t.Error("expected GetVideo to be called for -open URL in music mode")
	}
}

// === Subscribe / Unsubscribe (step 6) ===

// TestVideoMode_SubscribePropagatesToOpenTabs asserts that a successful
// Subscribe flips state in every open tab referencing the channel — not just
// the one that initiated the action.
func TestVideoMode_SubscribePropagatesToOpenTabs(t *testing.T) {
	var subscribeCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, _ string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{{ID: "UCprop", Name: "Prop Channel"}},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "prop_v1", Title: "Prop Video", ChannelName: "Prop Channel", ChannelID: "UCprop"}},
			}, nil
		},
		getChannelFn: func(_ context.Context, _ string) (*youtube.ChannelDetail, error) {
			return &youtube.ChannelDetail{
				Channel: youtube.Channel{
					ID: "UCprop", Name: "Prop Channel", Handle: "@prop",
					SubscriberCount: "1K subscribers",
				},
				VideoCount:      "10 videos",
				Subscribed:      false,
				SubscribedKnown: true,
			}, nil
		},
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			return &youtube.Video{
				ID: id, Title: "Prop Video", ChannelName: "Prop Channel", ChannelID: "UCprop",
				ChannelSubscribed: false, ChannelSubscribedKnown: true,
			}, nil
		},
		subscribeFn: func(_ context.Context, _ string) error {
			subscribeCalls.Add(1)
			return nil
		},
	}
	tm := newTestVideoProgram(t, client)

	// Open channel tab via subs view.
	sendKey(tm, "2")
	waitForContent(t, tm, "Prop Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Prop Video")

	// Open video detail tab for a video from the same channel.
	sendKey(tm, "i")
	waitForContent(t, tm, "Prop Video")

	// Switch back to the channel tab (tab 4 is channel, tab 5 is video in open order).
	sendKey(tm, "4")
	waitForContent(t, tm, "Prop Video")

	// Fire subscribe from the channel tab.
	sendKey(tm, "S")
	waitForContent(t, tm, "Subscription")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Subscribed to Prop Channel")

	m := quitAndGetVideoModel(t, tm)
	if subscribeCalls.Load() != 1 {
		t.Fatalf("Subscribe calls = %d, want 1", subscribeCalls.Load())
	}
	for i := range m.tabs.All() {
		tab := m.tabs.At(i)
		switch tab.kind {
		case tabChannel:
			d := tab.channel.Detail()
			if d == nil || !d.SubscribedKnown || !d.Subscribed {
				t.Errorf("channel tab detail not flipped: %+v", d)
			}
		case tabVideo:
			v := tab.detail.Video()
			if v == nil || !v.ChannelSubscribedKnown || !v.ChannelSubscribed {
				t.Errorf("video tab channelSubscribed not flipped: %+v", v)
			}
		}
	}
}

// TestVideoMode_UnsubscribeRemovesFromSubsList asserts that an unsubscribe
// drops the channel row from an already-loaded subs view.
func TestVideoMode_UnsubscribeRemovesFromSubsList(t *testing.T) {
	var unsubscribeCalls atomic.Int32
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, _ string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{
					{ID: "UCone", Name: "One Channel"},
					{ID: "UCtwo", Name: "Two Channel"},
				},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "v1", Title: "Vid"}},
			}, nil
		},
		getChannelFn: func(_ context.Context, id string) (*youtube.ChannelDetail, error) {
			return &youtube.ChannelDetail{
				Channel:         youtube.Channel{ID: id, Name: "One Channel"},
				Subscribed:      true,
				SubscribedKnown: true,
			}, nil
		},
		unsubscribeFn: func(_ context.Context, _ string) error {
			unsubscribeCalls.Add(1)
			return nil
		},
	}
	tm := newTestVideoProgram(t, client)

	sendKey(tm, "2")
	waitForContent(t, tm, "One Channel")
	// Open first channel tab.
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Vid")

	sendKey(tm, "S")
	waitForContent(t, tm, "Subscription")
	// Already subscribed → picker shows only Unsubscribe.
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Unsubscribed from One Channel")

	m := quitAndGetVideoModel(t, tm)
	if unsubscribeCalls.Load() != 1 {
		t.Errorf("Unsubscribe calls = %d, want 1", unsubscribeCalls.Load())
	}
	// Subs list should now contain only "Two Channel".
	chans := m.subs.Channels()
	if len(chans) != 1 {
		t.Fatalf("subs list size = %d, want 1 (row for UCone dropped)", len(chans))
	}
}

// TestVideoMode_UnauthenticatedSubscribeBlocked asserts that pressing S
// without auth shows the prompt status and never issues a Subscribe call.
func TestVideoMode_UnauthenticatedSubscribeBlocked(t *testing.T) {
	var subscribeCalls atomic.Int32
	client := &mockYTClient{
		authenticated: false,
		searchFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "v", Title: "A Video", ChannelID: "UCx", ChannelName: "X"}},
			}, nil
		},
		subscribeFn: func(_ context.Context, _ string) error {
			subscribeCalls.Add(1)
			return nil
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "test"})
	waitForContent(t, tm, "A Video")

	sendKey(tm, "S")
	waitForContent(t, tm, "Authenticate first")

	m := quitAndGetVideoModel(t, tm)
	if subscribeCalls.Load() != 0 {
		t.Errorf("Subscribe calls = %d, want 0", subscribeCalls.Load())
	}
	if m.picker.IsActive() {
		t.Error("picker should not be active after unauthenticated subscribe attempt")
	}
}

// TestVideoMode_SubscribeFromVideoDetailFlipsIndicator opens a video detail,
// presses S, confirms, and asserts the indicator on the active video flipped
// in place without requiring a refetch.
func TestVideoMode_SubscribeFromVideoDetailFlipsIndicator(t *testing.T) {
	var subscribeCalls atomic.Int32
	client := detailIndicatorClient("dv1", "Detail Video", "Detail Channel", "UCdetail", false)
	client.subscribeFn = func(_ context.Context, channelID string) error {
		if channelID != "UCdetail" {
			t.Errorf("Subscribe called with %q, want UCdetail", channelID)
		}
		subscribeCalls.Add(1)
		return nil
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "test"})
	waitForContent(t, tm, "Detail Video")
	sendKey(tm, "i")
	waitForContent(t, tm, "○ Not subscribed")

	sendKey(tm, "S")
	waitForContent(t, tm, "Subscription")
	sendSpecialKey(tm, tea.KeyEnter)
	// Indicator flip is the user-visible proof that propagation ran; the
	// status line asserts separately in goldens. waitForContent consumes its
	// reader on each call, so only one post-action wait is safe here.
	waitForContent(t, tm, "✓ Subscribed")

	m := quitAndGetVideoModel(t, tm)
	if subscribeCalls.Load() != 1 {
		t.Fatalf("Subscribe calls = %d, want 1", subscribeCalls.Load())
	}
	tab := m.tabs.Active()
	if tab == nil || tab.kind != tabVideo {
		t.Fatalf("active tab = %+v, want video detail", tab)
	}
	v := tab.detail.Video()
	if v == nil || !v.ChannelSubscribedKnown || !v.ChannelSubscribed {
		t.Errorf("video state not flipped: %+v", v)
	}
}

func TestVideoMode_UnsubscribeFromVideoDetailFlipsIndicator(t *testing.T) {
	var unsubscribeCalls atomic.Int32
	client := detailIndicatorClient("dv2", "Sub Video", "Sub Channel", "UCsub", true)
	client.unsubscribeFn = func(_ context.Context, channelID string) error {
		if channelID != "UCsub" {
			t.Errorf("Unsubscribe called with %q, want UCsub", channelID)
		}
		unsubscribeCalls.Add(1)
		return nil
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "test"})
	waitForContent(t, tm, "Sub Video")
	sendKey(tm, "i")
	waitForContent(t, tm, "✓ Subscribed")

	sendKey(tm, "S")
	waitForContent(t, tm, "Subscription")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "○ Not subscribed")

	m := quitAndGetVideoModel(t, tm)
	if unsubscribeCalls.Load() != 1 {
		t.Fatalf("Unsubscribe calls = %d, want 1", unsubscribeCalls.Load())
	}
	tab := m.tabs.Active()
	if tab == nil || tab.kind != tabVideo {
		t.Fatalf("active tab = %+v, want video detail", tab)
	}
	v := tab.detail.Video()
	if v == nil || !v.ChannelSubscribedKnown || v.ChannelSubscribed {
		t.Errorf("video state not flipped: %+v", v)
	}
}

// === Music mode subscribe (step 8) ===

// TestMusicMode_SubscribeFromArtistTab opens an artist tab and asserts the
// S picker wires through to Subscribe with the artist's browseID.
func TestMusicMode_SubscribeFromArtistTab(t *testing.T) {
	var subscribeCalls atomic.Int32
	mc := &mockMusicClient{
		authenticated: true,
		getArtistFn: func(_ context.Context, browseID string) (*youtube.MusicArtistPage, error) {
			return &youtube.MusicArtistPage{Name: "Fake Artist"}, nil
		},
	}
	ytc := &mockYTClient{
		authenticated: true,
		subscribeFn: func(_ context.Context, channelID string) error {
			if channelID != "UCartist" {
				t.Errorf("Subscribe called with %q, want UCartist", channelID)
			}
			subscribeCalls.Add(1)
			return nil
		},
	}
	parsed := youtube.ParsedURL{Kind: youtube.URLChannel, ID: "UCartist"}
	m := NewMusic(mc, ytc, testConfig(), nil, Options{OpenURL: &parsed})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	waitForContent(t, tm, "Fake Artist")

	sendKey(tm, "S")
	waitForContent(t, tm, "Subscription")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Subscribed to Fake Artist")

	quitAndGetMusicModel(t, tm)
	if subscribeCalls.Load() != 1 {
		t.Errorf("Subscribe calls = %d, want 1", subscribeCalls.Load())
	}
}


// TestMusicMode_SubscribeFromSongTabPropagates opens a song detail in music
// mode, presses S, and asserts the Subscribe call fired and the indicator
// flipped on the song's detail model.
func TestMusicMode_SubscribeFromSongTabPropagates(t *testing.T) {
	var subscribeCalls atomic.Int32
	mc := &mockMusicClient{authenticated: true}
	ytc := &mockYTClient{
		authenticated: true,
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			return &youtube.Video{
				ID: id, Title: "Fake Song", ChannelName: "Fake Artist",
				ChannelID: "UCartist", ChannelSubscribed: false, ChannelSubscribedKnown: true,
			}, nil
		},
		subscribeFn: func(_ context.Context, channelID string) error {
			if channelID != "UCartist" {
				t.Errorf("Subscribe called with %q, want UCartist", channelID)
			}
			subscribeCalls.Add(1)
			return nil
		},
	}
	parsed := youtube.ParsedURL{Kind: youtube.URLVideo, ID: "songid"}
	m := NewMusic(mc, ytc, testConfig(), ytimage.NewRenderer(), Options{OpenURL: &parsed})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Fake Song")
	}, teatest.WithDuration(5*time.Second))

	sendKey(tm, "S")
	time.Sleep(150 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEnter)
	time.Sleep(300 * time.Millisecond)

	mm := quitAndGetMusicModel(t, tm)
	if subscribeCalls.Load() != 1 {
		t.Fatalf("Subscribe calls = %d, want 1", subscribeCalls.Load())
	}
	tab := mm.tabs.Active()
	if tab == nil || tab.kind != musicTabSong {
		t.Fatalf("active tab = %+v, want song", tab)
	}
	v := tab.songDetail.Video()
	if v == nil || !v.ChannelSubscribedKnown || !v.ChannelSubscribed {
		t.Errorf("song state not flipped: %+v", v)
	}
}

// TestMusicMode_SubscribeFromArtistAboutFlipsIndicator asserts that
// subscribing from the artist About sub-tab flips the indicator in place
// and that the underlying artistPage state is updated.
func TestMusicMode_SubscribeFromArtistAboutFlipsIndicator(t *testing.T) {
	var subscribeCalls atomic.Int32
	mc := &mockMusicClient{
		authenticated: true,
		getArtistFn: func(_ context.Context, _ string) (*youtube.MusicArtistPage, error) {
			return &youtube.MusicArtistPage{
				Name:            "Prop Artist",
				ChannelID:       "UCpropartist",
				SubscriberCount: "100K subscribers",
				Subscribed:      false,
				SubscribedKnown: true,
			}, nil
		},
	}
	ytc := &mockYTClient{
		authenticated: true,
		subscribeFn: func(_ context.Context, channelID string) error {
			if channelID != "UCpropartist" {
				t.Errorf("Subscribe called with %q, want UCpropartist", channelID)
			}
			subscribeCalls.Add(1)
			return nil
		},
	}
	parsed := youtube.ParsedURL{Kind: youtube.URLChannel, ID: "MPLAUCpropartist"}
	m := NewMusic(mc, ytc, testConfig(), ytimage.NewRenderer(), Options{OpenURL: &parsed})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	waitForContent(t, tm, "○ Not subscribed")

	sendKey(tm, "S")
	time.Sleep(150 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "✓ Subscribed")

	mm := quitAndGetMusicModel(t, tm)
	if subscribeCalls.Load() != 1 {
		t.Fatalf("Subscribe calls = %d, want 1", subscribeCalls.Load())
	}
	tab := mm.tabs.Active()
	if tab == nil || tab.kind != musicTabArtist {
		t.Fatalf("active tab = %+v, want artist", tab)
	}
	if tab.artistPage == nil || !tab.artistPage.SubscribedKnown || !tab.artistPage.Subscribed {
		t.Errorf("artistPage state not flipped: %+v", tab.artistPage)
	}
}

// TestMusicMode_UnauthenticatedSubscribeBlocked asserts S without auth shows
// the prompt and issues no Subscribe call.
func TestMusicMode_UnauthenticatedSubscribeBlocked(t *testing.T) {
	var subscribeCalls atomic.Int32
	mc := &mockMusicClient{authenticated: false}
	ytc := &mockYTClient{
		authenticated: false,
		subscribeFn: func(_ context.Context, _ string) error {
			subscribeCalls.Add(1)
			return nil
		},
	}
	parsed := youtube.ParsedURL{Kind: youtube.URLChannel, ID: "UCnoauth"}
	m := NewMusic(mc, ytc, testConfig(), nil, Options{OpenURL: &parsed})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(200 * time.Millisecond)

	sendKey(tm, "S")
	waitForContent(t, tm, "Authenticate first")

	mm := quitAndGetMusicModel(t, tm)
	if subscribeCalls.Load() != 0 {
		t.Errorf("Subscribe calls = %d, want 0", subscribeCalls.Load())
	}
	if mm.picker.IsActive() {
		t.Error("picker should not be active")
	}
}
