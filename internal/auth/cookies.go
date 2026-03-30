package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"

	"github.com/browserutils/kooky"
	"github.com/browserutils/kooky/browser/brave"
	"github.com/browserutils/kooky/browser/chrome"
	"github.com/browserutils/kooky/browser/chromium"
	"github.com/browserutils/kooky/browser/edge"
	"github.com/browserutils/kooky/browser/firefox"
)

// SupportedBrowsers lists the browsers that can be used for cookie extraction.
var SupportedBrowsers = []string{"brave", "chrome", "chromium", "firefox", "edge"}

type browserReader func(ctx context.Context, filename string, filters ...kooky.Filter) ([]*kooky.Cookie, error)

// browserConfig maps browser names to their cookie reader and possible cookie file paths.
var browserConfigs = map[string]struct {
	reader browserReader
	paths  []string // relative to home directory
}{
	"brave": {
		reader: brave.ReadCookies,
		paths: []string{
			".config/BraveSoftware/Brave-Browser/Default/Cookies",
			".config/BraveSoftware/Brave-Browser/Default/Network/Cookies",
		},
	},
	"chrome": {
		reader: chrome.ReadCookies,
		paths: []string{
			".config/google-chrome/Default/Cookies",
			".config/google-chrome/Default/Network/Cookies",
		},
	},
	"chromium": {
		reader: chromium.ReadCookies,
		paths: []string{
			".config/chromium/Default/Cookies",
			".config/chromium/Default/Network/Cookies",
		},
	},
	"firefox": {
		reader: firefox.ReadCookies,
		paths: []string{
			".mozilla/firefox/*.default-release/cookies.sqlite",
			".mozilla/firefox/*.default/cookies.sqlite",
		},
	},
	"edge": {
		reader: edge.ReadCookies,
		paths: []string{
			".config/microsoft-edge/Default/Cookies",
			".config/microsoft-edge/Default/Network/Cookies",
		},
	},
}

// ExtractCookies reads YouTube cookies from the specified browser into an
// in-memory cookie jar. Cookies are never written to disk.
func ExtractCookies(ctx context.Context, browser string) (http.CookieJar, error) {
	cfg, ok := browserConfigs[browser]
	if !ok {
		return nil, fmt.Errorf("unsupported browser %q (supported: %v)", browser, SupportedBrowsers)
	}

	cookieFile, err := findCookieFile(cfg.paths)
	if err != nil {
		return nil, fmt.Errorf("%s cookie file not found: %w", browser, err)
	}

	filters := []kooky.Filter{
		kooky.Valid,
		kooky.DomainHasSuffix("youtube.com"),
	}

	cookies, err := cfg.reader(ctx, cookieFile, filters...)
	if err != nil {
		return nil, fmt.Errorf("read %s cookies from %s: %w", browser, cookieFile, err)
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("no YouTube cookies found in %s (are you logged in?)", browser)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	byDomain := map[string][]*http.Cookie{}
	for _, c := range cookies {
		hc := &c.Cookie
		if hc.Domain == "" {
			continue
		}
		byDomain[hc.Domain] = append(byDomain[hc.Domain], hc)
	}

	for domain, hcookies := range byDomain {
		host := domain
		if host[0] == '.' {
			host = host[1:]
		}
		u := &url.URL{Scheme: "https", Host: host, Path: "/"}
		jar.SetCookies(u, hcookies)
	}

	return jar, nil
}

func findCookieFile(paths []string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	for _, rel := range paths {
		matches, _ := filepath.Glob(filepath.Join(home, rel))
		if len(matches) > 0 {
			return matches[0], nil
		}
	}

	return "", fmt.Errorf("tried paths: %v", paths)
}

// HTTPClient creates an http.Client with the given cookie jar and SAPISIDHASH
// auth transport for authenticated YouTube API requests.
func HTTPClient(jar http.CookieJar) *http.Client {
	client := &http.Client{Jar: jar}

	ytURL, _ := url.Parse("https://www.youtube.com")
	for _, c := range jar.Cookies(ytURL) {
		if c.Name == "SAPISID" {
			client.Transport = &AuthTransport{
				SAPISID: c.Value,
				Jar:     jar,
			}
			break
		}
	}

	return client
}
