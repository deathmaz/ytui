package shared

import (
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/deathmaz/ytui/internal/youtube"
)

func TestNewList(t *testing.T) {
	l := NewList(VideoDelegate{})

	// Verify the list is configured correctly
	if l.FilteringEnabled() {
		t.Error("expected filtering disabled")
	}
	if l.ShowTitle() {
		t.Error("expected title hidden")
	}
}

func TestVideoItem(t *testing.T) {
	item := VideoItem{
		Video: youtube.Video{
			Title:       "Test Video",
			ChannelName: "Test Channel",
		},
	}

	if item.FilterValue() != "Test Video" {
		t.Errorf("FilterValue = %q, want %q", item.FilterValue(), "Test Video")
	}
	if item.Title() != "Test Video" {
		t.Errorf("Title = %q, want %q", item.Title(), "Test Video")
	}
	if item.Description() != "Test Channel" {
		t.Errorf("Description = %q, want %q", item.Description(), "Test Channel")
	}

	// Verify it implements list.Item
	var _ list.Item = item
}

func TestShouldLoadMore(t *testing.T) {
	mkItems := func(n int) []list.Item {
		out := make([]list.Item, n)
		for i := 0; i < n; i++ {
			out[i] = VideoItem{Video: youtube.Video{Title: "x"}}
		}
		return out
	}

	cases := []struct {
		name      string
		total     int
		index     int
		threshold int
		want      bool
	}{
		{"empty", 0, 0, 5, false},
		{"short list at top", 3, 0, 5, false},
		{"short list after one j", 3, 1, 5, false},
		{"short list at last item", 3, 2, 5, true},
		{"list equal to threshold at top", 5, 0, 5, false},
		{"list equal to threshold middle", 5, 2, 5, false},
		{"list equal to threshold at end", 5, 4, 5, true},
		{"longer list at top", 10, 0, 5, false},
		{"longer list one-before-threshold", 10, 4, 5, false},
		{"longer list at threshold boundary", 10, 5, 5, true},
		{"longer list at end", 10, 9, 5, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewList(VideoDelegate{})
			l.SetItems(mkItems(tc.total))
			l.Select(tc.index)
			got := ShouldLoadMore(l, tc.threshold)
			if got != tc.want {
				t.Errorf("ShouldLoadMore(total=%d, index=%d, threshold=%d) = %v, want %v",
					tc.total, tc.index, tc.threshold, got, tc.want)
			}
		})
	}
}
