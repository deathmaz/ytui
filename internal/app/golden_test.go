package app

import (
	"context"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/player"
	"github.com/deathmaz/ytui/internal/youtube"
)

const goldenTimeout = 3 * time.Second

func freshRender(t *testing.T, tm *teatest.TestModel) []byte {
	t.Helper()
	io.ReadAll(tm.Output())
	tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	time.Sleep(20 * time.Millisecond)
	out := readAll(t, tm.Output())
	if len(out) == 0 {
		time.Sleep(200 * time.Millisecond)
		out = readAll(t, tm.Output())
	}
	if len(out) == 0 {
		t.Fatal("captured output is empty after forced re-render")
	}
	return out
}

func captureGolden(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	out := freshRender(t, tm)
	tm.Quit()
	tm.WaitFinished(t, teatest.WithFinalTimeout(goldenTimeout))
	teatest.RequireEqualOutput(t, out)
}

func waitThenCapture(t *testing.T, tm *teatest.TestModel, waitFor string) {
	t.Helper()
	waitForContent(t, tm, waitFor)
	captureGolden(t, tm)
}

func readAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func typeQuery(tm *teatest.TestModel, query string) {
	tm.Type(query)
	sendSpecialKey(tm, tea.KeyEnter)
}

func TestGolden_Video_Search_Empty(t *testing.T) {
	tm := newTestVideoProgram(t, nil)
	time.Sleep(200 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Video_Search_WithResults(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "v1", Title: "Learn Go Programming", ChannelName: "Go Tutorials", ViewCount: "1.2M views", PublishedAt: "2 days ago", DurationStr: "12:34"},
					{ID: "v2", Title: "Advanced Go Patterns", ChannelName: "Code Academy", ViewCount: "500K views", PublishedAt: "1 week ago", DurationStr: "45:00"},
					{ID: "v3", Title: "Go Concurrency Deep Dive", ChannelName: "Tech Talk", ViewCount: "300K views", PublishedAt: "3 days ago", DurationStr: "28:15"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "golang"})
	waitThenCapture(t, tm, "Learn Go Programming")
}

func TestGolden_Video_Search_WithThumbnails(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "v1", Title: "Learn Go Programming", ChannelName: "Go Tutorials", ViewCount: "1.2M views", PublishedAt: "2 days ago", DurationStr: "12:34"},
					{ID: "v2", Title: "Advanced Go Patterns", ChannelName: "Code Academy", ViewCount: "500K views", PublishedAt: "1 week ago", DurationStr: "45:00"},
					{ID: "v3", Title: "Go Concurrency Deep Dive", ChannelName: "Tech Talk", ViewCount: "300K views", PublishedAt: "3 days ago", DurationStr: "28:15"},
				},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	tm := newTestVideoProgramFull(t, client, cfg, Options{SearchQuery: "golang"})
	waitThenCapture(t, tm, "Learn Go Programming")
}

