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
	mu    sync.RWMutex
	cache map[string]cachedThumb
}

type cachedThumb struct {
	transmitStr string
	placeholder string
}

// NewRenderer creates a new thumbnail renderer.
func NewRenderer() *Renderer {
	return &Renderer{
		cache: make(map[string]cachedThumb),
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
}

// FetchCmd returns a tea.Cmd that fetches and encodes a thumbnail.
func (r *Renderer) FetchCmd(url string, cols, rows int) tea.Cmd {
	if url == "" {
		return nil
	}

	r.mu.RLock()
	_, ok := r.cache[url]
	r.mu.RUnlock()
	if ok {
		return nil
	}

	return func() tea.Msg {
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
