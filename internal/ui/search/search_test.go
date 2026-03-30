package search

import (
	"testing"
)

func TestSetQuery(t *testing.T) {
	m := New(nil)

	m.SetQuery("test query")

	if m.Query() != "test query" {
		t.Errorf("Query() = %q, want %q", m.Query(), "test query")
	}
	if m.input.Value() != "test query" {
		t.Errorf("input.Value() = %q, want %q", m.input.Value(), "test query")
	}
	if m.InputFocused() {
		t.Error("input should be blurred after SetQuery")
	}
}

func TestSetQuery_Empty(t *testing.T) {
	m := New(nil)
	m.SetQuery("")

	if m.Query() != "" {
		t.Errorf("Query() = %q, want empty", m.Query())
	}
}

func TestRefresh_NoQuery(t *testing.T) {
	m := New(nil)
	cmd := m.Refresh()
	if cmd != nil {
		t.Error("Refresh with no query should return nil")
	}
}

func TestRefresh_WithQuery(t *testing.T) {
	m := New(nil)
	m.SetQuery("test")
	cmd := m.Refresh()
	if cmd == nil {
		t.Error("Refresh with query should return a command")
	}
	if !m.searching {
		t.Error("Refresh should set searching=true")
	}
}

func TestRefresh_WhileSearching(t *testing.T) {
	m := New(nil)
	m.SetQuery("test")
	m.searching = true
	cmd := m.Refresh()
	if cmd != nil {
		t.Error("Refresh while already searching should return nil")
	}
}
