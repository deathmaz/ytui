package shared

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
)

// NewList creates a list.Model with standard ytui settings.
func NewList(delegate list.ItemDelegate) list.Model {
	l := list.New(nil, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.SetShowPagination(true)
	l.KeyMap.Quit = key.NewBinding()
	l.KeyMap.GoToStart = key.NewBinding(key.WithKeys("g", "home"))
	l.KeyMap.GoToEnd = key.NewBinding(key.WithKeys("G", "end"))
	return l
}
