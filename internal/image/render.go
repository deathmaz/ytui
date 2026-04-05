package image

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// ThumbnailLoadedMsg is sent when a thumbnail has been fetched and encoded.
type ThumbnailLoadedMsg struct {
	URL         string
	TransmitStr string // Kitty transmit sequence (include in View once)
	Placeholder string // placeholder grid (include in View always)
	Err         error
}

// Renderer manages thumbnail fetching, encoding, and caching.
type Renderer struct {
	mu        sync.RWMutex
	cache     map[string]cachedThumb
	inflight  map[string]bool // URLs currently being fetched
	requested map[string]bool // all URLs ever passed to FetchCmd (for testing)
	sem       chan struct{}   // concurrency limiter
}

type cachedThumb struct {
	transmitStr string
	placeholder string
}

// NewRenderer creates a new thumbnail renderer.
func NewRenderer() *Renderer {
	return &Renderer{
		cache:     make(map[string]cachedThumb),
		inflight:  make(map[string]bool),
		requested: make(map[string]bool),
		sem:       make(chan struct{}, 3), // max 3 concurrent fetches
	}
}

// Get returns the cached thumbnail, or empty strings if not cached.
func (r *Renderer) Get(url string) (transmitStr, placeholder string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c := r.cache[url]
	return c.transmitStr, c.placeholder
}

// Store caches thumbnail data.
func (r *Renderer) Store(url, transmitStr, placeholder string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[url] = cachedThumb{transmitStr: transmitStr, placeholder: placeholder}
	delete(r.inflight, url)
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
		r.cache[msg.URL] = cachedThumb{transmitStr: msg.TransmitStr, placeholder: msg.Placeholder}
	}
	return true
}

// WasRequested reports whether FetchCmd was ever called for a URL.
func (r *Renderer) WasRequested(url string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
