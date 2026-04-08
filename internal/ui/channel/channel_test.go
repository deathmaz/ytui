package channel

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/config"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/youtube"
)

type stubClient struct{ youtube.Client }

func (stubClient) IsAuthenticated() bool { return false }
func (stubClient) Search(_ context.Context, _, _ string) (*youtube.Page[youtube.Video], error) {
	return &youtube.Page[youtube.Video]{}, nil
}
func (stubClient) GetVideo(_ context.Context, _ string) (*youtube.Video, error) {
	return &youtube.Video{}, nil
}
func (stubClient) GetComments(_ context.Context, _, _ string) (*youtube.Page[youtube.Comment], error) {
	return &youtube.Page[youtube.Comment]{}, nil
}
func (stubClient) GetReplies(_ context.Context, _, _ string) (*youtube.Page[youtube.Comment], error) {
	return &youtube.Page[youtube.Comment]{}, nil
}
func (stubClient) GetSubscriptions(_ context.Context, _ string) (*youtube.Page[youtube.Channel], error) {
	return &youtube.Page[youtube.Channel]{}, nil
}
func (stubClient) GetFeed(_ context.Context, _ string) (*youtube.Page[youtube.Video], error) {
	return &youtube.Page[youtube.Video]{}, nil
}
func (stubClient) GetChannelVideos(_ context.Context, _, _ string) (*youtube.Page[youtube.Video], error) {
	return &youtube.Page[youtube.Video]{}, nil
}
func (stubClient) GetChannelPlaylists(_ context.Context, _, _ string) (*youtube.Page[youtube.Playlist], error) {
	return &youtube.Page[youtube.Playlist]{}, nil
}
func (stubClient) GetChannelPosts(_ context.Context, _, _ string) (*youtube.Page[youtube.Post], error) {
	return &youtube.Page[youtube.Post]{}, nil
}
func (stubClient) GetChannelStreams(_ context.Context, _, _ string) (*youtube.Page[youtube.Video], error) {
	return &youtube.Page[youtube.Video]{}, nil
}
func (stubClient) GetPlaylistVideos(_ context.Context, _, _ string) (*youtube.Page[youtube.Video], error) {
	return &youtube.Page[youtube.Video]{}, nil
}
func (stubClient) GetPostComments(_ context.Context, _, _ string) (*youtube.Page[youtube.Comment], error) {
	return &youtube.Page[youtube.Comment]{}, nil
}

func newTestChannel(imgR *ytimage.Renderer, thumbH int) Model {
	cfg := config.ThumbnailConfig{Enabled: true, Height: thumbH}
	thumbList := shared.NewThumbList(imgR, shared.VideoThumbURL, thumbH)
	delegate := shared.NewThumbDelegate(imgR, thumbH, shared.VideoThumbURL, shared.RenderVideoText)
	m := New(stubClient{}, delegate, thumbList, cfg)
	m.SetSize(80, 24)
	return m
}

// TestRefetchThumbs_VideosTab verifies that RefetchThumbs returns a non-nil
// cmd when the videos sub-tab has visible items with evicted cache entries.
func TestRefetchThumbs_VideosTab(t *testing.T) {
	imgR := ytimage.NewRendererWithMax(2)
	m := newTestChannel(imgR, 5)

	// Populate video list with an item.
	imgR.Store("https://fake/v1.jpg", "TX_V1", "PL_V1")
	m.videoList.SetItems([]list.Item{shared.VideoItem{Video: youtube.Video{
		ID: "v1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake/v1.jpg", Width: 320}},
	}}})

	m.activeTab = tabVideos

	// All cached → nil.
	if cmd := m.RefetchThumbs(); cmd != nil {
		t.Error("should return nil when all cached")
	}

	// Evict v1 by filling the LRU.
	imgR.Store("https://fake/x.jpg", "TX_X", "PL_X")
	imgR.Store("https://fake/y.jpg", "TX_Y", "PL_Y") // evicts v1

	if cmd := m.RefetchThumbs(); cmd == nil {
		t.Error("should return non-nil for evicted video thumbnail")
	}
}

// TestRefetchThumbs_StreamsTab verifies that switching to the streams
// sub-tab triggers refetch for evicted stream thumbnails.
func TestRefetchThumbs_StreamsTab(t *testing.T) {
	imgR := ytimage.NewRendererWithMax(2)
	m := newTestChannel(imgR, 5)

	imgR.Store("https://fake/s1.jpg", "TX_S1", "PL_S1")
	m.streamList.SetItems([]list.Item{shared.VideoItem{Video: youtube.Video{
		ID: "s1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake/s1.jpg", Width: 320}},
	}}})
	m.streamLoaded = true

	m.activeTab = tabStreams

	if cmd := m.RefetchThumbs(); cmd != nil {
		t.Error("should return nil when all cached")
	}

	// Evict s1.
	imgR.Store("https://fake/x.jpg", "TX_X", "PL_X")
	imgR.Store("https://fake/y.jpg", "TX_Y", "PL_Y")

	if cmd := m.RefetchThumbs(); cmd == nil {
		t.Error("should return non-nil for evicted stream thumbnail")
	}
}

// TestRefetchThumbs_PlaylistsTab verifies refetch for the playlists sub-tab
// which uses a separate plThumbList instance.
func TestRefetchThumbs_PlaylistsTab(t *testing.T) {
	imgR := ytimage.NewRendererWithMax(2)
	m := newTestChannel(imgR, 5)

	imgR.Store("https://fake/pl1.jpg", "TX_PL1", "PL_PL1")
	m.playlistList.SetItems([]list.Item{shared.PlaylistItem{Playlist: youtube.Playlist{
		ID: "pl1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake/pl1.jpg", Width: 320}},
	}}})
	m.playlistLoaded = true

	m.activeTab = tabPlaylists

	if cmd := m.RefetchThumbs(); cmd != nil {
		t.Error("should return nil when all cached")
	}

	// Evict pl1 (shared renderer).
	imgR.Store("https://fake/x.jpg", "TX_X", "PL_X")
	imgR.Store("https://fake/y.jpg", "TX_Y", "PL_Y")

	if cmd := m.RefetchThumbs(); cmd == nil {
		t.Error("should return non-nil for evicted playlist thumbnail")
	}
}

// TestOnTabSwitch_IncludesRefetch verifies that onTabSwitch includes a
// refetch command for the target sub-tab's thumbnails.
func TestOnTabSwitch_IncludesRefetch(t *testing.T) {
	imgR := ytimage.NewRendererWithMax(2)
	m := newTestChannel(imgR, 5)

	// Set up videos with a cached thumbnail.
	imgR.Store("https://fake/v1.jpg", "TX_V1", "PL_V1")
	m.videoList.SetItems([]list.Item{shared.VideoItem{Video: youtube.Video{
		ID: "v1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake/v1.jpg", Width: 320}},
	}}})
	m.videoLoaded = true

	// Evict v1.
	imgR.Store("https://fake/x.jpg", "TX_X", "PL_X")
	imgR.Store("https://fake/y.jpg", "TX_Y", "PL_Y")

	// Switch to videos tab — onTabSwitch should include a refetch.
	m.activeTab = tabVideos
	cmd := m.onTabSwitch()
	if cmd == nil {
		t.Error("onTabSwitch to videos should return refetch cmd for evicted thumbnail")
	}
}
