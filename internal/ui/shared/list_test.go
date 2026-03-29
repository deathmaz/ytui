package shared

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
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
