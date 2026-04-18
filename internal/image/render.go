package image

import (
	"container/list"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// ThumbnailLoadedMsg is sent when a thumbnail has been fetched and encoded.
type ThumbnailLoadedMsg struct {
	URL         string
	TransmitStr string // Kitty transmit sequence (include in View once)
	Placeholder string // placeholder grid (include in View always)
	Err         error
}

const defaultMaxCache = 200

// Renderer manages thumbnail fetching, encoding, and caching.
// The cache is LRU-bounded to limit memory usage.
type Renderer struct {
	mu       sync.Mutex
	cache    map[string]*list.Element
	order    *list.List
	maxSize  int
	inflight map[string]bool // URLs currently being fetched
	sem      chan struct{}    // concurrency limiter

	// Test-only: tracks all URLs ever passed to FetchCmd.
	requested map[string]bool
}

type lruEntry struct {
	url   string
	thumb cachedThumb
}

type cachedThumb struct {
	transmitStr string
	placeholder string
}

// NewRenderer creates a new thumbnail renderer with the default cache size.
func NewRenderer() *Renderer {
	return NewRendererWithMax(defaultMaxCache)
}

// NewRendererWithMax creates a renderer with a custom cache capacity.
func NewRendererWithMax(maxEntries int) *Renderer {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &Renderer{
		cache:     make(map[string]*list.Element),
		order:     list.New(),
		maxSize:   maxEntries,
		inflight:  make(map[string]bool),
		requested: make(map[string]bool),
		sem:       make(chan struct{}, 3),
	}
}

// Get returns the cached thumbnail, or empty strings if not cached.
// Promotes the entry to the front of the LRU list.
func (r *Renderer) Get(url string) (transmitStr, placeholder string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if elem, ok := r.cache[url]; ok {
		r.order.MoveToFront(elem)
		e := elem.Value.(*lruEntry)
		return e.thumb.transmitStr, e.thumb.placeholder
	}
	return "", ""
}

// Store caches thumbnail data, evicting the oldest entry if at capacity.
func (r *Renderer) Store(url, transmitStr, placeholder string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storeLocked(url, transmitStr, placeholder)
	delete(r.inflight, url)
}

func (r *Renderer) storeLocked(url, transmitStr, placeholder string) {
	if elem, ok := r.cache[url]; ok {
		r.order.MoveToFront(elem)
		elem.Value.(*lruEntry).thumb = cachedThumb{transmitStr, placeholder}
		return
	}
	entry := &lruEntry{url: url, thumb: cachedThumb{transmitStr, placeholder}}
	elem := r.order.PushFront(entry)
	r.cache[url] = elem
	if r.order.Len() > r.maxSize {
		r.evictLocked()
	}
}

func (r *Renderer) evictLocked() {
	back := r.order.Back()
	if back == nil {
		return
	}
	r.order.Remove(back)
	delete(r.cache, back.Value.(*lruEntry).url)
}

// ClearInflight removes a URL from the in-flight set without caching,
// allowing it to be retried.
func (r *Renderer) ClearInflight(url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.inflight, url)
}

// HandleLoaded processes a ThumbnailLoadedMsg if it was initiated by this
// renderer. Returns true if the message was handled (URL was in the inflight
// set), false otherwise. This enables multiple renderers to coexist safely.
func (r *Renderer) HandleLoaded(msg ThumbnailLoadedMsg) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.inflight[msg.URL] {
		return false
	}
	delete(r.inflight, msg.URL)
	if msg.Err == nil && msg.Placeholder != "" {
		r.storeLocked(msg.URL, msg.TransmitStr, msg.Placeholder)
	}
	return true
}

// WasRequested reports whether FetchCmd was ever called for a URL.
func (r *Renderer) WasRequested(url string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requested[url]
}

// FetchCmd returns a tea.Cmd that fetches and encodes a thumbnail.
func (r *Renderer) FetchCmd(url string, cols, rows int) tea.Cmd {
	if url == "" {
		return nil
	}

	r.mu.Lock()
	_, cached := r.cache[url]
	_, fetching := r.inflight[url]
	if cached || fetching {
		r.mu.Unlock()
		return nil
	}
	r.requested[url] = true
	r.inflight[url] = true
	r.mu.Unlock()

	return func() tea.Msg {
		// Acquire semaphore slot to limit concurrency
		r.sem <- struct{}{}
		defer func() { <-r.sem }()

		img, err := FetchImage(url)
		if err != nil {
			return ThumbnailLoadedMsg{URL: url, Err: err}
		}

		transmitStr, placeholder, err := EncodeForKitty(img, cols, rows)
		if err != nil {
			return ThumbnailLoadedMsg{URL: url, Err: err}
		}

		return ThumbnailLoadedMsg{
			URL:         url,
			TransmitStr: transmitStr,
			Placeholder: placeholder,
		}
	}
}
