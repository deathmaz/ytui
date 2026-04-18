package app

import (
	"os"
	"testing"

	"github.com/deathmaz/ytui/internal/imagetest"
)

// TestMain routes ytimage.RawWrite to io.Discard by default so transmit
// bytes don't pollute the test runner's console. Individual tests install
// their own Capture via captureRawWrites when they need to assert on the
// byte stream.
func TestMain(m *testing.M) {
	imagetest.RouteToDiscard()
	code := m.Run()
	imagetest.RestoreStdout()
	os.Exit(code)
}

func captureRawWrites(t *testing.T) *imagetest.Capture {
	t.Helper()
	return imagetest.Install(t)
}

func drainCapturedRawWrites() { imagetest.DrainCurrent() }
