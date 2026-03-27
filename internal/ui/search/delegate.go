package search

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/deathmaz/ytui/internal/ui/styles"
	"github.com/deathmaz/ytui/internal/youtube"
)

// videoItem wraps a Video for the list component.
type videoItem struct {
	video youtube.Video
}

func (v videoItem) FilterValue() string { return v.video.Title }
func (v videoItem) Title() string       { return v.video.Title }
func (v videoItem) Description() string { return v.video.ChannelName }

// videoDelegate renders video items in the list.
type videoDelegate struct{}

func (d videoDelegate) Height() int                             { return 3 }
func (d videoDelegate) Spacing() int                            { return 0 }
func (d videoDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d videoDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	vi, ok := item.(videoItem)
	if !ok {
		return
	}

	v := vi.video
	isSelected := index == m.Index()

	// Cursor
	cursor := "  "
	if isSelected {
		cursor = "> "
	}

	// Title line
	titleStyle := styles.Title
	if isSelected {
		titleStyle = styles.SelectedTitle
	}
	title := titleStyle.Render(truncate(v.Title, m.Width()-4))

	// Meta line
	meta := styles.Subtitle.Render(v.ChannelName)
	if v.ViewCount != "" {
		meta += styles.Dim.Render("  "+v.ViewCount)
	}
	if v.PublishedAt != "" {
		meta += styles.Dim.Render("  "+v.PublishedAt)
	}

	// Duration
	dur := ""
	if v.DurationStr != "" {
		dur = styles.Accent.Render("  " + v.DurationStr)
	}

	fmt.Fprintf(w, "%s%s%s\n%s  %s\n", cursor, title, dur, "  ", meta)
}

func truncate(s string, max int) string {
	if max <= 3 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-3]) + "..."
}
