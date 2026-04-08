package shared

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	ytimage "github.com/deathmaz/ytui/internal/image"
	"github.com/deathmaz/ytui/internal/youtube"
)

// helper to create items with known thumbnail URLs.
func testItems(urls ...string) []list.Item {
	items := make([]list.Item, len(urls))
	for i, u := range urls {
		items[i] = VideoItem{Video: youtube.Video{
			ID:         u,
			Thumbnails: []youtube.Thumbnail{{URL: u, Width: 320, Height: 180}},
		}}
	}
	return items
}

// stabilize calls WrapView enough times to get past the initial retransmit
// and its repeat frame, returning the ThumbList to a stable skip state.
func stabilize(tl *ThumbList, items []list.Item, view string) {
	for i := 0; i < 5; i++ {
		tl.WrapView(items, view)
	}
}

func TestWrapView_TransmitsOnFirstCall(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tl := NewThumbList(imgR, VideoThumbURL)

	imgR.Store("http://thumb/a", "TRANSMIT_A", "PLACE_A")
	imgR.Store("http://thumb/b", "TRANSMIT_B", "PLACE_B")

	items := testItems("http://thumb/a", "http://thumb/b")
	deleteAll := ytimage.DeleteAll()

	out := tl.WrapView(items, "VIEW")
	if !strings.Contains(out, deleteAll) {
		t.Error("first call should contain DeleteAll")
	}
	if !strings.Contains(out, "TRANSMIT_A") {
		t.Error("first call should contain TRANSMIT_A")
	}
	if !strings.Contains(out, "TRANSMIT_B") {
		t.Error("first call should contain TRANSMIT_B")
	}
}

func TestWrapView_RepeatsOnceThenSkips(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tl := NewThumbList(imgR, VideoThumbURL)

	imgR.Store("http://thumb/a", "TX_A", "PL_A")

	items := testItems("http://thumb/a")

	// Frame 1: initial retransmit.
	out1 := tl.WrapView(items, "V")
	if !strings.Contains(out1, "TX_A") {
		t.Error("frame 1 should transmit")
	}

	// Frame 2: repeat retransmit (safety net).
	out2 := tl.WrapView(items, "V")
	if !strings.Contains(out2, "TX_A") {
		t.Error("frame 2 should repeat transmit")
	}

	// Frame 3+: stable skip.
	for i := 0; i < 5; i++ {
		out := tl.WrapView(items, "V")
		if out != "V" {
			t.Errorf("frame %d: expected plain view, got length %d", i+3, len(out))
		}
	}
}

func TestWrapView_RetransmitsWhenNewImageLoads(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tl := NewThumbList(imgR, VideoThumbURL)

	imgR.Store("http://thumb/a", "TX_A", "PL_A")

	items := testItems("http://thumb/a", "http://thumb/b")

	// Stabilize with A cached, B not.
	stabilize(tl, items, "V")

	// B loads.
	imgR.Store("http://thumb/b", "TX_B", "PL_B")

	out := tl.WrapView(items, "V")
	if !strings.Contains(out, "TX_A") || !strings.Contains(out, "TX_B") {
		t.Error("should retransmit all when new image loads")
	}

	// After stabilize, should skip.
	stabilize(tl, items, "V")
	out2 := tl.WrapView(items, "V")
	if out2 != "V" {
		t.Errorf("expected plain view after stabilize, got length %d", len(out2))
	}
}

func TestWrapView_RetransmitsOnPageChange(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tl := NewThumbList(imgR, VideoThumbURL)

	imgR.Store("http://thumb/a", "TX_A", "PL_A")
	imgR.Store("http://thumb/x", "TX_X", "PL_X")

	deleteAll := ytimage.DeleteAll()

	stabilize(tl, testItems("http://thumb/a"), "V1")

	out := tl.WrapView(testItems("http://thumb/x"), "V2")
	if !strings.Contains(out, deleteAll) {
		t.Error("page change should contain DeleteAll")
	}
	if !strings.Contains(out, "TX_X") {
		t.Error("should transmit X")
	}
}

