package auth

import (
	"crypto/sha1"
	"fmt"
	"net/http"
	"time"
)

// AuthTransport rewrites InnerTube requests to www.youtube.com and adds
// SAPISIDHASH authentication headers required for authenticated API calls.
type AuthTransport struct {
	Base    http.RoundTripper
	SAPISID string
	Jar     http.CookieJar
}

func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "youtubei.googleapis.com" {
		req.URL.Host = "www.youtube.com"
		req.Host = "www.youtube.com"

		// Re-add cookies for the rewritten host
		for _, c := range t.Jar.Cookies(req.URL) {
			req.AddCookie(c)
		}

		now := time.Now().Unix()
		hash := sha1.Sum([]byte(fmt.Sprintf("%d %s https://www.youtube.com", now, t.SAPISID)))
		req.Header.Set("Authorization", fmt.Sprintf("SAPISIDHASH %d_%x", now, hash))
		req.Header.Set("Origin", "https://www.youtube.com")
	}
	return t.base().RoundTrip(req)
}

func (t *AuthTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}
