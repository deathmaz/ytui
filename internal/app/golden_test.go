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
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "test"})
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
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "test"})
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
		getCommentsFn: func(_ context.Context, _, _ string) (*youtube.Page[youtube.Comment], error) {
			return &youtube.Page[youtube.Comment]{}, nil
		},
	}
	// Search → results → open 3 tabs via i → back → j → i
	tm := newTestVideoProgramWithOpts(t, client, Options{SearchQuery: "tabs"})
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