func TestWrapView_Invalidate(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tl := NewThumbList(imgR, VideoThumbURL)

	imgR.Store("http://thumb/a", "TX_A", "PL_A")

	items := testItems("http://thumb/a")
	deleteAll := ytimage.DeleteAll()

	stabilize(tl, items, "V")

	tl.Invalidate()

	out := tl.WrapView(items, "V")
	if !strings.Contains(out, deleteAll) {
		t.Error("should contain DeleteAll after Invalidate")
	}
	if !strings.Contains(out, "TX_A") {
		t.Error("should re-transmit A after Invalidate")
	}
}

func TestWrapView_InvalidatePurgesOldImagesBeforeCacheReady(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tl := NewThumbList(imgR, VideoThumbURL)

	imgR.Store("http://thumb/a", "TX_A", "PL_A")

	deleteAll := ytimage.DeleteAll()

	stabilize(tl, testItems("http://thumb/a"), "VA")

	tl.Invalidate()

	// Uncached items: should still send DeleteAll to purge old images.
	out := tl.WrapView(testItems("http://thumb/new"), "VB")
	if !strings.Contains(out, deleteAll) {
		t.Error("should send DeleteAll after Invalidate even with no cached images")
	}
}

func TestWrapView_DeduplicatesSharedURLs(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tl := NewThumbList(imgR, VideoThumbURL)

	imgR.Store("http://thumb/a", "TX_A", "PL_A")

	items := testItems("http://thumb/a", "http://thumb/a")

	out := tl.WrapView(items, "V")
	if count := strings.Count(out, "TX_A"); count != 1 {
		t.Errorf("expected TX_A exactly once, got %d times", count)
	}
}

func TestWrapView_CrossThumbListDeleteAll(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tlA := NewThumbList(imgR, VideoThumbURL)
	tlB := NewThumbList(imgR, VideoThumbURL)

	imgR.Store("http://thumb/a", "TX_A", "PL_A")
	imgR.Store("http://thumb/x", "TX_X", "PL_X")

	// A transmits + stabilizes.
	stabilize(tlA, testItems("http://thumb/a"), "VA")

	// B transmits — sends DeleteAll which clears A's images from Kitty.
	tlB.WrapView(testItems("http://thumb/x"), "VB")

	// A renders again. Gen mismatch → must retransmit.
	out := tlA.WrapView(testItems("http://thumb/a"), "VA")
	if !strings.Contains(out, "TX_A") {
		t.Error("A must retransmit after B's DeleteAll cleared its images")
	}

	// Stabilize A, then confirm it skips.
	stabilize(tlA, testItems("http://thumb/a"), "VA")
	out2 := tlA.WrapView(testItems("http://thumb/a"), "VA")
	if out2 != "VA" {
		t.Errorf("A should skip on stable frame, got length %d", len(out2))
	}
}

// TestWrapView_DeleteStaleSentOnlyOnce verifies that after Invalidate with
// uncached items, the bare DeleteAll is sent once, not on every frame.
func TestWrapView_DeleteStaleSentOnlyOnce(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tl := NewThumbList(imgR, VideoThumbURL)

	imgR.Store("http://thumb/a", "TX_A", "PL_A")
	stabilize(tl, testItems("http://thumb/a"), "V")

	tl.Invalidate()

	items := testItems("http://thumb/uncached")
	deleteAll := ytimage.DeleteAll()

	// First call: bare DeleteAll.
	out1 := tl.WrapView(items, "V")
	if !strings.Contains(out1, deleteAll) {
		t.Error("first call after Invalidate should send DeleteAll")
	}

	// Subsequent calls: no more DeleteAll until something changes.
	for i := 0; i < 3; i++ {
		out := tl.WrapView(items, "V")
		if out != "V" {
			t.Errorf("call %d: should return plain view, got length %d", i+2, len(out))
		}
	}
}