func TestGolden_Video_Feed_Unauthenticated(t *testing.T) {
	tm := newTestVideoProgram(t, &mockYTClient{authenticated: false})
	sendKey(tm, "1")
	time.Sleep(300 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Video_Feed_Loading(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getFeedFn: func(ctx context.Context, token string) (*youtube.Page[youtube.Video], error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Second):
				return &youtube.Page[youtube.Video]{}, nil
			}
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "1")
	time.Sleep(300 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Video_Feed_WithItems(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getFeedFn: func(_ context.Context, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "f1", Title: "New Release: Go 1.22", ChannelName: "Go Team", ViewCount: "50K views", PublishedAt: "1 hour ago", DurationStr: "5:30"},
					{ID: "f2", Title: "Building CLI Tools in Go", ChannelName: "Gopher Academy", ViewCount: "12K views", PublishedAt: "3 hours ago", DurationStr: "22:10"},
					{ID: "f3", Title: "TUI Applications with Bubble Tea", ChannelName: "Charm", ViewCount: "8K views", PublishedAt: "5 hours ago", DurationStr: "35:45"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "1")
	waitThenCapture(t, tm, "New Release")
}

func TestGolden_Video_Feed_WithThumbnails(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getFeedFn: func(_ context.Context, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "f1", Title: "New Release: Go 1.22", ChannelName: "Go Team", ViewCount: "50K views", PublishedAt: "1 hour ago", DurationStr: "5:30",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/f1.jpg", Width: 320}}},
					{ID: "f2", Title: "Building CLI Tools in Go", ChannelName: "Gopher Academy", ViewCount: "12K views", PublishedAt: "3 hours ago", DurationStr: "22:10",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/f2.jpg", Width: 320}}},
				},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	tm := newTestVideoProgramFull(t, client, cfg, Options{})
	sendKey(tm, "1")
	waitThenCapture(t, tm, "New Release")
}

func TestGolden_Video_Subs_Unauthenticated(t *testing.T) {
	tm := newTestVideoProgram(t, &mockYTClient{authenticated: false})
	sendKey(tm, "2")
	time.Sleep(300 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Video_Subs_WithChannels(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{
					{Name: "Go Team Official", Handle: "@golang", SubscriberCount: "250K subscribers"},
					{Name: "Charm CLI", Handle: "@charmcli", SubscriberCount: "50K subscribers"},
					{Name: "The Primeagen", Handle: "@ThePrimeagen", SubscriberCount: "1.2M subscribers"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitThenCapture(t, tm, "Go Team Official")
}

func TestGolden_Video_Detail_Info(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "d1", Title: "Test Video for Detail", ChannelName: "Test Channel", URL: "https://www.youtube.com/watch?v=d1"},
				},
			}, nil
		},
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			return &youtube.Video{
				ID: id, Title: "Test Video for Detail", ChannelName: "Test Channel",
				ViewCount: "1.5M views", LikeCount: "45K", DurationStr: "15:30",
				PublishedAt: "2 weeks ago", Description: "This is a test video description for golden file testing.",
				URL: "https://www.youtube.com/watch?v=d1",
			}, nil
		},
		getCommentsFn: func(_ context.Context, videoID, token string) (*youtube.Page[youtube.Comment], error) {
			return &youtube.Page[youtube.Comment]{
				Items: []youtube.Comment{
					{AuthorName: "User1", Content: "Great video!", LikeCount: "120", PublishedAt: "1 day ago"},
					{AuthorName: "User2", Content: "Very helpful, thanks for sharing.", LikeCount: "45", PublishedAt: "2 days ago"},
				},
				NextToken: "comments_page2",
			}, nil
		},
	}
	// Search → results appear → press i to open detail
	tm := newTestVideoProgramFull(t, client, nil, Options{SearchQuery: "test"}, "d1")
	waitForContent(t, tm, "Test Video for Detail")
	sendKey(tm, "i")
	waitThenCapture(t, tm, "1.5M views")
}

func TestGolden_Video_Detail_Comments(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "c1", Title: "Comments Test Video", ChannelName: "Test Channel", URL: "https://www.youtube.com/watch?v=c1"},
				},
			}, nil
		},
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			return &youtube.Video{
				ID: id, Title: "Comments Test Video", ChannelName: "Test Channel",
				ViewCount: "1M views", URL: "https://www.youtube.com/watch?v=c1",
				CommentsToken: "initial_comments",
			}, nil
		},
		getCommentsFn: func(_ context.Context, videoID, token string) (*youtube.Page[youtube.Comment], error) {
			return &youtube.Page[youtube.Comment]{
				Items: []youtube.Comment{
					{AuthorName: "Alice", Content: "This is amazing!", LikeCount: "500", PublishedAt: "3 hours ago"},
					{AuthorName: "Bob", Content: "I learned so much from this.", LikeCount: "200", PublishedAt: "5 hours ago"},
					{AuthorName: "Charlie", Content: "Can you make a follow-up?", LikeCount: "89", PublishedAt: "1 day ago"},
				},
				NextToken: "more_comments",
			}, nil
		},
	}
	// Search → results → i to open detail → tab to switch to comments
	tm := newTestVideoProgramFull(t, client, nil, Options{SearchQuery: "test"}, "c1")
	waitForContent(t, tm, "Comments Test Video")
	sendKey(tm, "i")
	time.Sleep(500 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(500 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Video_URLInput(t *testing.T) {
	tm := newTestVideoProgram(t, nil)
	sendKey(tm, "O")
	time.Sleep(300 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Video_QualityPicker(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "p1", Title: "Picker Test Video", URL: "https://www.youtube.com/watch?v=p1"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "picker"})
	waitForContent(t, tm, "Picker Test Video")
	// User would press P here, but format fetching needs external binary.
	// Simulate the result arriving:
	tm.Send(formatsLoadedMsg{
		url: "https://www.youtube.com/watch?v=p1",
		formats: []player.Format{
			{ID: "best", Display: "Best Quality"},
			{ID: "1080", Display: "1080p (mp4)"},
			{ID: "720", Display: "720p (mp4)"},
			{ID: "480", Display: "480p (mp4)"},
			{ID: "audio", Display: "Audio Only (m4a)"},
		},
	})
	waitThenCapture(t, tm, "Best Quality")
}

func TestGolden_Video_HelpExpanded(t *testing.T) {
	tm := newTestVideoProgram(t, nil)
	sendKey(tm, "?")
	time.Sleep(200 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Video_MultipleTabs(t *testing.T) {
	videos := map[string]youtube.Video{
		"tab1": {ID: "tab1", Title: "First Video Tab", ChannelName: "Channel A", ViewCount: "100K views", DurationStr: "10:00", URL: "https://youtube.com/watch?v=tab1"},
		"tab2": {ID: "tab2", Title: "Second Video Tab", ChannelName: "Channel B", ViewCount: "200K views", DurationStr: "20:00", URL: "https://youtube.com/watch?v=tab2"},
		"tab3": {ID: "tab3", Title: "Third Video Tab", ChannelName: "Channel C", ViewCount: "300K views", DurationStr: "30:00", URL: "https://youtube.com/watch?v=tab3"},
	}
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{videos["tab1"], videos["tab2"], videos["tab3"]},
			}, nil
		},
		getVideoFn: func(_ context.Context, id string) (*youtube.Video, error) {
			v := videos[id]
			return &v, nil
		},
	}
	// Search → results → open 3 tabs via i → back → j → i
	tm := newTestVideoProgramFull(t, client, nil, Options{SearchQuery: "tabs"}, "tab1", "tab2", "tab3")
	waitForContent(t, tm, "First Video Tab")
	sendKey(tm, "i")
	time.Sleep(200 * time.Millisecond)
	sendKey(tm, "3")
	time.Sleep(100 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "j")
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "i")
	time.Sleep(200 * time.Millisecond)
	sendKey(tm, "3")
	time.Sleep(100 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEscape)
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "j")
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "j")
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "i")
	waitThenCapture(t, tm, "Third Video Tab")
}

