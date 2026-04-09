package shared

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	ytimage "github.com/deathmaz/ytui/internal/image"
)

// thumbDebugFile is the debug log file, opened once if YTUI_THUMB_DEBUG is set.
var thumbDebugFile *os.File

func init() {
	if path := os.Getenv("YTUI_THUMB_DEBUG"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err == nil {
			thumbDebugFile = f
		}
	}
}

func thumbLog(format string, args ...interface{}) {
	if thumbDebugFile == nil {
		return
	}
	ts := time.Now().Format("15:04:05.000")
	fmt.Fprintf(thumbDebugFile, "%s  "+format+"\n", append([]interface{}{ts}, args...)...)
	thumbDebugFile.Sync()
}

// globalDeleteGen is bumped every time any ThumbList sends DeleteAll.
// Because DeleteAll is a global Kitty operation (clears ALL images, not
// just the sender's), every other ThumbList must retransmit on the next
// WrapView call. Each ThumbList records the gen it last transmitted at;
// a mismatch forces retransmit.
var globalDeleteGen atomic.Uint64

func init() {
	globalDeleteGen.Store(1) // start at 1 so zero means "never transmitted"
}

// ThumbList manages Kitty image transmit sequences for lists with thumbnails.
// One ThumbList per renderer, shared across all lists using that renderer.
//
// Strategy: on every call, build a cheap fingerprint of which visible URLs
// have cached images. If the fingerprint is identical to the previous call
// (cursor blink, idle re-render) return the plain view — zero image
// overhead. When the fingerprint differs (new image loaded, page change,
// view switch) send DeleteAll + re-transmit ALL visible images. This is
// the same as the pre-optimisation code, just not on every frame.
type ThumbList struct {
	imgR            *ytimage.Renderer
	getURL          func(list.Item) string
	thumbRows       int    // row height used for fetch/encode
	lastFingerprint string // cached-URL fingerprint from previous WrapView
	lastDeleteGen   uint64 // globalDeleteGen value when we last transmitted
	repeatCount     int    // frames left to keep retransmitting after a change
}

