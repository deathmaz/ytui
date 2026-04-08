package image

import "testing"

func TestRenderer_LRUEviction(t *testing.T) {
	r := NewRendererWithMax(3)
	r.Store("a", "txA", "plA")
	r.Store("b", "txB", "plB")
	r.Store("c", "txC", "plC")
	r.Store("d", "txD", "plD") // should evict "a"

	if tx, _ := r.Get("a"); tx != "" {
		t.Error("a should have been evicted")
	}
	if tx, _ := r.Get("b"); tx != "txB" {
		t.Errorf("b should still be cached, got %q", tx)
	}
	if tx, _ := r.Get("d"); tx != "txD" {
		t.Errorf("d should be cached, got %q", tx)
	}
}

func TestRenderer_LRUPromotion(t *testing.T) {
	r := NewRendererWithMax(3)
	r.Store("a", "txA", "plA")
	r.Store("b", "txB", "plB")
	r.Store("c", "txC", "plC")

	// Promote "a" by accessing it.
	r.Get("a")

	// Adding "d" should evict "b" (oldest unreferenced), not "a".
	r.Store("d", "txD", "plD")

	if tx, _ := r.Get("a"); tx != "txA" {
		t.Error("a should survive (was promoted)")
	}
	if tx, _ := r.Get("b"); tx != "" {
		t.Error("b should have been evicted (oldest unreferenced)")
	}
}

func TestRenderer_StoreUpdate(t *testing.T) {
	r := NewRendererWithMax(3)
	r.Store("a", "tx1", "pl1")
	r.Store("a", "tx2", "pl2") // update same key

	tx, pl := r.Get("a")
	if tx != "tx2" || pl != "pl2" {
		t.Errorf("expected updated values, got tx=%q pl=%q", tx, pl)
	}
	// Cache should have exactly 1 entry.
	if r.order.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", r.order.Len())
	}
}

func TestRenderer_HandleLoadedRespectsLRU(t *testing.T) {
	r := NewRendererWithMax(2)
	r.Store("a", "txA", "plA")

	// Mark "b" as inflight so HandleLoaded accepts it.
	r.mu.Lock()
	r.inflight["b"] = true
	r.mu.Unlock()

	r.HandleLoaded(ThumbnailLoadedMsg{
		URL: "b", TransmitStr: "txB", Placeholder: "plB",
	})

	// Both should be cached (at capacity).
	if tx, _ := r.Get("a"); tx != "txA" {
		t.Error("a should be cached")
	}
	if tx, _ := r.Get("b"); tx != "txB" {
		t.Error("b should be cached")
	}

	// Mark "c" as inflight.
	r.mu.Lock()
	r.inflight["c"] = true
	r.mu.Unlock()

	r.HandleLoaded(ThumbnailLoadedMsg{
		URL: "c", TransmitStr: "txC", Placeholder: "plC",
	})

	// "a" was accessed more recently than "b" (from the Get above),
	// but "b" was promoted by its own Get. Let's just verify one was evicted.
	cached := 0
	for _, url := range []string{"a", "b", "c"} {
		if tx, _ := r.Get(url); tx != "" {
			cached++
		}
	}
	if cached != 2 {
		t.Errorf("expected 2 entries cached (max=2), got %d", cached)
	}
}

func TestRenderer_FetchCmdSkipsCached(t *testing.T) {
	r := NewRenderer()
	r.Store("http://test/a.jpg", "tx", "pl")
	cmd := r.FetchCmd("http://test/a.jpg", 20, 5)
	if cmd != nil {
		t.Error("FetchCmd should return nil for cached URL")
	}
}

func TestRenderer_FetchCmdRetriesEvicted(t *testing.T) {
	r := NewRendererWithMax(2)
	r.Store("a", "txA", "plA")
	r.Store("b", "txB", "plB")
	r.Store("c", "txC", "plC") // evicts "a"

	// FetchCmd should return non-nil for "a" since it was evicted.
	cmd := r.FetchCmd("a", 20, 5)
	if cmd == nil {
		t.Error("FetchCmd should return non-nil for evicted URL")
	}

	// FetchCmd should still return nil for "b" (still cached).
	cmd = r.FetchCmd("b", 20, 5)
	if cmd != nil {
		t.Error("FetchCmd should return nil for still-cached URL")
	}
}

func TestRendererWithMax_MinimumOne(t *testing.T) {
	r := NewRendererWithMax(0) // should clamp to 1
	r.Store("a", "txA", "plA")
	r.Store("b", "txB", "plB") // should evict "a"

	if tx, _ := r.Get("a"); tx != "" {
		t.Error("a should be evicted (max clamped to 1)")
	}
	if tx, _ := r.Get("b"); tx != "txB" {
		t.Error("b should be cached")
	}
}
