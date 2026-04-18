package shared

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

// PlaylistItem wraps a Playlist for use with bubbles/list.
type PlaylistItem struct {
	Playlist youtube.Playlist
}

func (p PlaylistItem) FilterValue() string { return p.Playlist.Title }
func (p PlaylistItem) Title() string       { return p.Playlist.Title }
func (p PlaylistItem) Description() string { return p.Playlist.VideoCount }

// PlaylistThumbURL extracts the thumbnail URL from a PlaylistItem.
func PlaylistThumbURL(item list.Item) string {
	pi, ok := item.(PlaylistItem)
	if !ok {
		return ""
	}
	return BestThumbnailURL(pi.Playlist.Thumbnails)
}

// RenderPlaylistText renders the text portion of a playlist item for use
// alongside a thumbnail. This is the ThumbRenderFunc for playlist lists.
func RenderPlaylistText(w io.Writer, item list.Item, m list.Model, isSelected bool, width int) {
	pi, ok := item.(PlaylistItem)
	if !ok {
		return
	}
	cursor, title, meta := playlistTextParts(pi.Playlist, isSelected, width)
	fmt.Fprintf(w, "%s%s\n  %s", cursor, title, meta)
}

func playlistTextParts(p youtube.Playlist, isSelected bool, titleWidth int) (cursor, title, meta string) {
	cursor = "  "
	if isSelected {
		cursor = "> "
	}
	titleStyle := styles.Title
	if isSelected {
		titleStyle = styles.SelectedTitle
	}
	title = titleStyle.Render(Truncate(p.Title, titleWidth))
	if p.VideoCount != "" {
		meta = styles.Dim.Render(p.VideoCount)
	}
	return
}

// PlaylistDelegate renders playlist items in a text-only two-line list.
type PlaylistDelegate struct{}

func (d PlaylistDelegate) Height() int                             { return 2 }
func (d PlaylistDelegate) Spacing() int                            { return 1 }
func (d PlaylistDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d PlaylistDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	pi, ok := item.(PlaylistItem)
	if !ok {
		return
	}
	isSelected := index == m.Index()
	cursor, title, meta := playlistTextParts(pi.Playlist, isSelected, m.Width()-4)
	fmt.Fprintf(w, "%s%s\n%s  %s", cursor, title, "  ", meta)
}
