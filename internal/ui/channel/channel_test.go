package channel

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/deathmaz/ytui/internal/config"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/imagetest"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/youtube"
)

func installRawCapture(t *testing.T) *imagetest.Capture { return imagetest.Install(t) }

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

// TestRenderSpinner_IncludesDeleteAll verifies that loading spinners route
// through WrapView so that DELETE_STALE fires and clears stale images.
func TestRenderSpinner_IncludesDeleteAll(t *testing.T) {
	imgR := ytimage.NewRendererWithMax(200)
	m := newTestChannel(imgR, 5)
	m.channel = youtube.Channel{ID: "ch1", Name: "TestChan"}
	deleteAll := ytimage.DeleteAll()

	tests := []struct {
		name  string
		setup func()
		render func() string
	}{
		{
			name: "videos_loading",
			setup: func() {
				m.videoLoading = true
				m.videoLoaded = false
				m.thumbList.Invalidate()
			},
			render: m.renderVideos,
		},
		{
			name: "playlists_loading",
			setup: func() {
				m.playlistLoading = true
				m.playlistLoaded = false
				m.plThumbList.Invalidate()
			},
			render: m.renderPlaylists,
		},
		{
			name: "streams_loading",
			setup: func() {
				m.streamLoading = true
				m.streamLoaded = false
				m.thumbList.Invalidate()
			},
			render: m.renderStreams,
		},
		{
			name: "posts_loading",
			setup: func() {
				m.postLoading = true
				m.postLoaded = false
				m.thumbList.Invalidate()
			},
			render: m.renderPosts,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := installRawCapture(t)
			tt.setup()
			_ = tt.render()
			if !strings.Contains(cap.String(), deleteAll) {
				t.Errorf("loading spinner should include DeleteAll to clear stale images")
			}
		})
	}
}

// TestRenderPosts_RoutedThroughWrapView verifies that even loaded posts
// (which have no thumbnails) route through WrapView to clear stale images.
func TestRenderPosts_RoutedThroughWrapView(t *testing.T) {
	cap := installRawCapture(t)
	imgR := ytimage.NewRendererWithMax(200)
	m := newTestChannel(imgR, 5)

	// Simulate: videos were showing, then user switches to posts.
	imgR.Store("https://fake/v1.jpg", "TX_V1", "PL_V1")
	m.postLoaded = true
	m.postLoading = false

	// Invalidate (as onTabSwitch does for posts).
	m.thumbList.Invalidate()
	_ = m.renderPosts()
	deleteAll := ytimage.DeleteAll()
	if !strings.Contains(cap.String(), deleteAll) {
		t.Error("posts view should include DeleteAll after invalidation to clear stale video images")
	}

	// Subsequent render should skip (no more DeleteAll).
	cap.Reset()
	_ = m.renderPosts()
	if strings.Contains(cap.String(), deleteAll) {
		t.Error("posts view should not include DeleteAll on subsequent stable frames")
	}
}

// TestOnTabSwitch_PostsInvalidates verifies that switching to the posts
// sub-tab invalidates the video ThumbList so stale images are cleared.
func TestOnTabSwitch_PostsInvalidates(t *testing.T) {
	cap := installRawCapture(t)
	imgR := ytimage.NewRendererWithMax(200)
	m := newTestChannel(imgR, 5)

	// Simulate stable video images.
	imgR.Store("https://fake/v1.jpg", "TX_V1", "PL_V1")
	items := []list.Item{shared.VideoItem{Video: youtube.Video{
		ID: "v1", Thumbnails: []youtube.Thumbnail{{URL: "https://fake/v1.jpg", Width: 320}},
	}}}
	m.videoList.SetItems(items)
	m.videoLoaded = true

	// Stabilize the video ThumbList.
	for i := 0; i < 5; i++ {
		m.thumbList.WrapView(shared.VisibleItems(m.videoList), "V")
	}

	// Posts already loaded (so loadPosts returns nil early).
	m.postLoaded = true

	// Switch to posts.
	m.activeTab = tabPosts
	m.onTabSwitch()

	// renderPosts should now include DeleteAll.
	cap.Reset()
	_ = m.renderPosts()
	if !strings.Contains(cap.String(), ytimage.DeleteAll()) {
		t.Error("after switching to posts, renderPosts should clear stale video images")
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