func TestGolden_Video_StatusMessage(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getFeedFn: func(_ context.Context, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "v1", Title: "Feed Video", ChannelName: "Channel", ViewCount: "1K", DurationStr: "5:00"}},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "1")
	time.Sleep(300 * time.Millisecond)
	sendKey(tm, "a")
	time.Sleep(200 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Video_StartupWarning(t *testing.T) {
	tm := newTestVideoProgramWithOpts(t, nil, Options{
		Warning: "client params scrape failed: web: connection refused",
	})
	waitThenCapture(t, tm, "client params scrape failed")
}

func TestGolden_Music_Search_Empty(t *testing.T) {
	tm := newTestMusicProgram(t, nil, nil)
	time.Sleep(200 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Music_Search_WithResults(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
			return &youtube.MusicSearchResult{
				TopResult: &youtube.MusicItem{Title: "Top Hit Song", Subtitle: "Famous Artist", Type: youtube.MusicSong, VideoID: "top1"},
				Shelves: []youtube.MusicShelf{
					{Title: "Songs", Items: []youtube.MusicItem{
						{Title: "Song One", Subtitle: "Artist A", Type: youtube.MusicSong},
						{Title: "Song Two", Subtitle: "Artist B", Type: youtube.MusicSong},
					}},
					{Title: "Albums", Items: []youtube.MusicItem{
						{Title: "Great Album", Subtitle: "Artist A", Type: youtube.MusicAlbum},
					}},
				},
			}, nil
		},
	}
	tm := newTestMusicProgramWithOpts(t, nil, mc, nil, Options{SearchQuery: "music test"})
	waitThenCapture(t, tm, "Top Hit Song")
}

func TestGolden_Music_Search_WithThumbnails(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
			return &youtube.MusicSearchResult{
				TopResult: &youtube.MusicItem{Title: "Top Hit Song", Subtitle: "Famous Artist", Type: youtube.MusicSong, VideoID: "top1",
					Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/top1.jpg", Width: 226, Height: 226}}},
				Shelves: []youtube.MusicShelf{
					{Title: "Songs", Items: []youtube.MusicItem{
						{Title: "Song One", Subtitle: "Artist A", Type: youtube.MusicSong,
							Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/song1.jpg", Width: 226, Height: 226}}},
					}},
					{Title: "Albums", Items: []youtube.MusicItem{
						{Title: "Great Album", Subtitle: "Album • Artist A • 2024", Type: youtube.MusicAlbum, BrowseID: "album1",
							Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/album1.jpg", Width: 226, Height: 226}}},
						{Title: "Another Album", Subtitle: "Album • Artist B • 2023", Type: youtube.MusicAlbum, BrowseID: "album2",
							Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/album2.jpg", Width: 226, Height: 226}}},
					}},
				},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	tm := newTestMusicProgramFull(t, nil, mc, nil, cfg, Options{SearchQuery: "album test"})
	waitThenCapture(t, tm, "Great Album")
}

func TestGolden_Music_StartupWarning(t *testing.T) {
	tm := newTestMusicProgramWithOpts(t, nil, nil, nil, Options{
		Warning: "client params scrape failed: music: connection refused",
	})
	waitThenCapture(t, tm, "client params scrape failed")
}

func TestGolden_Music_Home_NotLoaded(t *testing.T) {
	tm := newTestMusicProgram(t, nil, &mockMusicClient{authenticated: true})
	sendKey(tm, "1")
	time.Sleep(300 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Music_Home_WithShelves(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		getHomeFn: func(_ context.Context) ([]youtube.MusicShelf, error) {
			return []youtube.MusicShelf{
				{Title: "Quick picks", Items: []youtube.MusicItem{
					{Title: "Trending Song", Subtitle: "Popular Artist", Type: youtube.MusicSong},
					{Title: "New Release", Subtitle: "Rising Star", Type: youtube.MusicSong},
				}},
				{Title: "Listen again", Items: []youtube.MusicItem{
					{Title: "Old Favorite", Subtitle: "Classic Band", Type: youtube.MusicSong},
				}},
			}, nil
		},
	}
	tm := newTestMusicProgram(t, nil, mc)
	sendKey(tm, "1")
	waitThenCapture(t, tm, "Trending Song")
}

func TestGolden_Music_Library_Unauthenticated(t *testing.T) {
	tm := newTestMusicProgram(t, nil, &mockMusicClient{authenticated: false})
	sendKey(tm, "2")
	time.Sleep(300 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Music_Library_WithSections(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		getLibSecFn: func(_ context.Context, browseID string) (*youtube.LibrarySectionResult, error) {
			return &youtube.LibrarySectionResult{
				Items: []youtube.MusicItem{
					{Title: "My Playlist", Subtitle: "25 songs", Type: youtube.MusicPlaylist},
					{Title: "Liked Songs", Subtitle: "100 songs", Type: youtube.MusicPlaylist},
				},
				Continuation: "lib_more",
			}, nil
		},
	}
	tm := newTestMusicProgram(t, nil, mc)
	sendKey(tm, "2")
	waitThenCapture(t, tm, "My Playlist")
}

func TestGolden_Music_ArtistPage(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
			return &youtube.MusicSearchResult{
				Shelves: []youtube.MusicShelf{
					{Title: "Artists", Items: []youtube.MusicItem{
						{Title: "Test Artist", Subtitle: "Artist", Type: youtube.MusicArtist, BrowseID: "artist1"},
					}},
				},
			}, nil
		},
		getArtistFn: func(_ context.Context, browseID string) (*youtube.MusicArtistPage, error) {
			return &youtube.MusicArtistPage{
				Name: "Test Artist",
				Shelves: []youtube.MusicShelf{
					{Title: "Songs", Items: []youtube.MusicItem{
						{Title: "Hit Song", Subtitle: "Test Artist", Type: youtube.MusicSong, VideoID: "s1"},
						{Title: "Another Hit", Subtitle: "Test Artist", Type: youtube.MusicSong, VideoID: "s2"},
					}},
					{Title: "Albums", Items: []youtube.MusicItem{
						{Title: "Debut Album", Subtitle: "2024", Type: youtube.MusicAlbum, BrowseID: "alb1"},
					}},
				},
			}, nil
		},
	}
	// Search → results → Enter to open artist
	tm := newTestMusicProgramWithOpts(t, nil, mc, nil, Options{SearchQuery: "artist"})
	waitForContent(t, tm, "Test Artist")
	sendSpecialKey(tm, tea.KeyEnter)
	waitThenCapture(t, tm, "Hit Song")
}

func TestGolden_Music_AlbumPage(t *testing.T) {
	const thumbURL = "https://fake-thumb.example.com/album.jpg"
	mc := &mockMusicClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
			return &youtube.MusicSearchResult{
				Shelves: []youtube.MusicShelf{
					{Title: "Albums", Items: []youtube.MusicItem{
						{Title: "Test Album", Subtitle: "Test Artist", Type: youtube.MusicAlbum, BrowseID: "album1"},
					}},
				},
			}, nil
		},
		getAlbumFn: func(_ context.Context, browseID string) (*youtube.MusicAlbumPage, error) {
			return &youtube.MusicAlbumPage{
				Title: "Test Album", Artist: "Test Artist", Year: "2024",
				AlbumType: "Album", TrackCount: "10 songs", Duration: "42 min",
				Description: "A great album for testing.", PlaylistID: "PLtest",
				Thumbnails: []youtube.Thumbnail{{URL: thumbURL, Width: 226, Height: 226}},
				Tracks: []youtube.MusicItem{
					{Title: "Track One", Subtitle: "Test Artist", Type: youtube.MusicSong, VideoID: "t1"},
					{Title: "Track Two", Subtitle: "Test Artist", Type: youtube.MusicSong, VideoID: "t2"},
					{Title: "Track Three", Subtitle: "Test Artist", Type: youtube.MusicSong, VideoID: "t3"},
				},
			}, nil
		},
	}
	imgR := ytimage.NewRenderer()
	imgR.Store(thumbURL, "", "[ALBUM ART]")
	// Search → results → Enter to open album
	tm := newTestMusicProgramWithOpts(t, nil, mc, imgR, Options{SearchQuery: "album"})
	waitForContent(t, tm, "Test Album")
	sendSpecialKey(tm, tea.KeyEnter)
	waitThenCapture(t, tm, "Track One")
}

func TestGolden_Music_Home_SecondSubTab(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		getHomeFn: func(_ context.Context) ([]youtube.MusicShelf, error) {
			return []youtube.MusicShelf{
				{Title: "Quick picks", Items: []youtube.MusicItem{
					{Title: "Trending Song", Subtitle: "Popular Artist", Type: youtube.MusicSong},
				}},
				{Title: "Listen again", Items: []youtube.MusicItem{
					{Title: "Old Favorite", Subtitle: "Classic Band", Type: youtube.MusicSong},
					{Title: "Another Classic", Subtitle: "Retro Group", Type: youtube.MusicSong},
				}},
			}, nil
		},
	}
	tm := newTestMusicProgram(t, nil, mc)
	sendKey(tm, "1")
	time.Sleep(300 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(200 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Music_Library_SecondSection(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		getLibSecFn: func(_ context.Context, browseID string) (*youtube.LibrarySectionResult, error) {
			if browseID == youtube.LibrarySections[1].BrowseID {
				return &youtube.LibrarySectionResult{
					Items: []youtube.MusicItem{
						{Title: "Liked Song A", Subtitle: "Artist X", Type: youtube.MusicSong},
						{Title: "Liked Song B", Subtitle: "Artist Y", Type: youtube.MusicSong},
					},
				}, nil
			}
			return &youtube.LibrarySectionResult{
				Items: []youtube.MusicItem{{Title: "My Playlist", Subtitle: "25 songs", Type: youtube.MusicPlaylist}},
			}, nil
		},
	}
	tm := newTestMusicProgram(t, nil, mc)
	sendKey(tm, "2")
	time.Sleep(400 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(200 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Music_ArtistPage_AlbumsSubTab(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
			return &youtube.MusicSearchResult{
				Shelves: []youtube.MusicShelf{
					{Title: "Artists", Items: []youtube.MusicItem{
						{Title: "Test Artist", Subtitle: "Artist", Type: youtube.MusicArtist, BrowseID: "artist1"},
					}},
				},
			}, nil
		},
		getArtistFn: func(_ context.Context, browseID string) (*youtube.MusicArtistPage, error) {
			return &youtube.MusicArtistPage{
				Name: "Test Artist",
				Shelves: []youtube.MusicShelf{
					{Title: "Songs", Items: []youtube.MusicItem{
						{Title: "Hit Song", Subtitle: "Test Artist", Type: youtube.MusicSong, VideoID: "s1"},
					}},
					{Title: "Albums", Items: []youtube.MusicItem{
						{Title: "Debut Album", Subtitle: "2023", Type: youtube.MusicAlbum, BrowseID: "alb1"},
						{Title: "Second Album", Subtitle: "2024", Type: youtube.MusicAlbum, BrowseID: "alb2"},
					}},
					{Title: "Videos", Items: []youtube.MusicItem{
						{Title: "Music Video", Subtitle: "Test Artist", Type: youtube.MusicVideo, VideoID: "mv1"},
					}},
				},
			}, nil
		},
	}
	// Search → results → Enter to open artist → Tab to switch to Albums shelf
	tm := newTestMusicProgramWithOpts(t, nil, mc, nil, Options{SearchQuery: "artist"})
	waitForContent(t, tm, "Test Artist")
	sendSpecialKey(tm, tea.KeyEnter)
	time.Sleep(400 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(200 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Music_URLInput(t *testing.T) {
	tm := newTestMusicProgram(t, nil, nil)
	sendKey(tm, "O")
	time.Sleep(300 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Music_HelpExpanded(t *testing.T) {
	tm := newTestMusicProgram(t, nil, nil)
	sendKey(tm, "?")
	time.Sleep(200 * time.Millisecond)
	captureGolden(t, tm)
}

func TestGolden_Music_MultipleTabs(t *testing.T) {
	mc := &mockMusicClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
			return &youtube.MusicSearchResult{
				Shelves: []youtube.MusicShelf{
					{Title: "Artists", Items: []youtube.MusicItem{
						{Title: "Artist One", Subtitle: "Artist", Type: youtube.MusicArtist, BrowseID: "a1"},
						{Title: "Album One", Subtitle: "Album", Type: youtube.MusicAlbum, BrowseID: "al1"},
					}},
				},
			}, nil
		},
		getArtistFn: func(_ context.Context, browseID string) (*youtube.MusicArtistPage, error) {
			return &youtube.MusicArtistPage{
				Name: "Artist " + browseID,
				Shelves: []youtube.MusicShelf{
					{Title: "Songs", Items: []youtube.MusicItem{
						{Title: "Song by " + browseID, Subtitle: "Artist " + browseID, Type: youtube.MusicSong},
					}},
				},
			}, nil
		},
		getAlbumFn: func(_ context.Context, browseID string) (*youtube.MusicAlbumPage, error) {
			return &youtube.MusicAlbumPage{
				Title: "Album " + browseID, Artist: "Test",
				Tracks: []youtube.MusicItem{{Title: "Track 1", Subtitle: "Test", Type: youtube.MusicSong}},
			}, nil
		},
	}
	// Search → open first result (artist) → back to search → move down → open second (album)
	tm := newTestMusicProgramWithOpts(t, nil, mc, nil, Options{SearchQuery: "multi"})
	waitForContent(t, tm, "Artist One")
	sendSpecialKey(tm, tea.KeyEnter)
	time.Sleep(300 * time.Millisecond)
	// Back to search
	sendKey(tm, "3")
	time.Sleep(100 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEscape) // blur input
	time.Sleep(50 * time.Millisecond)
	sendKey(tm, "j") // move to Album One
	time.Sleep(50 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyEnter)
	waitThenCapture(t, tm, "Track 1")
}

func TestGolden_Video_Channel_FromSubs(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{
					{ID: "UCfake_ch_001", Name: "Fake Channel", Handle: "@fakechannel", SubscriberCount: "100K subscribers"},
				},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "cv1", Title: "Channel Video One", ChannelName: "Fake Channel", ViewCount: "10K views", PublishedAt: "1 day ago", DurationStr: "8:00"},
					{ID: "cv2", Title: "Channel Video Two", ChannelName: "Fake Channel", ViewCount: "5K views", PublishedAt: "3 days ago", DurationStr: "12:30"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitThenCapture(t, tm, "Channel Video One")
}

func TestGolden_Video_Channel_FromSubs_WithThumbnails(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{
					{ID: "UCfake_ch_001", Name: "Fake Channel", Handle: "@fakechannel", SubscriberCount: "100K subscribers"},
				},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "cv1", Title: "Channel Video One", ChannelName: "Fake Channel", ViewCount: "10K views", PublishedAt: "1 day ago", DurationStr: "8:00",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/cv1.jpg", Width: 320}}},
					{ID: "cv2", Title: "Channel Video Two", ChannelName: "Fake Channel", ViewCount: "5K views", PublishedAt: "3 days ago", DurationStr: "12:30",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/cv2.jpg", Width: 320}}},
				},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	tm := newTestVideoProgramFull(t, client, cfg, Options{})
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitThenCapture(t, tm, "Channel Video One")
}

func TestGolden_Video_Channel_CKey(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "s1", Title: "Test Search Result", ChannelName: "Test Channel", ChannelID: "UCfake_test_ch", ViewCount: "1K views", PublishedAt: "1 day ago", DurationStr: "5:00"},
				},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "cv1", Title: "Channel Vid From C Key", ChannelName: "Test Channel", ViewCount: "2K views", PublishedAt: "2 days ago", DurationStr: "10:00"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "test"})
	waitForContent(t, tm, "Test Search Result")
	sendKey(tm, "c")
	waitThenCapture(t, tm, "Channel Vid From C Key")
}

func TestGolden_Video_Channel_CKey_WithThumbnails(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		searchFn: func(_ context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "s1", Title: "Test Search Result", ChannelName: "Test Channel", ChannelID: "UCfake_test_ch", ViewCount: "1K views", PublishedAt: "1 day ago", DurationStr: "5:00"},
				},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "cv1", Title: "Channel Vid From C Key", ChannelName: "Test Channel", ViewCount: "2K views", PublishedAt: "2 days ago", DurationStr: "10:00",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/cv1.jpg", Width: 320}}},
				},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	tm := newTestVideoProgramFull(t, client, cfg, Options{SearchQuery: "test"})
	waitForContent(t, tm, "Test Search Result")
	sendKey(tm, "c")
	waitThenCapture(t, tm, "Channel Vid From C Key")
}

func TestGolden_Video_Channel_Playlists(t *testing.T) {
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
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "cv1", Title: "A Video", ChannelName: "Fake Channel"}},
			}, nil
		},
		getChannelPlaylistsFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Playlist], error) {
			return &youtube.Page[youtube.Playlist]{
				Items: []youtube.Playlist{
					{ID: "PLfake001", Title: "Best Of Compilation", VideoCount: "24"},
					{ID: "PLfake002", Title: "Tutorials Series", VideoCount: "12"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "A Video")
	sendSpecialKey(tm, tea.KeyTab)
	waitThenCapture(t, tm, "Best Of Compilation")
}

func TestGolden_Video_Channel_Playlists_WithThumbnails(t *testing.T) {
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
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "cv1", Title: "A Video", ChannelName: "Fake Channel"}},
			}, nil
		},
		getChannelPlaylistsFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Playlist], error) {
			return &youtube.Page[youtube.Playlist]{
				Items: []youtube.Playlist{
					{ID: "PLfake001", Title: "Best Of Compilation", VideoCount: "24",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/pl1.jpg", Width: 320}}},
					{ID: "PLfake002", Title: "Tutorials Series", VideoCount: "12",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/pl2.jpg", Width: 320}}},
				},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	tm := newTestVideoProgramFull(t, client, cfg, Options{})
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "A Video")
	sendSpecialKey(tm, tea.KeyTab)
	waitThenCapture(t, tm, "Best Of Compilation")
}

func TestGolden_Video_Playlist_Detail(t *testing.T) {
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
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "cv1", Title: "A Video", ChannelName: "Fake Channel"}},
			}, nil
		},
		getChannelPlaylistsFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Playlist], error) {
			return &youtube.Page[youtube.Playlist]{
				Items: []youtube.Playlist{
					{ID: "PLfake001", Title: "Best Of Compilation", VideoCount: "24"},
				},
			}, nil
		},
		getPlaylistVideosFn: func(_ context.Context, playlistID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "pv1", Title: "Playlist Video One", ChannelName: "Fake Channel", ViewCount: "5K views", DurationStr: "8:00"},
					{ID: "pv2", Title: "Playlist Video Two", ChannelName: "Fake Channel", ViewCount: "3K views", DurationStr: "6:30"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "A Video")
	sendSpecialKey(tm, tea.KeyTab)
	waitForContent(t, tm, "Best Of Compilation")
	sendSpecialKey(tm, tea.KeyEnter)
	waitThenCapture(t, tm, "Playlist Video One")
}

