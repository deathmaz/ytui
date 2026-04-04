package shared

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/deathmaz/ytui/internal/youtube"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 8, "hello..."},
		{"max 3", "hello", 3, "hello"},
		{"max 0", "hello", 0, "hello"},
		{"negative max", "hello", -1, "hello"},
		{"empty string", "", 10, ""},
		{"unicode", "こんにちは世界", 5, "こん..."},
		{"emoji", "👍👍👍👍👍", 4, "👍..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestVideoDelegate_Height_Default(t *testing.T) {
	d := VideoDelegate{}
	if got := d.Height(); got != 2 {
		t.Errorf("Height() = %d, want 2", got)
	}
}

func TestVideoDelegate_Height_WithThumbnails(t *testing.T) {
	d := NewVideoDelegate(nil, 5)
	if got := d.Height(); got != 5 {
		t.Errorf("Height() = %d, want 5", got)
	}
}

func TestVideoDelegate_Height_CustomValues(t *testing.T) {
	for _, rows := range []int{3, 5, 7, 10} {
		d := NewVideoDelegate(nil, rows)
		if got := d.Height(); got != rows {
			t.Errorf("Height() with ThumbRows=%d = %d, want %d", rows, got, rows)
		}
	}
}

func TestVideoDelegate_Spacing(t *testing.T) {
	d := VideoDelegate{}
	if got := d.Spacing(); got != 1 {
		t.Errorf("Spacing() = %d, want 1", got)
	}
}

func TestVideoDelegate_Render_Default(t *testing.T) {
	d := VideoDelegate{}
	l := list.New(nil, d, 80, 24)
	l.SetItems([]list.Item{
		VideoItem{Video: youtube.Video{
			ID: "v1", Title: "Test Video", ChannelName: "Test Channel",
			ViewCount: "100 views", PublishedAt: "1 day ago", DurationStr: "5:00",
		}},
	})

	var buf bytes.Buffer
	d.Render(&buf, l, 0, l.Items()[0])
	out := buf.String()

	if !strings.Contains(out, "Test Video") {
		t.Error("output should contain title")
	}
	if !strings.Contains(out, "Test Channel") {
		t.Error("output should contain channel name")
	}
	if !strings.Contains(out, "100 views") {
		t.Error("output should contain view count")
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Errorf("default render should have 2 lines, got %d", len(lines))
	}
}

func TestVideoDelegate_Render_WithThumbnails_NoImage(t *testing.T) {
	// With ThumbRows set but no ImgR, should render with empty thumbnail space
	d := NewVideoDelegate(nil, 5)
	l := list.New(nil, d, 80, 24)
	l.SetItems([]list.Item{
		VideoItem{Video: youtube.Video{
			ID: "v1", Title: "Test Video", ChannelName: "Test Channel",
			ViewCount: "100 views", PublishedAt: "1 day ago", DurationStr: "5:00",
		}},
	})

	var buf bytes.Buffer
	d.Render(&buf, l, 0, l.Items()[0])
	out := buf.String()

	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Errorf("thumbnail render should have 5 lines, got %d", len(lines))
	}
	if !strings.Contains(out, "Test Video") {
		t.Error("output should contain title")
	}
	if !strings.Contains(out, "Test Channel") {
		t.Error("output should contain channel name")
	}
}

func TestVideoDelegate_ThumbCols(t *testing.T) {
	d := NewVideoDelegate(nil, 5)
	if got := d.thumbCols(); got != 20 {
		t.Errorf("thumbCols() with ThumbRows=5 = %d, want 20", got)
	}
	d = NewVideoDelegate(nil, 10)
	if got := d.thumbCols(); got != 40 {
		t.Errorf("thumbCols() with ThumbRows=10 = %d, want 40", got)
	}
}

func TestBestThumbnail(t *testing.T) {
	tests := []struct {
		name string
		v    youtube.Video
		want string
	}{
		{
			name: "picks largest by width",
			v: youtube.Video{
				Thumbnails: []youtube.Thumbnail{
					{URL: "http://small.jpg", Width: 120},
					{URL: "http://large.jpg", Width: 480},
					{URL: "http://medium.jpg", Width: 320},
				},
			},
			want: "http://large.jpg",
		},
		{
			name: "single thumbnail",
			v: youtube.Video{
				Thumbnails: []youtube.Thumbnail{
					{URL: "http://only.jpg", Width: 320},
				},
			},
			want: "http://only.jpg",
		},
		{
			name: "fallback to hqdefault",
			v:    youtube.Video{ID: "fake_vid_001"},
			want: "https://i.ytimg.com/vi/fake_vid_001/hqdefault.jpg",
		},
		{
			name: "no ID no thumbnails",
			v:    youtube.Video{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BestThumbnail(tt.v)
			if got != tt.want {
				t.Errorf("BestThumbnail() = %q, want %q", got, tt.want)
			}
		})
	}
}