// NewThumbList creates a new ThumbList with the given renderer, URL extractor,
// and thumbnail row height (used for fetch/encode dimensions).
func NewThumbList(imgR *ytimage.Renderer, getURL func(list.Item) string, thumbRows int) *ThumbList {
	if thumbRows <= 0 {
		thumbRows = 5
	}
	return &ThumbList{
		imgR:      imgR,
		getURL:    getURL,
		thumbRows: thumbRows,
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

// VisibleItems returns the slice of items currently shown on screen based on
// the list's paginator.
func VisibleItems(l list.Model) []list.Item {
	items := l.Items()
	if len(items) == 0 {
		return nil
	}
	p := l.Paginator
	start := p.Page * p.PerPage
	end := start + p.PerPage
	if start >= len(items) {
		return nil
	}
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// WrapView prepends Kitty image sequences for visible items.
//
// It builds a fingerprint of which visible URLs currently have cached
// images. When the fingerprint matches the previous call (cursor blink,
// idle re-render) the plain view is returned — zero image overhead.
// When the fingerprint differs (new image loaded, page change, view
// switch, refresh) DeleteAll + full re-transmit of every visible image
// is sent. This is equivalent to the original always-retransmit
// approach, just skipped on frames where nothing changed.
//
// Only pass VISIBLE items (use VisibleItems).
func (t *ThumbList) WrapView(items []list.Item, view string) string {
	if t == nil || t.imgR == nil {
		return view
	}

	// Single pass: build fingerprint and collect cached transmit data.
	// The fingerprint is the ordered list of visible URLs that have cached
	// image data. cachedTx holds (url, transmitStr) pairs for retransmit —
	// only populated when we'll actually need to transmit.
	type cachedEntry struct {
		url         string
		transmitStr string
	}
	var fp strings.Builder
	var cached []cachedEntry
	var seenURLs []string
	for _, item := range items {
		url := t.getURL(item)
		if url == "" || sliceContains(seenURLs, url) {
			continue
		}
		seenURLs = append(seenURLs, url)
		transmitStr, _ := t.imgR.Get(url)
		if transmitStr != "" {
			fp.WriteString(url)
			fp.WriteByte(0)
			cached = append(cached, cachedEntry{url, transmitStr})
		}
	}

	fingerprint := fp.String()
	if fingerprint == "" {
		// No images cached yet. Send a bare DeleteAll to purge stale
		// images from Kitty if:
		// (a) we were invalidated (view switch, loading spinner), OR
		// (b) the previous frame had cached images but this one doesn't
		//     (page scroll to uncached items — old virtual placements
		//     would otherwise linger until the first new image loads).
		if t.lastDeleteGen == 0 && t.lastFingerprint == "" {
			thumbLog("[%p] DELETE_STALE  items=%d", t, len(seenURLs))
			t.lastDeleteGen = globalDeleteGen.Add(1)
			return ytimage.DeleteAll() + view
		}
		if t.lastFingerprint != "" {
			thumbLog("[%p] DELETE_STALE (fp_cleared)  items=%d", t, len(seenURLs))
			t.lastFingerprint = ""
			t.lastDeleteGen = globalDeleteGen.Add(1)
			return ytimage.DeleteAll() + view
		}
		thumbLog("[%p] SKIP (no cached)  items=%d", t, len(seenURLs))
		return view
	}

	gen := globalDeleteGen.Load()
	changed := fingerprint != t.lastFingerprint || gen != t.lastDeleteGen
	if changed {
		// Schedule retransmit for this frame and one more. The repeat
		// ensures Kitty processes the data even if the first frame's
		// output was only partially consumed during rapid loading.
		t.repeatCount = 2
	}
	if t.repeatCount <= 0 {
		thumbLog("[%p] SKIP (stable)  gen=%d  cached=%d/%d",
			t, gen, len(cached), len(seenURLs))
		return view
	}
	t.repeatCount--

	oldGen := t.lastDeleteGen
	t.lastFingerprint = fingerprint
	t.lastDeleteGen = globalDeleteGen.Add(1)

	if thumbDebugFile != nil {
		reason := "repeat"
		if changed {
			reason = "fingerprint_changed"
			if gen != oldGen {
				reason = fmt.Sprintf("gen_mismatch(cur=%d,last=%d)", gen, oldGen)
			}
		}
		urls := make([]string, len(cached))
		for i, c := range cached {
			urls[i] = c.url
		}
		thumbLog("[%p] RETRANSMIT  reason=%s  oldGen=%d  newGen=%d  cached=%v",
			t, reason, oldGen, t.lastDeleteGen, urls)
	}

	var tx strings.Builder
	tx.WriteString(ytimage.DeleteAll())
	for _, c := range cached {
		tx.WriteString(c.transmitStr)
	}
	tx.WriteString(view)
	return tx.String()
}

// Invalidate forces the next WrapView call to re-transmit all images.
// Use when the view was hidden (loading spinner) or another ThumbList
// issued DeleteAll (channel sub-tab switches).
func (t *ThumbList) Invalidate() {
	if t == nil {
		return
	}
	thumbLog("[%p] INVALIDATE  oldGen=%d  oldFingerprint=%q",
		t, t.lastDeleteGen, t.lastFingerprint)
	t.lastFingerprint = ""
	t.lastDeleteGen = 0 // mismatch any gen ≥ 1 → forces retransmit
	t.repeatCount = 0
}

func sliceContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// RefetchCmd returns a tea.Cmd that re-fetches thumbnails for visible items
// whose cache entries were evicted by the LRU. Call this when switching to
// a view whose thumbnails may have been evicted while it was inactive.
func (t *ThumbList) RefetchCmd(l list.Model) tea.Cmd {
	if t == nil || t.imgR == nil {
		return nil
	}
	items := VisibleItems(l)
	thumbCols := t.thumbRows * 4
	thumbRows := t.thumbRows
	var cmds []tea.Cmd
	for _, item := range items {
		url := t.getURL(item)
		if url == "" {
			continue
		}
		cmd := t.imgR.FetchCmd(url, thumbCols, thumbRows)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
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

	// Only fetch thumbnails for visible items plus a ±1 page buffer.
	// Before the first resize PerPage may be 0; fall back to all items.
	p := m.Paginator
	if p.PerPage > 0 {
		start := p.Page*p.PerPage - p.PerPage
		if start < 0 {
			start = 0
		}
		end := p.Page*p.PerPage + p.PerPage*2
		if end > len(items) {
			end = len(items)
		}
		if start < len(items) {
			items = items[start:end]
		}
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
// lines on the right.
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