func TestGolden_Video_Playlist_Detail_WithThumbnails(t *testing.T) {
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
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "cv1", Title: "A Video", ChannelName: "Fake Channel"}},
			}, nil
		},
		getChannelPlaylistsFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Playlist], error) {
			return &youtube.Page[youtube.Playlist]{
				Items: []youtube.Playlist{
					{ID: "PLfake001", Title: "Best Of Compilation", VideoCount: "24",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/pl1.jpg", Width: 320}}},
				},
			}, nil
		},
		getPlaylistVideosFn: func(_ context.Context, playlistID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "pv1", Title: "Playlist Video One", ChannelName: "Fake Channel", ViewCount: "5K views", DurationStr: "8:00",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/pv1.jpg", Width: 320}}},
					{ID: "pv2", Title: "Playlist Video Two", ChannelName: "Fake Channel", ViewCount: "3K views", DurationStr: "6:30",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/pv2.jpg", Width: 320}}},
				},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	tm := newTestVideoProgramFull(t, client, cfg, Options{})
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "A Video")
	sendSpecialKey(tm, tea.KeyTab)
	waitForContent(t, tm, "Best Of Compilation")
	sendSpecialKey(tm, tea.KeyEnter)
	waitThenCapture(t, tm, "Playlist Video One")
}

