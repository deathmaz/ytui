package youtube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const fakeYouTubeHTML = `<!DOCTYPE html>
<html><head><title>YouTube</title></head><body>
<script>var ytcfg={};ytcfg.set({"INNERTUBE_API_KEY":"AIzaSyFake123TestKey","INNERTUBE_CLIENT_NAME":"WEB","INNERTUBE_CLIENT_VERSION":"2.20260501.00.00"});</script>
</body></html>`

const fakeYouTubeHTML_MissingKey = `<!DOCTYPE html>
<html><body>
<script>ytcfg.set({"INNERTUBE_CLIENT_VERSION":"2.20260501.00.00"});</script>
</body></html>`

const fakeYouTubeHTML_NoMatch = `<!DOCTYPE html>
<html><body><p>Nothing useful here</p></body></html>`

func TestScrapeClientConfig_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fakeYouTubeHTML))
	}))
	defer ts.Close()

	cfg, err := scrapeClientConfig(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientVersion != "2.20260501.00.00" {
		t.Errorf("ClientVersion = %q, want %q", cfg.ClientVersion, "2.20260501.00.00")
	}
	if cfg.APIKey != "AIzaSyFake123TestKey" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "AIzaSyFake123TestKey")
	}
}

func TestScrapeClientConfig_PartialMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fakeYouTubeHTML_MissingKey))
	}))
	defer ts.Close()

	_, err := scrapeClientConfig(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("expected error for partial match, got nil")
	}
}

func TestScrapeClientConfig_NoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fakeYouTubeHTML_NoMatch))
	}))
	defer ts.Close()

	_, err := scrapeClientConfig(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("expected error for no match, got nil")
	}
}

func TestScrapeClientConfig_NetworkError(t *testing.T) {
	_, err := scrapeClientConfig(context.Background(), "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error for network failure, got nil")
	}
}

func TestScrapeClientConfig_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := scrapeClientConfig(ctx, ts.URL)
	if err == nil {
		t.Fatal("expected error for timeout, got nil")
	}
}

func TestInitClientParams_ReturnsErrorOnFailure(t *testing.T) {
	origWeb, origMusic := webPageURL, musicPageURL
	webPageURL = "http://127.0.0.1:1"
	musicPageURL = "http://127.0.0.1:1"
	defer func() { webPageURL, musicPageURL = origWeb, origMusic }()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := InitClientParams(ctx)
	if err == nil {
		t.Fatal("expected error when scraping fails, got nil")
	}

	// clientParams should be zero-valued (not populated)
	if clientParams.web.ClientVersion != "" {
		t.Errorf("web ClientVersion should be empty, got %q", clientParams.web.ClientVersion)
	}
	if clientParams.music.ClientVersion != "" {
		t.Errorf("music ClientVersion should be empty, got %q", clientParams.music.ClientVersion)
	}
}

func TestInitClientParams_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fakeYouTubeHTML))
	}))
	defer ts.Close()

	origWeb, origMusic := webPageURL, musicPageURL
	webPageURL = ts.URL
	musicPageURL = ts.URL
	defer func() {
		webPageURL, musicPageURL = origWeb, origMusic
		clientParams.web = clientConfig{}
		clientParams.music = clientConfig{}
	}()

	err := InitClientParams(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if clientParams.web.ClientVersion != "2.20260501.00.00" {
		t.Errorf("web ClientVersion = %q, want %q", clientParams.web.ClientVersion, "2.20260501.00.00")
	}
	if clientParams.web.APIKey != "AIzaSyFake123TestKey" {
		t.Errorf("web APIKey = %q, want %q", clientParams.web.APIKey, "AIzaSyFake123TestKey")
	}
	if clientParams.music.ClientVersion != "2.20260501.00.00" {
		t.Errorf("music ClientVersion = %q, want %q", clientParams.music.ClientVersion, "2.20260501.00.00")
	}
}

func TestInitClientParams_PartialFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fakeYouTubeHTML))
	}))
	defer ts.Close()

	origWeb, origMusic := webPageURL, musicPageURL
	webPageURL = ts.URL
	musicPageURL = "http://127.0.0.1:1" // music fails
	defer func() {
		webPageURL, musicPageURL = origWeb, origMusic
		clientParams.web = clientConfig{}
		clientParams.music = clientConfig{}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := InitClientParams(ctx)
	if err == nil {
		t.Fatal("expected error for partial failure, got nil")
	}

	// Web should be populated, music should not
	if clientParams.web.ClientVersion != "2.20260501.00.00" {
		t.Errorf("web ClientVersion = %q, want %q", clientParams.web.ClientVersion, "2.20260501.00.00")
	}
	if clientParams.music.ClientVersion != "" {
		t.Errorf("music ClientVersion should be empty, got %q", clientParams.music.ClientVersion)
	}
}
