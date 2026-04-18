// Package imagetest provides test helpers for capturing the Kitty APC byte
// stream that internal/image.RawWrite would otherwise send to stdout.
//
// Bubble Tea v2's Cursed Renderer drops APC sequences from View content, so
// the image package streams Kitty transmits directly to stdout. Tests need
// to intercept those writes to assert on the transmit stream without
// polluting the test runner's console.
package imagetest

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"

	ytimage "github.com/deathmaz/ytui/internal/image"
)

// Capture is a mutex-guarded byte sink for image.RawWrite output.
type Capture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *Capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *Capture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func (c *Capture) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf.Reset()
}

var currentCapture *Capture

// Install routes image.RawWrite to a fresh Capture for this test and
// restores the default writer on cleanup.
func Install(t *testing.T) *Capture {
	t.Helper()
	c := &Capture{}
	ytimage.SetRawOutput(c)
	currentCapture = c
	t.Cleanup(func() {
		ytimage.SetRawOutput(os.Stdout)
		currentCapture = nil
	})
	return c
}

// DrainCurrent clears any pending bytes on the most-recently-installed
// Capture. Call from stabilize/warm-up helpers so bytes from warm-up frames
// don't leak into the next assertion.
func DrainCurrent() {
	if currentCapture != nil {
		currentCapture.Reset()
	}
}

// RouteToDiscard sends image.RawWrite to io.Discard. Use from TestMain to
// silence transmit bytes during tests that don't install their own Capture.
// Call RestoreStdout at TestMain teardown.
func RouteToDiscard() {
	ytimage.SetRawOutput(io.Discard)
}

// RestoreStdout points image.RawWrite back at os.Stdout.
func RestoreStdout() {
	ytimage.SetRawOutput(os.Stdout)
}