func newPostTestClient() *mockYTClient {
	return &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{
					{ID: "UCfake_post_ch", Name: "Fake Gaming Channel", Handle: "@FakeGaming"},
				},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "cv1", Title: "Ranked Ladder Session", ChannelName: "Fake Gaming Channel", DurationStr: "45:30"},
				},
			}, nil
		},
		getChannelPostsFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Post], error) {
			return &youtube.Page[youtube.Post]{
				Items: []youtube.Post{
					{ID: "fake_post_001", AuthorName: "Fake Gaming Channel", Content: "Who wants to see more challenge runs on our channel?\nNew adventures of the creative player number one coming soon!", LikeCount: "113", PublishedAt: "3 months ago", DetailParams: "fake_dp1"},
					{ID: "fake_post_002", AuthorName: "Fake Gaming Channel", Content: "Short documentary about the impact of global events on gaming\n- including my occasional appearances and commentary throughout", LikeCount: "61", PublishedAt: "4 months ago", DetailParams: "fake_dp2"},
					{ID: "fake_post_003", AuthorName: "Fake Gaming Channel", Content: "The new balance patch is officially out!\nWhat are your impressions?\nLink with description of all changes in comments below", LikeCount: "96", PublishedAt: "6 months ago", DetailParams: "fake_dp3"},
					{ID: "fake_post_004", AuthorName: "Fake Gaming Channel", Content: "Time to summarize our charity project, implemented together with a local community foundation for the arts", LikeCount: "431", PublishedAt: "7 months ago", DetailParams: "fake_dp4",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/charity_post.jpg", Width: 400, Height: 400}}},
					{ID: "fake_post_005", AuthorName: "Fake Gaming Channel", Content: "Your vote can determine the program of our channel for the next couple of months! Cast your vote right now in the poll", LikeCount: "109", PublishedAt: "8 months ago", DetailParams: "fake_dp5"},
					{ID: "fake_post_006", AuthorName: "Fake Gaming Channel", Content: "You wanted a continuation of the first-person challenge in grandmaster league? Welcome to our charity mini-tournament event!", LikeCount: "254", PublishedAt: "9 months ago", DetailParams: "fake_dp6",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/challenge_post.jpg", Width: 400, Height: 400}}},
					{ID: "fake_post_007", AuthorName: "Fake Gaming Channel", Content: "Are you ready for the new episode? Wednesday at 5pm - Video number 2100 on our channel: enjoy the show like never before!", LikeCount: "263", PublishedAt: "9 months ago", DetailParams: "fake_dp7",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/episode_post.jpg", Width: 400, Height: 400}}},
					{ID: "fake_post_008", AuthorName: "Fake Gaming Channel", Content: "What about the annual event streams this year? The yearly broadcasts have become a nice tradition for part of our community", LikeCount: "106", PublishedAt: "10 months ago", DetailParams: "fake_dp8",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/annual_post.jpg", Width: 400, Height: 400}}},
				},
			}, nil
		},
		getPostCommentsFn: func(_ context.Context, detailParams, token string) (*youtube.Page[youtube.Comment], error) {
			return &youtube.Page[youtube.Comment]{
				Items: []youtube.Comment{
					{ID: "fake_c1", AuthorName: "FakeUser42", Content: "More challenge runs please! The content is always great, thanks for the regular uploads on the channel!", LikeCount: "45", PublishedAt: "3 months ago", ReplyCount: 5, ReplyToken: "fake_reply1"},
					{ID: "fake_c2", AuthorName: "FakeGamer99", Content: "When will there be a new tournament? Really looking forward to the next season!", LikeCount: "23", PublishedAt: "3 months ago", ReplyCount: 2, ReplyToken: "fake_reply2"},
					{ID: "fake_c3", AuthorName: "FakeViewer01", Content: "Best gaming channel on YouTube! I watch every single day without fail!", LikeCount: "67", PublishedAt: "2 months ago"},
					{ID: "fake_c4", AuthorName: "FakeAnalyst", Content: "When will you review the new patch? There are many interesting changes for all the different playstyles", LikeCount: "34", PublishedAt: "2 months ago", ReplyCount: 1, ReplyToken: "fake_reply3"},
					{ID: "fake_c5", AuthorName: "FakeCasual", Content: "Thanks for the charity project! Really important work for the community.", LikeCount: "89", PublishedAt: "1 month ago"},
					{ID: "fake_c6", AuthorName: "FakeWatcher", Content: "Come to the next tournament! We will be cheering for you all the way!", LikeCount: "12", PublishedAt: "1 month ago"},
				},
				NextToken: "fake_comments_next_page",
			}, nil
		},
	}
}

func TestGolden_Video_Channel_Posts(t *testing.T) {
	tm := newTestVideoProgram(t, newPostTestClient())
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Gaming Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Ranked Ladder Session")
	sendSpecialKey(tm, tea.KeyTab) // playlists
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab) // posts
	waitThenCapture(t, tm, "challenge runs")
}

func TestGolden_Video_Channel_Streams(t *testing.T) {
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
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "cv1", Title: "A Video", ChannelName: "Fake Channel"}},
			}, nil
		},
		getChannelStreamsFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "ls1", Title: "Friday Night Stream", ChannelName: "Fake Channel", DurationStr: "4:32:10", ViewCount: "15K views"},
					{ID: "ls2", Title: "Weekend Marathon", ChannelName: "Fake Channel", DurationStr: "6:15:00", ViewCount: "8K views"},
				},
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "A Video")
	sendSpecialKey(tm, tea.KeyTab) // playlists
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab) // posts
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab) // livestreams
	waitThenCapture(t, tm, "Friday Night Stream")
}

