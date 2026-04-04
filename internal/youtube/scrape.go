package youtube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// clientConfig holds dynamically scraped InnerTube client parameters.
type clientConfig struct {
	ClientVersion string
	APIKey        string
}

var clientParams struct {
	web   clientConfig
	music clientConfig
}

// Overridable in tests.
var (
	webPageURL   = "https://www.youtube.com/"
	musicPageURL = "https://music.youtube.com/"
)

var (
	reClientVersion = regexp.MustCompile(`"INNERTUBE_CLIENT_VERSION":"([^"]+)"`)
	reAPIKey        = regexp.MustCompile(`"INNERTUBE_API_KEY":"([^"]+)"`)
)

const maxBodySize = 2 << 20 // 2 MB

// scrapeClientConfig fetches a YouTube page and extracts the client version
// and API key from the embedded JS config.
func scrapeClientConfig(ctx context.Context, pageURL string) (clientConfig, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return clientConfig{}, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return clientConfig{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return clientConfig{}, err
	}

	var cfg clientConfig

	if m := reClientVersion.FindSubmatch(body); m != nil {
		cfg.ClientVersion = string(m[1])
	}
	if m := reAPIKey.FindSubmatch(body); m != nil {
		cfg.APIKey = string(m[1])
	}

	if cfg.ClientVersion == "" || cfg.APIKey == "" {
		return clientConfig{}, fmt.Errorf("incomplete scrape from %s: version=%q key=%q", pageURL, cfg.ClientVersion, cfg.APIKey)
	}
	return cfg, nil
}

// InitClientParams scrapes YouTube and YouTube Music pages in parallel to
// extract current InnerTube client parameters. Returns an error if either
// scrape fails. Call this once before creating any clients.
func InitClientParams(ctx context.Context) error {
	type result struct {
		name string
		cfg  clientConfig
		err  error
	}

	ch := make(chan result, 2)

	go func() {
		cfg, err := scrapeClientConfig(ctx, webPageURL)
		ch <- result{"web", cfg, err}
	}()
	go func() {
		cfg, err := scrapeClientConfig(ctx, musicPageURL)
		ch <- result{"music", cfg, err}
	}()

	var errs []string
	for range 2 {
		r := <-ch
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.name, r.err))
			continue
		}
		switch r.name {
		case "web":
			clientParams.web = r.cfg
		case "music":
			clientParams.music = r.cfg
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("client params scrape failed: %s", strings.Join(errs, "; "))
	}
	return nil
}
