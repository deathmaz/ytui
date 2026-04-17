package picker

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/deathmaz/ytui/internal/ui/styles"
)

// Target identifies which caller opened the picker so a single SelectedMsg
// dispatch can fan out to the right handler.
type Target int

const (
	TargetQuality Target = iota
	TargetSubscribe
)

// Option is a single picker row. Key is the opaque identifier the caller
// receives back in SelectedMsg; Label is the text shown to the user.
type Option struct {
	Key   string
	Label string
}

// SelectedMsg is sent when the user confirms a choice.
type SelectedMsg struct {
	Target Target
	Key    string
}

// CancelledMsg is sent when the picker is dismissed. Target lets callers
// route the cancel to the right pending-state cleanup.
type CancelledMsg struct {
	Target Target
}

var overlayStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(styles.Red).
	Padding(1, 2)

// Model is a modal overlay that lets the user pick one of a list of options.
type Model struct {
	list   list.Model
	target Target
	active bool
	width  int
	height int
}

type optionItem struct{ opt Option }

func (i optionItem) FilterValue() string { return i.opt.Label }
func (i optionItem) Title() string       { return i.opt.Label }
func (i optionItem) Description() string { return "" }

// New creates a new picker.
func New() Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(1)
	delegate.SetSpacing(0)

	l := list.New(nil, delegate, 0, 0)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.KeyMap.Quit = key.NewBinding()

	return Model{list: l}
}

// Show activates the picker with the given options and sizes it for the
// current viewport.
func (m *Model) Show(target Target, title string, options []Option, width, height int) {
	m.active = true
	m.target = target
	m.width = width
	m.height = height
	m.list.Title = title

	items := make([]list.Item, len(options))
	for i, o := range options {
		items[i] = optionItem{opt: o}
	}
	m.list.SetItems(items)

	w := width * 2 / 3
	if w > 60 {
		w = 60
	}
	h := len(options) + 4
	if h > height-4 {
		h = height - 4
	}
	m.list.SetSize(w-4, h-2)
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
			if item, ok := m.list.SelectedItem().(optionItem); ok {
				m.active = false
				return m, func() tea.Msg {
					return SelectedMsg{Target: m.target, Key: item.opt.Key}
				}
			}
		case "esc", "q":
			m.active = false
			return m, func() tea.Msg {
				return CancelledMsg{Target: m.target}
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
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}