// TestWrapView_IncrementalLoading simulates the real-world pattern of
// 4 images loading one-by-one. Each new image triggers a retransmit
// (DeleteAll + all cached so far) followed by a repeat frame.
func TestWrapView_IncrementalLoading(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tl := NewThumbList(imgR, VideoThumbURL)

	items := testItems("http://thumb/a", "http://thumb/b", "http://thumb/c", "http://thumb/d")
	deleteAll := ytimage.DeleteAll()

	// No images cached yet — first call sends DELETE_STALE (new ThumbList
	// has lastDeleteGen=0), subsequent calls skip.
	tl.WrapView(items, "V")
	out := tl.WrapView(items, "V")
	if out != "V" {
		t.Errorf("should skip with no cached images on second call, got length %d", len(out))
	}

	// Image A arrives.
	imgR.Store("http://thumb/a", "TX_A", "PL_A")
	out = tl.WrapView(items, "V")
	if !strings.Contains(out, deleteAll) || !strings.Contains(out, "TX_A") {
		t.Error("should retransmit when first image loads")
	}

	// Image B arrives (during repeat window — fingerprint change resets counter).
	imgR.Store("http://thumb/b", "TX_B", "PL_B")
	out = tl.WrapView(items, "V")
	if !strings.Contains(out, "TX_A") || !strings.Contains(out, "TX_B") {
		t.Error("should retransmit A+B when B loads")
	}

	// Image C arrives.
	imgR.Store("http://thumb/c", "TX_C", "PL_C")
	out = tl.WrapView(items, "V")
	if !strings.Contains(out, "TX_A") || !strings.Contains(out, "TX_B") || !strings.Contains(out, "TX_C") {
		t.Error("should retransmit A+B+C when C loads")
	}

	// Image D arrives.
	imgR.Store("http://thumb/d", "TX_D", "PL_D")
	out = tl.WrapView(items, "V")
	if !strings.Contains(out, "TX_D") {
		t.Error("should retransmit all when D loads")
	}

	// Repeat frame.
	out = tl.WrapView(items, "V")
	if !strings.Contains(out, "TX_A") || !strings.Contains(out, "TX_D") {
		t.Error("repeat frame should retransmit all 4")
	}

	// Now stable.
	out = tl.WrapView(items, "V")
	if out != "V" {
		t.Errorf("should be stable after all loaded + repeat, got length %d", len(out))
	}
}

// TestWrapView_FingerprintChangeDuringRepeat verifies that a fingerprint
// change during the repeat window resets the counter (doesn't consume
// the repeat for stale data).
func TestWrapView_FingerprintChangeDuringRepeat(t *testing.T) {
	imgR := ytimage.NewRenderer()
	tl := NewThumbList(imgR, VideoThumbURL)

	imgR.Store("http://thumb/a", "TX_A", "PL_A")
	imgR.Store("http://thumb/x", "TX_X", "PL_X")

	// Initial transmit for A.
	tl.WrapView(testItems("http://thumb/a"), "V")

	// Page change during repeat window.
	out := tl.WrapView(testItems("http://thumb/x"), "V")
	if !strings.Contains(out, "TX_X") {
		t.Error("page change during repeat should transmit X")
	}

	// Repeat for the NEW page (not the old one).
	out2 := tl.WrapView(testItems("http://thumb/x"), "V")
	if !strings.Contains(out2, "TX_X") {
		t.Error("repeat should retransmit X")
	}
	if strings.Contains(out2, "TX_A") {
		t.Error("repeat should NOT contain old page's A")
	}

	// Now stable.
	out3 := tl.WrapView(testItems("http://thumb/x"), "V")
	if out3 != "V" {
		t.Errorf("should be stable, got length %d", len(out3))
	}
}

func TestWrapView_NilThumbList(t *testing.T) {
	var tl *ThumbList
	out := tl.WrapView(nil, "VIEW")
	if out != "VIEW" {
		t.Errorf("nil ThumbList should return view unchanged, got %q", out)
	}
}

func TestWrapView_NilRenderer(t *testing.T) {
	tl := &ThumbList{}
	out := tl.WrapView(nil, "VIEW")
	if out != "VIEW" {
		t.Errorf("nil renderer should return view unchanged, got %q", out)
	}
}

func TestInvalidate_Nil(t *testing.T) {
	var tl *ThumbList
	tl.Invalidate() // should not panic
}
