package image

import (
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "golang.org/x/image/webp"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

var (
	cacheDir     string
	cacheDirOnce sync.Once
)

func getCacheDir() string {
	cacheDirOnce.Do(func() {
		dir, err := os.UserCacheDir()
		if err != nil {
			return
		}
		cacheDir = filepath.Join(dir, "ytui", "thumbnails")
	})
	return cacheDir
}

// FetchImage downloads an image from a URL and returns the decoded image.
// Results are cached on disk by URL hash.
func FetchImage(url string) (image.Image, error) {
	if url == "" {
		return nil, fmt.Errorf("empty URL")
	}

	// Check disk cache
	cachePath := cachePathFor(url, getCacheDir())
	if cachePath != "" {
		if img, err := loadFromCache(cachePath); err == nil {
			return img, nil
		}
	}

	// Fetch from network
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch thumbnail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thumbnail HTTP %d", resp.StatusCode)
	}

	// Save to cache file while decoding
	var reader io.Reader = resp.Body
	var cacheFile *os.File
	if cachePath != "" {
		os.MkdirAll(filepath.Dir(cachePath), 0755)
		if f, err := os.Create(cachePath); err == nil {
			cacheFile = f
			defer f.Close()
			reader = io.TeeReader(resp.Body, f)
		}
	}

	img, _, err := image.Decode(reader)
	if err != nil {
		if cacheFile != nil {
			cacheFile.Close()
			os.Remove(cachePath)
		}
		return nil, fmt.Errorf("decode thumbnail: %w", err)
	}

	return img, nil
}

func cachePathFor(url, dir string) string {
	if dir == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(url))
	return filepath.Join(dir, fmt.Sprintf("%x", hash[:16]))
}

func loadFromCache(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}
