package shared

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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

// AppendItems sets or appends items on a list model. When shouldAppend is
// true, new items are added after existing ones; otherwise they replace.
func AppendItems(l *list.Model, newItems []list.Item, shouldAppend bool) tea.Cmd {
	if shouldAppend {
		existing := l.Items()
		items := make([]list.Item, len(existing), len(existing)+len(newItems))
		copy(items, existing)
		items = append(items, newItems...)
		return l.SetItems(items)
	}
	return l.SetItems(newItems)
}

// ShouldLoadMore returns true when the cursor is within threshold items
// of the end of the list. For lists shorter than or equal to the
// threshold, it fires only at the very last item so short preview
// shelves don't auto-paginate on the first `j`.
func ShouldLoadMore(l list.Model, threshold int) bool {
	total := len(l.Items())
	if total == 0 {
		return false
	}
	if total <= threshold {
		return l.Index() == total-1
	}
	return l.Index() >= total-threshold
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
