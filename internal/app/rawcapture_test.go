package app

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"

	ytimage "github.com/deathmaz/ytui/internal/image"
)

// TestMain routes ytimage.RawWrite to io.Discard by default so transmit
// bytes don't pollute the test runner's console. Individual tests that need
// to assert on the APC byte stream install a per-test capture via
// captureRawWrites.
func TestMain(m *testing.M) {
	ytimage.SetRawOutput(io.Discard)
	code := m.Run()
	ytimage.SetRawOutput(os.Stdout)
	os.Exit(code)
}

// rawCaptureBuf is a mutex-guarded byte buffer for capturing RawWrite output
// during a single test. Concurrent writes from background goroutines
// (thumbnail fetchers, spinner ticks) are serialized by the internal lock.
type rawCaptureBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (r *rawCaptureBuf) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

func (r *rawCaptureBuf) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

func (r *rawCaptureBuf) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf.Reset()
}

// currentRawCapture tracks the capture handle installed by the latest
// captureRawWrites call so helpers like wrapViewStabilize can drain warm-up
// bytes without plumbing the handle through every call site.
var currentRawCapture *rawCaptureBuf

// captureRawWrites swaps the image package's raw writer to a test buffer for
// this test. The writer is restored on cleanup. Use when asserting on the
// Kitty transmit byte stream that v2's renderer would otherwise drop from
// View content.
func captureRawWrites(t *testing.T) *rawCaptureBuf {
	t.Helper()
	b := &rawCaptureBuf{}
	ytimage.SetRawOutput(b)
	currentRawCapture = b
	t.Cleanup(func() {
		ytimage.SetRawOutput(io.Discard)
		currentRawCapture = nil
	})
	return b
}

// drainCapturedRawWrites clears any pending captured bytes. Called from
// wrapViewStabilize so warm-up-frame transmits don't leak into the next
// assertion window.
func drainCapturedRawWrites() {
	if currentRawCapture != nil {
		currentRawCapture.Reset()
	}
}
