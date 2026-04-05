package shared

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	ytimage "github.com/deathmaz/ytui/internal/image"
)

// ThumbList manages Kitty image transmit sequences for lists with thumbnails.
// One ThumbList per renderer, shared across all lists using that renderer.
type ThumbList struct {
	imgR        *ytimage.Renderer
	transmitted map[string]bool
	getURL      func(list.Item) string
}

// NewThumbList creates a new ThumbList with the given renderer and URL extractor.
func NewThumbList(imgR *ytimage.Renderer, getURL func(list.Item) string) *ThumbList {
	return &ThumbList{
		imgR:        imgR,
		transmitted: make(map[string]bool),
		getURL:      getURL,
	}
}

// Renderer returns the underlying image renderer, or nil.
func (t *ThumbList) Renderer() *ytimage.Renderer {
	if t == nil {
		return nil
	}
	return t.imgR
}

// HandleMsg processes a ThumbnailLoadedMsg if it was initiated by this
// renderer. Returns true if handled.
func (t *ThumbList) HandleMsg(msg tea.Msg) bool {
	if t == nil || t.imgR == nil {
		return false
	}
	if tlm, ok := msg.(ytimage.ThumbnailLoadedMsg); ok {
		return t.imgR.HandleLoaded(tlm)
	}
	return false
}

// WrapView prepends Kitty transmit sequences for cached but not-yet-transmitted
// images to the rendered view string.
func (t *ThumbList) WrapView(items []list.Item, view string) string {
	if t == nil || t.imgR == nil {
		return view
	}
	var tx strings.Builder
	for _, item := range items {
		url := t.getURL(item)
		if url == "" || t.transmitted[url] {
			continue
		}
		transmitStr, pl := t.imgR.Get(url)
		if pl != "" && transmitStr != "" {
			t.transmitted[url] = true
			tx.WriteString(transmitStr)
		}
	}
	if tx.Len() > 0 {
		return tx.String() + view
	}
	return view
}

// TriggerFetch forwards msg to a list so that the delegate's Update fires
// and triggers thumbnail fetches for newly loaded items. Call this after
// setting items on a list (e.g., after SetItems in a loaded-message handler).
func (t *ThumbList) TriggerFetch(l *list.Model, msg tea.Msg) tea.Cmd {
	if t == nil {
		return nil
	}
	var cmd tea.Cmd
	*l, cmd = l.Update(msg)
	return cmd
}

// ThumbRenderFunc renders the text content for an item alongside a thumbnail.
// width is the available width for text (total width minus thumbnail area).
type ThumbRenderFunc func(w io.Writer, item list.Item, m list.Model, isSelected bool, width int)

// thumbDelegate is a generic list delegate that renders items with thumbnails.
type thumbDelegate struct {
	imgR       *ytimage.Renderer
	thumbRows  int
	getURL     func(list.Item) string
	renderText ThumbRenderFunc
}

// NewThumbDelegate creates a delegate that renders items with thumbnail
// placeholders on the left and text on the right. The getURL callback
// extracts a thumbnail URL from an item (return "" to skip thumbnail).
// The renderText callback renders the text portion.
func NewThumbDelegate(imgR *ytimage.Renderer, thumbRows int, getURL func(list.Item) string, renderText ThumbRenderFunc) list.ItemDelegate {
	return thumbDelegate{
		imgR:       imgR,
		thumbRows:  thumbRows,
		getURL:     getURL,
		renderText: renderText,
	}
}

func (d thumbDelegate) Height() int  { return d.thumbRows }
func (d thumbDelegate) Spacing() int { return 1 }

func (d thumbDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	if d.imgR == nil {
		return nil
	}
	if _, ok := msg.(spinner.TickMsg); ok {
		return nil
	}

	items := m.Items()
	if len(items) == 0 {
		return nil
	}

	thumbCols := d.thumbRows * 4
	var cmds []tea.Cmd
	for _, item := range items {
		url := d.getURL(item)
		if url == "" {
			continue
		}
		cmd := d.imgR.FetchCmd(url, thumbCols, d.thumbRows)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (d thumbDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	isSelected := index == m.Index()

	url := d.getURL(item)
	if url == "" {
		d.renderText(w, item, m, isSelected, m.Width()-4)
		return
	}

	thumbCols := d.thumbRows * 4
	var thumbLines []string
	if d.imgR != nil {
		if _, pl := d.imgR.Get(url); pl != "" {
			thumbLines = strings.Split(pl, "\n")
		}
	}

	availWidth := m.Width() - thumbCols - 3
	var textBuf strings.Builder
	d.renderText(&textBuf, item, m, isSelected, availWidth)
	textLines := strings.Split(textBuf.String(), "\n")
	RenderWithThumb(w, thumbLines, textLines, thumbCols, d.thumbRows)
}

// RenderWithThumb renders a thumbnail placeholder grid on the left and text
// lines on the right. This is the shared layout helper used by all thumbnail
// delegates.
func RenderWithThumb(w io.Writer, thumbLines, textLines []string, thumbCols, thumbRows int) {
	emptyThumb := strings.Repeat(" ", thumbCols)
	for i := 0; i < thumbRows; i++ {
		if i < len(thumbLines) {
			fmt.Fprint(w, thumbLines[i])
		} else {
			fmt.Fprint(w, emptyThumb)
		}
		fmt.Fprint(w, " ") // gap between thumbnail and text
		if i < len(textLines) {
			fmt.Fprint(w, textLines[i])
		}
		if i < thumbRows-1 {
			fmt.Fprint(w, "\n")
		}
	}
}
