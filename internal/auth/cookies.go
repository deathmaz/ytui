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
)

// Default Brave cookie file paths to try (varies by Chromium version).
var braveCookiePaths = []string{
	".config/BraveSoftware/Brave-Browser/Default/Cookies",
	".config/BraveSoftware/Brave-Browser/Default/Network/Cookies",
}

// ExtractCookies reads YouTube cookies from Brave browser into an
// in-memory cookie jar. Cookies are never written to disk.
func ExtractCookies(ctx context.Context) (http.CookieJar, error) {
	cookieFile, err := findCookieFile()
	if err != nil {
		return nil, err
	}

	filters := []kooky.Filter{
		kooky.Valid,
		kooky.DomainHasSuffix("youtube.com"),
	}

	cookies, err := brave.ReadCookies(ctx, cookieFile, filters...)
	if err != nil {
		return nil, fmt.Errorf("read cookies from %s: %w", cookieFile, err)
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("no YouTube cookies found in Brave (are you logged in?)")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	byDomain := map[string][]*http.Cookie{}
	for _, c := range cookies {
		hc := &c.Cookie
		domain := hc.Domain
		if domain == "" {
			continue
		}
		byDomain[domain] = append(byDomain[domain], hc)
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

func findCookieFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	for _, rel := range braveCookiePaths {
		path := filepath.Join(home, rel)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("Brave cookie file not found (tried %v)", braveCookiePaths)
}

// HTTPClient creates an http.Client with the given cookie jar attached.
func HTTPClient(jar http.CookieJar) *http.Client {
	return &http.Client{Jar: jar}
}
