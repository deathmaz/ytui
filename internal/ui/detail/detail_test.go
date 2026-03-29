package detail

import (
	"testing"

	"github.com/deathmaz/ytui/internal/youtube"
)

func TestBestThumbnail(t *testing.T) {
	t.Run("PicksLargest", func(t *testing.T) {
		v := &youtube.Video{
			ID: "test123",
			Thumbnails: []youtube.Thumbnail{
				{URL: "https://example.com/small.jpg", Width: 120, Height: 90},
				{URL: "https://example.com/large.jpg", Width: 480, Height: 360},
				{URL: "https://example.com/medium.jpg", Width: 320, Height: 180},
			},
		}
		got := bestThumbnail(v)
		if got != "https://example.com/large.jpg" {
			t.Errorf("bestThumbnail = %q, want large.jpg", got)
		}
	})

	t.Run("FallbackWhenEmpty", func(t *testing.T) {
		v := &youtube.Video{ID: "abc123"}
		got := bestThumbnail(v)
		want := "https://i.ytimg.com/vi/abc123/hqdefault.jpg"
		if got != want {
			t.Errorf("bestThumbnail = %q, want %q", got, want)
		}
	})

	t.Run("EmptyVideoID", func(t *testing.T) {
		v := &youtube.Video{}
		got := bestThumbnail(v)
		if got != "" {
			t.Errorf("bestThumbnail = %q, want empty", got)
		}
	})

	t.Run("SingleThumbnail", func(t *testing.T) {
		v := &youtube.Video{
			Thumbnails: []youtube.Thumbnail{
				{URL: "https://example.com/only.jpg", Width: 200, Height: 150},
			},
		}
		got := bestThumbnail(v)
		if got != "https://example.com/only.jpg" {
			t.Errorf("bestThumbnail = %q, want only.jpg", got)
		}
	})
}
