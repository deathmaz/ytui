package shared

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	ytimage "github.com/deathmaz/ytui/internal/image"
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
type VideoDelegate struct {
	ImgR      *ytimage.Renderer
	ThumbRows int // thumbnail height in rows; 0 = no thumbnails
}

// NewVideoDelegate creates a VideoDelegate with thumbnail support.
func NewVideoDelegate(imgR *ytimage.Renderer, thumbRows int) VideoDelegate {
	return VideoDelegate{
		ImgR:      imgR,
		ThumbRows: thumbRows,
	}
}

func (d VideoDelegate) Height() int {
	if d.ThumbRows > 0 {
		return d.ThumbRows
	}
	return 2
}

func (d VideoDelegate) Spacing() int { return 1 }

func (d VideoDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	if d.ImgR == nil || d.ThumbRows <= 0 {
		return nil
	}

	// Only trigger thumbnail fetches on meaningful events.
	// Skip high-frequency messages like spinner ticks.
	if _, ok := msg.(spinner.TickMsg); ok {
		return nil
	}

	items := m.Items()
	if len(items) == 0 {
		return nil
	}

	thumbCols := d.thumbCols()
	var cmds []tea.Cmd
	for _, item := range items {
		vi, ok := item.(VideoItem)
		if !ok {
			continue
		}
		url := BestThumbnail(vi.Video)
		if url == "" {
			continue
		}
		cmd := d.ImgR.FetchCmd(url, thumbCols, d.ThumbRows)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (d VideoDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	vi, ok := item.(VideoItem)
	if !ok {
		return
	}

	v := vi.Video
	isSelected := index == m.Index()

	if d.ThumbRows > 0 {
		d.renderWithThumb(w, m, v, isSelected)
		return
	}

	cursor, title, dur, meta := videoTextParts(v, isSelected, m.Width()-4)
	fmt.Fprintf(w, "%s%s%s\n%s  %s", cursor, title, dur, "  ", meta)
}

func (d VideoDelegate) renderWithThumb(w io.Writer, m list.Model, v youtube.Video, isSelected bool) {
	thumbCols := d.thumbCols()

	// Get thumbnail placeholder from cache (transmits are handled by the
	// search model's View, not here — keeping APC escapes out of the
	// delegate output prevents the list from miscalculating layout).
	var thumbLines []string
	url := BestThumbnail(v)
	if url != "" && d.ImgR != nil {
		_, pl := d.ImgR.Get(url)
		if pl != "" {
			thumbLines = strings.Split(pl, "\n")
		}
	}

	availWidth := m.Width() - thumbCols - 3 // thumb + gap + cursor
	cursor, title, dur, meta := videoTextParts(v, isSelected, availWidth)

	textLines := []string{
		cursor + title + dur,
		"  " + meta,
	}

	// Render line-by-line: thumbnail on left, text on right
	emptyThumb := strings.Repeat(" ", thumbCols)
	for i := 0; i < d.ThumbRows; i++ {
		if i < len(thumbLines) {
			fmt.Fprint(w, thumbLines[i])
		} else {
			fmt.Fprint(w, emptyThumb)
		}
		fmt.Fprint(w, " ") // gap between thumbnail and text
		if i < len(textLines) {
			fmt.Fprint(w, textLines[i])
		}
		if i < d.ThumbRows-1 {
			fmt.Fprint(w, "\n")
		}
	}
}

// videoTextParts builds the common text elements for a video item.
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

func (d VideoDelegate) thumbCols() int {
	return d.ThumbRows * 4
}

// BestThumbnail returns the URL of the largest thumbnail, or a fallback.
func BestThumbnail(v youtube.Video) string {
	if len(v.Thumbnails) > 0 {
		best := v.Thumbnails[0]
		for _, t := range v.Thumbnails[1:] {
			if t.Width > best.Width {
				best = t
			}
		}
		return best.URL
	}
	if v.ID != "" {
		return "https://i.ytimg.com/vi/" + v.ID + "/hqdefault.jpg"
	}
	return ""
}

// Truncate truncates a string to max runes, appending "..." if needed.
func Truncate(s string, max int) string {
	if max <= 3 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-3]) + "..."
}
