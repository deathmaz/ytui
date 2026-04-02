package search

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func testConfig() Config {
	return Config{
		Placeholder: "Test...",
		Delegate:    list.NewDefaultDelegate(),
		SearchFn: func(_ context.Context, _, _ string) SearchResult {
			return SearchResult{}
		},
		SelectFn: func(_ list.Item) tea.Msg { return nil },
	}
}

func TestSetQuery(t *testing.T) {
	m := New(testConfig())

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
	m := New(testConfig())
	m.SetQuery("")

	if m.Query() != "" {
		t.Errorf("Query() = %q, want empty", m.Query())
	}
}

func TestRefresh_NoQuery(t *testing.T) {
	m := New(testConfig())
	cmd := m.Refresh()
	if cmd != nil {
		t.Error("Refresh with no query should return nil")
	}
}

func TestRefresh_WhileSearching(t *testing.T) {
	m := New(testConfig())
	m.SetQuery("test")
	m.searching = true
	cmd := m.Refresh()
	if cmd != nil {
		t.Error("Refresh while already searching should return nil")
	}
}
