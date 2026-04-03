package app

import "testing"

type testTab struct {
	id    string
	title string
}

func newTestTabSet(max int) TabSet[testTab] {
	return NewTabSet[testTab](max, func(t *testTab) string { return t.id })
}

func TestTabSet_OpenAndFind(t *testing.T) {
	ts := newTestTabSet(3)

	idx, err := ts.Open(testTab{id: "a", title: "Tab A"})
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}
	if idx != 0 {
		t.Fatalf("Open: got idx %d, want 0", idx)
	}

	idx, err = ts.Open(testTab{id: "b", title: "Tab B"})
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}
	if idx != 1 {
		t.Fatalf("Open: got idx %d, want 1", idx)
	}

	if ts.Len() != 2 {
		t.Fatalf("Len: got %d, want 2", ts.Len())
	}

	// Find existing
	found, ok := ts.Find("a")
	if !ok || found != 0 {
		t.Fatalf("Find(a): got (%d, %v), want (0, true)", found, ok)
	}

	// Find missing
	_, ok = ts.Find("z")
	if ok {
		t.Fatal("Find(z): expected not found")
	}
}

func TestTabSet_OpenMaxTabs(t *testing.T) {
	ts := newTestTabSet(2)

	ts.Open(testTab{id: "a"})
	ts.Open(testTab{id: "b"})

	_, err := ts.Open(testTab{id: "c"})
	if err != ErrMaxTabs {
		t.Fatalf("Open: got err %v, want ErrMaxTabs", err)
	}
}

func TestTabSet_Active(t *testing.T) {
	ts := newTestTabSet(3)

	if ts.Active() != nil {
		t.Fatal("Active: expected nil for empty set")
	}

	ts.Open(testTab{id: "a", title: "A"})
	ts.Open(testTab{id: "b", title: "B"})
	ts.SetActive(1)

	tab := ts.Active()
	if tab == nil || tab.id != "b" {
		t.Fatalf("Active: got %v, want tab b", tab)
	}
}

func TestTabSet_CloseMiddle(t *testing.T) {
	ts := newTestTabSet(4)

	ts.Open(testTab{id: "a"})
	ts.Open(testTab{id: "b"})
	ts.Open(testTab{id: "c"})
	ts.SetActive(1) // active = "b"

	newIdx, empty := ts.Close(1) // close "b"
	if empty {
		t.Fatal("Close: unexpected empty")
	}
	if ts.Len() != 2 {
		t.Fatalf("Close: Len got %d, want 2", ts.Len())
	}
	// After closing index 1, active should adjust
	if newIdx >= ts.Len() {
		t.Fatalf("Close: newIdx %d out of bounds (len=%d)", newIdx, ts.Len())
	}

	// Remaining tabs should be "a" and "c"
	if ts.All()[0].id != "a" || ts.All()[1].id != "c" {
		t.Fatalf("Close: unexpected tabs: %v", ts.All())
	}
}

func TestTabSet_CloseLast(t *testing.T) {
	ts := newTestTabSet(3)

	ts.Open(testTab{id: "a"})
	ts.SetActive(0)

	newIdx, empty := ts.Close(0)
	if !empty {
		t.Fatal("Close: expected empty")
	}
	if ts.Len() != 0 {
		t.Fatalf("Close: Len got %d, want 0", ts.Len())
	}
	_ = newIdx
}

func TestTabSet_CloseEndAdjustsIndex(t *testing.T) {
	ts := newTestTabSet(4)

	ts.Open(testTab{id: "a"})
	ts.Open(testTab{id: "b"})
	ts.Open(testTab{id: "c"})
	ts.SetActive(2) // active = "c" (last)

	newIdx, empty := ts.Close(2) // close "c"
	if empty {
		t.Fatal("Close: unexpected empty")
	}
	if newIdx != 1 {
		t.Fatalf("Close: got newIdx %d, want 1", newIdx)
	}
}

func TestTabSet_At(t *testing.T) {
	ts := newTestTabSet(3)

	if ts.At(0) != nil {
		t.Fatal("At: expected nil for empty set")
	}

	ts.Open(testTab{id: "a", title: "A"})
	ts.Open(testTab{id: "b", title: "B"})

	tab := ts.At(1)
	if tab == nil || tab.id != "b" {
		t.Fatalf("At(1): got %v, want tab b", tab)
	}

	if ts.At(5) != nil {
		t.Fatal("At(5): expected nil for out of bounds")
	}
}

func TestTabSet_SetActiveClamped(t *testing.T) {
	ts := newTestTabSet(3)
	ts.Open(testTab{id: "a"})
	ts.Open(testTab{id: "b"})

	ts.SetActive(99)
	if ts.ActiveIdx() != 1 {
		t.Fatalf("SetActive(99): got %d, want 1", ts.ActiveIdx())
	}

	ts.SetActive(-5)
	if ts.ActiveIdx() != 0 {
		t.Fatalf("SetActive(-5): got %d, want 0", ts.ActiveIdx())
	}
}