func TestGolden_Video_Channel_Streams_WithThumbnails(t *testing.T) {
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
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "cv1", Title: "A Video", ChannelName: "Fake Channel"}},
			}, nil
		},
		getChannelStreamsFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{
					{ID: "ls1", Title: "Friday Night Stream", ChannelName: "Fake Channel", DurationStr: "4:32:10", ViewCount: "15K views",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/ls1.jpg", Width: 320}}},
					{ID: "ls2", Title: "Weekend Marathon", ChannelName: "Fake Channel", DurationStr: "6:15:00", ViewCount: "8K views",
						Thumbnails: []youtube.Thumbnail{{URL: "https://fake.test/ls2.jpg", Width: 320}}},
				},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.Thumbnails.Enabled = true
	cfg.Thumbnails.Height = 5
	tm := newTestVideoProgramFull(t, client, cfg, Options{})
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "A Video")
	sendSpecialKey(tm, tea.KeyTab) // playlists
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab) // posts
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab) // livestreams
	waitThenCapture(t, tm, "Friday Night Stream")
}

func TestGolden_Video_Channel_About_Subscribed(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{{ID: "UCfake_ch_001", Name: "Fake Channel"}},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "cv1", Title: "A Video", ChannelName: "Fake Channel"}},
			}, nil
		},
		getChannelFn: func(_ context.Context, channelID string) (*youtube.ChannelDetail, error) {
			return &youtube.ChannelDetail{
				Channel: youtube.Channel{
					ID:              "UCfake_ch_001",
					Name:            "Fake Channel",
					Handle:          "@fakechannel",
					Description:     "A synthetic channel used in golden tests.",
					SubscriberCount: "1.2M subscribers",
				},
				VideoCount:      "123 videos",
				Subscribed:      true,
				SubscribedKnown: true,
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "A Video")
	sendSpecialKey(tm, tea.KeyTab) // playlists
	time.Sleep(150 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab) // posts
	time.Sleep(150 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab) // livestreams
	time.Sleep(150 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab) // about
	waitThenCapture(t, tm, "Subscribed")
}

