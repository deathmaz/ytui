package app

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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

	it := mi.item
	isSelected := index == m.Index()

	cursor := "  "
	if isSelected {
		cursor = "> "
	}

	// Type icon
	icon := typeIcon(it.Type)

	titleStyle := styles.Title
	if isSelected {
		titleStyle = styles.SelectedTitle
	}
	title := titleStyle.Render(shared.Truncate(icon+" "+it.Title, m.Width()-4))

	meta := styles.Dim.Render(shared.Truncate(it.Subtitle, m.Width()-4))

	fmt.Fprintf(w, "%s%s\n%s  %s", cursor, title, "  ", meta)
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
