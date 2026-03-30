package shared

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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

// VideoDelegate renders video items in a list.
type VideoDelegate struct{}

func (d VideoDelegate) Height() int                             { return 2 }
func (d VideoDelegate) Spacing() int                            { return 1 }
func (d VideoDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d VideoDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	vi, ok := item.(VideoItem)
	if !ok {
		return
	}

	v := vi.Video
	isSelected := index == m.Index()

	cursor := "  "
	if isSelected {
		cursor = "> "
	}

	titleStyle := styles.Title
	if isSelected {
		titleStyle = styles.SelectedTitle
	}
	title := titleStyle.Render(Truncate(v.Title, m.Width()-4))

	meta := styles.Subtitle.Render(v.ChannelName)
	if v.ViewCount != "" {
		meta += styles.Dim.Render("  " + v.ViewCount)
	}
	if v.PublishedAt != "" {
		meta += styles.Dim.Render("  " + v.PublishedAt)
	}

	dur := ""
	if v.DurationStr != "" {
		dur = styles.Accent.Render("  " + v.DurationStr)
	}

	fmt.Fprintf(w, "%s%s%s\n%s  %s", cursor, title, dur, "  ", meta)
}

// Truncate truncates a string to max runes, appending "..." if needed.
func Truncate(s string, max int) string {
	if max <= 3 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-3]) + "..."
}
