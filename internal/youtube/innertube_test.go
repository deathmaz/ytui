package youtube

import (
	"encoding/json"
	"os"
	"testing"
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
