package shared

import (
	"bytes"
	"os"
	"sync"
	"testing"

	"charm.land/bubbles/v2/list"
	ytimage "github.com/deathmaz/ytui/internal/image"
)

// rawCapture is a test sink for image.RawWrite. Each WrapView call under
// test flushes its captured raw bytes into a per-call string so existing
// assertions that look for transmit/DeleteAll bytes keep working after
// transmits moved out of the View string and into direct stdout writes.
type rawCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (r *rawCapture) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

func (r *rawCapture) flush() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.buf.String()
	r.buf.Reset()
	return s
}

// currentRawCapture is the capture handle most recently installed by
// installRawCapture. Used by stabilize() to drain warm-up bytes.
var currentRawCapture *rawCapture

// installRawCapture swaps the image package's raw writer for this test and
// restores it on cleanup. Returns the capture handle.
func installRawCapture(t *testing.T) *rawCapture {
	t.Helper()
	r := &rawCapture{}
	ytimage.SetRawOutput(r)
	currentRawCapture = r
	t.Cleanup(func() {
		ytimage.SetRawOutput(os.Stdout)
		currentRawCapture = nil
	})
	return r
}

// drainRawCapture clears any pending captured bytes. Called from stabilize()
// so warm-up-frame transmits don't leak into the next wrapCap assertion.
func drainRawCapture() {
	if currentRawCapture != nil {
		currentRawCapture.flush()
	}
}

// wrapCap calls WrapView, reads the transmit bytes that RawWrite captured
// during this call, and returns them prefixed to the returned view. Matches
// the pre-v2 return-value shape so legacy assertions keep working.
func wrapCap(cap *rawCapture, tl *ThumbList, items []list.Item, view string) string {
	out := tl.WrapView(items, view)
	return cap.flush() + out
}
