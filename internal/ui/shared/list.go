package shared

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/ui/styles"
)

// RenderSubTabBar renders a horizontal sub-tab bar using SubTab/ActiveSubTab styles.
func RenderSubTabBar(names []string, activeIdx int) string {
	labels := make([]string, len(names))
	for i, name := range names {
		s := styles.SubTab
		if i == activeIdx {
			s = styles.ActiveSubTab
		}
		labels[i] = s.Render(name)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, labels...)
}

// ShouldLoadMore returns true when the cursor is within threshold items
// of the end of the list. Use this after list.Update() to trigger
// auto-loading of the next page.
func ShouldLoadMore(l list.Model, threshold int) bool {
	total := len(l.Items())
	return total > 0 && l.Index() >= total-threshold
}

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