func TestGolden_Video_Channel_About_NotSubscribed(t *testing.T) {
	client := &mockYTClient{
		authenticated: true,
		getSubsFn: func(_ context.Context, token string) (*youtube.Page[youtube.Channel], error) {
			return &youtube.Page[youtube.Channel]{
				Items: []youtube.Channel{{ID: "UCfake_ch_002", Name: "Another Channel"}},
			}, nil
		},
		getChannelVideosFn: func(_ context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
			return &youtube.Page[youtube.Video]{
				Items: []youtube.Video{{ID: "cv2", Title: "Other Video", ChannelName: "Another Channel"}},
			}, nil
		},
		getChannelFn: func(_ context.Context, channelID string) (*youtube.ChannelDetail, error) {
			return &youtube.ChannelDetail{
				Channel: youtube.Channel{
					ID:              "UCfake_ch_002",
					Name:            "Another Channel",
					Handle:          "@anotherfake",
					SubscriberCount: "4.5K subscribers",
				},
				VideoCount:      "17 videos",
				Subscribed:      false,
				SubscribedKnown: true,
			}, nil
		},
	}
	tm := newTestVideoProgram(t, client)
	sendKey(tm, "2")
	waitForContent(t, tm, "Another Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Other Video")
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(150 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(150 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(150 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	waitThenCapture(t, tm, "Not subscribed")
}

func TestGolden_Video_Post_Detail_Content(t *testing.T) {
	tm := newTestVideoProgram(t, newPostTestClient())
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Gaming Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Ranked Ladder Session")
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	waitForContent(t, tm, "challenge runs")
	sendSpecialKey(tm, tea.KeyEnter)
	waitThenCapture(t, tm, "creative player")
}

func TestGolden_Video_Post_Detail_Comments(t *testing.T) {
	tm := newTestVideoProgram(t, newPostTestClient())
	sendKey(tm, "2")
	waitForContent(t, tm, "Fake Gaming Channel")
	sendSpecialKey(tm, tea.KeyEnter)
	waitForContent(t, tm, "Ranked Ladder Session")
	sendSpecialKey(tm, tea.KeyTab)
	time.Sleep(200 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab)
	waitForContent(t, tm, "challenge runs")
	sendSpecialKey(tm, tea.KeyEnter)
	time.Sleep(300 * time.Millisecond)
	sendSpecialKey(tm, tea.KeyTab) // switch to comments
	waitThenCapture(t, tm, "FakeUser42")
}
