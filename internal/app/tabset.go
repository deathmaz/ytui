package app

import "errors"

// ErrMaxTabs is returned when opening a tab would exceed the limit.
var ErrMaxTabs = errors.New("maximum tabs reached")

// TabSet manages a slice of dynamic tabs with an active index and max limit.
type TabSet[T any] struct {
	items    []T
	activeIdx int
	maxTabs   int
	idFn      func(*T) string // returns stable ID for dedup
}

// NewTabSet creates a TabSet with the given max limit and ID function.
func NewTabSet[T any](maxTabs int, idFn func(*T) string) TabSet[T] {
	return TabSet[T]{maxTabs: maxTabs, idFn: idFn}
}

// Find returns the index of the tab with the given ID, or -1 and false.
func (ts *TabSet[T]) Find(id string) (int, bool) {
	for i := range ts.items {
		if ts.idFn(&ts.items[i]) == id {
			return i, true
		}
	}
	return -1, false
}

// Open appends a tab if not at max. Returns the new tab's index.
// Returns ErrMaxTabs if the limit is reached.
func (ts *TabSet[T]) Open(tab T) (int, error) {
	if len(ts.items) >= ts.maxTabs {
		return -1, ErrMaxTabs
	}
	ts.items = append(ts.items, tab)
	return len(ts.items) - 1, nil
}

// Close removes the tab at idx. Returns the adjusted active index and
// whether the tab slice is now empty.
func (ts *TabSet[T]) Close(idx int) (newActiveIdx int, empty bool) {
	if idx < 0 || idx >= len(ts.items) {
		return ts.activeIdx, len(ts.items) == 0
	}
	ts.items = append(ts.items[:idx], ts.items[idx+1:]...)
	if len(ts.items) == 0 {
		ts.activeIdx = 0
		return 0, true
	}
	if ts.activeIdx >= len(ts.items) {
		ts.activeIdx = len(ts.items) - 1
	}
	return ts.activeIdx, false
}

// Active returns a pointer to the currently active tab, or nil.
func (ts *TabSet[T]) Active() *T {
	if ts.activeIdx < 0 || ts.activeIdx >= len(ts.items) {
		return nil
	}
	return &ts.items[ts.activeIdx]
}

// SetActive sets the active index, clamped to bounds.
func (ts *TabSet[T]) SetActive(idx int) {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ts.items) {
		idx = len(ts.items) - 1
	}
	ts.activeIdx = idx
}

// ActiveIdx returns the current active index.
func (ts *TabSet[T]) ActiveIdx() int {
	return ts.activeIdx
}

// Len returns the number of tabs.
func (ts *TabSet[T]) Len() int {
	return len(ts.items)
}

// All returns the underlying slice for iteration.
func (ts *TabSet[T]) All() []T {
	return ts.items
}

// At returns a pointer to the tab at the given index, or nil if out of bounds.
func (ts *TabSet[T]) At(idx int) *T {
	if idx < 0 || idx >= len(ts.items) {
		return nil
	}
	return &ts.items[idx]
}
