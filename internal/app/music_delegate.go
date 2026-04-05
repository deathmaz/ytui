package app

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/ui/shared"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

type musicDelegate struct{}

func (d musicDelegate) Height() int                             { return 2 }
func (d musicDelegate) Spacing() int                            { return 1 }
func (d musicDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d musicDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	mi, ok := item.(musicItem)
	if !ok {
		return
	}
	isSelected := index == m.Index()
	cursor, title, meta := musicTextParts(mi.item, isSelected, m.Width()-4)
	fmt.Fprintf(w, "%s%s\n%s  %s", cursor, title, "  ", meta)
}

// musicTrackDelegate renders album tracks (compact, one line per track).
type musicTrackDelegate struct{}

func (d musicTrackDelegate) Height() int                             { return 1 }
func (d musicTrackDelegate) Spacing() int                            { return 0 }
func (d musicTrackDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d musicTrackDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	mi, ok := item.(musicItem)
	if !ok {
		return
	}
	isSelected := index == m.Index()
	cursor := "  "
	if isSelected {
		cursor = "> "
	}
	titleStyle := styles.Title
	if isSelected {
		titleStyle = styles.SelectedTitle
	}
	title := titleStyle.Render(shared.Truncate(mi.item.Title, m.Width()-20))
	dur := styles.Dim.Render(mi.item.Subtitle)
	fmt.Fprintf(w, "%s♪ %s  %s", cursor, title, dur)
}

// newMusicDelegate returns a thumbnail-aware delegate when thumbnails are
// enabled, otherwise the plain text-only musicDelegate.
func newMusicDelegate(imgR *ytimage.Renderer, thumbRows int) list.ItemDelegate {
	if imgR == nil || thumbRows <= 0 {
		return musicDelegate{}
	}
	return shared.NewThumbDelegate(imgR, thumbRows, albumThumbURL, renderMusicText)
}

// albumThumbURL extracts a thumbnail URL from album items only.
func albumThumbURL(item list.Item) string {
	mi, ok := item.(musicItem)
	if !ok || mi.item.Type != youtube.MusicAlbum {
		return ""
	}
	return shared.BestThumbnailURL(mi.item.Thumbnails)
}

func renderMusicText(w io.Writer, item list.Item, m list.Model, isSelected bool, width int) {
	mi, ok := item.(musicItem)
	if !ok {
		return
	}
	cursor, title, meta := musicTextParts(mi.item, isSelected, width)
	fmt.Fprintf(w, "%s%s\n  %s", cursor, title, meta)
}

func musicTextParts(it youtube.MusicItem, isSelected bool, width int) (cursor, title, meta string) {
	cursor = "  "
	if isSelected {
		cursor = "> "
	}
	icon := typeIcon(it.Type)
	titleStyle := styles.Title
	if isSelected {
		titleStyle = styles.SelectedTitle
	}
	title = titleStyle.Render(shared.Truncate(icon+" "+it.Title, width))
	meta = styles.Dim.Render(shared.Truncate(it.Subtitle, width))
	return
}

func typeIcon(t youtube.MusicItemType) string {
	switch t {
	case youtube.MusicSong:
		return "♪"
	case youtube.MusicAlbum:
		return "◉"
	case youtube.MusicArtist:
		return "♫"
	case youtube.MusicPlaylist:
		return "≡"
	case youtube.MusicVideo:
		return "▶"
	default:
		return "•"
	}
}
