package auth

import (
	"crypto/sha1"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

type recordingTransport struct {
	lastReq *http.Request
}

func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.lastReq = req
	return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
}

func TestAuthTransport_RewritesHost(t *testing.T) {
	recorder := &recordingTransport{}
	jar, _ := cookiejar.New(nil)

	transport := &AuthTransport{
		Base:    recorder,
		SAPISID: "fake_sapisid_value",
		Jar:     jar,
	}

	req, _ := http.NewRequest("POST", "https://youtubei.googleapis.com/youtubei/v1/browse", nil)
	transport.RoundTrip(req)

	if recorder.lastReq.URL.Host != "www.youtube.com" {
		t.Errorf("host = %q, want %q", recorder.lastReq.URL.Host, "www.youtube.com")
	}
	if recorder.lastReq.Host != "www.youtube.com" {
		t.Errorf("Host header = %q, want %q", recorder.lastReq.Host, "www.youtube.com")
	}
}

func TestAuthTransport_AddsSAPISIDHASH(t *testing.T) {
	recorder := &recordingTransport{}
	jar, _ := cookiejar.New(nil)

	transport := &AuthTransport{
		Base:    recorder,
		SAPISID: "fake_sapisid_value",
		Jar:     jar,
	}

	req, _ := http.NewRequest("POST", "https://youtubei.googleapis.com/youtubei/v1/browse", nil)
	transport.RoundTrip(req)

	auth := recorder.lastReq.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "SAPISIDHASH ") {
		t.Errorf("Authorization = %q, want SAPISIDHASH prefix", auth)
	}

	// Verify hash format: SAPISIDHASH timestamp_hex
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected 'SAPISIDHASH value', got %q", auth)
	}
	hashParts := strings.SplitN(parts[1], "_", 2)
	if len(hashParts) != 2 {
		t.Fatalf("expected 'timestamp_hash', got %q", parts[1])
	}
}

func TestAuthTransport_SAPISIDHASHFormat(t *testing.T) {
	// Verify the hash computation matches the expected format
	sapisid := "test_sapisid"
	timestamp := int64(1234567890)
	origin := "https://www.youtube.com"

	hash := sha1.Sum([]byte(fmt.Sprintf("%d %s %s", timestamp, sapisid, origin)))
	expected := fmt.Sprintf("SAPISIDHASH %d_%x", timestamp, hash)

	if !strings.HasPrefix(expected, "SAPISIDHASH 1234567890_") {
		t.Errorf("unexpected format: %s", expected)
	}
}

func TestAuthTransport_AddsCookies(t *testing.T) {
	recorder := &recordingTransport{}
	jar, _ := cookiejar.New(nil)

	// Add a cookie for youtube.com
	ytURL, _ := url.Parse("https://www.youtube.com")
	jar.SetCookies(ytURL, []*http.Cookie{
		{Name: "SID", Value: "fake_sid"},
		{Name: "SAPISID", Value: "fake_sapisid"},
	})

	transport := &AuthTransport{
		Base:    recorder,
		SAPISID: "fake_sapisid",
		Jar:     jar,
	}

	req, _ := http.NewRequest("POST", "https://youtubei.googleapis.com/youtubei/v1/browse", nil)
	transport.RoundTrip(req)

	// Check cookies were added for the rewritten host
	cookies := recorder.lastReq.Cookies()
	found := map[string]bool{}
	for _, c := range cookies {
		found[c.Name] = true
	}
	if !found["SID"] {
		t.Error("SID cookie not forwarded")
	}
	if !found["SAPISID"] {
		t.Error("SAPISID cookie not forwarded")
	}
}

func TestAuthTransport_SkipsNonYouTube(t *testing.T) {
	recorder := &recordingTransport{}
	jar, _ := cookiejar.New(nil)

	transport := &AuthTransport{
		Base:    recorder,
		SAPISID: "fake_sapisid",
		Jar:     jar,
	}

	req, _ := http.NewRequest("GET", "https://example.com/test", nil)
	transport.RoundTrip(req)

	if recorder.lastReq.URL.Host != "example.com" {
		t.Errorf("should not rewrite non-YouTube host, got %q", recorder.lastReq.URL.Host)
	}
	if recorder.lastReq.Header.Get("Authorization") != "" {
		t.Error("should not add auth header for non-YouTube requests")
	}
}

func TestAuthTransport_SetsOrigin(t *testing.T) {
	recorder := &recordingTransport{}
	jar, _ := cookiejar.New(nil)

	transport := &AuthTransport{
		Base:    recorder,
		SAPISID: "fake_sapisid",
		Jar:     jar,
	}

	req, _ := http.NewRequest("POST", "https://youtubei.googleapis.com/youtubei/v1/browse", nil)
	transport.RoundTrip(req)

	origin := recorder.lastReq.Header.Get("Origin")
	if origin != "https://www.youtube.com" {
		t.Errorf("Origin = %q, want %q", origin, "https://www.youtube.com")
	}
}
