package youtube

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/tidwall/gjson"
)

func loadFixture(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", path, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to parse fixture %s: %v", path, err)
	}
	return m
}

func TestParseSearchResponse(t *testing.T) {
	raw := loadFixture(t, "testdata/search_response.json")
	page, err := parseSearchResponse(raw)
	if err != nil {
		t.Fatalf("parseSearchResponse: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("expected search results, got 0")
	}

	t.Run("FirstResultFields", func(t *testing.T) {
		v := page.Items[0]
		if v.ID == "" {
			t.Error("missing ID")
		}
		if v.Title == "" {
			t.Error("missing Title")
		}
		if v.ChannelName == "" {
			t.Error("missing ChannelName")
		}
		if v.URL != VideoURL(v.ID) {
			t.Errorf("URL = %q, want %q", v.URL, VideoURL(v.ID))
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		if !page.HasMore {
			t.Error("expected HasMore=true")
		}
		if page.NextToken == "" {
			t.Error("expected non-empty NextToken")
		}
	})

	t.Run("ThumbnailsAndMetadata", func(t *testing.T) {
		// First result should have thumbnails, duration, views
		v := page.Items[0]
		if len(v.Thumbnails) == 0 {
			t.Error("expected thumbnails")
		}
		for i, th := range v.Thumbnails {
			if th.URL == "" {
				t.Errorf("thumbnail %d missing URL", i)
			}
			if th.Width == 0 || th.Height == 0 {
				t.Errorf("thumbnail %d missing dimensions", i)
			}
		}
		if v.DurationStr == "" {
			t.Error("missing DurationStr")
		}
		if v.ViewCount == "" {
			t.Error("missing ViewCount")
		}
		if v.ChannelID == "" {
			t.Error("missing ChannelID")
		}
	})
}

func TestParseSearchContinuation(t *testing.T) {
	raw := loadFixture(t, "testdata/search_continuation.json")
	page, err := parseSearchResponse(raw)
	if err != nil {
		t.Fatalf("parseSearchResponse (continuation): %v", err)
	}

	if len(page.Items) == 0 {
		t.Fatal("expected continuation results, got 0")
	}

	v := page.Items[0]
	if v.ID == "" {
		t.Error("first continuation result missing ID")
	}
	if v.Title == "" {
		t.Error("first continuation result missing Title")
	}
}

func TestEnrichVideo(t *testing.T) {
	raw := loadFixture(t, "testdata/next_response.json")

	v := &Video{ID: "446E-r0rXHI"}
	enrichVideo(v, raw)

	// Assert concrete values from the known fixture
	if v.ViewCount == "" {
		t.Error("enrichVideo did not set ViewCount")
	}
	if v.PublishedAt == "" {
		t.Error("enrichVideo did not set PublishedAt")
	}
	if v.LikeCount == "" {
		t.Error("enrichVideo did not set LikeCount")
	}
	if v.SubscriberCount == "" {
		t.Error("enrichVideo did not set SubscriberCount")
	}
	if v.Description == "" {
		t.Error("enrichVideo did not set Description")
	}

	// Verify the description looks like real content (not a gjson artifact)
	if len(v.Description) < 20 {
		t.Errorf("Description too short (%d chars), likely not parsed correctly", len(v.Description))
	}
}

func TestFormatSeconds(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0", "0:00"},
		{"59", "0:59"},
		{"60", "1:00"},
		{"61", "1:01"},
		{"3599", "59:59"},
		{"3600", "1:00:00"},
		{"3661", "1:01:01"},
		{"7200", "2:00:00"},
		{"", ""},
		{"abc", "abc"},
		{"-1", "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := formatSeconds(tt.input)
			if got != tt.want {
				t.Errorf("formatSeconds(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewInnerTubeWEB(t *testing.T) {
	it := newInnerTubeWEB(nil)
	if it.Adaptor == nil {
		t.Fatal("expected non-nil Adaptor")
	}
}

func TestNewInnerTubeMusic(t *testing.T) {
	it := newInnerTubeMusic(nil)
	if it.Adaptor == nil {
		t.Fatal("expected non-nil Adaptor")
	}
}

func TestVideoURL(t *testing.T) {
	got := VideoURL("abc123")
	want := "https://www.youtube.com/watch?v=abc123"
	if got != want {
		t.Errorf("VideoURL = %q, want %q", got, want)
	}
}

func TestParseChannelVideos(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_channel_videos_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatal(err)
	}

	var videos []Video
	var nextToken string
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		contents := tr.Get("content.richGridRenderer.contents")
		parseChannelVideoItems(contents, &videos, &nextToken)
		return false
	})

	if len(videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(videos))
	}

	v := videos[0]
	if v.ID != "fake_ch_vid_001" {
		t.Errorf("ID = %q, want fake_ch_vid_001", v.ID)
	}
	if v.Title != "Fake Channel Video One" {
		t.Errorf("Title = %q", v.Title)
	}
	if v.ChannelName != "Fake Channel" {
		t.Errorf("ChannelName = %q", v.ChannelName)
	}
	if v.DurationStr != "12:34" {
		t.Errorf("DurationStr = %q", v.DurationStr)
	}
	if v.ViewCount != "1,234 views" {
		t.Errorf("ViewCount = %q", v.ViewCount)
	}
	if len(v.Thumbnails) == 0 {
		t.Error("expected thumbnails")
	}
	if v.URL != VideoURL(v.ID) {
		t.Errorf("URL = %q", v.URL)
	}

	if nextToken != "fake_channel_videos_continuation_token" {
		t.Errorf("NextToken = %q", nextToken)
	}
}

func TestParseChannelVideosContinuation(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_channel_videos_continuation.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatal(err)
	}

	var videos []Video
	var nextToken string
	data.Get("onResponseReceivedActions").ForEach(func(_, action gjson.Result) bool {
		items := action.Get("appendContinuationItemsAction.continuationItems")
		if items.Exists() {
			parseChannelVideoItems(items, &videos, &nextToken)
		}
		return true
	})

	if len(videos) != 1 {
		t.Fatalf("expected 1 video, got %d", len(videos))
	}
	if videos[0].ID != "fake_ch_vid_003" {
		t.Errorf("ID = %q", videos[0].ID)
	}
}

