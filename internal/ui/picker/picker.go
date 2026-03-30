package picker

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/player"
	"github.com/deathmaz/ytui/internal/ui/styles"
)

var overlayStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(styles.Red).
	Padding(1, 2)

// SelectedMsg is sent when a format is selected.
type SelectedMsg struct {
	Format player.Format
}

// CancelledMsg is sent when the picker is dismissed.
type CancelledMsg struct{}

// Model is a quality picker overlay.
type Model struct {
	list   list.Model
	active bool
	width  int
	height int
}

type formatItem struct {
	format player.Format
}

func (f formatItem) FilterValue() string { return f.format.Display }
func (f formatItem) Title() string       { return f.format.Display }
func (f formatItem) Description() string { return "" }

// New creates a new quality picker.
func New() Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(1)
	delegate.SetSpacing(0)

	l := list.New(nil, delegate, 0, 0)
	l.Title = "Select Quality"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.KeyMap.Quit = key.NewBinding() // disable built-in quit

	return Model{list: l}
}

// Show activates the picker with the given formats.
func (m *Model) Show(formats []player.Format, width, height int) {
	m.active = true
	m.width = width
	m.height = height

	items := make([]list.Item, len(formats))
	for i, f := range formats {
		items[i] = formatItem{format: f}
	}
	m.list.SetItems(items)

	// Size the overlay
	w := width * 2 / 3
	if w > 60 {
		w = 60
	}
	h := len(formats) + 4
	if h > height-4 {
		h = height - 4
	}
	m.list.SetSize(w-4, h-2) // account for border+padding
}

// IsActive reports whether the picker is visible.
func (m Model) IsActive() bool {
	return m.active
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(formatItem); ok {
				m.active = false
				return m, func() tea.Msg {
					return SelectedMsg{Format: item.format}
				}
			}
		case "esc", "q":
			m.active = false
			return m, func() tea.Msg {
				return CancelledMsg{}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if !m.active {
		return ""
	}
	content := overlayStyle.Render(m.list.View())

	// Center the overlay
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}
