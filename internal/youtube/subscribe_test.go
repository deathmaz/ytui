package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestSubscribe_Unauthenticated(t *testing.T) {
	c := &InnerTubeClient{authenticated: false}
	if err := c.Subscribe(context.Background(), "UCx"); err == nil {
		t.Fatal("expected auth error, got nil")
	}
}

func TestSubscribe_EmptyChannelID(t *testing.T) {
	c := &InnerTubeClient{authenticated: true}
	if err := c.Subscribe(context.Background(), ""); err == nil {
		t.Fatal("expected empty channelID error, got nil")
	}
}

func TestSubscribe_Success(t *testing.T) {
	fixture := readFile(t, "testdata/fake_subscribe_response.json")
	var captured recorder
	c := newFakeClient(t, fixture, &captured)

	if err := c.Subscribe(context.Background(), "UCfake123456789abcdef_AB"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if captured.method != http.MethodPost {
		t.Errorf("method = %q, want POST", captured.method)
	}
	if !strings.HasSuffix(captured.path, "/youtubei/v1/subscription/subscribe") {
		t.Errorf("path = %q, want suffix /youtubei/v1/subscription/subscribe", captured.path)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(captured.body, &body); err != nil {
		t.Fatalf("body unmarshal: %v (raw=%s)", err, captured.body)
	}
	ids, _ := body["channelIds"].([]interface{})
	if len(ids) != 1 || ids[0] != "UCfake123456789abcdef_AB" {
		t.Errorf("channelIds = %v, want [UCfake123456789abcdef_AB]", ids)
	}
	if _, ok := body["context"].(map[string]interface{}); !ok {
		t.Error("body.context missing — adaptor should merge client context")
	}
}

func TestSubscribe_MismatchedConfirmation(t *testing.T) {
	// Fixture reports subscribed=false; Subscribe asked for true.
	fixture := readFile(t, "testdata/fake_unsubscribe_response.json")
	c := newFakeClient(t, fixture, nil)

	err := c.Subscribe(context.Background(), "UCfake123456789abcdef_AB")
	if !errors.Is(err, ErrSubscriptionNotConfirmed) {
		t.Fatalf("err = %v, want ErrSubscriptionNotConfirmed", err)
	}
}

func TestUnsubscribe_Success(t *testing.T) {
	fixture := readFile(t, "testdata/fake_unsubscribe_response.json")
	var captured recorder
	c := newFakeClient(t, fixture, &captured)

	if err := c.Unsubscribe(context.Background(), "UCfake123456789abcdef_AB"); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if !strings.HasSuffix(captured.path, "/youtubei/v1/subscription/unsubscribe") {
		t.Errorf("path = %q, want suffix /youtubei/v1/subscription/unsubscribe", captured.path)
	}
}

type recorder struct {
	method string
	path   string
	body   []byte
}

type rewriteTransport struct {
	srv      *httptest.Server
	captured *recorder
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	if t.captured != nil {
		t.captured.method = req.Method
		t.captured.path = req.URL.Path
		t.captured.body = body
	}
	u, _ := url.Parse(t.srv.URL)
	u.Path = req.URL.Path
	u.RawQuery = req.URL.RawQuery
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header.Clone()
	return http.DefaultClient.Do(newReq)
}

// newFakeClient builds an authenticated InnerTubeClient routed at an httptest
// server that returns responseBody for every request. captured may be nil.
func newFakeClient(t *testing.T, responseBody string, captured *recorder) *InnerTubeClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(srv.Close)

	// Snapshot the package-global clientParams so concurrent tests (or future
	// t.Parallel) don't race. Restore on cleanup.
	prev := clientParams.web
	clientParams.web = clientConfig{ClientVersion: "test", APIKey: "testkey"}
	t.Cleanup(func() { clientParams.web = prev })

	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{Jar: jar, Transport: &rewriteTransport{srv: srv, captured: captured}}

	c, err := NewInnerTubeClient(httpClient)
	if err != nil {
		t.Fatalf("NewInnerTubeClient: %v", err)
	}
	return c
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