func TestParseChannelPlaylists(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_channel_playlists_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatal(err)
	}

	var playlists []Playlist
	var nextToken string
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		sections := tr.Get("content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			items := section.Get("itemSectionRenderer.contents.0.gridRenderer.items")
			if items.Exists() {
				parseChannelPlaylistItems(items, &playlists, &nextToken)
			}
			return true
		})
		return false
	})

	if len(playlists) != 2 {
		t.Fatalf("expected 2 playlists, got %d", len(playlists))
	}

	p := playlists[0]
	if p.ID != "PLfake_playlist_001" {
		t.Errorf("ID = %q", p.ID)
	}
	if p.Title != "Fake Playlist One" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.VideoCount != "12" {
		t.Errorf("VideoCount = %q", p.VideoCount)
	}
	if p.URL != PlaylistURL(p.ID) {
		t.Errorf("URL = %q", p.URL)
	}
	if len(p.Thumbnails) == 0 {
		t.Error("expected thumbnails")
	}

	if nextToken != "fake_playlists_continuation_token" {
		t.Errorf("NextToken = %q", nextToken)
	}
}

func TestParseChannelPosts(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_channel_posts_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatal(err)
	}

	var posts []Post
	var nextToken string
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		sections := tr.Get("content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			section.Get("itemSectionRenderer.contents").ForEach(func(_, item gjson.Result) bool {
				bpt := item.Get("backstagePostThreadRenderer.post.backstagePostRenderer")
				if bpt.Exists() {
					posts = append(posts, parseBackstagePost(bpt))
				}
				return true
			})
			token := section.Get("continuationItemRenderer.continuationEndpoint.continuationCommand.token")
			if token.Exists() {
				nextToken = token.String()
			}
			return true
		})
		return false
	})

	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}

	p := posts[0]
	if p.ID != "fake_post_001" {
		t.Errorf("ID = %q", p.ID)
	}
	if p.AuthorName != "Fake Channel" {
		t.Errorf("AuthorName = %q", p.AuthorName)
	}
	if p.AuthorID != "UCfake_channel_001" {
		t.Errorf("AuthorID = %q", p.AuthorID)
	}
	if p.Content != "This is a fake community post with some bold text" {
		t.Errorf("Content = %q", p.Content)
	}
	if p.LikeCount != "1.2K" {
		t.Errorf("LikeCount = %q", p.LikeCount)
	}
	if p.PublishedAt != "2 days ago" {
		t.Errorf("PublishedAt = %q", p.PublishedAt)
	}
	if p.CommentsToken != "fake_post_comments_token_001" {
		t.Errorf("CommentsToken = %q", p.CommentsToken)
	}
	if len(p.Thumbnails) == 0 {
		t.Error("expected thumbnails for post with image")
	}

	// Second post has no image
	if len(posts[1].Thumbnails) != 0 {
		t.Error("expected no thumbnails for post without image")
	}

	if nextToken != "fake_posts_continuation_token" {
		t.Errorf("NextToken = %q", nextToken)
	}
}

