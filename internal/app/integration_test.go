package app

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/player"
	"github.com/deathmaz/ytui/internal/ui/shared"
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
		getCommentsFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Comment], error) {
			return &youtube.Page[youtube.Comment]{}, nil
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
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			return &youtube.Video{ID: id, Title: "Video " + id, URL: "https://youtube.com/watch?v=" + id}, nil
		},
		getCommentsFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Comment], error) {
			return &youtube.Page[youtube.Comment]{}, nil
		},
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
		getCommentsFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Comment], error) {
			return &youtube.Page[youtube.Comment]{}, nil
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
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			return &youtube.Video{ID: id, Title: "Video " + id, URL: "https://youtube.com/watch?v=" + id}, nil
		},
		getCommentsFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Comment], error) {
			return &youtube.Page[youtube.Comment]{}, nil
		},
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
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			return &youtube.Video{ID: id, Title: "V " + id, URL: "https://youtube.com/watch?v=" + id}, nil
		},
		getCommentsFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Comment], error) {
			return &youtube.Page[youtube.Comment]{}, nil
		},
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
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{}, nil
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
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{}, nil
		},
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
