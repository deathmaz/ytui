package shared

import (
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/deathmaz/ytui/internal/imagetest"
)

// wrapCap calls WrapView, flushes the captured raw-write bytes from this
// call, and returns them prefixed to the view. Matches the pre-v2 return
// shape so legacy substring assertions keep working after Kitty transmits
// moved from View content to direct stdout writes.
func wrapCap(c *imagetest.Capture, tl *ThumbList, items []list.Item, view string) string {
	out := tl.WrapView(items, view)
	prefix := c.String()
	c.Reset()
	return prefix + out
}

func installRawCapture(t *testing.T) *imagetest.Capture { return imagetest.Install(t) }

func drainRawCapture() { imagetest.DrainCurrent() }