func TestParsePlaylistVideos(t *testing.T) {
	raw := loadFixture(t, "testdata/fake_playlist_videos_response.json")
	data, err := toGJSON(raw)
	if err != nil {
		t.Fatal(err)
	}

	var videos []Video
	var nextToken string
	tabs := data.Get("contents.twoColumnBrowseResultsRenderer.tabs")
	tabs.ForEach(func(_, tab gjson.Result) bool {
		tr := tab.Get("tabRenderer")
		if !tr.Exists() || !tr.Get("selected").Bool() {
			return true
		}
		sections := tr.Get("content.sectionListRenderer.contents")
		sections.ForEach(func(_, section gjson.Result) bool {
			section.Get("itemSectionRenderer.contents").ForEach(func(_, content gjson.Result) bool {
				items := content.Get("playlistVideoListRenderer.contents")
				if items.Exists() {
					parsePlaylistVideoItems(items, &videos, &nextToken)
				}
				return true
			})
			return true
		})
		return false
	})

	if len(videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(videos))
	}

	v := videos[0]
	if v.ID != "fake_plv_001" {
		t.Errorf("ID = %q", v.ID)
	}
	if v.Title != "Playlist Video One" {
		t.Errorf("Title = %q", v.Title)
	}
	if v.ChannelName != "Fake Creator" {
		t.Errorf("ChannelName = %q", v.ChannelName)
	}
	if v.ChannelID != "UCfake_creator_001" {
		t.Errorf("ChannelID = %q", v.ChannelID)
	}
	if v.DurationStr != "10:00" {
		t.Errorf("DurationStr = %q", v.DurationStr)
	}
	if v.URL != VideoURL(v.ID) {
		t.Errorf("URL = %q", v.URL)
	}
	if len(v.Thumbnails) == 0 {
		t.Error("expected thumbnails")
	}

	if nextToken != "fake_playlist_continuation_token" {
		t.Errorf("NextToken = %q", nextToken)
	}
}

func TestParseYouTubeURL(t *testing.T) {
	tests := []struct {
		name string
		input string
		kind URLKind
		id   string
	}{
		{"standard video", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", URLVideo, "dQw4w9WgXcQ"},
		{"video no www", "https://youtube.com/watch?v=abc123", URLVideo, "abc123"},
		{"mobile video", "https://m.youtube.com/watch?v=abc123", URLVideo, "abc123"},
		{"music video", "https://music.youtube.com/watch?v=abc123", URLVideo, "abc123"},
		{"short URL", "https://youtu.be/dQw4w9WgXcQ", URLVideo, "dQw4w9WgXcQ"},
		{"short URL no scheme", "youtu.be/dQw4w9WgXcQ", URLVideo, "dQw4w9WgXcQ"},
		{"raw video ID", "dQw4w9WgXcQ", URLVideo, "dQw4w9WgXcQ"},
		{"playlist", "https://www.youtube.com/playlist?list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", URLPlaylist, "PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf"},
		{"music playlist", "https://music.youtube.com/playlist?list=OLAK5uy_test", URLPlaylist, "OLAK5uy_test"},
		{"channel ID", "https://www.youtube.com/channel/UCxxxxxx", URLChannel, "UCxxxxxx"},
		{"channel handle", "https://www.youtube.com/@TestChannel", URLChannel, "@TestChannel"},
		{"video with extra params", "https://www.youtube.com/watch?v=abc123&t=120", URLVideo, "abc123"},
		{"invidious bare path", "https://yewtu.be/EiBg91LTOYk", URLVideo, "EiBg91LTOYk"},
		{"invidious watch", "https://yewtu.be/watch?v=EiBg91LTOYk", URLVideo, "EiBg91LTOYk"},
		{"invidious other instance", "https://vid.puffyan.us/watch?v=dQw4w9WgXcQ", URLVideo, "dQw4w9WgXcQ"},
		{"empty", "", URLUnknown, ""},
		{"garbage URL", "https://example.com/something", URLUnknown, ""},
		{"invalid raw ID too long", "2434234234234", URLUnknown, ""},
		{"invalid raw ID too short", "abc", URLUnknown, ""},
		{"invalid raw ID special chars", "abc!@#$%^&*(", URLUnknown, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseYouTubeURL(tt.input)
			if got.Kind != tt.kind {
				t.Errorf("Kind = %d, want %d", got.Kind, tt.kind)
			}
			if got.ID != tt.id {
				t.Errorf("ID = %q, want %q", got.ID, tt.id)
			}
		})
	}
}
