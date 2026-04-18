package shared

import (
	"fmt"
	"io"
	"unicode/utf8"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

// VideoItem wraps a Video for use with bubbles/list.
type VideoItem struct {
	Video youtube.Video
}

func (v VideoItem) FilterValue() string { return v.Video.Title }
func (v VideoItem) Title() string       { return v.Video.Title }
func (v VideoItem) Description() string { return v.Video.ChannelName }

// VideoDelegate renders video items in a text-only two-line list.
// For thumbnail-enabled lists, use NewThumbDelegate with VideoThumbURL
// and RenderVideoText instead.
type VideoDelegate struct{}

func (d VideoDelegate) Height() int                             { return 2 }
func (d VideoDelegate) Spacing() int                            { return 1 }
func (d VideoDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d VideoDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	vi, ok := item.(VideoItem)
	if !ok {
		return
	}
	isSelected := index == m.Index()
	cursor, title, dur, meta := videoTextParts(vi.Video, isSelected, m.Width()-4)
	fmt.Fprintf(w, "%s%s%s\n%s  %s", cursor, title, dur, "  ", meta)
}

// VideoThumbURL extracts the thumbnail URL from a VideoItem.
// Returns "" for non-VideoItem items.
func VideoThumbURL(item list.Item) string {
	vi, ok := item.(VideoItem)
	if !ok {
		return ""
	}
	return BestThumbnail(vi.Video)
}

// RenderVideoText renders the text portion of a video item for use
// alongside a thumbnail. This is the ThumbRenderFunc for video lists.
func RenderVideoText(w io.Writer, item list.Item, m list.Model, isSelected bool, width int) {
	vi, ok := item.(VideoItem)
	if !ok {
		return
	}
	cursor, title, dur, meta := videoTextParts(vi.Video, isSelected, width)
	fmt.Fprintf(w, "%s%s%s\n  %s", cursor, title, dur, meta)
}

func videoTextParts(v youtube.Video, isSelected bool, titleWidth int) (cursor, title, dur, meta string) {
	cursor = "  "
	if isSelected {
		cursor = "> "
	}
	titleStyle := styles.Title
	if isSelected {
		titleStyle = styles.SelectedTitle
	}
	title = titleStyle.Render(Truncate(v.Title, titleWidth))
	if v.DurationStr != "" {
		dur = styles.Accent.Render("  " + v.DurationStr)
	}
	meta = styles.Subtitle.Render(v.ChannelName)
	if v.ViewCount != "" {
		meta += styles.Dim.Render("  " + v.ViewCount)
	}
	if v.PublishedAt != "" {
		meta += styles.Dim.Render("  " + v.PublishedAt)
	}
	return
}

// BestThumbnail returns the URL of the largest thumbnail for a video,
// falling back to the standard YouTube thumbnail URL if none are available.
func BestThumbnail(v youtube.Video) string {
	if url := BestThumbnailURL(v.Thumbnails); url != "" {
		return url
	}
	if v.ID != "" {
		return "https://i.ytimg.com/vi/" + v.ID + "/hqdefault.jpg"
	}
	return ""
}

// BestThumbnailURL returns the URL of the largest thumbnail from a slice.
func BestThumbnailURL(thumbs []youtube.Thumbnail) string {
	if len(thumbs) == 0 {
		return ""
	}
	best := thumbs[0]
	for _, t := range thumbs[1:] {
		if t.Width > best.Width {
			best = t
		}
	}
	return best.URL
}

// Truncate truncates a string to max runes, appending "..." if needed.
func Truncate(s string, max int) string {
	if max <= 3 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-3]) + "..."
}
