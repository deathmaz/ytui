package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/deathmaz/ytui/internal/youtube"
)

// mockYTClient implements youtube.Client for testing.
type mockYTClient struct {
	mu            sync.Mutex
	authenticated bool
	searchFn      func(ctx context.Context, query, token string) (*youtube.Page[youtube.Video], error)
	getVideoFn    func(ctx context.Context, id string) (*youtube.Video, error)
	getCommentsFn func(ctx context.Context, videoID, token string) (*youtube.Page[youtube.Comment], error)
	getRepliesFn  func(ctx context.Context, commentID, token string) (*youtube.Page[youtube.Comment], error)
	getSubsFn     func(ctx context.Context, token string) (*youtube.Page[youtube.Channel], error)
	getFeedFn     func(ctx context.Context, token string) (*youtube.Page[youtube.Video], error)
	getChannelFn  func(ctx context.Context, channelID, token string) (*youtube.Page[youtube.Video], error)

	// Call tracking
	feedCalls   []string // tokens passed to GetFeed
	subsCalls   []string // tokens passed to GetSubscriptions
	searchCalls []string // queries passed to Search
}

func (m *mockYTClient) IsAuthenticated() bool { return m.authenticated }

func (m *mockYTClient) Search(ctx context.Context, query, token string) (*youtube.Page[youtube.Video], error) {
	m.mu.Lock()
	m.searchCalls = append(m.searchCalls, query)
	m.mu.Unlock()
	if m.searchFn != nil {
		return m.searchFn(ctx, query, token)
	}
	return &youtube.Page[youtube.Video]{}, nil
}

func (m *mockYTClient) GetVideo(ctx context.Context, id string) (*youtube.Video, error) {
	if m.getVideoFn != nil {
		return m.getVideoFn(ctx, id)
	}
	return &youtube.Video{ID: id, Title: "Test Video"}, nil
}

func (m *mockYTClient) GetComments(ctx context.Context, videoID, token string) (*youtube.Page[youtube.Comment], error) {
	if m.getCommentsFn != nil {
		return m.getCommentsFn(ctx, videoID, token)
	}
	return &youtube.Page[youtube.Comment]{}, nil
}

func (m *mockYTClient) GetReplies(ctx context.Context, commentID, token string) (*youtube.Page[youtube.Comment], error) {
	if m.getRepliesFn != nil {
		return m.getRepliesFn(ctx, commentID, token)
	}
	return &youtube.Page[youtube.Comment]{}, nil
}

func (m *mockYTClient) GetSubscriptions(ctx context.Context, token string) (*youtube.Page[youtube.Channel], error) {
	m.mu.Lock()
	m.subsCalls = append(m.subsCalls, token)
	m.mu.Unlock()
	if m.getSubsFn != nil {
		return m.getSubsFn(ctx, token)
	}
	if !m.authenticated {
		return nil, fmt.Errorf("authentication required for subscriptions")
	}
	return &youtube.Page[youtube.Channel]{}, nil
}

func (m *mockYTClient) GetFeed(ctx context.Context, token string) (*youtube.Page[youtube.Video], error) {
	m.mu.Lock()
	m.feedCalls = append(m.feedCalls, token)
	m.mu.Unlock()
	if m.getFeedFn != nil {
		return m.getFeedFn(ctx, token)
	}
	if !m.authenticated {
		return nil, fmt.Errorf("authentication required for subscription feed")
	}
	return &youtube.Page[youtube.Video]{}, nil
}

func (m *mockYTClient) GetChannelVideos(ctx context.Context, channelID, token string) (*youtube.Page[youtube.Video], error) {
	if m.getChannelFn != nil {
		return m.getChannelFn(ctx, channelID, token)
	}
	return &youtube.Page[youtube.Video]{}, nil
}

// mockMusicClient implements youtube.MusicAPI for testing.
type mockMusicClient struct {
	mu            sync.Mutex
	authenticated bool
	searchFn      func(ctx context.Context, query, cont string) (*youtube.MusicSearchResult, error)
	getHomeFn     func(ctx context.Context) ([]youtube.MusicShelf, error)
	getLibSecFn   func(ctx context.Context, browseID string) (*youtube.LibrarySectionResult, error)
	getLibContFn  func(ctx context.Context, cont string) (*youtube.LibrarySectionResult, error)
	getArtistFn   func(ctx context.Context, browseID string) (*youtube.MusicArtistPage, error)
	getAlbumFn    func(ctx context.Context, browseID string) (*youtube.MusicAlbumPage, error)
	browseMoreFn  func(ctx context.Context, browseID, params string) ([]youtube.MusicItem, error)
	getTracksFn   func(ctx context.Context, browseID string) ([]youtube.MusicItem, string, error)

	homeCalls    int
	libSecCalls  []string
	searchMCalls []string
}

func (m *mockMusicClient) IsAuthenticated() bool { return m.authenticated }

func (m *mockMusicClient) Search(ctx context.Context, query, cont string) (*youtube.MusicSearchResult, error) {
	m.mu.Lock()
	m.searchMCalls = append(m.searchMCalls, query)
	m.mu.Unlock()
	if m.searchFn != nil {
		return m.searchFn(ctx, query, cont)
	}
	return &youtube.MusicSearchResult{}, nil
}

func (m *mockMusicClient) GetHome(ctx context.Context) ([]youtube.MusicShelf, error) {
	m.mu.Lock()
	m.homeCalls++
	m.mu.Unlock()
	if m.getHomeFn != nil {
		return m.getHomeFn(ctx)
	}
	return nil, nil
}

func (m *mockMusicClient) GetLibrarySection(ctx context.Context, browseID string) (*youtube.LibrarySectionResult, error) {
	m.mu.Lock()
	m.libSecCalls = append(m.libSecCalls, browseID)
	m.mu.Unlock()
	if m.getLibSecFn != nil {
		return m.getLibSecFn(ctx, browseID)
	}
	return &youtube.LibrarySectionResult{}, nil
}

func (m *mockMusicClient) GetLibraryContinuation(ctx context.Context, cont string) (*youtube.LibrarySectionResult, error) {
	if m.getLibContFn != nil {
		return m.getLibContFn(ctx, cont)
	}
	return &youtube.LibrarySectionResult{}, nil
}

func (m *mockMusicClient) GetArtist(ctx context.Context, browseID string) (*youtube.MusicArtistPage, error) {
	if m.getArtistFn != nil {
		return m.getArtistFn(ctx, browseID)
	}
	return &youtube.MusicArtistPage{Name: "Test Artist"}, nil
}

func (m *mockMusicClient) GetAlbum(ctx context.Context, browseID string) (*youtube.MusicAlbumPage, error) {
	if m.getAlbumFn != nil {
		return m.getAlbumFn(ctx, browseID)
	}
	return &youtube.MusicAlbumPage{Title: "Test Album"}, nil
}

func (m *mockMusicClient) BrowseMore(ctx context.Context, browseID, params string) ([]youtube.MusicItem, error) {
	if m.browseMoreFn != nil {
		return m.browseMoreFn(ctx, browseID, params)
	}
	return nil, nil
}

func (m *mockMusicClient) GetAlbumTracks(ctx context.Context, browseID string) ([]youtube.MusicItem, string, error) {
	if m.getTracksFn != nil {
		return m.getTracksFn(ctx, browseID)
	}
	return nil, "", nil
}
